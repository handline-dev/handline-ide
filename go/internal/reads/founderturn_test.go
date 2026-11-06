package reads

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/sparenotice"
)

func answerRow(t *testing.T, e *Env, source, id string) map[string]any {
	t.Helper()
	for _, r := range pending(t, e)["answer"].([]map[string]any) {
		if r["source"] == source && r["id"] == id {
			return r
		}
	}
	return nil
}

func noteLost(t *testing.T, e *Env, prefix string, id int64, agent, why string) {
	t.Helper()
	if err := db.NoteUndelivered(e.DB, prefix, id, 100, agent, why); err != nil {
		t.Fatal(err)
	}
}

// Two messages lost together warn apart: the later one having been written does not clear the earlier.
func TestTwoMessagesLostTogetherEachWarn(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, ref, typed_at, edge_ok) VALUES"+
		"(5, 1, 'human', 'bob', 'one', 'queued', 'say', 'abc', 2, 1), (6, 1, 'human', 'bob', 'two', 'queued', 'say', 'abd', 2, 1)")
	noteLost(t, e, db.LostMessagePrefix, 5, "bob", "lost")
	noteLost(t, e, db.LostMessagePrefix, 6, "bob", "lost")
	if answerRow(t, e, "machine", "delivery:"+db.MsgPrefix+"abc") == nil || answerRow(t, e, "machine", "delivery:"+db.MsgPrefix+"abd") == nil {
		t.Fatalf("%v", pending(t, e)["answer"])
	}
}

// A delivery from before the refusal does not end the warning: the task went away and came back, and this one was refused.
func TestAnOldDeliveryDoesNotEndARefusal(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	runAll(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(7, 'the task', 'gone', 'human', 1, 'human')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(5, 50, 'human', 'gone', 'b', 'landed', 'task', 1)",
		"INSERT INTO task_requests(task_id, request_id) VALUES(7, 5)")
	noteLost(t, e, db.RefusedTaskPrefix, 7, "gone", "refused")
	if answerRow(t, e, "machine", "undelivered:7") == nil {
		t.Fatalf("%v", pending(t, e)["answer"])
	}
}

// A board line linked to a task, from an older daemon, does not say the task waits on the founder.
func TestAnOldBoardLineDoesNotWaitOnTheFounder(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	runAll(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(7, 'the task', 'bob', 'human', 1, 'human')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, landed_at, edge_ok) VALUES(5, 1, '"+db.NoticeRequester+"', 'human', 'T7 unblocked', 'landed', 'say', 1, 1)",
		"INSERT INTO task_requests(task_id, request_id) VALUES(7, 5)")
	if c := pending(t, e)["counts"].(map[string]any); c["needs_you"] != 0 {
		t.Fatalf("%v", c)
	}
}

// A message never seen to arrive is a warning until it is seen to, is withdrawn, or a later message of the founder's to that agent is written.
func TestALostMessageIsAWarningUntilItsStateSaysItIsOver(t *testing.T) {
	t.Parallel()
	for name, over := range map[string]string{
		"it landed":     "UPDATE requests SET landed_at = 9 WHERE id = 5",
		"it was read":   "UPDATE requests SET read_at = 9 WHERE id = 5",
		"it was closed": "UPDATE requests SET state = 'withdrawn' WHERE id = 5",
		"a later one was written": "INSERT INTO requests(id, ts, requester, target, body, state, kind, typed_at, edge_ok) " +
			"VALUES(6, 9, 'human', 'bob', 'again', 'queued', 'say', 9, 1)",
	} {
		e := testEnv(t)
		run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, ref, edge_ok) VALUES(5, 1, 'human', 'bob', 'ship it\nnow', 'queued', 'say', 'abc', 1)")
		noteLost(t, e, db.LostMessagePrefix, 5, "bob", "was written to its session, and its transcript never showed it (x): it may or may not have been read")
		r := answerRow(t, e, "machine", "delivery:"+db.MsgPrefix+"abc")
		if r == nil {
			t.Fatalf("%s: no row: %v", name, pending(t, e)["answer"])
		}
		w := r["warning"].(map[string]any)
		if r["kind"] != "warning" || r["agent"] != nil || r["task"] != nil || w["kind"] != "delivery" || w["agent"] != "bob" || w["ref"] != db.MsgPrefix+"abc" ||
			r["title"] != "Your message to bob may not have been read: ship it" {
			t.Fatalf("%s: %v", name, r)
		}
		if c := pending(t, e)["counts"].(map[string]any)["answer"]; c != 1 {
			t.Fatalf("%s: the count holds the warning: %v", name, c)
		}
		run(t, e, over)
		if r := answerRow(t, e, "machine", "delivery:"+db.MsgPrefix+"abc"); r != nil {
			t.Errorf("%s: it stayed: %v", name, r)
		}
	}
}

