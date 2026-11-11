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
	"github.com/handline/handline/go/kit/actor"
)

// ForYou is the for_you topic: an item that joins the count of what waits on the founder (opened, or back from Later) and one that leaves
// it (answered anywhere, closed, withdrawn, deferred in the founder's prefs), each with the count after it; a rewritten question opens
// again. The first look says nothing: for_you.list shows what is there.
type ForYou struct {
	Reads *reads.Env
	seen  map[string]map[string]any
	asked map[string]string // the founder's question on each task item seen
}

// ForYouSignature says how many deferrals have run out their time: the database does not change then, so the watch would not look.
func ForYouSignature(q *sql.DB) string {
	later, err := prefs.LaterOf(q, actor.OwnerKey)
	if err != nil {
		return ""
	}
	n := 0
	for _, l := range later {
		if l.AfterTask == 0 && l.Over(time.Now(), nil) {
			n++
		}
	}
	return fmt.Sprint(n)
}

func (*ForYou) Name() string { return "for_you" }

func (*ForYou) Tables() []string {
	return []string{"agent_state", "cursors", "events", "meta", "prefs", "prompt_blocks", "requests", "task_blocks", "task_memos", "task_requests", "task_tags", "task_turns", "tasks"} // meta: the pause moves a task between doing and queued
}

func (w *ForYou) Scan(_ *sql.DB, emit Emit) error {
	items, _, err := w.Reads.ForYou()
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
			err := w.Reads.DB.QueryRow("SELECT "+db.TurnAskSQL("u")+" FROM task_turns u WHERE task_id = ? AND agent = '"+actor.OwnerKey+"'", it["id"]).Scan(&q)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			asked[k] = q
		}
	}
	before, askedBefore := w.seen, w.asked
	w.seen, w.asked = now, asked
	if before == nil {
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
		if _, err := emit(stream.Entry{Topic: "for_you", Type: "for_you.closed", Title: it["title"].(string),
			Data: map[string]any{"kind": it["kind"], "id": it["id"], "count": count}}); err != nil {
			return err
		}
	}
	for _, k := range opened {
		it := now[k]
		count++
		if _, err := emit(stream.Entry{Topic: "for_you", Type: "for_you.opened", Title: it["title"].(string),
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
		if _, err := emit(stream.Entry{Topic: "for_you", Type: "for_you.opened", Title: asked[k],
			Data: map[string]any{"item": now[k], "count": count}}); err != nil {
			return err
		}
	}
	return nil
}
