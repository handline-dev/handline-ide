package reads

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

var asAgentConn = &rpc.Conn{Caller: rpc.Caller{Role: "agent", Info: map[string]any{"name": "bob"}}}

func pending(t *testing.T, e *Env) map[string]any {
	t.Helper()
	got, err := e.requestsPending(asFounderConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	return got.(map[string]any)
}

func taskIDs(rows any) string {
	var out []int64
	for _, r := range rows.([]map[string]any) {
		out = append(out, r["task"].(int64))
	}
	return fmt.Sprint(out)
}

// A task counts the comments of anyone but the founder above the founder's marker, from the floor when it has none, and says where the first
// unread one is.
func TestATaskCountsTheCommentsAboveTheFoundersMarkerAndNamesTheFirst(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'one', 'human', 'bob', 1, 'bob'), (2, 'two', 'bob', 'human', 1, 'human')")
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(5, 1, 'bob', 'a', 10), (6, 1, 'human', 'mine', 11), (7, 1, 'bob', 'b', 12), (8, 2, 'bob', 'c', 13)")
	got, err := e.Tasks([]int64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if got[1]["unread"] != int64(2) || got[1]["first_unread"] != int64(5) || got[2]["unread"] != int64(1) || got[2]["first_unread"] != int64(8) {
		t.Fatalf("the founder's own comment is not unread: %v %v / %v %v", got[1]["unread"], got[1]["first_unread"], got[2]["unread"], got[2]["first_unread"])
	}
	run(t, e, "INSERT INTO cursors(name, row_id, updated) VALUES('task_seen:human:1', 5, 1)")
	run(t, e, "INSERT INTO meta(name, value, updated) VALUES('task_seen_floor', '7', 1)")
	got, _ = e.Tasks([]int64{1, 2})
	if got[1]["unread"] != int64(0) || got[1]["first_unread"] != nil || got[2]["unread"] != int64(1) {
		t.Fatalf("the floor (7) is above T1's marker (5) and below T2's comment: %v %v / %v", got[1]["unread"], got[1]["first_unread"], got[2]["unread"])
	}
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(9, 1, 'bob', 'new', 14)")
	got, _ = e.Tasks([]int64{1})
	if got[1]["unread"] != int64(1) || got[1]["first_unread"] != int64(9) {
		t.Fatalf("a comment above the floor is unread: %v %v", got[1]["unread"], got[1]["first_unread"])
	}
	list, err := e.tasksList(asAgentConn, map[string]any{"state": "open"})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range list.(map[string]any)["items"].([]any) {
		if it.(map[string]any)["unread"] != int64(0) || it.(map[string]any)["first_unread"] != nil {
			t.Fatalf("an agent has none unread: %v", it)
		}
	}
}

// Requests are grouped by task: a question is an answer row, unread comments on a task the founder answers for are a comment row, and a task
// the founder read drops out of the comments but never out of the answers. Needs you counts the task they own and the one waiting on their answer.
func TestRequestsPendingGroupsByTaskAndNeverReadsAQuestionAway(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES"+
		"(1, 'asks', 'bob', 'human', 1, 'human'), (2, 'comments', 'bob', 'human', 1, 'human'), (3, 'not mine', 'bob', 'amy', 1, 'amy'), "+
		"(4, 'owned', 'human', 'amy', 1, 'amy'), (5, 'a draft', 'bob', 'amy', 1, NULL)")
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(1, 1, 'bob', 'q @human?', 10), (2, 1, 'bob', 'more', 11), "+
		"(3, 2, 'bob', 'fyi', 12), (4, 3, 'bob', 'not for the founder', 13)")
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, memo_id, edge_ok) VALUES(20, 10, 'bob', 'human', 'q @human?', 'queued', 'ask', 1, 1)")
	run(t, e, "INSERT INTO task_requests(task_id, request_id) VALUES(1, 20)")
	got := pending(t, e)
	answer, comments, counts := got["answer"].([]map[string]any), got["comments"].([]map[string]any), got["counts"].(map[string]any)
	if taskIDs(got["answer"]) != "[1]" || answer[0]["kind"] != "ask" || answer[0]["unread"] != int64(2) || answer[0]["first_unread"] != int64(1) {
		t.Fatalf("one row for the task with the question, with its unread comments: %v", answer)
	}
	if taskIDs(got["comments"]) != "[2]" || comments[0]["kind"] != "comment" {
		t.Fatalf("comments on a task the founder answers for, and none on T3 or on T1 (already an answer): %v", comments)
	}
	if counts["answer"] != 1 || counts["comments"] != 1 || counts["needs_you"] != 2 || counts["drafts"] != 1 {
		t.Fatalf("the badges' counts: %v", counts)
	}
	deferNav(t, e, `{"comment:1":{"until":"`+stamp(time.Now().Unix()+3600)+`"}}`)
	if got := pending(t, e); taskIDs(got["answer"]) != "[]" {
		t.Fatalf("a question the founder put off with Later is not in the count: %v", got["answer"])
	}
	deferNav(t, e, `{}`)
	for _, id := range []int64{1, 2} { // read both: the comment row goes, the question does not
		run(t, e, fmt.Sprintf("INSERT INTO cursors(name, row_id, updated) VALUES('task_seen:human:%d', 9, 1)", id))
	}
	got = pending(t, e)
	if taskIDs(got["answer"]) != "[1]" || taskIDs(got["comments"]) != "[]" {
		t.Fatalf("a read task drops out of the comments and a question stays: %v / %v", got["answer"], got["comments"])
	}
	if a := got["answer"].([]map[string]any)[0]; a["unread"] != int64(0) || a["first_unread"] != nil {
		t.Fatalf("the question row has no unread comments left: %v", a)
	}
}

