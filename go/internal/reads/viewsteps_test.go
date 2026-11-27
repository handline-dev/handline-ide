package reads

import (
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
)

// An agent's view carries the step count of the task it holds, as task.get answers it: none from another agent or on a done task, and
// a new count shows at the next read.
func TestAgentViewCarriesTheStepsOfItsTask(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	addAgent(t, e, "amy", "")
	bob, amy := teamtest.Key(t, e.Home, "bob"), teamtest.Key(t, e.Home, "amy")
	steps := func() string { return fmt.Sprint(viewOfAgent(t, e, bob)["steps"]) }
	set := func(by string, done, total int) {
		t.Helper()
		if err := db.SetTaskTodo(e.DB, 1, &db.TaskTodo{By: by, Done: done, Total: total, Current: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'a', ?, 'p1', 1, 'p1')", bob)
	run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES(?, json_object('name', ?, 'state', 'running', 'task', json_object('id', 1, 'title', 'a')), 0)`, bob, bob)
	if got := steps(); got != "<nil>" {
		t.Fatalf("no count given: %s", got)
	}
	set(bob, 3, 7)
	if got := steps(); got != "map[done:3 total:7]" {
		t.Fatalf("its own count: %s", got)
	}
	set(bob, 4, 7)
	if got := steps(); got != "map[done:4 total:7]" {
		t.Fatalf("a new count: %s", got)
	}
	set(amy, 1, 2)
	if got := steps(); got != "<nil>" {
		t.Fatalf("another agent's count: %s", got)
	}
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(1, 1, ?, 'look', ?, 1)", amy, bob)
	if got := steps(); got != "map[done:1 total:2]" {
		t.Fatalf("the count of the agent acting on it as a turn: %s", got)
	}
	run(t, e, "DELETE FROM task_turns")
	set(bob, 4, 7)
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id = 1")
	if got := steps(); got != "<nil>" {
		t.Fatalf("a done task: %s", got)
	}
	run(t, e, "UPDATE tasks SET done_evidence = NULL WHERE id = 1")
	run(t, e, `UPDATE agent_view SET view = json_set(view, '$.task', NULL)`)
	if got := steps(); got != "<nil>" {
		t.Fatalf("no task held: %s", got)
	}
}
