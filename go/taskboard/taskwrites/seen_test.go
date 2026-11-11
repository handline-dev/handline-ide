package taskwrites

import (
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
)

func seenCall(t *testing.T, mk func(string, ...Teller) *rpc.Server, params, rid string) map[string]any {
	t.Helper()
	return call(t, mk("", &fakeTeller{}), fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.seen","params":{%s"request_id":%q}}`, params, rid))[0]
}

func TestSeenMovesTheMarkerForwardOnlyAndReadsAwayNeitherASayNorAQuestion(t *testing.T) {
	t.Parallel()
	d, mk := discardEnv(t)
	exec(t, d,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'one', 'worker', 'manager', 1)",
		"INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(3, 1, 'worker', 'a', 1), (8, 1, 'worker', 'b', 2)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES"+
			"(21, 1, 'worker', 'p1', 'decide @human', 'queued', 'ask', 1)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(22, 1, 'worker', 'p1', 'a reply', 'landed', 'say', 1)",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 21), (1, 22)")
	cur := func() int64 { n, _ := db.Cursor(d, db.SeenKey(1)); return n }
	if r := seenCall(t, mk, `"task":1,"upto":3,`, "s1"); r["result"] == nil || cur() != 3 {
		t.Fatalf("%v marker %d", r, cur())
	}
	if seenCall(t, mk, `"task":1,"upto":1,`, "s2"); cur() != 3 {
		t.Fatalf("a lower number moved the marker back to %d", cur())
	}
	if n := count(t, d, "SELECT COUNT(*) FROM requests WHERE id = 22 AND read_at IS NULL"); n != 1 {
		t.Fatalf("a reply say linked to the task is not a comment notice and stays unread: %d", n)
	}
	if seenCall(t, mk, `"task":1,`, "s3"); cur() != 8 {
		t.Fatalf("no upto is the newest comment: %d", cur())
	}
	if n := count(t, d, "SELECT COUNT(*) FROM requests WHERE id = 21 AND read_at IS NULL AND state = 'queued'"); n != 1 {
		t.Fatalf("a question that waits is never read away: %d", n)
	}
	if r := seenCall(t, mk, `"task":99,`, "s4"); r["error"] == nil {
		t.Fatalf("an unknown task is not found: %v", r)
	}
}

func TestTheSeenFloorIsSetOnceToTheNewestCommentAndNotMovedAfter(t *testing.T) {
	t.Parallel()
	d, _ := discardEnv(t)
	exec(t, d,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'one', 'worker', 'manager', 1)",
		"INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(4, 1, 'worker', 'a', 1), (9, 1, 'worker', 'b', 2)")
	if err := SetSeenFloor(d); err != nil {
		t.Fatal(err)
	}
	exec(t, d, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(12, 1, 'worker', 'c', 3)")
	if err := SetSeenFloor(d); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := db.Meta(d, db.SeenFloorMeta); !ok || v != "9" {
		t.Fatalf("the floor is %q (set), want 9 and not moved by the second run", v)
	}
}

// task.seen on an ended task marks its result seen, whatever upto says; on an open task it leaves no mark, so the later end is unseen.
func TestSeenMarksAnEndedTasksResultOnly(t *testing.T) {
	t.Parallel()
	d, mk := discardEnv(t)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at, done_evidence, done_at) VALUES(1, 'open', 'worker', 'p1', 1, NULL, NULL), (2, 'ended', 'worker', 'p1', 1, 'x', 5)")
	seenCall(t, mk, `"task":1,`, "o")
	seenCall(t, mk, `"task":2,"upto":1,`, "e")
	if at, _ := db.Cursor(d, db.ResultSeenKey(1)); at != 0 {
		t.Fatalf("an open task got a mark: %d", at)
	}
	if at, _ := db.Cursor(d, db.ResultSeenKey(2)); at < 5 {
		t.Fatalf("the mark %d is before the end at 5, so RequestRow.ended.seen stays false", at)
	}
}

