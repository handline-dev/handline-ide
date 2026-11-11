package reads

import (
	"database/sql"
	"sort"

	"github.com/handline/handline/go/internal/transcript"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/taskboard/handoff"
)

const bashRowsSQL = "requester = '" + actor.OwnerKey + "' AND kind = 'say' AND body LIKE '!%' AND typed_at IS NOT NULL"

// BashLine is a founder's ! line: who it went to, and whether it has landed.
type BashLine struct {
	ID     int64
	Agent  string
	Landed bool
}

// BashLines are the founder's ! lines sent from second since on.
func BashLines(q *sql.DB, since int64) ([]BashLine, error) {
	rows, err := q.Query("SELECT id, target, landed_at IS NOT NULL FROM requests WHERE ts >= ? AND "+bashRowsSQL+" ORDER BY id", since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BashLine
	for rows.Next() {
		var l BashLine
		if err := rows.Scan(&l.ID, &l.Agent, &l.Landed); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// BashItems are the bash items of agent's sessions from second from up to second to (0 for no end), oldest first, read from the transcripts of
// the sessions the founder's ! lines in that span went to; with no such line no transcript is read. Lines outside the span pair too, so that
// one at its edge does not take another's command. waiting are the lines whose output is not recorded yet.
func (e *Env) BashItems(agent string, from, to int64) (items []map[string]any, waiting []int64, err error) {
	sent, err := query(e.DB, "SELECT id, ts, typed_at, landed_at, typed_session FROM requests WHERE target = ? AND "+bashRowsSQL+" ORDER BY id", agent)
	if err != nil {
		return nil, nil, err
	}
	var commands []transcript.Bash
	read := map[string]bool{}
	for _, r := range sent {
		at := when(r)
		if at < from || (to != 0 && at > to) {
			continue
		}
		session, err := e.sessionTypedInto(agent, r)
		if err != nil {
			return nil, nil, err
		}
		if session == "" || read[session] {
			continue
		}
		read[session] = true
		path := (&handoff.Env{Home: e.Home}).FindTranscript(session, "")
		if path == "" {
			continue
		}
		found, err := transcript.BashCommands(path)
		if err != nil {
			continue // the transcript went between finding it and opening it: as if not found
		}
		commands = append(commands, found...)
	}
	sort.SliceStable(commands, func(i, j int) bool { return commands[i].ID < commands[j].ID })
	pairs := pairBash(sent, commands)
	for i, c := range commands {
		if pairs[i] != 0 && !c.Done {
			waiting = append(waiting, pairs[i])
		}
		if c.At < from || (to != 0 && c.At > to) {
			continue
		}
		items = append(items, bashItemOf(c, pairs[i]))
	}
	return items, waiting, nil
}

// sessionTypedInto is the session a ! line was typed into: as it recorded, else the one the agent had started by then.
func (e *Env) sessionTypedInto(agent string, r row) (string, error) {
	if s := r.str("typed_session"); s != "" {
		return s, nil
	}
	starts, err := query(e.DB, "SELECT session_id FROM session_starts WHERE agent = ? AND ts <= ? AND session_id IS NOT NULL ORDER BY id DESC LIMIT 1", agent, r.i("typed_at"))
	if err != nil || len(starts) == 0 {
		return "", err
	}
	return starts[0].str("session_id"), nil
}

// pairBash gives each command the ! line that ran it, by position, 0 for none: lines in order take the commands recorded between their typing
// and their landing, the oldest first, since a session runs queued lines in order. A line that never landed has no pair.
func pairBash(sent []row, commands []transcript.Bash) []int64 {
	pairs := make([]int64, len(commands))
	for _, r := range sent {
		if !r.set("landed_at") {
			continue
		}
		for i, c := range commands {
			if pairs[i] == 0 && c.At >= r.i("typed_at") && c.At <= r.i("landed_at") {
				pairs[i] = r.i("id")
				break
			}
		}
	}
	return pairs
}

func bashItemOf(c transcript.Bash, request int64) map[string]any {
	item := map[string]any{"id": c.ID, "at": ts(c.At), "kind": "bash", "command": c.Command, "done": c.Done}
	if c.Done {
		item["stdout"], item["stderr"] = c.Stdout, c.Stderr
		if c.Dropped > 0 {
			item["dropped_lines"] = c.Dropped
		}
	}
	if request != 0 {
		item["request"] = request
	}
	return item
}

// withBashItems adds to a chat.transcript page the bash items in its span: from the oldest item when older ones remain, else from the start,
// up to before. A since page spans from since's second.
func (e *Env) withBashItems(agent string, items []map[string]any, before, since *int64, more bool) ([]map[string]any, error) {
	var from, to int64
	switch {
	case since != nil:
		from = *since / 1_000_000
	case more && len(items) > 0:
		from = items[0]["id"].(int64) / 1_000_000
	}
	if before != nil {
		to = *before / 1_000_000
	}
	bash, _, err := e.BashItems(agent, from, to)
	if err != nil {
		return nil, err
	}
	for _, it := range bash {
		if before == nil || it["id"].(int64) < *before {
			items = append(items, it)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i]["id"].(int64) < items[j]["id"].(int64) })
	return items, nil
}
