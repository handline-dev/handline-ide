package taskwrites

import (
	"testing"

	"github.com/handline/handline/go/data/db"
)

// An ask for a lease blocks the task on a gate with no watch and releases it; the grant closes the gate and brings it back, and so does
// a dropped ask.
func TestALeaseWaitBlocksTheTaskOnAGateThatTheGrantOpens(t *testing.T) {
	home := t.TempDir()
	sdb, tell, _ := reportEnvIn(t, home,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at) VALUES(9, 'ship', 'w', 'boss', 1, 'boss', 5)",
		"INSERT INTO requests(id, requester, target, kind, state, ts, body, delivered_at) VALUES(20, 'boss', 'w', 'task', 'landed', 1, 'x', 6)",
		"UPDATE requests SET queue_order = 1 WHERE id = 20",
		"INSERT INTO task_requests(task_id, request_id) VALUES(9, 20)",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('w', 'IDLE', 1, 9)")
	if st := taskState(t, sdb, 9); st != "doing" {
		t.Fatalf("before: %s", st)
	}
	gate, err := LeaseWait(sdb, home, tell, "w", 9, "simulator")
	if err != nil || gate != 10 {
		t.Fatalf("gate %d %v", gate, err)
	}
	g, err := db.GetTask(sdb, gate)
	if err != nil || g.Owner != db.Gate || g.Title != "T9 waits on a simulator" {
		t.Fatalf("gate %+v %v", g, err)
	}
	if st := taskState(t, sdb, 9); st != "queued" {
		t.Fatalf("waiting: %s", st)
	}
	// The gate asks no one: no request goes to it or hangs on it, so nobody, the founder included, is told of it.
	if n := count(t, sdb, "SELECT COUNT(*) FROM requests WHERE target = 'gate' OR id IN (SELECT request_id FROM task_requests WHERE task_id = 10)"); n != 0 {
		t.Fatalf("%d requests on the gate", n)
	}
	if bs, _ := db.TaskBlockers(sdb, 9); len(bs) != 1 || bs[0].ID != gate {
		t.Fatalf("blockers %+v", bs)
	}
	if err := LeaseGranted(sdb, home, tell, gate, "simulator"); err != nil {
		t.Fatal(err)
	}
	if bs, _ := db.TaskBlockers(sdb, 9); len(bs) != 0 {
		t.Fatalf("still blocked: %+v", bs)
	}
	if g, _ := db.GetTask(sdb, gate); g.DoneEvidence.String != "a simulator is reserved for it" {
		t.Fatalf("gate %+v", g)
	}
	again, err := LeaseWait(sdb, home, tell, "w", 9, "simulator")
	if err != nil {
		t.Fatal(err)
	}
	if err := LeaseDropped(sdb, home, tell, again, "T9 no longer waits"); err != nil {
		t.Fatal(err)
	}
	if g, _ := db.GetTask(sdb, again); g.DoneEvidence.String != "dropped: T9 no longer waits" {
		t.Fatalf("dropped gate %+v", g)
	}
	if bs, _ := db.TaskBlockers(sdb, 9); len(bs) != 0 {
		t.Fatalf("a dropped gate still blocks: %+v", bs)
	}
}
