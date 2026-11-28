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

// Row is one transcript_events row before it is numbered: its kind and body, in the shape Replies and Tools read.
type Row struct {
	Kind string         `json:"kind"`
	Body map[string]any `json:"body"`
}

// Record stores what one line of a Claude Code session transcript holds for chat: an assistant entry's text blocks (kind text, each a
// reply item), thinking and tool calls (tool_use), and a user entry's tool results (tool_result, their output cut to ResultMax). A
// subagent's line is stored only from its own transcript, under parent, the tool call that started it (RecordUnder); its text is no reply.
func Record(q *sql.DB, agent, session string, line []byte) error {
	return RecordUnder(q, agent, session, "", line)
}

// RecordUnder is Record for a line of the subagent the tool call parent started; "" is the session's own line.
func RecordUnder(q *sql.DB, agent, session, parent string, line []byte) error {
	at, rows := RowsUnder(line, parent)
	if len(rows) == 0 {
		return nil
	}
	return Write(q, agent, session, at, rows)
}

// Rows is what Record stores of line, and when the line was written; none for a line that holds nothing for chat. A runner reads its
// sessions' lines with it, and the master writes the rows (Write).
func Rows(line []byte) (at int64, rows []Row) { return RowsUnder(line, "") }

// RowsUnder is Rows of a subagent's line, each row naming parent, the tool call that started the subagent; "" is Rows.
func RowsUnder(line []byte, parent string) (at int64, rows []Row) {
	var e struct {
		Type      string `json:"type"`
		Sidechain bool   `json:"isSidechain"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		Result json.RawMessage `json:"toolUseResult"` // an object for most tools; a string or a list for an error or an MCP tool
	}
	if json.Unmarshal(line, &e) != nil || e.Sidechain != (parent != "") || (e.Type != "assistant" && e.Type != "user") {
		return 0, nil
	}
	var parts []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Thinking  string          `json:"thinking"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     map[string]any  `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
		IsError   bool            `json:"is_error"`
	}
	if json.Unmarshal(e.Message.Content, &parts) != nil { // a plain-string content is a person's prompt, which chat has already
		return 0, nil
	}
	results := 0 // a line's file change belongs to its result only when it holds one
	for _, p := range parts {
		if p.Type == "tool_result" {
			results++
		}
	}
	for _, p := range parts {
		switch {
		case e.Type == "assistant" && p.Type == "text" && strings.TrimSpace(p.Text) != "":
			rows = append(rows, Row{"text", map[string]any{"text": p.Text}})
		case e.Type == "assistant" && p.Type == "thinking" && strings.TrimSpace(p.Thinking) != "":
			rows = append(rows, Row{"thinking", map[string]any{"text": p.Thinking}})
		case e.Type == "assistant" && p.Type == "tool_use" && p.ID != "":
			rows = append(rows, Row{"tool_use", map[string]any{"tool_use": map[string]any{"id": p.ID, "tool": p.Name, "input": p.Input}}})
		case e.Type == "user" && p.Type == "tool_result" && p.ToolUseID != "":
			text, dropped := FitOutput(resultText(p.Content), ResultMax)
			result := map[string]any{"tool_use_id": p.ToolUseID, "ok": !p.IsError, "text": text, "truncated": dropped > 0, "dropped_lines": dropped}
			var change struct {
				Patch []patchHunk `json:"structuredPatch"`
			}
			if json.Unmarshal(e.Result, &change) == nil && len(change.Patch) > 0 && results == 1 {
				result["diff"], result["added"], result["removed"] = diffOf(change.Patch)
			}
			rows = append(rows, Row{"tool_result", map[string]any{"tool_result": result}})
		}
	}
	if parent != "" {
		for _, r := range rows {
			r.Body["parent"] = parent
		}
	}
	at = time.Now().Unix()
	if ts, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
		at = ts.Unix()
	}
	return at, rows
}

// patchHunk is a hunk of a file change as Claude Code writes it on an Edit, MultiEdit or Write result line (toolUseResult.structuredPatch).
type patchHunk struct {
	OldStart int      `json:"oldStart"`
	NewStart int      `json:"newStart"`
	Lines    []string `json:"lines"`
}

// diffOf is a file change as stored: its hunks (old_start, new_start, lines that start with "+", "-" or " ") cut to ResultMax bytes of
// lines from the start, with the lines left out, and how many lines it adds and removes in all.
func diffOf(patch []patchHunk) (diff map[string]any, added, removed int64) {
	left, dropped := ResultMax, int64(0)
	hunks := []any{}
	for _, h := range patch {
		var kept []string
		for _, l := range h.Lines {
			switch {
			case strings.HasPrefix(l, "+"):
				added++
			case strings.HasPrefix(l, "-"):
				removed++
			}
			if dropped > 0 || len(l) > left { // the start is kept: past the first line that does not fit, none is
				dropped++
				continue
			}
			left -= len(l)
			kept = append(kept, l)
		}
		if len(kept) > 0 {
			hunks = append(hunks, map[string]any{"old_start": h.OldStart, "new_start": h.NewStart, "lines": kept})
		}
	}
	return map[string]any{"hunks": hunks, "dropped_lines": dropped}, added, removed
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

// Write numbers rows after the agent's newest and stores them in one transaction; a text row carries its reply item id.
func Write(q *sql.DB, agent, session string, at int64, rows []Row) (err error) {
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
		if r.Kind == "text" && r.Body["parent"] == nil {
			item = ItemID(at, ItemReply, seq*2) // as the structured sessions' writer numbered them
		}
		body, err := json.Marshal(r.Body)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES(?,?,?,NULL,?,?,?,?)",
			agent, seq, session, at, r.Kind, item, string(body)); err != nil {
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
