package db

import (
	"slices"
	"testing"
	"time"
)

// The tasks the pause keeps from an agent: queued for it, not told, passing neither as the founder's nor by task.send_now, and held by
// nothing else.
func TestThePauseHoldsOnlyTheAgentsUntoldTasksThatNothingElseHolds(t *testing.T) {
	q := open(t)
	now := time.Unix(100, 0)
	queued := func(from string) (int64, int64) {
		t.Helper()
		tid := seedTask(t, q, "b", from)
		SetTaskQueued(q, tid, from)
		rid := ask(t, q, from, "b", "work", "task")
		LinkRequest(q, tid, rid)
		return tid, rid
	}
	cs := seedTask(t, q, "c", "a") // an older task of another agent's on the same request
	held, heldRid := queued("a")
	LinkRequest(q, cs, heldRid)
	merged := seedTask(t, q, "b", "a") // a second task of b's on the same delivery
	SetTaskQueued(q, merged, "a")
	LinkRequest(q, merged, heldRid)
	founders, _ := queued(Human)
	told, toldRid := queued("a")
	StampTaskDelivered(q, toldRid)
	typed, typedRid := queued("a") // typed, its showing not yet confirmed
	q.Exec("UPDATE requests SET delivered_at = 6 WHERE id = ?", typedRid)
	blocked, _ := queued("a")
	AddBlock(q, blocked, held)
	turned, _ := queued("a")
	PushTurn(q, turned, Turn{Agent: Human, By: "a", Since: 1})
	gated, _ := queued("a")
	PushTurn(q, gated, Turn{Agent: Gate, By: "a", Since: 1})
	passed, passedRid := queued("a")
	q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'b', ?, json_object('request_id', ?))", PausePassed, passedRid)
	asked := seedTask(t, q, "b", "a") // a task with only an ask on it: no delivery the pause holds
	SetTaskQueued(q, asked, "a")
	LinkRequest(q, asked, ask(t, q, "a", "b", "a question", "ask"))

	if got, err := PauseHeldTasks(q, "b", now); err != nil || got != nil {
		t.Fatalf("not paused: %v %v", got, err)
	}
	SetMeta(q, PauseKey, `{"since": 1}`)
	got, err := PauseHeldTasks(q, "b", now)
	if err != nil || !slices.Equal(got, []int64{held, merged}) {
		t.Fatalf("held %v (%v); want only T%d and T%d, not the founder's T%d, told T%d, typed T%d, blocked T%d, turned T%d, gated T%d, passed T%d, asked T%d",
			got, err, held, merged, founders, told, typed, blocked, turned, gated, passed, asked)
	}
	if other, _ := PauseHeldTasks(q, "c", now); len(other) != 0 {
		t.Fatalf("another agent's: %v", other)
	}
}

func TestAMachinesIDIsMadeOnceAndKept(t *testing.T) {
	q := open(t)
	a, err := MachineID(q)
	if err != nil || len(a) != 16 {
		t.Fatalf("%q %v", a, err)
	}
	if b, _ := MachineID(q); b != a {
		t.Fatalf("a second ask made another: %q %q", a, b)
	}
}
