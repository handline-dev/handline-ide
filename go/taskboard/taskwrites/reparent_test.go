package taskwrites

import (
	"fmt"
	"testing"
)

// refusalOf is an error reply as "<code>: <message>", "" for a result.
func refusalOf(r map[string]any) string {
	if e, ok := r["error"].(map[string]any); ok {
		return fmt.Sprintf("%v: %v", e["data"].(map[string]any)["code"], e["message"])
	}
	return ""
}

func TestEditMovesATaskUnderAnotherAndTheParentWaitsOnItWhileBothAreOpen(t *testing.T) {
	t.Parallel()
	db, _, mk := discardEnvHome(t)
	// T1 (lead's) and T2 (boss's current task) are stories; T3 sits under T1; T4 under T3; T5 and T6 are done; T7 is discarded.
	exec(t, db,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, priority, parent_task) VALUES"+
			"(1, 'story one', 'lead', 'manager', 1, 2, NULL), (2, 'story two', 'boss', 'manager', 1, 2, NULL),"+
			"(3, 'child', 'worker', 'manager', 1, 2, 1), (4, 'grandchild', 'worker', 'manager', 1, 2, 3),"+
			"(5, 'finished', 'worker', 'manager', 1, 2, NULL), (6, 'done story', 'lead', 'manager', 1, 2, NULL),"+
			"(7, 'dropped', 'lead', 'manager', 1, 2, NULL)",
		"UPDATE tasks SET done_evidence = 'shipped' WHERE id IN (5, 6)",
		"UPDATE tasks SET discarded_by = 'manager' WHERE id = 7",
		"INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES (1, 3, 1), (3, 4, 1)",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('boss', 'IDLE', 1, 2)")
	parent := func(tid int) int {
		return count(t, db, fmt.Sprintf("SELECT COALESCE(parent_task, 0) FROM tasks WHERE id = %d", tid))
	}
	waits := func(tid, on int) bool {
		return count(t, db, fmt.Sprintf("SELECT COUNT(*) FROM task_blocks WHERE task_id = %d AND blocks_on = %d", tid, on)) == 1
	}
	f := &fakeTeller{}
	srv := mk("manager", f)
	n := 0
	edit := func(tid int, params string) map[string]any {
		n++
		return call(t, srv, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"task.edit","params":{"task":%d,%s"request_id":"p%d"}}`, n, tid, params, n))[0]
	}
	said := func(r map[string]any) any { return r["result"].(map[string]any)["said"] }
	if r := edit(3, `"parent":2,`); said(r) != "task T3 moved under T2" || parent(3) != 2 || waits(1, 3) || !waits(2, 3) {
		t.Fatalf("T3 moves under T2, which waits on it in place of T1: %v", r)
	}
	if got := fmt.Sprint(f.edited[len(f.edited)-1].Freed); got != "[lead boss]" {
		t.Fatalf("T1 waits on nothing now, so lead is reconsidered; T2 waits, so boss lets it go: %s", got)
	}
	if count(t, db, "SELECT COUNT(*) FROM events WHERE kind = 'unblock_task' AND json_extract(payload, '$.task_id') = 1 AND json_extract(payload, '$.blocks_on') = 3") != 1 {
		t.Fatal("T1's wait on T3 ending is kept in its history")
	}
	for _, c := range []struct {
		tid    int
		params string
		want   string
	}{
		{1, `"parent":1,`, "refused: task T1 cannot be its own parent"},
		{2, `"parent":4,`, "refused: task T2 cannot go under T4: T4 sits under it"},
		{3, `"parent":7,`, "refused: task T7 is discarded - a task cannot go under it"},
		{3, `"parent":2,"no_parent":true,`, "bad_request: no_parent and parent together: choose one"},
		{3, `"parent":2,`, "refused: task T3 already sits there - nothing to move"},
		{1, `"no_parent":true,`, "refused: task T1 already sits there - nothing to move"},
	} {
		if got := refusalOf(edit(c.tid, c.params)); got != c.want {
			t.Fatalf("T%d %s: %q, want %q", c.tid, c.params, got, c.want)
		}
	}
	if r := edit(5, `"parent":2,`); refusalOf(r) != "" || parent(5) != 2 || waits(2, 5) {
		t.Fatalf("a done child goes under an open parent without a wait: %v", r)
	}
	if r := edit(4, `"parent":6,`); refusalOf(r) != "" || parent(4) != 6 || waits(3, 4) || waits(6, 4) {
		t.Fatalf("a done parent takes an open child without the wait, and the old parent stops waiting: %v", r)
	}
	if r := edit(3, `"no_parent":true,`); said(r) != "task T3 moved from under T2 to the top" || parent(3) != 0 || waits(2, 3) {
		t.Fatalf("no_parent moves T3 to the top and T2 stops waiting on it: %v", r)
	}
	if got := fmt.Sprint(f.edited[len(f.edited)-1].Freed); got != "[boss]" {
		t.Fatalf("T2 waits on nothing once T3 leaves, so boss is reconsidered: %s", got)
	}
	if k := count(t, db, "SELECT COUNT(*) FROM events WHERE kind = 'edit_task' AND json_extract(payload, '$.parent.old') = 'T2' AND json_extract(payload, '$.parent.new') = ''"); k != 1 {
		t.Fatalf("the move to the top is kept as an edit of parent: %d", k)
	}
}

func TestAMoveWhoseWaitWouldCloseALoopIsRefusedAndUndone(t *testing.T) {
	t.Parallel()
	db, _, mk := discardEnvHome(t)
	// T1 waits on T2: T1 under T2 would have T2 wait on T1 too, a loop.
	exec(t, db,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, priority, parent_task) VALUES"+
			"(1, 'one', 'worker', 'manager', 1, 2, 3), (2, 'two', 'lead', 'manager', 1, 2, NULL), (3, 'old story', 'lead', 'manager', 1, 2, NULL)",
		"INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES (1, 2, 1), (3, 1, 1)")
	r := call(t, mk("manager", &fakeTeller{}),
		`{"jsonrpc":"2.0","id":1,"method":"task.edit","params":{"task":1,"parent":2,"request_id":"q1"}}`)
	if got := refusalOf(r[0]); got == "" || got[:8] != "refused:" {
		t.Fatalf("T2 waiting on T1, which waits on T2, is a loop: %v", r[0])
	}
	if count(t, db, "SELECT COALESCE(parent_task, 0) FROM tasks WHERE id = 1") != 3 ||
		count(t, db, "SELECT COUNT(*) FROM task_blocks WHERE task_id = 3 AND blocks_on = 1") != 1 {
		t.Fatal("the refused move left T1 under T3, which still waits on it")
	}
}

func TestAMoveTellsTheOldAndNewParentsOwnersOneLineEach(t *testing.T) {
	t.Parallel()
	db, _, mk := discardEnvHome(t)
	// T3 (gp's) sits under worker's T1 and moves under other's T2; manager opened it and moves it.
	exec(t, db,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, priority, parent_task) VALUES"+
			"(1, 'old story', 'worker', 'manager', 1, 2, NULL), (2, 'new story', 'other', 'manager', 1, 2, NULL),"+
			"(3, 'child', 'gp', 'manager', 1, 2, 1)",
		"INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES (1, 3, 1)")
	f := &fakeTeller{db: db}
	r := call(t, mk("manager", f), `{"jsonrpc":"2.0","id":1,"method":"task.edit","params":{"task":3,"parent":2,"request_id":"m1"}}`)
	if refusalOf(r[0]) != "" {
		t.Fatalf("the move: %v", r[0])
	}
	got := map[string]string{}
	for _, n := range f.notified {
		got[n.To] = n.Text
	}
	if len(got) != 2 || got["worker"] != "task T3 moved under T2" || got["other"] != "task T3 moved under T2" {
		t.Fatalf("worker's T1 stops waiting and other's T2 starts: each owner gets the move, nobody else: %v", f.notified)
	}
}
