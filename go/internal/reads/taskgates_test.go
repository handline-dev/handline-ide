package reads

import (
	"reflect"
	"testing"
)

// A task keeps how the gates it waited on opened: what each watched, its result and the line it opened with, newest first. A gate
// still watching is a blocker, not one of these; so is no gate at all, a gate of another task, or one closed by hand with no result.
func TestATaskKeepsHowTheGatesItWaitedOnOpened(t *testing.T) {
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'ship', 'bob', 'p1', 1, 'p1')")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(2, 'the other half', 'bob', 'p1', 1, 'p1')")
	gate := func(id int, label, result, evidence string, doneAt int) {
		t.Helper()
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at, watch_vendor, watch_target, watch_until, "+
			"watch_label, watch_result, done_evidence, done_at) VALUES(?, 'a wait', 'gate', 'bob', 1, 'bob', 2, 'github', 'o/r#7', 'landed', ?, "+
			"NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, 0))", id, label, result, evidence, doneAt)
	}
	gate(3, "PR #7 CI", "passed", "ci passed", 100)
	gate(4, "PR #7 to land", "failed", "ci failed: build, lint", 200)
	gate(5, "PR #7 to land", "failed", "changes requested", 300)
	gate(6, "PR #7 to land", "", "", 0)                 // still watching
	gate(7, "PR #7 to land", "", "closed by hand", 400) // done by a person: its watch did not open it
	gate(8, "PR #9 to land", "passed", "merged 984c657", 500)
	for _, b := range [][2]int{{1, 3}, {1, 4}, {1, 5}, {1, 6}, {1, 7}, {2, 8}, {1, 2}} {
		run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(?, ?, 1)", b[0], b[1])
	}
	var got [][3]any
	for _, g := range taskRead(t, e, 1)["gates"].([]any) {
		m := g.(map[string]any)
		got = append(got, [3]any{m["task"], m["result"], m["label"].(string) + ": " + m["evidence"].(string)})
	}
	want := [][3]any{{float64(5), "failed", "PR #7 to land: changes requested"}, {float64(4), "failed", "PR #7 to land: ci failed: build, lint"},
		{float64(3), "passed", "PR #7 CI: ci passed"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\n got %v\nwant %v", got, want)
	}
	if gates := taskRead(t, e, 2)["gates"].([]any); len(gates) != 1 || gates[0].(map[string]any)["evidence"] != "merged 984c657" {
		t.Fatalf("the other task's: %v", gates)
	}
	if gates := taskRead(t, e, 3)["gates"].([]any); len(gates) != 0 {
		t.Fatalf("a task that waited on none: %v", gates)
	}
}
