package reads

import (
	"encoding/json"
	"fmt"
	"github.com/handline/handline/go/data/team/teamtest"
	"testing"
)

func TestATasksTimelineIsEverythingThatHappenedOnItInOrder(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', 'bob', 'amy', 10)")
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(1, 10, 'amy', 'bob', 'the brief\nmore', 'landed', 'task', 1), "+
		"(2, 30, 'amy', 'cy', 'T1 handed to you', 'landed', 'task', 1)")
	run(t, e, "INSERT INTO task_requests(task_id, request_id) VALUES(1, 1), (1, 2)")
	run(t, e, "INSERT INTO task_memos(task_id, author, body, created_at) VALUES(1, 'amy', 'a memo', 20.5)")
	for _, ev := range []struct {
		ts            int
		who, kind, at string
	}{
		{20, "amy", "edit_task", `"body": {"old": "the brief", "new": "a new brief"}`},
		{30, "amy", "task_handoff", `"from": "bob", "to": "cy", "note": "yours", "request_id": 2, "at": 30.2`},
		{40, "cy", "mark_task_done", `"from": "doing"`},
		{50, "amy", "reject_task", `"note": "not yet"`},
	} {
		run(t, e, "INSERT INTO events(ts, agent, kind, payload) VALUES(?, ?, ?, ?)", ev.ts, ev.who, ev.kind, `{"task_id": 1, `+ev.at+`}`)
	}
	got, err := e.taskGet(nil, map[string]any{"task": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, r := range got.(map[string]any)["timeline"].([]any) {
		m := r.(map[string]any)
		rows = append(rows, fmt.Sprintf("%v|%v|%v", m["kind"], m["who"], m["text"]))
	}
	want := []string{"request|amy → bob|landed  the brief", "edit|amy|a new brief", "memo|amy|a memo", "handoff|bob → cy|yours",
		"state|cy|doing -> done", "reject|amy|not yet"}
	if fmt.Sprint(rows) != fmt.Sprint(want) {
		t.Fatalf("got  %q\nwant %q", rows, want)
	}
}

// A comment's tell, its @human ask and a return that forwards it (one withdrawn too) are the comment's row; the other requests stay.
func TestATasksTimelineLeavesOutAMemosTellAndAskButKeepsTheOtherRequests(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', 'bob', 'amy', 10)")
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, notice, edge_ok) VALUES(1, 10, 'amy', 'bob', 'the brief', 'landed', 'task', 0, 1), "+
		"(2, 20, 'amy', 'bob', 'a memo tell', 'landed', 'say', 2, 1), (3, 30, 'cy', 'bob', 'a child result', 'landed', 'say', 1, 1), "+
		"(4, 40, 'bob', 'p1', '@human ship it?', 'queued', NULL, 0, 1), (5, 50, 'p1', 'bob', 'T1 is your turn again: human passed it: a', 'landed', 'task', 0, 1)")
	run(t, e, "UPDATE requests SET memo_id = 9 WHERE id = 4") // the ask bob's @human comment opened
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, notice, edge_ok) VALUES(6, 60, 'p1', 'cy', 'T1 is your turn again: b', 'withdrawn', 'task', 0, 1)")
	run(t, e, "INSERT INTO task_requests(task_id, request_id) VALUES(1, 1), (1, 2), (1, 3), (1, 4), (1, 5), (1, 6)")
	// the founder's comment that passed bob's turn back, told bob by that return
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(7, 1, 'p1', 'now', 50)")
	run(t, e, "INSERT INTO memo_tells(memo_id, reader, request_id) VALUES(7, 'bob', 5), (7, 'cy', 6)") // 6 was withdrawn before it reached cy
	got, err := e.taskGet(nil, map[string]any{"task": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, r := range got.(map[string]any)["timeline"].([]any) {
		rows = append(rows, fmt.Sprint(r.(map[string]any)["text"]))
	}
	want := []string{"landed  the brief", "landed  a child result", "now"}
	if fmt.Sprint(rows) != fmt.Sprint(want) {
		t.Fatalf("got  %q\nwant %q", rows, want)
	}
}

func TestATasksPageNamesItsSubtasksParentMergeAndWhomToAsk(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for _, a := range []string{"amy", "eng", "eng.bob", "eng.cy", "dee"} {
		addAgent(t, e, a, "")
	}
	nameOf := map[string]string{} // by key, so the wants read by name
	key := func(name string) string { k := teamtest.Key(t, e.Home, name); nameOf[k] = name; return k }
	amy, bob, cy, dee := key("amy"), key("eng.bob"), key("eng.cy"), key("dee")
	key("eng")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'the story', ?, ?, 10), "+
		"(2, 'a', ?, ?, 10), (3, 'b', ?, ?, 10), (4, 'c', ?, ?, 10)", amy, amy, bob, amy, cy, bob, cy, bob)
	run(t, e, "UPDATE tasks SET parent_task = 1 WHERE id = 2")
	run(t, e, "UPDATE tasks SET parent_task = 2 WHERE id IN (3, 4)")
	run(t, e, "UPDATE tasks SET done_evidence = 'sha', done_at = 20 WHERE id = 3")
	run(t, e, "UPDATE tasks SET discarded_by = ?, discarded_at = 20, merged_into = 3 WHERE id = 4", bob)
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, edge_ok) VALUES(1, 1, 'p1', ?, 'x', 'landed', 1), "+
		"(2, 2, ?, 'p1', 'y', 'landed', 1), (3, 3, 'p1', ?, 'z', 'landed', 1)", cy, dee, cy)
	got, err := e.taskGet(nil, map[string]any{"task": json.Number("2")})
	if err != nil {
		t.Fatal(err)
	}
	d := got.(map[string]any)
	subs, _ := json.Marshal(d["subtasks"])
	if want := `[{"block_reason":null,"evidence":"sha","hold_reason":null,"id":3,"owner":"` + cy + `","state":"done","title":"b","waiting":null},` +
		`{"block_reason":null,"evidence":null,"hold_reason":null,"id":4,"owner":"` + cy + `","state":"discarded","title":"c","waiting":null}]`; string(subs) != want {
		t.Fatalf("subtasks %s, want %s", subs, want)
	}
	if d["parent_title"] != "the story" || d["merged_into"] != nil {
		t.Fatalf("parent_title %v, merged_into %v", d["parent_title"], d["merged_into"])
	}
	people := map[string][]string{}
	for k, keys := range d["people"].(map[string]any) {
		for _, key := range keys.([]string) {
			people[k] = append(people[k], nameOf[key])
		}
	}
	if fmt.Sprint(people) != "map[others:[amy] recent:[eng.cy dee] team:[eng eng.bob]]" {
		t.Fatalf("people %s", people)
	}
	if got, _ = e.taskGet(nil, map[string]any{"task": json.Number("4")}); got.(map[string]any)["merged_into"] != int64(3) {
		t.Fatalf("merged_into %v", got.(map[string]any)["merged_into"])
	}
}

