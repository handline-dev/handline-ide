package reads

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

func clockOf(t *testing.T, e *Env, id int64) (map[string]any, map[string]any) {
	t.Helper()
	got, err := e.Tasks([]int64{id})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := got[id]["time"].(map[string]any)
	return got[id], c
}

func TestATasksClockRunsOnlyWhileItIsDoing(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'q', 'bob', 'human', 1, 'human')")
	_, c := clockOf(t, e, 1)
	if c["length_s"] != int64(4*3600) || c["default"] != true || c["start"] != nil || c["end"] != nil || c["told"] != 0 {
		t.Fatalf("a queued task has a length and no counting: %v", c)
	}
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, timeout_s, timeout_at, sent_at) VALUES(2, 'd', 'bob', 'human', 1, 'human', 600, ?, 1)", now+300)
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'RUNNING', 0, 2)")
	task, c := clockOf(t, e, 2)
	if task["state"] != "doing" || c["length_s"] != int64(600) || c["default"] != false || c["start"] != ts(now+300-600) ||
		c["end"] != ts(now+300) || c["overdue_since"] != nil {
		t.Fatalf("%v %v", task["state"], c)
	}
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id = 2")
	if task, c = clockOf(t, e, 2); task["time"] != nil {
		t.Fatalf("a done task has no clock: %v", task["time"])
	}
}

func TestAnOverClockSaysSinceItsEndAndCountsTheNoticesOfItsBox(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, timeout_s, timeout_at, sent_at) VALUES(1, 'd', 'bob', 'human', 1, 'human', 600, ?, 1)", now-100)
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'RUNNING', 0, 1)")
	if _, c := clockOf(t, e, 1); c["overdue_since"] != ts(now-100) || c["told"] != 0 {
		t.Fatalf("no notice yet: over since its end: %v", c)
	}
	// One notice before an edit of the timeout started a new box, two after it.
	run(t, e, `INSERT INTO events(ts, agent, kind, payload) VALUES
		(1, 'bob', 'task_timeout_exceeded', '{"task_id": 1, "over_s": 0}'),
		(2, 'human', 'edit_task', '{"task_id": 1, "timeout": {"old": "5m", "new": "10m"}}'),
		(3, 'bob', 'task_timeout_exceeded', '{"task_id": 1, "over_s": 0}'),
		(4, 'bob', 'task_timeout_exceeded', '{"task_id": 1, "over_s": 600}')`)
	if _, c := clockOf(t, e, 1); c["overdue_since"] != ts(now-100) || c["told"] != 2 {
		t.Fatalf("told counts the notices of this box: %v", c)
	}
}

func TestAPausedClockOverItsLengthKeepsHowFarOver(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, timeout_s, timeout_left, sent_at) VALUES(1, 'd', 'bob', 'human', 1, 'human', 600, -120, 1)")
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'RUNNING', 0, 1)")
	if _, c := clockOf(t, e, 1); c["left_s"] != int64(-120) || c["end"] != nil {
		t.Fatalf("%v", c)
	}
}

// The tick never tells a task with an open child, or one the founder holds, that it is overdue, so the clock does not say so either.
func TestAPastDeadlineIsNotOverdueWhileAChildIsOpenOrTheFounderHoldsIt(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, timeout_s, timeout_at, sent_at) VALUES(1, 'd', 'bob', 'human', 1, 'human', 600, ?, 1)", now-100)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES(2, 'child', 'amy', 'bob', 1, 1)")
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'RUNNING', 0, 1)")
	if task, c := clockOf(t, e, 1); task["state"] != "doing" || c["overdue_since"] != nil {
		t.Fatalf("an open child: not overdue: %v %v", task["state"], c)
	}
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id = 2")
	if _, c := clockOf(t, e, 1); c["overdue_since"] != ts(now-100) {
		t.Fatalf("the child done: overdue: %v", c)
	}
	run(t, e, "UPDATE tasks SET owner = 'human' WHERE id = 1")
	if task, c := clockOf(t, e, 1); task["state"] != "doing" || c["overdue_since"] != nil {
		t.Fatalf("the founder holds it: not overdue: %v %v", task["state"], c)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestATaskListsTheOpenTasksThatHoldItAndNoDoneOne(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'held', 'bob', 'human', 1), (2, 'open', 'amy', 'human', 1), (3, 'done', 'amy', 'human', 1)")
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id = 3")
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(1, 2, 1), (1, 3, 1)")
	task, _ := clockOf(t, e, 1)
	if got, _ := task["blocked_by"].([]int64); len(got) != 1 || got[0] != 2 || task["block_reason"] != "waiting on T2" {
		t.Fatalf("%v", task)
	}
	if task, _ = clockOf(t, e, 2); len(task["blocked_by"].([]int64)) != 0 {
		t.Fatalf("nothing holds it: %v", task["blocked_by"])
	}
}

func TestAllWithBeforePagesThroughEveryTaskNewestFirst(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for i := 1; i <= 5; i++ {
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(?, 't', 'bob', 'human', 1)", i)
	}
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id IN (2, 4)")
	c := &rpc.Conn{Caller: rpc.Caller{Role: "founder"}}
	var seen []int64
	params := map[string]any{"all": true, "limit": json.Number("2")}
	for {
		got, err := e.tasksList(c, params)
		if err != nil {
			t.Fatal(err)
		}
		page := got.(map[string]any)
		items := page["items"].([]any)
		for _, it := range items {
			seen = append(seen, it.(map[string]any)["id"].(int64))
		}
		if page["more"] != true {
			break
		}
		params["before"] = json.Number(strconv.FormatInt(seen[len(seen)-1], 10))
	}
	if len(seen) != 5 || seen[0] != 5 || seen[4] != 1 {
		t.Fatalf("every task once, newest first: %v", seen)
	}
}

func TestADoingTaskGivenNoTimeoutCountsFourHoursBack(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	end := time.Now().Unix() + 3600
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, timeout_at, sent_at) VALUES(1, 'd', 'bob', 'human', 1, 'human', ?, 1)", end)
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'RUNNING', 0, 1)")
	_, c := clockOf(t, e, 1)
	if c["length_s"] != int64(4*3600) || c["default"] != true || c["start"] != ts(end-4*3600) || c["end"] != ts(end) {
		t.Fatalf("%v", c)
	}
}

func TestAPausedClockSaysWhatItHasLeftAndNoStartOrEnd(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, timeout_s, timeout_left, sent_at) VALUES(1, 'd', 'bob', 'human', 1, 'human', 600, 240, 1)")
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'RUNNING', 0, 1)")
	task, c := clockOf(t, e, 1)
	if task["state"] != "doing" || c["left_s"] != int64(240) || c["start"] != nil || c["end"] != nil {
		t.Fatalf("%v %v", task["state"], c)
	}
	run(t, e, "UPDATE tasks SET timeout_left = NULL, timeout_at = ? WHERE id = 1", time.Now().Unix()+240)
	if _, c = clockOf(t, e, 1); c["left_s"] != nil || c["end"] == nil {
		t.Fatalf("a running clock has an end and nothing kept: %v", c)
	}
}
