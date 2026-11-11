package reads

import (
	"encoding/json"
	"fmt"
	"testing"
)

// A turn handed over with a comment reads as that comment: its ask is the comment's text as it reads now, memo names it, and the
// timeline's push row carries the same memo.
func TestATurnReadsTheCommentItWasHandedOverWith(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'pick', 'amy', 'amy', 1, 'amy')")
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(7, 1, 'amy', 'A or B? I recommend A', 10)")
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since, memo) VALUES(1, 1, 'p1', '', 'amy', 10, 7)")
	run(t, e, `INSERT INTO events(id, ts, agent, kind, payload) VALUES(1, 10, 'amy', 'task_turn_push', '{"task_id":1,"to":"p1","ask":"","memo":7}')`)
	got, err := e.taskGet(nil, map[string]any{"task": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	task := got.(map[string]any)
	turn := task["turns"].([]any)[0].(map[string]any)
	if turn["ask"] != "A or B? I recommend A" || turn["memo"] != int64(7) {
		t.Fatalf("%v", turn)
	}
	var push map[string]any
	for _, r := range task["timeline"].([]any) {
		if row := r.(map[string]any); row["kind"] == "turn" {
			push = row
		}
	}
	if push == nil || push["turn"] != "push" || push["memo"] != int64(7) {
		t.Fatalf("the push row: %v", push)
	}
}

// An agent's unread @human comment is a note row on whatever task: listed in answer, counted in notes and not in answer, gone once the
// founder reads the task. A comment that only mentions @human later is no note.
func TestAnUnreadAtHumanCommentIsANoteNotTheFoundersTurn(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(2, 'ship', 'amy', 'lead', 1, 'lead'), (3, 'other', 'amy', 'lead', 1, 'lead')")
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES"+
		"(8, 2, 'amy', '@human shipped to beta', 20), (9, 3, 'amy', 'told @human already', 21)")
	pending := func() map[string]any {
		t.Helper()
		got, err := e.requestsPending(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)
	}
	p := pending()
	answer, counts := p["answer"].([]map[string]any), p["counts"].(map[string]any)
	if len(answer) != 1 || answer[0]["kind"] != "note" || answer[0]["task"] != int64(2) || answer[0]["id"] != "8" || answer[0]["agent"] != "amy" ||
		answer[0]["ask"] != "shipped to beta" || answer[0]["source"] != "comment" {
		t.Fatalf("%v", answer)
	}
	if counts["answer"] != 0 || counts["notes"] != 1 {
		t.Fatalf("a note is the bell's dot, not its count: %v", counts)
	}
	run(t, e, "INSERT INTO cursors(name, row_id, updated) VALUES('task_seen:human:2', 8, 1)")
	if p := pending(); len(p["answer"].([]map[string]any)) != 0 || p["counts"].(map[string]any)["notes"] != 0 {
		t.Fatalf("read, the note is gone: %v", p)
	}
}

// The founder's part in a task is a task_comment card in the chat it concerns: the turn put on them in the chat of the agent below, their
// comment that gave it back in the chat of the agent it went back to, an @human comment in its author's. Each reads the comment as it is now.
// An @human comment that a turn on the founder hangs on is one card, the turn's, and a question asked again is a turn card too.
func TestTheFoundersTurnTheirAnswerAndANoteAreCardsInTheAgentsChat(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'pick', 'amy', 'amy', 1, 'amy')")
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES"+
		"(7, 1, 'amy', 'A or B?', 10), (8, 1, 'p1', 'A', 20), (9, 1, 'amy', '@human A is in', 30), (10, 1, 'amy', 'plain', 40), (11, 1, 'amy', '@human C or D?', 50)")
	run(t, e, `INSERT INTO events(id, ts, agent, kind, payload) VALUES`+
		`(1, 10, 'amy', 'task_turn_push', '{"task_id":1,"to":"p1","ask":"","memo":7}'), `+
		`(2, 20, 'p1', 'task_turn_pop', '{"task_id":1,"agent":"p1","outcome":"pass","note":"A","to":"amy","memo":8}'), `+
		`(3, 30, 'amy', 'memo_add', '{"task_id":1,"memo":9,"now":"@human A is in"}'), `+
		`(4, 40, 'amy', 'memo_add', '{"task_id":1,"memo":10,"now":"plain"}'), `+
		`(5, 50, 'amy', 'memo_add', '{"task_id":1,"memo":11,"now":"@human C or D?"}'), `+
		`(6, 50, 'amy', 'task_turn_ask', '{"task_id":1,"agent":"p1","from":"A or B?","ask":"@human C or D?","memo":11}')`)
	run(t, e, "UPDATE task_memos SET body = 'A or B? I recommend A' WHERE id = 7")
	items, err := e.transcriptItems("amy", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items {
		if it["kind"] == "task_comment" {
			c := it["task_comment"].(map[string]any)
			got = append(got, fmt.Sprintf("%s %v %s %q", c["kind"], c["memo"], c["who"], c["text"]))
		}
	}
	want := `[turn 7 amy "A or B? I recommend A" answer 8 human "A" note 9 amy "@human A is in" turn 11 amy "@human C or D?"]`
	if fmt.Sprint(got) != want {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
	for id := int64(1); id <= 3; id++ {
		if _, agent, ok, err := e.TaskEventItem(id); err != nil || !ok || agent != "amy" {
			t.Fatalf("event %d goes live to %q %v %v", id, agent, ok, err)
		}
	}
	if _, _, ok, _ := e.TaskEventItem(5); ok {
		t.Fatal("the comment a turn hangs on goes live once, as the turn")
	}
	if items, _ := e.transcriptItems("bob", nil); len(items) != 0 {
		t.Fatalf("not bob's: %v", items)
	}
}
