package reads

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/handline/handline/go/protocol/rpc"
)

// order decided lists the tasks the founder decided on newest decision first, so an old task decided today comes before a new one decided
// yesterday; its pages, cursor to cursor, neither repeat nor skip a task.
func TestDecidedOrderPagesByTheNewestDecision(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for id := 1; id <= 5; id++ {
		run(t, e, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(%d, 't', 'bob', 'bob', 1)", id))
		run(t, e, fmt.Sprintf("INSERT INTO task_comments(id, task_id, author, body, created_at) VALUES(%d, %d, 'p1', 'yes', 1)", id, id))
	}
	pop := func(ts, task int) {
		run(t, e, fmt.Sprintf(`INSERT INTO events(ts, agent, kind, payload) VALUES(%d, 'p1', 'task_turn_pop', '{"task_id": %d, "agent": "p1", "to": "bob", "comment": %d}')`,
			ts, task, task))
	}
	// Decided in the order 5, 1, 4, 3, 2, with T2 a second time last; T3 and T2's last decision share a second, so the event's order, not
	// the time and then the task's id, puts T2 first.
	pop(10, 2)
	pop(10, 5)
	pop(20, 1)
	pop(30, 4)
	pop(40, 3)
	pop(40, 2)
	founder := &rpc.Conn{Caller: rpc.Caller{Role: "owner"}}
	var ids []any
	params := map[string]any{"decided_since": "1970-01-01T00:00:00Z", "order": "decided", "limit": json.Number("2")}
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("the cursor never ends")
		}
		list, err := e.tasksList(founder, params)
		if err != nil {
			t.Fatal(err)
		}
		out := list.(map[string]any)
		for _, it := range out["items"].([]any) {
			ids = append(ids, it.(map[string]any)["id"])
		}
		next, _ := out["next"].(string)
		if (next != "") != (out["more"] == true) {
			t.Fatalf("next %q with more %v", next, out["more"])
		}
		if next == "" {
			break
		}
		params = map[string]any{"decided_since": "1970-01-01T00:00:00Z", "order": "decided", "limit": json.Number("2"), "cursor": next}
	}
	if got := fmt.Sprint(ids); got != "[2 3 4 1 5]" {
		t.Errorf("decided order %s, want [2 3 4 1 5]", got)
	}
	for name, p := range map[string]map[string]any{
		"no decided_since":   {"order": "decided"},
		"before":             {"decided_since": "1970-01-01T00:00:00Z", "order": "decided", "before": json.Number("3")},
		"ids":                {"decided_since": "1970-01-01T00:00:00Z", "order": "decided", "ids": []any{json.Number("1")}},
		"a cursor, id order": {"decided_since": "1970-01-01T00:00:00Z", "cursor": keyCursor(5, 2)},
	} {
		if _, err := e.tasksList(founder, p); err == nil {
			t.Errorf("%s: no bad_request", name)
		}
	}
}
