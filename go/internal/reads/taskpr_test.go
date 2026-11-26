package reads

import (
	"encoding/json"
	"testing"
)

// taskRead is task.get of task id as the wire carries it.
func taskRead(t *testing.T, e *Env, id int64) map[string]any {
	t.Helper()
	got, err := e.taskGet(nil, map[string]any{"task": json.Number(itoa(id))})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}

// A task carries its pull request as a number and, when it was named with one, the repository the number is in.
func TestATaskCarriesItsPullRequestAndTheRepositoryItIsIn(t *testing.T) {
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr, pr_repo) VALUES(1, 'ship', 'bob', 'p1', 1, 'p1', 412, 'acme/app')")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr) VALUES(2, 'ship too', 'bob', 'p1', 1, 'p1', 7)")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(3, 'no pull request', 'bob', 'p1', 1, 'p1')")
	for id, want := range map[int64][2]any{1: {float64(412), "acme/app"}, 2: {float64(7), nil}, 3: {nil, nil}} {
		if task := taskRead(t, e, id); task["pr"] != want[0] || task["pr_repo"] != want[1] {
			t.Errorf("task %d: pr %v in %v, want %v", id, task["pr"], task["pr_repo"], want)
		}
	}
}
