package reads

import (
	"fmt"
	"testing"

	"github.com/handline/handline/go/protocol/rpc"
)

// A task says whether it is a founder request of its own and the founder's newest decision on it (a turn of his given back with a comment);
// tasks.list keeps only the founder's requests, or only the tasks he decided on since a time, closed ones too.
func TestATaskCarriesMyRequestAndTheFoundersNewestDecision(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, request, done_evidence, done_at) VALUES(1, 'mine', 'bob', 'p1', 1, 1, 'ok', 300)")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, request) VALUES(2, 'under mine', 'bob', 'bob', 1, 1), (3, 'own', 'bob', 'bob', 1, NULL)")
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(9, 1, 'p1', 'old answer', 40), (10, 1, 'bob', 'A or B?', 90), "+
		"(11, 1, 'p1', 'A', 200), (12, 2, 'bob', 'done', 60)")
	ev := func(ts int, kind, agent, payload string) {
		run(t, e, fmt.Sprintf("INSERT INTO events(ts, agent, kind, payload) VALUES(%d, '%s', '%s', '%s')", ts, agent, kind, payload))
	}
	ev(30, "task_turn_push", "bob", `{"task_id": 1, "to": "p1", "ask": "old ask"}`)
	ev(50, "task_turn_pop", "p1", `{"task_id": 1, "agent": "p1", "to": "bob", "memo": 9}`)
	ev(100, "task_turn_push", "bob", `{"task_id": 1, "to": "p1", "ask": "", "memo": 10}`)
	ev(200, "task_turn_pop", "p1", `{"task_id": 1, "agent": "p1", "to": "bob", "memo": 11}`)
	ev(210, "task_turn_pop", "bob", `{"task_id": 2, "agent": "bob", "to": "p1", "memo": 12}`) // an agent's pass is no decision
	ev(220, "task_turn_pop", "p1", `{"task_id": 3, "agent": "p1", "to": "bob"}`)              // nor one with no comment
	ev(230, "task_turn_pop", "p1", `{"task_id": 3, "agent": "bob", "to": "amy", "memo": 12}`) // nor the founder giving back bob's turn
	got, err := e.Tasks([]int64{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"at": ts(int64(200)), "ask": "A or B?", "answer": "A"}
	if d, _ := got[1]["decided"].(map[string]any); fmt.Sprint(d) != fmt.Sprint(want) || got[1]["my_request"] != true {
		t.Errorf("T1: decided %v my_request %v", got[1]["decided"], got[1]["my_request"])
	}
	for _, id := range []int64{2, 3} {
		if got[id]["decided"] != nil || got[id]["my_request"] != false {
			t.Errorf("T%d: decided %v my_request %v", id, got[id]["decided"], got[id]["my_request"])
		}
	}
	founder := &rpc.Conn{Caller: rpc.Caller{Role: "owner"}}
	for _, c := range []struct {
		params map[string]any
		want   string
	}{
		{map[string]any{"my_requests": true, "all": true}, "[1]"},
		{map[string]any{"my_requests": true}, "[]"}, // T1 is done
		{map[string]any{"decided_since": "1970-01-01T00:03:00Z"}, "[1]"},
		{map[string]any{"decided_since": "1970-01-01T00:03:30Z"}, "[]"},
	} {
		list, err := e.tasksList(founder, c.params)
		if err != nil {
			t.Fatal(err)
		}
		var ids []any
		for _, it := range list.(map[string]any)["items"].([]any) {
			ids = append(ids, it.(map[string]any)["id"])
		}
		if s := fmt.Sprint(ids); s != c.want && !(c.want == "[]" && ids == nil) {
			t.Errorf("%v: %s, want %s", c.params, s, c.want)
		}
	}
}
