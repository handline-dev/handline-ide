// Package transcript reads an agent's stored transcript rows for chat: reply, tool and session items of chat.transcript.
// Record writes these rows from a terminal session's transcript while chat_transcript_rows is on.
package transcript

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
)

// Item sources of chat.transcript's ids, and the id itself: the time in seconds, a source, and a number within it.
const (
	ItemReply   = 0
	ItemTool    = 3 // 1 is the prompts', 2 the task events'; a tool item takes its tool_use event's seq
	ItemSession = 4 // a session start; its number is the session_starts row
	ItemBash    = 5 // a person's ! line; its number is its place in the session's transcript
)

func ItemID(at, source, n int64) int64 { return at*1_000_000 + source*100_000 + n%100_000 }

// Replies are agent's text events as chat.transcript's reply items, newest first up to limit: id, time and text.
func Replies(db *sql.DB, agent string, limit int) ([]Reply, error) {
	rows, err := db.Query("SELECT item, at, body FROM transcript_events WHERE agent = ? AND kind = 'text' AND item IS NOT NULL ORDER BY seq DESC LIMIT ?", agent, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reply
	for rows.Next() {
		var r Reply
		var body string
		if err := rows.Scan(&r.ID, &r.At, &body); err != nil {
			return nil, err
		}
		var b struct {
			Text string `json:"text"`
		}
		json.Unmarshal([]byte(body), &b)
		r.Text = b.Text
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReplyByID is reply item id and the agent it is of; ok is false when there is no such reply.
func ReplyByID(db *sql.DB, id int64) (r Reply, agent string, ok bool, err error) {
	var body string
	err = db.QueryRow("SELECT agent, at, body FROM transcript_events WHERE kind = 'text' AND item = ? ORDER BY seq DESC LIMIT 1", id).Scan(&agent, &r.At, &body)
	if err == sql.ErrNoRows {
		return r, "", false, nil
	}
	if err != nil {
		return r, "", false, err
	}
	var b struct {
		Text string `json:"text"`
	}
	json.Unmarshal([]byte(body), &b)
	r.ID, r.Text = id, b.Text
	return r, agent, true, nil
}

// IsReply is whether item id is one of agent's reply items (a text event with that item id).
func IsReply(db *sql.DB, agent string, id int64) (bool, error) {
	var n int
	err := db.QueryRow("SELECT count(*) FROM transcript_events WHERE agent = ? AND kind = 'text' AND item = ?", agent, id).Scan(&n)
	return n > 0, err
}

type Reply struct {
	ID, At int64
	Text   string
}

// Tool is a tool call as chat.transcript's tool item, the call with its result when one came, or a thinking block (Thinking, its text in
// Text). Output is nil with no result; Dropped is the lines the store cut from the end of the output. Parent is the item id of the call
// that started the subagent that made it, 0 for the session's own. Diff, Added and Removed are the file change its result made.
type Tool struct {
	ID, At     int64
	Thinking   bool
	Name       string
	Text       string
	Output     *string
	Dropped    int64
	UseID      string
	Background bool // the call asked to run in the background
	Parent     int64
	Diff       map[string]any
	Added      int64
	Removed    int64
}

// ToolWindow is which of an agent's tool calls a page needs: at most Limit of them, the newest with an item id below Before, or with
// Since the oldest from that second on. A zero is no bound. Calls share a second, so Before is held to the id itself: a call the page
// would drop must not use up a slot.
type ToolWindow struct {
	Before, Since int64
	Limit         int
}

// toolItemSQL is a tool_use row's item id (ItemID of its second, ItemTool and seq), as SQL.
var toolItemSQL = "(at * 1000000 + " + strconv.Itoa(ItemTool*100000) + " + seq % 100000)"

// Tools are agent's tool_use and thinking events in the window, a call paired by tool_use_id with its tool_result, oldest first. A call
// with no result yet is an item with no output. Only the results of the calls read are read. A thinking block takes the tool items'
// numbers: its seq is its own.
func Tools(db *sql.DB, agent string, w ToolWindow) ([]Tool, error) {
	query, args := "SELECT seq, at, kind, body FROM transcript_events WHERE agent = ? AND kind IN ('tool_use', 'thinking')", []any{agent}
	order := "DESC"
	if w.Since > 0 {
		query, args, order = query+" AND at >= ?", append(args, w.Since/1_000_000), "ASC"
	} else if w.Before > 0 {
		query, args = query+" AND "+toolItemSQL+" < ?", append(args, w.Before)
	}
	rows, err := db.Query(query+" ORDER BY seq "+order+" LIMIT ?", append(args, w.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tool
	byUse := map[string]int{}
	var ids []any
	parents := map[string][]child{} // the tool_use id of the call that started a subagent -> the items that subagent made
	oldest := int64(0)
	for rows.Next() {
		var seq, at int64
		var kind, body string
		if err := rows.Scan(&seq, &at, &kind, &body); err != nil {
			return nil, err
		}
		var b struct {
			Text   string `json:"text"`
			Parent string `json:"parent"`
			Use    struct {
				ID    string         `json:"id"`
				Tool  string         `json:"tool"`
				Input map[string]any `json:"input"`
			} `json:"tool_use"`
		}
		json.Unmarshal([]byte(body), &b)
		if b.Parent != "" {
			parents[b.Parent] = append(parents[b.Parent], child{len(out), seq})
		}
		if oldest == 0 || seq < oldest {
			oldest = seq
		}
		if kind == "thinking" {
			out = append(out, Tool{ID: ItemID(at, ItemTool, seq), At: at, Thinking: true, Text: b.Text})
			continue
		}
		byUse[b.Use.ID] = len(out)
		ids = append(ids, b.Use.ID)
		out = append(out, Tool{ID: ItemID(at, ItemTool, seq), At: at, Name: b.Use.Tool, Text: turns.ToolText(b.Use.Input), UseID: b.Use.ID,
			Background: b.Use.Tool == "Bash" && b.Use.Input["run_in_background"] == true})
	}
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	if err := parentItems(db, agent, parents, byUse, out); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return ordered(out, w), nil
	}
	res, err := db.Query("SELECT body FROM transcript_events WHERE agent = ? AND kind = 'tool_result' AND seq > ? AND json_extract(body, '$.tool_result.tool_use_id') IN ("+
		strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+") ORDER BY seq", append([]any{agent, oldest}, ids...)...)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	for res.Next() {
		var body string
		if err := res.Scan(&body); err != nil {
			return nil, err
		}
		var b struct {
			Result struct {
				ID      string         `json:"tool_use_id"`
				Text    string         `json:"text"`
				Cut     bool           `json:"truncated"`
				Dropped int64          `json:"dropped_lines"`
				Diff    map[string]any `json:"diff"`
				Added   int64          `json:"added"`
				Removed int64          `json:"removed"`
			} `json:"tool_result"`
		}
		json.Unmarshal([]byte(body), &b)
		if i, ok := byUse[b.Result.ID]; ok && out[i].Output == nil {
			text := b.Result.Text
			out[i].Output, out[i].Dropped = &text, b.Result.Dropped
			out[i].Diff, out[i].Added, out[i].Removed = b.Result.Diff, b.Result.Added, b.Result.Removed
			if b.Result.Cut && out[i].Dropped < 1 {
				out[i].Dropped = 1 // a row written before the count was kept: a cut output never reads as whole
			}
		}
	}
	if err := res.Err(); err != nil {
		return nil, err
	}
	return ordered(out, w), nil
}

// ordered is out oldest first, as every other source: a page that is not a since page was read newest first.
func ordered(out []Tool, w ToolWindow) []Tool {
	if w.Since == 0 {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

// child is an item a subagent made: its place in the items read and its row's seq.
type child struct {
	i   int
	seq int64
}

// parentItems sets each subagent item's Parent: the item id of the call, by its tool_use id in parents, that started the subagent. A call
// among the items read is taken from them; another is the newest tool_use row with that id before its first child, read back from there.
func parentItems(db *sql.DB, agent string, parents map[string][]child, byUse map[string]int, out []Tool) error {
	for use, children := range parents {
		item := int64(0)
		if i, ok := byUse[use]; ok {
			item = out[i].ID
		} else {
			first := children[0].seq
			for _, c := range children {
				first = min(first, c.seq)
			}
			var seq, at int64
			err := db.QueryRow("SELECT seq, at FROM transcript_events WHERE agent = ? AND seq < ? AND kind = 'tool_use' AND "+
				"json_extract(body, '$.tool_use.id') = ? ORDER BY seq DESC LIMIT 1", agent, first, use).Scan(&seq, &at)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return err
			}
			item = ItemID(at, ItemTool, seq)
		}
		for _, c := range children {
			out[c.i].Parent = item
		}
	}
	return nil
}

// FitOutput is text cut to at most max bytes at a character boundary, and the lines the cut left out (0 when nothing was).
func FitOutput(text string, max int) (string, int64) {
	kept, cut := cutText(text, max)
	if !cut {
		return text, 0
	}
	return kept, int64(linesIn(text[len(kept):]))
}

// cutText is s cut to at most max bytes at a character boundary, and whether it was cut.
func cutText(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	s = s[:max]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s, true
}

// linesIn is how many lines s holds, a last one with no newline included: a cut that leaves anything out cuts at least one.
func linesIn(s string) int {
	n := strings.Count(s, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
