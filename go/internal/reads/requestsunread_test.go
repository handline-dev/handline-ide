package reads

import (
	"encoding/json"
	"testing"
)

func runAll(t *testing.T, e *Env, qs ...string) {
	t.Helper()
	for _, q := range qs {
		run(t, e, q)
	}
}

// A task with several things waiting on the founder is one row, of the strongest kind among them: a decision outranks a question.
func TestARequestsRowTakesTheStrongestKindOfWhatWaitsOnTheTask(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'both', 'bob', 'p1', 1, 'p1')")
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(1, 1, 'bob', 'q @human?', 10)")
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, memo_id, edge_ok) VALUES(20, 10, 'bob', 'p1', 'q @human?', 'queued', 'ask', 1, 1)")
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(21, 20, 'bob', 'p1', 'decide this', 'queued', 'ask', 1)")
	run(t, e, "INSERT INTO task_requests(task_id, request_id) VALUES(1, 20), (1, 21)")
	answer := pending(t, e)["answer"].([]map[string]any)
	if len(answer) != 1 || answer[0]["kind"] != "decide" {
		t.Fatalf("one row, the stronger kind: %v", answer)
	}
}

func TestTheAnswerRowsAreNewestFirst(t *testing.T) {
	e := testEnv(t)
	runAll(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'old', 'bob', 'p1', 1, 'p1'), (2, 'new', 'bob', 'p1', 1, 'p1')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(20, 10, 'bob', 'p1', 'a', 'queued', 'ask', 1), (21, 500, 'bob', 'p1', 'b', 'queued', 'ask', 1)",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 20), (2, 21)")
	if got := taskIDs(pending(t, e)["answer"]); got != "[2 1]" {
		t.Fatalf("newest first: %s", got)
	}
}

// Unread comments are a row for a task the founder owns, and for none that was discarded.
func TestTheCommentRowsCoverATaskTheFounderOwnsAndSkipADiscardedOne(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	runAll(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'mine', 'p1', 'amy', 1, 'amy')",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, discarded_by, discarded_at) VALUES(2, 'gone', 'bob', 'p1', 1, 'p1', 'p1', 5)",
		"INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(1, 1, 'bob', 'fyi', 10), (2, 2, 'bob', 'late', 11)")
	if got := taskIDs(pending(t, e)["comments"]); got != "[1]" {
		t.Fatalf("the founder's own task is listed and the discarded one is not: %s", got)
	}
}

// task.get gives an agent no unread, whatever the founder has not read.
func TestTaskGetGivesAnAgentNoUnread(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'one', 'bob', 'p1', 1, 'p1')")
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(1, 1, 'bob', 'fyi', 10)")
	for _, tc := range []struct {
		name string
		want int64
	}{{"founder", 1}, {"agent", 0}} {
		conn := asFounderConn
		if tc.name == "agent" {
			conn = asAgentConn
		}
		got, err := e.taskGet(conn, map[string]any{"task": json.Number("1")})
		if err != nil {
			t.Fatal(err)
		}
		if n := got.(map[string]any)["unread"]; n != tc.want {
			t.Fatalf("%s sees unread %v, want %d", tc.name, n, tc.want)
		}
	}
}