func TestTheResultFloorIsSetOnce(t *testing.T) {
	t.Parallel()
	d, _ := discardEnv(t)
	if err := db.SetMeta(d, db.ResultSeenFloorMeta, "7"); err != nil {
		t.Fatal(err)
	}
	if err := SetResultSeenFloor(d); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := db.Meta(d, db.ResultSeenFloorMeta); v != "7" {
		t.Fatalf("moved to %q", v)
	}
}

// A request the founder ends is already seen; one an agent ends is not.
func TestAResultTheFounderEndedIsAlreadySeen(t *testing.T) {
	t.Parallel()
	d, mk := discardEnv(t)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, reporter, created_at) VALUES(1, 'a', 'worker', 'p1', 'p1', 1), (2, 'b', 'worker', 'p1', 'p1', 1)")
	done := func(who string, id int) {
		callAs(t, mk, who, "task.done", fmt.Sprintf(`{"task":%d,"evidence":"it is built","request_id":"d%d"}`, id, id))
	}
	done("", 1)
	done("worker", 2)
	if at, _ := db.Cursor(d, db.ResultSeenKey(1)); at < int64(count(t, d, "SELECT CAST(done_at AS INTEGER) FROM tasks WHERE id = 1")) {
		t.Fatalf("the founder's own end is not seen: mark %d", at)
	}
	if at, _ := db.Cursor(d, db.ResultSeenKey(2)); at != 0 {
		t.Fatal("an agent's end is marked seen")
	}
	if n := count(t, d, "SELECT COUNT(*) FROM requests WHERE target = 'p1'"); n != 0 {
		t.Fatalf("%d lines to the founder", n)
	}
}

// With the founder holding the task, neither its last blocker's discard nor its child's writes them a line; the task still comes free.
func TestAnEndTellsAFounderHoldingTheNextTaskNothing(t *testing.T) {
	t.Parallel()
	d, mk := discardEnv(t)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, parent_task, created_at) VALUES(1, 'held', 'p1', 'gp', NULL, 1), (2, 'blocker', 'worker', 'gp', NULL, 1), (3, 'child', 'worker', 'gp', 1, 1)",
		"INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(1, 2, 1), (1, 3, 1)")
	f := &fakeTeller{}
	for _, id := range []int{3, 2} {
		r := call(t, mk("gp", f), fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.discard","params":{"task":%d,"reason":"not needed","request_id":"d%d"}}`, id, id))
		if r[0]["error"] != nil {
			t.Fatal(r[0])
		}
	}
	if n := count(t, d, "SELECT COUNT(*) FROM requests WHERE target = 'p1'"); n != 0 {
		t.Fatalf("%d lines to the founder", n)
	}
	if n := count(t, d, "SELECT COUNT(*) FROM task_blocks WHERE task_id = 1"); n != 0 {
		t.Fatalf("%d blocks left", n)
	}
}

// A merge folds a task away as a discard: the founder's own merge is already seen, an agent's is not.
func TestAResultTheFounderMergedAwayIsAlreadySeen(t *testing.T) {
	t.Parallel()
	d, mk := discardEnv(t)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'keep', 'worker', 'p1', 1), (2, 'two', 'worker', 'p1', 1), (3, 'three', 'worker', 'p1', 1)")
	merge := func(who string, id int) {
		callAs(t, mk, who, "task.merge", fmt.Sprintf(`{"into":1,"tasks":[%d],"request_id":"m%d"}`, id, id))
	}
	merge("", 2)
	merge("worker", 3)
	if at, _ := db.Cursor(d, db.ResultSeenKey(2)); at == 0 {
		t.Fatal("the founder's own merge is not seen")
	}
	if at, _ := db.Cursor(d, db.ResultSeenKey(3)); at != 0 {
		t.Fatal("an agent's merge is marked seen")
	}
}

// callAs calls method with params as who, and fails the test on an error.
func callAs(t *testing.T, mk func(string, ...Teller) *rpc.Server, who, method, params string) {
	t.Helper()
	r := call(t, mk(who, &fakeTeller{}), `{"jsonrpc":"2.0","id":2,"method":"`+method+`","params":`+params+`}`)
	if r[0]["error"] != nil {
		t.Fatal(r[0])
	}
}