func TestWhilePausedATaskItsAgentWasNotToldReadsQueuedOnItsPage(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at) VALUES(1, 'a', 'bob', 'amy', 1, 'amy', 2)")
	run(t, e, "INSERT INTO agent_state(agent, state, current_task, updated) VALUES('bob', 'IDLE', 1, 1)")
	run(t, e, `INSERT INTO meta(name, value, updated) VALUES('daemon_pause', '{"since": 1}', 1)`)
	state := func() any {
		got, err := e.taskGet(nil, map[string]any{"task": json.Number("1")})
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)["state"]
	}
	if s := state(); s != "queued" {
		t.Fatalf("paused, not yet told: %v", s)
	}
	run(t, e, "UPDATE tasks SET delivered_at = 3 WHERE id = 1")
	if s := state(); s != "doing" {
		t.Fatalf("told: %v", s)
	}
}

func TestThePauseHoldsNeitherASelfStartedTaskNorTheFoundersNorAGatesOnTheirPages(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at) VALUES(1, 'a', 'bob', 'amy', 1, 'amy', 2), "+
		"(2, 'b', 'p1', 'amy', 1, 'amy', 2), (3, 'c', 'gate', 'amy', 1, 'amy', 2)")
	run(t, e, "INSERT INTO agent_state(agent, state, current_task, updated) VALUES('bob', 'IDLE', 1, 1)")
	run(t, e, `INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'bob', 'task_self_start', '{"task_id": 1}')`)
	state := func(id int) any {
		got, err := e.taskGet(nil, map[string]any{"task": json.Number(fmt.Sprint(id))})
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)["state"]
	}
	unpaused := []any{state(1), state(2), state(3)}
	if unpaused[0] != "doing" {
		t.Fatalf("setup: %v", unpaused)
	}
	run(t, e, `INSERT INTO meta(name, value, updated) VALUES('daemon_pause', '{"since": 1}', 1)`)
	for i, want := range unpaused {
		if got := state(i + 1); got != want {
			t.Errorf("task %d: %v paused, %v unpaused", i+1, got, want)
		}
	}
}

// A turn's push and pop are rows of their own, a turn's return is the turn's, and a task the daemon delivered on someone's behalf says so.
func TestATasksTimelineHasItsTurnsAndSaysWhatTheDaemonDelivered(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', '"+bob+"', 'amy', 10)")
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, notice, edge_ok) VALUES(1, 10, 'p1', '"+bob+"', 'a', 'landed', 'task', 0, 1), "+
		"(2, 30, 'p1', '"+bob+"', 'T1 is your turn again', 'landed', 'task', 0, 1)")
	run(t, e, "INSERT INTO task_requests(task_id, request_id) VALUES(1, 1), (1, 2)")
	run(t, e, `INSERT INTO events(ts, agent, kind, payload) VALUES
		(10, '', 'task_forward', '{"task_id": 1, "request_id": 1, "carries": "queue", "from": 0}'),
		(20, '`+bob+`', 'task_turn_push', '{"task_id": 1, "to": "p1", "ask": "now?"}'),
		(30, 'p1', 'task_turn_pop', '{"task_id": 1, "agent": "p1", "outcome": "pass", "note": "", "to": "`+bob+`", "request_id": 2}'),
		(40, '`+bob+`', 'task_turn_pop', '{"task_id": 1, "agent": "`+bob+`", "outcome": "cleared"}')`)
	got, err := e.taskGet(nil, map[string]any{"task": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, r := range got.(map[string]any)["timeline"].([]any) {
		m := r.(map[string]any)
		rows = append(rows, fmt.Sprintf("%v|%v|%v|%v|%v", m["kind"], m["who"], m["text"], m["forwards"], m["turn"]))
	}
	want := []string{"request|p1 → " + bob + "|landed  a|queue|<nil>", "turn|" + bob + " → p1|now?|<nil>|push", "turn|p1 → " + bob + "||<nil>|pop"}
	if fmt.Sprint(rows) != fmt.Sprint(want) {
		t.Fatalf("got  %q\nwant %q", rows, want)
	}
}
