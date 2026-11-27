package watcher

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/internal/prefs"
	"github.com/handline/handline/go/internal/reads"
)

// ForYou is the for_you topic: an item that joins the count of what waits on the founder (opened, or back from Later) and one that leaves
// it (answered anywhere, closed, withdrawn, deferred in the founder's prefs), each with the count after it; a rewritten question opens
// again. The first look says nothing: requests.pending shows what is there.
type ForYou struct {
	Reads *reads.Env
	seen  map[string]map[string]map[string]any // by person
	asked map[string]map[string]string         // by person: their question on each task item seen
}

// ForYouSignature says how many deferrals, of every person, have run out their time: the database does not change then, so the watch would not look.
func ForYouSignature(q *sql.DB) string {
	people, err := db.People(q)
	if err != nil {
		return ""
	}
	n := 0
	for _, p := range people {
		later, err := prefs.LaterOf(q, p)
		if err != nil {
			return ""
		}
		for _, l := range later {
			if l.AfterTask == 0 && l.Over(time.Now(), nil) {
				n++
			}
		}
	}
	return fmt.Sprint(n)
}

func (*ForYou) Name() string { return "for_you" }

func (*ForYou) Tables() []string {
	return []string{"agent_state", "cursors", "events", "meta", "people", "prefs", "prompt_blocks", "requests", "task_blocks", "task_comments", "task_requests", "task_tags", "task_turns", "tasks"} // meta: the pause moves a task between doing and queued
}

// Scan looks at each person's items: an event of one is that person's alone (its Agent).
func (w *ForYou) Scan(_ *sql.DB, emit Emit) error {
	people, err := db.People(w.Reads.DB)
	if err != nil {
		return err
	}
	if w.seen == nil {
		w.seen, w.asked = map[string]map[string]map[string]any{}, map[string]map[string]string{}
	}
	for _, p := range people {
		if err := w.scan(p, emit); err != nil {
			return err
		}
	}
	return nil
}

func (w *ForYou) scan(person string, emit Emit) error {
	items, _, err := w.Reads.For(person).ForYou()
	if err != nil {
		return err
	}
	now := map[string]map[string]any{}
	for _, it := range items {
		if it["later"] == nil {
			now[fmt.Sprint(it["kind"], "\x00", it["id"])] = it
		}
	}
	asked := map[string]string{}
	for k, it := range now {
		if it["kind"] == "task" {
			var q string
			err := w.Reads.DB.QueryRow("SELECT "+db.TurnAskSQL("u")+" FROM task_turns u WHERE task_id = ? AND agent = ?", it["id"], person).Scan(&q)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			asked[k] = q
		}
	}
	before, askedBefore := w.seen[person], w.asked[person]
	_, looked := w.seen[person]
	w.seen[person], w.asked[person] = now, asked
	if !looked {
		return nil
	}
	var closed, opened []string
	for k := range before {
		if now[k] == nil {
			closed = append(closed, k)
		}
	}
	for k := range now {
		if before[k] == nil {
			opened = append(opened, k)
		}
	}
	sort.Strings(closed)
	sort.Strings(opened)
	count := len(before)
	for _, k := range closed {
		it := before[k]
		count--
		if _, err := emit(stream.Entry{Topic: "for_you", Type: "for_you.closed", Title: it["title"].(string), Agent: person,
			Data: map[string]any{"kind": it["kind"], "id": it["id"], "count": count}}); err != nil {
			return err
		}
	}
	for _, k := range opened {
		it := now[k]
		count++
		if _, err := emit(stream.Entry{Topic: "for_you", Type: "for_you.opened", Title: it["title"].(string), Agent: person,
			Data: map[string]any{"item": it, "count": count}}); err != nil {
			return err
		}
	}
	var reasked []string
	for k, q := range asked {
		if prev, ok := askedBefore[k]; ok && before[k] != nil && q != "" && q != prev {
			reasked = append(reasked, k)
		}
	}
	sort.Strings(reasked)
	for _, k := range reasked {
		if _, err := emit(stream.Entry{Topic: "for_you", Type: "for_you.opened", Title: asked[k], Agent: person,
			Data: map[string]any{"item": now[k], "count": count}}); err != nil {
			return err
		}
	}
	return nil
}
