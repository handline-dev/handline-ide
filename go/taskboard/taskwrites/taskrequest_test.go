package taskwrites

import (
	"fmt"
	"testing"
)

// Each new task is given the founder request it serves (db.SetTaskRequest), and a later task of none that a task of a request is made to
// wait on joins that request.
func TestEachTaskIsTaggedWithTheFounderRequestItServes(t *testing.T) {
	db, mk := discardEnv(t)
	exec(t, db,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, request) VALUES(3, 'request', 'manager', 'p1', 1, 3), (2, 'ops', 'other', 'gp', 1, NULL)",
		"INSERT INTO agent_state(agent, state, current_task, updated) VALUES('manager', 'busy', 3, 1), ('other', 'busy', 2, 1)")
	line := func(method string, id int, params, rid string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":{%s,"request_id":%q}}`, id, method, params, rid)
	}
	newTask := func(id int, title, extra string) string {
		return line("task.new", id, fmt.Sprintf(`"owner":"worker","title":%q,"body":"b"%s`, title, extra), title)
	}
	ok := func(r []map[string]any) {
		t.Helper()
		for _, x := range r {
			if x["error"] != nil {
				t.Fatalf("%v", x)
			}
		}
	}
	idOf := func(title string) (id int64) {
		db.QueryRow("SELECT id FROM tasks WHERE title = ?", title).Scan(&id)
		return id
	}
	check := func(title string, want any) {
		t.Helper()
		var got any
		db.QueryRow("SELECT request FROM tasks WHERE title = ?", title).Scan(&got)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: request %v, want %v", title, got, want)
		}
	}

	ok(call(t, mk("", &fakeTeller{}), newTask(2, "founder's", "")))
	ok(call(t, mk("manager", &fakeTeller{}), newTask(2, "child", `,"parent":2`), newTask(3, "held", `,"no_parent":true`)))
	ok(call(t, mk("other", &fakeTeller{}), newTask(2, "own work", "")))
	check("founder's", idOf("founder's")) // its own
	check("child", nil)                   // its parent's, none
	check("held", 3)                      // the held task's
	check("own work", nil)                // none

	// a task of request 3 made to wait on a later task of none brings it in; an earlier task of none stays as it is.
	exec(t, db, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(100, 'audit', 'other', 'gp', 1)")
	ok(call(t, mk("manager", &fakeTeller{}),
		line("task.edit", 2, fmt.Sprintf(`"task":%d,"blocked_by":100`, idOf("held")), "e1"),
		line("task.edit", 3, `"task":3,"blocked_by":2`, "e2")))
	check("audit", 3)
	check("ops", nil)
	ok(call(t, mk("manager", &fakeTeller{}), newTask(2, "under audit", `,"parent":100`)))
	check("under audit", 3)

	// a task that answers to the founder is its own request whoever opens it; one the founder opens for an agent to answer to is not.
	ok(call(t, mk("manager", &fakeTeller{}), newTask(2, "for the founder", `,"reporter":"human","no_parent":true`)))
	exec(t, db, "INSERT INTO agent_state(agent, state, current_task, updated) VALUES('p1', 'idle', 3, 1)")
	ok(call(t, mk("", &fakeTeller{}), newTask(2, "for manager", `,"reporter":"manager"`)))
	check("for the founder", idOf("for the founder"))
	check("for manager", nil)

	// a blocker that serves another request stays in it.
	exec(t, db, "INSERT INTO tasks(id, title, owner, opened_by, created_at, request) VALUES(200, 'other audit', 'other', 'p1', 1, 200)")
	ok(call(t, mk("manager", &fakeTeller{}), line("task.edit", 2, fmt.Sprintf(`"task":%d,"blocked_by":200`, idOf("held")), "e3")))
	check("other audit", 200)

	// a held task that is done or discarded gives no request.
	for i, end := range []string{"done_evidence = 'ok'", "discarded_by = 'gp'"} {
		exec(t, db, "UPDATE tasks SET done_evidence = NULL, discarded_by = NULL WHERE id = 3", "UPDATE tasks SET "+end+" WHERE id = 3",
			"UPDATE agent_state SET current_task = 3 WHERE agent = 'manager'")
		title := fmt.Sprintf("after the end %d", i)
		ok(call(t, mk("manager", &fakeTeller{}), newTask(2, title, `,"no_parent":true`)))
		check(title, nil)
	}
}

