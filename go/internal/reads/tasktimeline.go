package reads

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/textsafe"
)

// seconds is a stamp to the fraction of a second: a column, or a number in an event's payload.
func seconds(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// taskTimeline is TaskDetail.timeline, as actions.task_timeline: the task's memos, handoffs (a request that carried one is that handoff's
// row), the other requests linked to it (not a memo's tell, the ask an @human memo opens or a return that told a memo, which repeat the memo), the edits of its brief, its marks done and the notes it was sent back with, oldest first to the
// fraction of a second. parts cuts a text at the images it names; a memo, a handoff, an edit and a reject row carry them.
func (e *Env) taskTimeline(tid int64, memos, reqs []row, parts func(string) []any) ([]any, error) {
	type entry struct {
		at  float64
		row map[string]any
	}
	var out []entry
	add := func(at float64, who, kind, text string, memo, edited any, imaged bool) {
		text = textsafe.Text(text)
		r := map[string]any{"at": ts(at), "who": who, "kind": kind, "text": text, "memo": memo, "edited_at": edited}
		if imaged {
			if p := parts(text); p != nil {
				r["parts"] = p
			}
		}
		out = append(out, entry{at, r})
	}
	for _, m := range memos {
		var edited any
		if m.set("edited_at") {
			edited = ts(m["edited_at"])
		}
		add(seconds(m["created_at"]), m.str("author"), "memo", m.str("body"), m.i("id"), edited, true)
	}
	events, err := query(e.DB, "SELECT ts, agent, kind, payload FROM events WHERE kind IN ('task_handoff', 'edit_task', 'mark_task_done', "+
		"'reject_task', 'task_turn_push', 'task_turn_pop', 'task_forward') AND "+eventTask+" = ? ORDER BY id", tid)
	if err != nil {
		return nil, err
	}
	// A request that forwards a comment or a turn's return is that comment's or turn's row, and one that carried a handoff the handoff's.
	hidden, err := db.IDs(e.DB, "SELECT l.request_id FROM task_requests l WHERE l.task_id = ? AND l.request_id IN ("+db.ForwardedSQL+")", tid)
	if err != nil {
		return nil, err
	}
	carried := map[int64]bool{}
	for _, rid := range hidden {
		carried[rid] = true
	}
	forwards := map[int64]string{} // what the daemon delivered on someone's behalf: the queued task, or the task sent again
	for _, ev := range events {
		p := decodeObject(ev.str("payload"))
		switch ev.str("kind") {
		case "task_handoff":
			carried[int64(seconds(p["request_id"]))] = true
		case "task_forward":
			forwards[int64(seconds(p["request_id"]))] = text(p["carries"])
		}
	}
	for _, r := range reqs {
		if carried[r.i("id")] || r.i("notice") == 2 || r.set("memo_id") { // a memo's tell, or the ask an @human memo opened, repeats the memo row
			continue
		}
		first := ""
		if lines := textsafe.SplitLines(strings.TrimSpace(r.str("body"))); len(lines) > 0 {
			first = lines[0]
		}
		add(seconds(r["ts"]), r.str("requester")+" → "+r.str("target"), "request", fmt.Sprintf("%s  %s", r.str("state"), first),
			nil, nil, false)
		if f, ok := forwards[r.i("id")]; ok {
			out[len(out)-1].row["forwards"] = f
		}
	}
	for _, ev := range events {
		p := decodeObject(ev.str("payload"))
		at := seconds(ev["ts"])
		switch ev.str("kind") {
		case "task_handoff":
			if p["at"] != nil {
				at = seconds(p["at"])
			}
			note, _ := p["note"].(string)
			add(at, fmt.Sprintf("%v → %v", p["from"], p["to"]), "handoff", note, nil, nil, true)
		case "edit_task":
			if b, ok := p["body"].(map[string]any); ok {
				text, _ := b["new"].(string)
				add(at, ev.str("agent"), "edit", text, nil, nil, true)
			}
		case "mark_task_done":
			if from, _ := p["from"].(string); from != "" {
				add(at, ev.str("agent"), "state", from+" -> done", nil, nil, false)
			}
		case "reject_task":
			note, _ := p["note"].(string)
			add(at, ev.str("agent"), "reject", note, nil, nil, true)
		case "task_turn_push":
			add(at, fmt.Sprintf("%s → %s", ev.str("agent"), text(p["to"])), "turn", text(p["ask"]), nil, nil, false)
			out[len(out)-1].row["turn"] = "push"
		case "task_turn_pop":
			if to := text(p["to"]); to != "" { // a stack cleared by an end or a draft gives the task to no one; a note is a comment's row
				add(at, fmt.Sprintf("%s → %s", text(p["agent"]), to), "turn", "", nil, nil, false)
				out[len(out)-1].row["turn"] = "pop"
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at < out[j].at })
	rows := make([]any, len(out))
	for i, x := range out {
		rows[i] = x.row
	}
	return rows, nil
}
