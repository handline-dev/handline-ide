package reads

import (
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

// An idle agent whose held tasks all wait names the first task they wait on, its holder and how many more; one held task that does not
// wait, a working agent or nothing held names none. A turn on its task, the founder's too, is what that task waits on.
func TestAgentViewSaysWhatItsHeldTasksWaitOn(t *testing.T) {
	t.Parallel()
	waits := func(t *testing.T, e *Env, agent string) string {
		t.Helper()
		return fmt.Sprint(viewOfAgent(t, e, agent)["waits_on"])
	}
	setup := func(t *testing.T) (*Env, string, string) {
		e := testEnv(t)
		addAgent(t, e, "bob", "")
		addAgent(t, e, "amy", "")
		bob, amy := teamtest.Key(t, e.Home, "bob"), teamtest.Key(t, e.Home, "amy")
		run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES(?, json_object('name', ?, 'state', 'idle', 'status', json_object('state', 'idle')), 0)`, bob, bob)
		// 1 is bob's current task, doing; 2 and 3 are amy's, 4 is the gate's.
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'a', ?, 'p1', 1, 'p1'), "+
			"(2, 'b', ?, 'p1', 1, 'p1'), (3, 'c', ?, 'p1', 1, 'p1'), (4, 'd', 'gate', 'p1', 1, 'p1')", bob, amy, amy)
		run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES(?, 'IDLE', 0, 1)", bob)
		return e, bob, amy
	}

	e, bob, amy := setup(t)
	if got := waits(t, e, bob); got != "<nil>" {
		t.Fatalf("a held task that waits on nothing: %s", got)
	}
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(1, 2, 1)")
	if got, want := waits(t, e, bob), "map[holder:"+amy+" more:0 task:2]"; got != want {
		t.Fatalf("one blocker: %s, want %s", got, want)
	}
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(1, 3, 1), (1, 4, 1)")
	if got, want := waits(t, e, bob), "map[holder:"+amy+" more:2 task:2]"; got != want {
		t.Fatalf("three blockers: %s, want %s", got, want)
	}
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id IN (2, 3)")
	if got := waits(t, e, bob); got != "map[holder:<nil> more:0 task:4]" {
		t.Fatalf("a blocker no agent holds: %s", got)
	}
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(5, 'e', ?, 'p1', 1, 'p1')", bob)
	if got := waits(t, e, bob); got != "<nil>" {
		t.Fatalf("another held task that does not wait: %s", got)
	}
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(5, 1, 'p1', 'look', ?, 1)", bob)
	if got := waits(t, e, bob); got != "map[holder:<nil> more:1 task:4]" {
		t.Fatalf("a founder's turn on the other: %s", got)
	}
	run(t, e, `UPDATE agent_view SET view = json_set(view, '$.status.state', 'running')`)
	if got := waits(t, e, bob); got != "<nil>" {
		t.Fatalf("a working agent: %s", got)
	}

	e, bob, _ = setup(t)
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(1, 1, 'p1', 'look', ?, 1)", bob)
	if got := waits(t, e, bob); got != "map[holder:p1 more:0 task:1]" {
		t.Fatalf("the founder's turn: %s", got)
	}

	e, bob, amy = setup(t)
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, dormant, created_at) VALUES(1, 2, 1, 1)")
	if got := waits(t, e, bob); got != "<nil>" {
		t.Fatalf("a dormant block holds nothing: %s", got)
	}
	// bob's child 7 is amy's draft: it waits for the founder to run it.
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES(7, 'g', ?, ?, 1, 1)", amy, bob)
	if got := waits(t, e, bob); got != "map[holder:p1 more:0 task:7]" {
		t.Fatalf("a draft child: %s", got)
	}
	// amy acts on bob's 1 as a turn, and it waits on her draft: her own 2 and 3 wait on nothing.
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(1, 1, ?, 'look', ?, 1)", amy, bob)
	if got, want := waits(t, e, bob), "map[holder:"+amy+" more:0 task:1]"; got != want {
		t.Fatalf("amy's turn on bob's task: %s, want %s", got, want)
	}
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id IN (2, 3)")
	run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES(?, json_object('name', ?, 'state', 'idle'), 0)`, amy, amy)
	if got := waits(t, e, amy); got != "map[holder:p1 more:0 task:7]" {
		t.Fatalf("the turn's agent waits on what the task waits on: %s", got)
	}
	run(t, e, `UPDATE agent_view SET view = json_set(view, '$.state', 'running') WHERE agent = ?`, amy)
	if got := waits(t, e, amy); got != "<nil>" {
		t.Fatalf("a view stored with no status works by its state: %s", got)
	}

	e, bob, amy = setup(t)
	// bob's own child 6 waits on amy's 2: the child names no one, what it waits on is counted.
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, parent_task) VALUES(6, 'f', ?, ?, 1, ?, 1)", bob, bob, bob)
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(6, 2, 1)")
	if got, want := waits(t, e, bob), "map[holder:"+amy+" more:0 task:2]"; got != want {
		t.Fatalf("a child of its own that waits: %s, want %s", got, want)
	}
	if got := waits(t, e, amy); got != "<nil>" {
		t.Fatalf("nothing held waits: %s", got)
	}
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id IN (1, 2, 3, 6)")
	if got := waits(t, e, bob); got != "<nil>" {
		t.Fatalf("nothing held: %s", got)
	}
}
