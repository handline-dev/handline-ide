package transcript

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ResultMax is the most of a tool's output a row keeps, in bytes; Keep is how many rows an agent keeps, the oldest pruned every 500.
const (
	ResultMax = 16 << 10
	Keep      = 4000
)

// row is one transcript_events row before it is numbered: its kind and body, in the shape Replies and Tools read.
type row struct {
	kind string
	body map[string]any
}

// Record stores what one line of a Claude Code session transcript holds for chat: an assistant entry's text blocks (kind text, each a
// reply item) and tool calls (tool_use), and a user entry's tool results (tool_result, their output cut to ResultMax). Any other line, and
// a subagent's, stores nothing.
func Record(q *sql.DB, agent, session string, line []byte) error {
	var e struct {
		Type      string `json:"type"`
		Sidechain bool   `json:"isSidechain"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &e) != nil || e.Sidechain || (e.Type != "assistant" && e.Type != "user") {
		return nil
	}
	var parts []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     map[string]any  `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
		IsError   bool            `json:"is_error"`
	}
	if json.Unmarshal(e.Message.Content, &parts) != nil { // a plain-string content is a person's prompt, which chat has already
		return nil
	}
	var rows []row
	for _, p := range parts {
		switch {
		case e.Type == "assistant" && p.Type == "text" && strings.TrimSpace(p.Text) != "":
			rows = append(rows, row{"text", map[string]any{"text": p.Text}})
		case e.Type == "assistant" && p.Type == "tool_use" && p.ID != "":
			rows = append(rows, row{"tool_use", map[string]any{"tool_use": map[string]any{"id": p.ID, "tool": p.Name, "input": p.Input}}})
		case e.Type == "user" && p.Type == "tool_result" && p.ToolUseID != "":
			text, dropped := FitOutput(resultText(p.Content), ResultMax)
			rows = append(rows, row{"tool_result", map[string]any{"tool_result": map[string]any{
				"tool_use_id": p.ToolUseID, "ok": !p.IsError, "text": text, "truncated": dropped > 0, "dropped_lines": dropped}}})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	at := time.Now().Unix()
	if ts, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
		at = ts.Unix()
	}
	return write(q, agent, session, at, rows)
}

// resultText is a tool result's content as text: a string as it is, a list of blocks as their text parts joined by newlines.
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct{ Type, Text string }
	json.Unmarshal(raw, &blocks)
	var texts []string
	for _, b := range blocks {
		if b.Type == "text" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// write numbers rows after the agent's newest and stores them in one transaction; a text row carries its reply item id.
func write(q *sql.DB, agent, session string, at int64, rows []row) (err error) {
	tx, err := q.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	var seq int64 // the prune keeps the newest rows, so the largest seq is always there
	if err = tx.QueryRow("SELECT COALESCE(MAX(seq), 0) FROM transcript_events WHERE agent = ?", agent).Scan(&seq); err != nil {
		return err
	}
	for _, r := range rows {
		seq++
		var item any
		if r.kind == "text" {
			item = ItemID(at, ItemReply, seq*2) // as the structured sessions' writer numbered them
		}
		body, err := json.Marshal(r.body)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES(?,?,?,NULL,?,?,?,?)",
			agent, seq, session, at, r.kind, item, string(body)); err != nil {
			return err
		}
		if seq%500 == 0 {
			if _, err = tx.Exec("DELETE FROM transcript_events WHERE agent = ? AND seq <= ?", agent, seq-Keep); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
