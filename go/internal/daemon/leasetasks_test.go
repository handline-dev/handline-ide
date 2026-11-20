package daemon

import (
	"errors"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/scheduler/leases"
	"github.com/handline/handline/go/scheduler/runs"
)

// The daemon's leases wait on its task board: with This Mac's one turn at a shared run held, a2's ask blocks its current task on a
// gate, and the turn coming free reserves it for that task, closes the gate and lets a2 take it.
func TestALeaseAskWaitsAsAGateOnTheAgentsTask(t *testing.T) {
	d := start(t, fakeAuth{key: "k"}, "")
	if err := runs.Set(d.db, runs.Run{Name: "shots", Cmds: []string{"@browser"}, AtOnce: 1, SetBy: "a1"}); err != nil {
		t.Fatal(err)
	}
	d.runs.Changed() // as run.set does
	const shots = "mac:shots"
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'one', 'a1', 'p1', 1, 'p1'), (2, 'two', 'a2', 'p1', 1, 'p1')",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('a1', 'RUNNING', 1, 1), ('a2', 'RUNNING', 1, 2)",
	} {
		if _, err := d.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if l, err := d.leases.Take(nil, shots, "a1", true, 0, time.Minute); err != nil || l == nil {
		t.Fatalf("a1: %v %v", l, err)
	}
	_, err := d.leases.Take(nil, shots, "a2", true, 0, time.Minute)
	var bl *leases.Blocked
	if !errors.As(err, &bl) || bl.Task != 2 {
		t.Fatalf("a2: %v", err)
	}
	if bs, err := db.TaskBlockers(d.db, 2); err != nil || len(bs) != 1 || bs[0].ID != bl.Gate || bs[0].Owner != db.Gate {
		t.Fatalf("blockers %+v %v", bs, err)
	}
	d.leases.Check(nil) // the board still reads T2 as a2's, so the ask stays
	if q := d.leases.View(); len(q) != 2 || q[1].Kind != shots || q[1].Max != 1 || len(q[1].Queue) != 1 || q[1].Queue[0].Task != 2 {
		t.Fatalf("after a check: %+v", q)
	}
	d.leases.ReleaseAgent("a1", "", false, "the session ended")
	if bs, _ := db.TaskBlockers(d.db, 2); len(bs) != 0 {
		t.Fatalf("still blocked: %+v", bs)
	}
	if l, err := d.leases.Take(nil, shots, "a2", true, 0, time.Minute); err != nil || l == nil || l.Reserved() {
		t.Fatalf("a2 again: %+v %v", l, err)
	}
}
