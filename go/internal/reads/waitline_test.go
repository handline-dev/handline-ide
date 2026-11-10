package reads

import (
	"encoding/json"
	"strconv"
	"testing"
)

// A queued task with no hold record names the other task its owner is on, on its own page and in its parent's sub-task list; one whose
// owner holds nothing keeps the not-looked-at line.
func TestAQueuedTasksWaitLineNamesTheTaskItsOwnerIsOn(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, parent_task) VALUES"+
		"(1, 'on it', 'bob', 'human', 1, 'human', NULL), (2, 'next', 'bob', 'human', 1, 'human', 3), (3, 'parent', 'cy', 'human', 1, 'human', NULL)")
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'BUSY', 1, 1)")
	get := func(id int) map[string]any {
		t.Helper()
		got, err := e.taskGet(nil, map[string]any{"task": json.Number(strconv.Itoa(id))})
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)
	}
	const busy = "waiting: bob is on T1; starts when it finishes"
	if got := get(2)["hold_reason"]; got != busy {
		t.Errorf("queued behind bob's task: %v", got)
	}
	if subs := get(3)["subtasks"].([]any); len(subs) != 1 || subs[0].(map[string]any)["hold_reason"] != busy {
		t.Errorf("the parent's sub-task row: %v", subs)
	}
	if got := get(3)["hold_reason"]; got != "waiting: the daemon has not looked at it yet - it does on its next tick" {
		t.Errorf("cy holds nothing: %v", got)
	}
}