// Rule a: an agent's task opened in the turn that brought a founder's message, when no parent or held task gives it a request.
func TestATaskOpenedInTheTurnOfAFounderMessageIsItsRequest(t *testing.T) {
	db, mk := discardEnv(t)
	exec(t, db,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, request, from_request) VALUES(3, 'held request', 'other', 'p1', 1, 3, NULL), "+
			"(4, 'made from 9', 'gp', 'p1', 1, 4, 9)",
		"INSERT INTO agent_state(agent, state, current_task, updated) VALUES('other', 'busy', 3, 1)",
		"INSERT INTO events(ts, agent, kind) VALUES(10, 'manager', 'Stop'), (10, 'other', 'Stop'), (10, 'gp', 'Stop')",
		// 5 landed before manager's last Stop; 7 in its turn, top-level; 8 to other in its turn; 9 to gp, already T4.
		"INSERT INTO requests(id, requester, target, body, state, kind, ts, landed_at, founder_auth) VALUES"+
			"(5, 'p1', 'manager', 'old', 'landed', 'say', 5, 5, 1), (7, 'p1', 'manager', 'new ask', 'landed', 'say', 11, 11, 1), "+
			"(8, 'p1', 'other', 'another', 'landed', 'say', 11, 11, 1), (9, 'p1', 'gp', 'made', 'landed', 'say', 11, 11, 1)")
	newTask := func(id int, title string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"task.new","params":{"owner":"worker","title":%q,"body":"b","request_id":%q}}`, id, title, title)
	}
	row := func(title string) string {
		var req, origin, from, parent any
		db.QueryRow("SELECT request, turn_origin, from_request, parent_task FROM tasks WHERE title = ?", title).Scan(&req, &origin, &from, &parent)
		return fmt.Sprint(req, origin, from, parent)
	}
	for _, c := range []struct{ agent, title string }{{"manager", "first"}, {"manager", "second"}, {"other", "held wins"}, {"gp", "already a task"}} {
		if r := call(t, mk(c.agent, &fakeTeller{}), newTask(2, c.title)); r[0]["error"] != nil {
			t.Fatalf("%v", r[0])
		}
	}
	var first int64
	db.QueryRow("SELECT id FROM tasks WHERE title = 'first'").Scan(&first)
	for title, want := range map[string]string{
		"first":          fmt.Sprintf("%d 7 <nil> <nil>", first), // a root, from 7 by turn_origin; from_request untouched
		"second":         fmt.Sprintf("%d <nil> <nil> <nil>", first),
		"held wins":      "3 <nil> <nil> <nil>",
		"already a task": "4 <nil> <nil> <nil>",
	} {
		if got := row(title); got != want {
			t.Errorf("%s: request turn_origin from_request parent = %s, want %s", title, got, want)
		}
	}

	// A reply in 7's thread, in manager's next turn, joins 7's request; a reply in a thread nothing holds is no request.
	exec(t, db, "INSERT INTO events(ts, agent, kind) VALUES(20, 'manager', 'Stop')",
		"INSERT INTO requests(id, requester, target, body, state, kind, ts, landed_at, founder_auth, parent_id) VALUES"+
			"(101, 'manager', 'p1', 'plan?', 'landed', 'say', 21, 21, 0, 7), (102, 'p1', 'manager', 'yes, go ahead', 'landed', 'say', 22, 22, 1, 101)")
	if r := call(t, mk("manager", &fakeTeller{}), newTask(2, "reply")); r[0]["error"] != nil {
		t.Fatalf("%v", r[0])
	}
	if got, want := row("reply"), fmt.Sprintf("%d <nil> <nil> <nil>", first); got != want {
		t.Errorf("reply: %s, want %s", got, want)
	}
	exec(t, db, "INSERT INTO events(ts, agent, kind) VALUES(30, 'manager', 'Stop')",
		"INSERT INTO requests(id, requester, target, body, state, kind, ts, landed_at, founder_auth, parent_id) VALUES"+
			"(200, 'p1', 'manager', 'about 5', 'landed', 'say', 31, 31, 1, 5)")
	if r := call(t, mk("manager", &fakeTeller{}), newTask(2, "loose reply")); r[0]["error"] != nil {
		t.Fatalf("%v", r[0])
	}
	if got := row("loose reply"); got != "<nil> <nil> <nil> <nil>" {
		t.Errorf("loose reply: %s", got)
	}
}
