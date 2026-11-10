package reads

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// task.get's sub-task and blocker items carry what the web's look() reads of a task, so its list draws each row's state without reading
// the task: a held one says why, a blocked one what blocks it, a done one its evidence.
func TestATaskGetsSubtasksAndBlockersCarryWhatTheirLookReads(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, parent_task, done_evidence) VALUES"+
		"(1, 'parent', 'bob', 'human', 1, 'human', NULL, NULL), (2, 'held', 'bob', 'human', 1, 'human', 1, NULL), "+
		"(3, 'failed', 'bob', 'human', 1, 'human', 1, 'FAIL @abcdef1: broke'), (4, 'blocked', 'bob', 'human', 1, 'human', 1, NULL), "+
		"(5, 'blocker', 'bob', 'human', 1, 'human', NULL, NULL)")
	run(t, e, `INSERT INTO meta(name, value, updated) VALUES('taskhold:2', '{"why": "bob is mid-turn", "on": "owner", "able": false, "since": 1792000000}', 0)`)
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(4, 5, 1), (1, 4, 1)")
	got, err := e.taskGet(nil, map[string]any{"task": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	task := got.(map[string]any)
	subs := map[int64]map[string]any{}
	for _, s := range task["subtasks"].([]any) {
		m := s.(map[string]any)
		subs[m["id"].(int64)] = m
	}
	if h := subs[2]; h["state"] != "queued" || h["waiting"].(map[string]any)["on"] != "owner" || !strings.Contains(fmt.Sprint(h["hold_reason"]), "bob is mid-turn") || h["block_reason"] != nil {
		t.Errorf("held: %v", h)
	}
	if f := subs[3]; f["state"] != "done" || f["evidence"] != "FAIL @abcdef1: broke" || f["waiting"] != nil || f["hold_reason"] != nil {
		t.Errorf("failed: %v", f)
	}
	if b := subs[4]; b["block_reason"] != "waiting on T5" || b["waiting"].(map[string]any)["on"] != "task" || b["evidence"] != nil {
		t.Errorf("blocked: %v", b)
	}
	bs := task["blockers"].([]any)
	if len(bs) != 1 {
		t.Fatalf("%v", bs)
	}
	if b := bs[0].(map[string]any); b["task"] != int64(4) || b["title"] != "blocked" || b["owner"] != "bob" || b["reason"] != "T4 (bob) blocked" ||
		b["state"] != "queued" || b["block_reason"] != "waiting on T5" || b["waiting"].(map[string]any)["on"] != "task" {
		t.Errorf("blocker: %v", b)
	}
}
