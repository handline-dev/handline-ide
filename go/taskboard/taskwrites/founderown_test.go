package taskwrites

import "testing"

// An agent's handoff or owner edit to the founder is the founder's turn on top of the task, which stays the agent's; a handoff's note is the
// comment the turn hangs on, and one with no note is refused. An agent still opens no task for the founder.
func TestAnAgentsGiveToTheFounderIsTheFoundersTurn(t *testing.T) {
	t.Parallel()
	for i, give := range []string{
		`{"jsonrpc":"2.0","id":2,"method":"task.handoff","params":{"task":1,"to":"human","note":"ship now?","request_id":"h1"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"task.edit","params":{"task":1,"owner":"human","request_id":"e1"}}`,
	} {
		db, mk := discardEnv(t)
		exec(t, db, "INSERT INTO tasks(id, title, owner, opened_by, created_at, sent_at) VALUES(1, 'build', 'worker', 'manager', 1, 1)")
		r := call(t, mk("worker", &fakeTeller{}), give)
		if r[0]["error"] != nil {
			t.Fatalf("give %d: %v", i, r[0])
		}
		if count(t, db, "SELECT COUNT(*) FROM tasks WHERE id = 1 AND owner = 'worker'") != 1 ||
			count(t, db, "SELECT COUNT(*) FROM task_turns WHERE task_id = 1 AND agent = 'human' AND pushed_by = 'worker'") != 1 {
			t.Fatalf("give %d: the task stays the worker's under the founder's turn", i)
		}
		if i == 0 && count(t, db, "SELECT COUNT(*) FROM task_turns u JOIN task_memos m ON m.id = u.memo WHERE u.ask = '' AND m.author = 'worker' AND m.body = 'ship now?'") != 1 {
			t.Fatal("the handoff's note is a comment the turn hangs on")
		}
		// handed again, the founder's turn moves to the new note: nothing is handed over a second time
		tell := &fakeTeller{}
		again := call(t, mk("worker", tell), `{"jsonrpc":"2.0","id":3,"method":"task.handoff","params":{"task":1,"to":"human","note":"or later?","request_id":"h2"}}`)
		if again[0]["error"] != nil || len(tell.handed) != 0 || count(t, db, "SELECT COUNT(*) FROM task_turns u JOIN task_memos m ON m.id = u.memo WHERE m.body = 'or later?'") != 1 {
			t.Fatalf("give %d: %v, handed %v", i, again[0], tell.handed)
		}
		other := call(t, mk("manager", &fakeTeller{}), `{"jsonrpc":"2.0","id":4,"method":"task.handoff","params":{"task":1,"to":"human","note":"x","request_id":"h4"}}`)
		if e, _ := other[0]["error"].(map[string]any); e == nil {
			t.Fatalf("give %d: one not below the founder's turn does not move it", i)
		}
	}
	db, mk := discardEnv(t)
	exec(t, db, "INSERT INTO tasks(id, title, owner, opened_by, created_at, sent_at) VALUES(1, 'build', 'worker', 'manager', 1, 1)")
	r := call(t, mk("worker", &fakeTeller{}), `{"jsonrpc":"2.0","id":2,"method":"task.handoff","params":{"task":1,"to":"human","request_id":"h3"}}`)
	if e, _ := r[0]["error"].(map[string]any); e == nil || count(t, db, "SELECT COUNT(*) FROM task_turns") != 0 {
		t.Fatalf("a handoff to the founder with no note is refused: %v", r[0])
	}
	r = call(t, mk("worker", &fakeTeller{}),
		`{"jsonrpc":"2.0","id":3,"method":"task.new","params":{"owner":"human","title":"d","time_box":"1h","request_id":"n1"}}`)
	if e, _ := r[0]["error"].(map[string]any); e == nil || e["message"] != newForFounder {
		t.Fatalf("%v", r[0])
	}
	if count(t, db, "SELECT COUNT(*) FROM tasks WHERE owner = 'human'") != 0 {
		t.Fatal("no task for the founder")
	}
}
