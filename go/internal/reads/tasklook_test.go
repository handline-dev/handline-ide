package reads

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// task.get's sub-task and blocker items carry what the web's look() reads of a task, so its list draws each row's state without reading
// the task: a held one says why, a blocked one what blocks it, a done one its evidence.
func TestATaskGetsSubtasksAndBlockersCarryWhatTheirLookReads(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, parent_task, done_evidence) VALUES"+
		"(1, 'parent', 'bob', 'p1', 1, 'p1', NULL, NULL), (2, 'held', 'bob', 'p1', 1, 'p1', 1, NULL), "+
		"(3, 'failed', 'bob', 'p1', 1, 'p1', 1, 'FAIL @abcdef1: broke'), (4, 'blocked', 'bob', 'p1', 1, 'p1', 1, NULL), "+
		"(5, 'blocker', 'bob', 'p1', 1, 'p1', NULL, NULL)")
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

// task.get's todo is the count kept with the working agent's newest note, while the task is open.
func TestATaskGetsTodoIsItsKeptCountWhileItIsOpen(t *testing.T) {
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, done_evidence, discarded_by) VALUES"+
		"(1, 'open', 'bob', 'p1', 1, 'p1', NULL, NULL), (2, 'done', 'bob', 'p1', 1, 'p1', 'ok', NULL), (3, 'gone', 'bob', 'p1', 1, 'p1', NULL, 'p1'), "+
		"(4, 'another agent counted', 'bob', 'p1', 1, 'p1', NULL, NULL)")
	was := db.Now
	db.Now = func() int64 { return 1000 }
	defer func() { db.Now = was }()
	for id, by := range map[int64]string{1: "bob", 2: "bob", 3: "bob", 4: "carol"} {
		if err := db.SetTaskTodo(e.DB, id, &db.TaskTodo{By: by, Done: 3, Total: 5, Current: "wiring the gate"}); err != nil {
			t.Fatal(err)
		}
	}
	todo := func(id string) any {
		t.Helper()
		got, err := e.taskGet(nil, map[string]any{"task": json.Number(id)})
		if err != nil {
			t.Fatal(err)
		}
		v, ok := got.(map[string]any)["todo"]
		if !ok {
			return nil
		}
		return v
	}
	want := map[string]any{"done": 3, "total": 5, "current": "wiring the gate", "updated_at": ts(int64(1000))}
	if got := todo("1"); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("open: %v, want %v", got, want)
	}
	for _, id := range []string{"2", "3", "4"} {
		if got := todo(id); got != nil {
			t.Errorf("T%s: %v, want no todo", id, got)
		}
	}
}
