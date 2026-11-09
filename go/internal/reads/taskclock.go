package reads

import (
	"time"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
)

// taskClock is the spec's TaskClock: the task's time box, null once it is done or discarded; it runs while doing and nothing holds it, and
// while it is paused left_s is what it had left (below zero when over). start is end less the length, so a clock that paused and resumed
// reads as begun later than it was. A clock past its end runs on, over since its end; told counts the notices of this box.
func (e *Env) taskClock(t row, state string) (any, error) {
	if state == "done" || state == "discarded" {
		return nil, nil
	}
	length := t.i("timeout_s")
	isDefault := !t.set("timeout_s")
	if isDefault || length == 0 {
		length = db.DefaultTaskTimeoutS
	}
	told, err := db.TimeoutNotices(e.DB, t.i("id"))
	if err != nil {
		return nil, err
	}
	clock := map[string]any{"length_s": length, "default": isDefault, "start": nil, "end": nil, "overdue_since": nil, "told": int(told.N), "left_s": nil}
	if !t.set("timeout_at") {
		if t.set("timeout_left") {
			clock["left_s"] = t.i("timeout_left")
		}
		return clock, nil
	}
	if state != "doing" {
		return clock, nil
	}
	now, end := time.Now().Unix(), t.i("timeout_at")
	clock["start"], clock["end"] = ts(end-length), ts(end)
	waits, err := db.TaskWaits(e.DB, t.i("id"))
	if err != nil {
		return nil, err
	}
	if verdict.Overdue(state, db.HolderOf(actingOf(t)), waits, end, now) {
		clock["overdue_since"] = ts(end)
	}
	return clock, nil
}