func TestAMessageNeverWrittenSaysItWasNotRead(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, ref, edge_ok) VALUES(5, 1, 'human', 'bob', 'ship it', 'queued', 'say', 'abc', 1)")
	noteLost(t, e, db.LostMessagePrefix, 5, "bob", "was not written to its session (no inbox): it was not read")
	if r := answerRow(t, e, "machine", "delivery:"+db.MsgPrefix+"abc"); r == nil || r["title"] != "Your message to bob was not read: ship it" {
		t.Fatalf("%v", r)
	}
}

// A task whose delivery was refused is a warning on the task's own row until it is delivered, given to another or ended.
func TestARefusedTaskIsAWarningOnItsRowUntilItMoves(t *testing.T) {
	t.Parallel()
	for name, over := range map[string]string{
		"delivered":        "INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(5, 200, 'human', 'gone', 'b', 'queued', 'task', 1); INSERT INTO task_requests(task_id, request_id) VALUES(7, 5)",
		"given to another": "UPDATE tasks SET owner = 'bob' WHERE id = 7",
		"ended":            "UPDATE tasks SET done_evidence = 'x', done_at = 9 WHERE id = 7",
	} {
		e := testEnv(t)
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(7, 'the task', 'gone', 'human', 1, 'human')")
		noteLost(t, e, db.RefusedTaskPrefix, 7, "gone", "unknown target 'gone'")
		r := answerRow(t, e, "machine", "undelivered:7")
		if r == nil {
			t.Fatalf("%s: no row", name)
		}
		w := r["warning"].(map[string]any)
		if r["task"] != int64(7) || r["title"] != "the task" || r["kind"] != "warning" || w["kind"] != "undelivered" || w["agent"] != "gone" ||
			r["ask"] != "T7 could not be delivered to gone: unknown target 'gone'" {
			t.Fatalf("%s: %v", name, r)
		}
		run(t, e, over)
		if r := answerRow(t, e, "machine", "undelivered:7"); r != nil {
			t.Errorf("%s: it stayed: %v", name, r)
		}
	}
}

// What the login watch keeps is a warning with the time it kept, and it leaves when the watch clears it.
func TestAnAccountThatNeedsALoginIsAWarningWhileTheWatchKeepsIt(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if err := db.SetMeta(e.DB, db.LoginNeedPrefix+"alpha", "100"); err != nil {
		t.Fatal(err)
	}
	r := answerRow(t, e, "machine", "login:alpha")
	if r == nil || r["kind"] != "warning" || r["title"] != "Account alpha needs a login." || r["last_at"] != ts(int64(100)) || r["warning"].(map[string]any)["account"] != "alpha" {
		t.Fatalf("%v", pending(t, e)["answer"])
	}
	if err := db.ClearMeta(e.DB, db.LoginNeedPrefix+"alpha"); err != nil {
		t.Fatal(err)
	}
	if r := answerRow(t, e, "machine", "login:alpha"); r != nil {
		t.Fatalf("it stayed: %v", r)
	}
}

// The warnings stay out of daemon.status: a lost message or a login must not turn the health line held.
func TestAWarningIsNotAMachineNeed(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, ref, edge_ok) VALUES(5, 1, 'human', 'bob', 'x', 'queued', 'say', 'abc', 1)")
	noteLost(t, e, db.LostMessagePrefix, 5, "bob", "lost")
	if needs := e.machineNeeds(); len(needs) != 0 {
		t.Fatalf("%v", needs)
	}
	items, count, err := e.ForYou()
	if err != nil || count != 1 || items[0]["kind"] != "machine" || items[0]["id"] != "delivery:"+db.MsgPrefix+"abc" || items[0]["asks"] != "do" {
		t.Fatalf("for_you holds it: %v %d %v", items, count, err)
	}
}

// The weekly loss rides its founder task: the row is the task's, says it is a warning about the account and asks as the daemon.
func TestTheSpareNoticeIsAWarningOnItsTask(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	runAll(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(7, '23% of alpha lost', 'human', 'daemon', 1, 'daemon')",
		"INSERT INTO task_tags(task_id, tag) VALUES(7, '"+sparenotice.NoticeTag+"')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(5, 1, 'daemon', 'human', 'b', 'queued', 'task', 1)",
		"INSERT INTO task_requests(task_id, request_id) VALUES(7, 5)")
	raw, _ := json.Marshal(map[string]any{"until": time.Now().Unix() + 99, "lost": "l", "account": "alpha"})
	if err := db.SetMeta(e.DB, "spare_notice_until:7", string(raw)); err != nil {
		t.Fatal(err)
	}
	rows := pending(t, e)["answer"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("%v", rows)
	}
	w, _ := rows[0]["warning"].(map[string]any)
	if rows[0]["kind"] != "warning" || rows[0]["task"] != int64(7) || rows[0]["agent"] != nil || w["kind"] != "weekly_use" || w["account"] != "alpha" {
		t.Fatalf("%v", rows[0])
	}
}

