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

func TestSeenMovesTheMarkerForwardOnlyAndReadsAwaySaysButNeverAQuestion(t *testing.T) {
	t.Parallel()
	d, mk := discardEnv(t)
	exec(t, d,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'one', 'worker', 'manager', 1)",
		"INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(3, 1, 'worker', 'a', 1), (8, 1, 'worker', 'b', 2)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES"+
			"(20, 1, 'task-board', 'human', 'T1 memo added', 'landed', 'say', 1), (21, 1, 'worker', 'human', 'decide @human', 'queued', 'ask', 1)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(22, 1, 'worker', 'human', 'a reply', 'landed', 'say', 1)",
		"INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'task-board', 'request_comment_told', '{\"task_id\":1,\"request_id\":20,\"count\":1}')",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 20), (1, 21), (1, 22)")
	cur := func() int64 { n, _ := db.Cursor(d, db.SeenKey(1)); return n }
	if r := seenCall(t, mk, `"task":1,"upto":3,`, "s1"); r["result"] == nil || cur() != 3 {
		t.Fatalf("%v marker %d", r, cur())
	}
	if seenCall(t, mk, `"task":1,"upto":1,`, "s2"); cur() != 3 {
		t.Fatalf("a lower number moved the marker back to %d", cur())
	}
	if n := count(t, d, "SELECT COUNT(*) FROM requests WHERE id = 20 AND read_at IS NOT NULL"); n != 1 {
		t.Fatalf("the say that announced the comments is read: %d", n)
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
