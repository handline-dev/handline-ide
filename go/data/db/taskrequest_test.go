package db

import (
	"fmt"
	"testing"
)

// The turn rule (turnRequest), case by case: manager's last Stop was at 10, and T1 holds founder message 1 (a top-level one) by from_request.
// Each case adds requests, events and links, then opens a task by manager for owner (worker unless named) and reads its request:
// "own" for a request of its own, "T1" for T1's, "" for none.
func TestTheTurnRuleTakesTheNewestFounderMessageOfTheTurn(t *testing.T) {
	msg := func(id int, requester, kind string, at, auth int, parent string) string {
		return fmt.Sprintf("INSERT INTO requests(id, requester, target, body, state, kind, ts, landed_at, founder_auth, parent_id) "+
			"VALUES(%d, '%s', 'manager', 'm', 'landed', '%s', %d, %d, %d, %s)", id, requester, kind, at, at, auth, parent)
	}
	for _, c := range []struct {
		name, owner, want string
		setup             []string
	}{
		{"a top-level founder message in the turn", "", "own", []string{msg(5, Human, "say", 11, 1, "NULL")}},
		{"one that landed in the second of the Stop", "", "own", []string{msg(5, Human, "say", 10, 1, "NULL")}},
		{"one to another agent", "", "", []string{msg(5, Human, "say", 11, 1, "NULL"), "UPDATE requests SET target = 'other' WHERE id = 5"}},
		{"only one from before the last Stop", "", "", []string{msg(5, Human, "say", 9, 1, "NULL")}},
		{"a StopFailure ends a turn too", "", "", []string{msg(5, Human, "say", 11, 1, "NULL"), "INSERT INTO events(ts, agent, kind) VALUES(12, 'manager', 'StopFailure')"}},
		{"the newest of two: a free one after one T1 holds", "", "own", []string{msg(5, Human, "say", 11, 1, "1"), msg(6, Human, "say", 12, 1, "NULL")}},
		{"the newest of two: one T1 holds after a free one", "", "T1", []string{msg(5, Human, "say", 11, 1, "NULL"), msg(6, Human, "say", 12, 1, "1")}},
		{"an agent's message", "", "", []string{msg(5, "gp", "say", 11, 1, "NULL")}},
		{"a founder's task ask, not a say", "", "", []string{msg(5, Human, "task", 11, 1, "NULL")}},
		{"words the founder's connection did not send", "", "", []string{msg(5, Human, "say", 11, 0, "NULL")}},
		{"a reply in a thread linked to T1", "", "T1", []string{msg(5, Human, "say", 3, 1, "NULL"), "INSERT INTO task_requests(task_id, request_id) VALUES(1, 5)", msg(6, Human, "say", 11, 1, "5")}},
		{"a task for the founder", Human, "", []string{msg(5, Human, "say", 11, 1, "NULL")}},
		{"a gate", Gate, "", []string{msg(5, Human, "say", 11, 1, "NULL")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			q := open(t)
			for _, s := range append([]string{
				msg(1, Human, "say", 2, 1, "NULL"),
				"INSERT INTO tasks(id, title, owner, opened_by, created_at, request, from_request) VALUES(1, 'held', 'manager', 'human', 1, 1, 1)",
				"INSERT INTO events(ts, agent, kind) VALUES(10, 'manager', 'Stop')",
			}, c.setup...) {
				if _, err := q.Exec(s); err != nil {
					t.Fatalf("%s: %v", s, err)
				}
			}
			owner := c.owner
			if owner == "" {
				owner = "worker"
			}
			tid, err := NewTask(q, "new", owner, "manager", 0, DefaultPriority, "report", 0, "")
			if err != nil {
				t.Fatal(err)
			}
			var req any
			if err := q.QueryRow("SELECT request FROM tasks WHERE id = ?", tid).Scan(&req); err != nil {
				t.Fatal(err)
			}
			got := map[string]string{"<nil>": "", fmt.Sprint(tid): "own", "1": "T1"}[fmt.Sprint(req)]
			if got != c.want {
				t.Errorf("request %v (%q), want %q", req, got, c.want)
			}
		})
	}
}