// A turn on the founder is a row with who pushed it and what it asks, with or without an open request; a task they merely own is not.
func TestATurnIsARowAndATaskMerelyOwnedIsNot(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	runAll(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'a turn', 'bob', 'amy', 1, 'amy'), (2, 'owned', 'human', 'amy', 1, 'amy')",
		"INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(1, 1, 'human', 'pick one', 'bob', 5)")
	got := pending(t, e)
	rows := got["answer"].([]map[string]any)
	if taskIDs(got["answer"]) != "[1]" || rows[0]["kind"] != "decide" || rows[0]["source"] != "task" || rows[0]["agent"] != "bob" || rows[0]["ask"] != "pick one" {
		t.Fatalf("%v", rows)
	}
}

// The board's old lines to the founder are hidden from their inbox, and new requests are told in no line at all.
func TestTheBoardsOldLinesAreHiddenFromTheFoundersInbox(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, landed_at, edge_ok) VALUES"+
		"(5, 1, '"+db.NoticeRequester+"', 'human', 'T1 done', 'landed', 'say', 1, 1), (6, 1, 'bob', 'human', 'a reply', 'landed', 'say', 1, 1)")
	rows, err := e.mailboxRows()
	if err != nil || len(rows) != 1 || rows[0].id != 6 {
		t.Fatalf("%+v %v", rows, err)
	}
}

func requestRowOf(t *testing.T, e *Env, task int64) map[string]any {
	t.Helper()
	rows, err := e.RequestRows(time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r["task"] == task {
			return r
		}
	}
	return nil
}

// A request's result is unseen from when it ends: before the floor it counts as seen, a mark before it ended does not cover it, a mark after does.
func TestAFinishedRequestIsSeenByTheFloorOrAMarkSinceItEnded(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	run(t, e, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, done_evidence, done_at, discarded_by, discarded_at) VALUES"+
		"(1, 'open', 'bob', 'human', 1, 'human', NULL, NULL, NULL, NULL), "+
		"(2, 'old', 'bob', 'human', 1, 'human', 'x', %[1]d, NULL, NULL), "+
		"(3, 'done', 'bob', 'human', 1, 'human', 'x', %[2]d, NULL, NULL), "+
		"(4, 'thrown', 'bob', 'human', 1, 'human', NULL, NULL, 'amy', %[2]d), "+
		"(5, 'too old', 'bob', 'human', 1, 'human', NULL, NULL, 'amy', %[3]d)", now-500, now-100, now-requestWindowS-5))
	if err := db.SetMeta(e.DB, db.ResultSeenFloorMeta, fmt.Sprint(now-200)); err != nil {
		t.Fatal(err)
	}
	if r := requestRowOf(t, e, 1); r["ended"] != nil || r["status"] != reqDoing {
		t.Fatalf("open: %v", r)
	}
	ended := func(id int64) map[string]any { return requestRowOf(t, e, id)["ended"].(map[string]any) }
	if ended(2)["seen"] != true || ended(2)["outcome"] != "done" {
		t.Fatalf("before the floor: %v", ended(2))
	}
	if ended(3)["seen"] != false || ended(3)["at"] != ts(now-100) {
		t.Fatalf("unseen: %v", ended(3))
	}
	if r := requestRowOf(t, e, 4); r["progress"] != "discarded" || r["status"] != reqDone || r["ended"].(map[string]any)["outcome"] != "discarded" || r["ended"].(map[string]any)["seen"] != false {
		t.Fatalf("discarded: %v", r)
	}
	if r := requestRowOf(t, e, 5); r != nil {
		t.Fatalf("a discard outside the window is not listed: %v", r)
	}
	count := func() any { return pending(t, e)["counts"].(map[string]any)["unseen_results"] }
	if count() != 2 {
		t.Fatalf("%v", count())
	}
	list := func() int {
		got, err := e.requestsList(asFounder, map[string]any{"unseen": true})
		if err != nil {
			t.Fatal(err)
		}
		return len(got.(map[string]any)["items"].([]any))
	}
	if list() != 2 {
		t.Fatal(list())
	}
	if err := db.SetCursor(e.DB, db.ResultSeenKey(3), now-101); err != nil {
		t.Fatal(err)
	}
	if ended(3)["seen"] != false {
		t.Fatal("a mark from before it ended covers it")
	}
	if err := db.SetCursor(e.DB, db.ResultSeenKey(3), now-99); err != nil {
		t.Fatal(err)
	}
	if ended(3)["seen"] != true || count() != 1 || list() != 1 {
		t.Fatalf("%v %v %v", ended(3), count(), list())
	}
}

// unread_comments counts the comments above the founder's marker.
func TestARequestsUnreadCommentsComeFromTheMarker(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	runAll(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'a', 'bob', 'human', 1, 'human')",
		"INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(1, 1, 'bob', 'a', 1), (2, 1, 'bob', 'b', 2)",
		"INSERT INTO cursors(name, row_id, updated) VALUES('task_seen:human:1', 1, 1)")
	if r := requestRowOf(t, e, 1); r["unread_comments"] != int64(1) {
		t.Fatalf("%v", r)
	}
}
