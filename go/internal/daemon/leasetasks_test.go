package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/scheduler/leases"
)

// The daemon's leases wait on its task board: with one browser held, a2's ask blocks its current task on a gate, and the browser
// coming free reserves it for that task, closes the gate and lets a2 take it.
func TestALeaseAskWaitsAsAGateOnTheAgentsTask(t *testing.T) {
	d := start(t, fakeAuth{key: "k"}, "")
	if err := os.WriteFile(filepath.Join(d.cfg.Home, "config.yaml"), []byte("browser_max: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'one', 'a1', 'p1', 1, 'p1'), (2, 'two', 'a2', 'p1', 1, 'p1')",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('a1', 'RUNNING', 1, 1), ('a2', 'RUNNING', 1, 2)",
	} {
		if _, err := d.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if l, err := d.leases.Take(nil, leases.Browser, "a1", true, 0, time.Minute); err != nil || l == nil {
		t.Fatalf("a1: %v %v", l, err)
	}
	_, err := d.leases.Take(nil, leases.Browser, "a2", true, 0, time.Minute)
	var bl *leases.Blocked
	if !errors.As(err, &bl) || bl.Task != 2 {
		t.Fatalf("a2: %v", err)
	}
	if bs, err := db.TaskBlockers(d.db, 2); err != nil || len(bs) != 1 || bs[0].ID != bl.Gate || bs[0].Owner != db.Gate {
		t.Fatalf("blockers %+v %v", bs, err)
	}
	d.leases.Check(nil) // the board still reads T2 as a2's, so the ask stays
	if q := d.leases.View(); len(q[2].Queue) != 1 || q[2].Queue[0].Task != 2 {
		t.Fatalf("after a check: %+v", q)
	}
	d.leases.ReleaseAgent("a1", "", false, "the session ended")
	if bs, _ := db.TaskBlockers(d.db, 2); len(bs) != 0 {
		t.Fatalf("still blocked: %+v", bs)
	}
	if l, err := d.leases.Take(nil, leases.Browser, "a2", true, 0, time.Minute); err != nil || l == nil || l.Reserved() {
		t.Fatalf("a2 again: %+v %v", l, err)
	}
}