// A task the founder handed to themself asks them nothing: it is the Tasks page's Needs you, not their turn.
func TestRequestsPendingLeavesOutATaskTheFounderQueuedToThemself(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'my story', 'human', 'human', 1, 'human'), "+
		"(2, 'handed', 'human', 'bob', 1, 'bob')")
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(20, 10, 'human', 'human', 'story', 'queued', 'task', 1), "+
		"(21, 11, 'bob', 'human', 'T2 handed to you', 'queued', 'task', 1)")
	run(t, e, "INSERT INTO task_requests(task_id, request_id) VALUES(1, 20), (2, 21)")
	got := pending(t, e)
	if taskIDs(got["answer"]) != "[2]" || got["counts"].(map[string]any)["answer"] != 1 {
		t.Fatalf("only the agent's handoff is the founder's turn: %v", got["answer"])
	}
}

// Comment rows sort newest first, and the higher task id first when two tasks last heard at the same second.
func TestRequestsPendingCommentsTieOnLastAtBreakByTaskIDDescending(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES"+
		"(1, 'a', 'bob', 'human', 1, 'human'), (2, 'b', 'bob', 'human', 1, 'human'), (3, 'c', 'bob', 'human', 1, 'human')")
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(1, 1, 'bob', 'x', 10), (2, 2, 'bob', 'y', 10), (3, 3, 'bob', 'z', 5)")
	if got := taskIDs(pending(t, e)["comments"]); got != "[2 1 3]" {
		t.Fatalf("newest first, equal times by task id descending: %s", got)
	}
}

func TestTasksListServesOpenAndDraftWholeAndClosedByCursorWithBothKinds(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for i := 1; i <= 5; i++ {
		run(t, e, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(%d, 'q', 'bob', 'amy', 1, 'amy')", i))
	}
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(6, 'd', 'bob', 'amy', 1), (7, 'd2', 'bob', 'amy', 1)")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at) VALUES(10, 'doing', 'bob', 'amy', 1, 'amy', 5)")
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'WORKING', 1, 10)")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, done_evidence, done_at) VALUES(8, 'done', 'bob', 'amy', 1, 'x', 100)")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, discarded_by, discarded_at) VALUES(9, 'gone', 'bob', 'amy', 1, 'amy', 200)")
	ids := func(params map[string]any) (string, map[string]any) {
		got, err := e.tasksList(asFounderConn, params)
		if err != nil {
			t.Fatal(err)
		}
		res := got.(map[string]any)
		var out []any
		for _, it := range res["items"].([]any) {
			out = append(out, it.(map[string]any)["id"])
		}
		return fmt.Sprint(out), res
	}
	if got, res := ids(map[string]any{"state": "open", "limit": json.Number("2")}); got != "[10 5 4 3 2 1]" || res["more"] != false {
		t.Fatalf("every open row, the doing one too, whatever the limit: %s %v", got, res["more"])
	}
	if got, res := ids(map[string]any{"state": "draft", "limit": json.Number("1")}); got != "[7 6]" || res["more"] != false {
		t.Fatalf("every draft: %s %v", got, res["more"])
	}
	first, res := ids(map[string]any{"state": "closed", "order": "closed", "limit": json.Number("1")})
	next, _ := res["next"].(string)
	if first != "[9]" || next == "" {
		t.Fatalf("closed pages, the discard first (it closed last): %s %q", first, next)
	}
	if rest, _ := ids(map[string]any{"state": "closed", "order": "closed", "cursor": next}); rest != "[8]" {
		t.Fatalf("done and discarded in one list: %s", rest)
	}
	if _, err := e.tasksList(asFounderConn, map[string]any{"state": "closed"}); err == nil {
		t.Fatal("closed without order closed is refused")
	}
}
