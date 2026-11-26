package taskwrites

import (
	"database/sql"
	"strings"
	"testing"
)

// task.wait then done opens a gate that names the task it closes when it passes; without then the gate closes none. It goes only with a
// pull request waited on until landed, whose pass names the merge commit: any other watch, or any other word, is a bad request and
// opens no gate.
func TestTaskWaitThenDoneOpensAGateThatClosesTheTask(t *testing.T) {
	loggedIn(t)
	sdb, _, run := reportEnv(t, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(9, 'ship', 'w', 'boss', 1, 'boss')")
	gate := func(id float64) (title string, closes sql.NullInt64) {
		t.Helper()
		if err := sdb.QueryRow("SELECT title, watch_closes FROM tasks WHERE id = ? AND owner = 'gate'", int64(id)).Scan(&title, &closes); err != nil {
			t.Fatal(err)
		}
		return title, closes
	}
	r := run("w", "task.wait", "w1", `,"task":9,"watch":{"vendor":"github","target":"o/r#7","until":"landed"},"then":"done"`)
	if r["error"] != nil {
		t.Fatalf("%v", r["error"])
	}
	res := r["result"].(map[string]any)
	if s := res["said"].(string); !strings.HasPrefix(s, "T9 waits on gate T") || !strings.Contains(s, " (watching PR #7 to land; a merge closes T9 with its commit)") {
		t.Fatal(s)
	}
	if title, closes := gate(res["gate"].(float64)); title != "T9 waits on PR #7 to land, then is done" || closes.Int64 != 9 {
		t.Fatalf("%q closes %v", title, closes)
	}
	r = run("w", "task.wait", "w2", `,"task":9,"watch":{"vendor":"github","target":"o/r#7","until":"landed"}`)
	if title, closes := gate(r["result"].(map[string]any)["gate"].(float64)); title != "T9 waits on PR #7 to land" || closes.Valid {
		t.Fatalf("no then: %q closes %v", title, closes)
	}
	for _, c := range []struct{ params, why string }{
		{`,"task":9,"watch":{"vendor":"github","target":"o/r#7"},"then":"done"`, "then done goes with a pull request waited on until landed"},
		{`,"task":9,"watch":{"vendor":"github","target":"o/r#7","until":"closed"},"then":"done"`, "then done goes with a pull request waited on until landed"},
		{`,"task":9,"watch":{"vendor":"github","target":"o/r#12","until":"done"},"then":"done"`, "then done goes with a pull request waited on until landed"},
		{`,"task":9,"watch":{"vendor":"github","target":"o/r#7","until":"landed"},"then":"merge"`, "then is done"},
		{`,"task":9,"watch":{"vendor":"github","target":"o/r#7","until":"landed"},"then":true`, "then is done"},
	} {
		if e := errText(run("w", "task.wait", "bad-"+c.params, c.params)); !strings.Contains(e, c.why) {
			t.Errorf("%s: %q", c.params, e)
		}
	}
	if n := count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE owner = 'gate'"); n != 2 {
		t.Fatalf("%d gates", n)
	}
}

// The gate closes the task in its holder's name, so asking for that is asking for a done: only who may mark the task done may ask
// (its assignee, its owner, its parent's owner, the founder). Anyone else is refused with who may, and no gate is opened; the same
// agent may still make the task wait, which only gives it back to its holder.
func TestOnlyWhoMayCloseATaskMayHaveAMergeCloseIt(t *testing.T) {
	loggedIn(t)
	sdb, _, run := reportEnv(t,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(8, 'the story', 'lead', 'boss', 1, 'boss')",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, parent_task) VALUES(9, 'ship', 'w', 'boss', 1, 'boss', 8)")
	gates := func() int { return count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE owner = 'gate'") }
	landed := `,"task":9,"watch":{"vendor":"github","target":"o/r#7","until":"landed"}`
	e := errText(run("mgr", "task.wait", "m1", landed+`,"then":"done"`))
	if !strings.Contains(e, "task T9 can be closed by its merge only on the word of who may mark it done: its assignee (") ||
		!strings.Contains(e, "Without --then done the wait only gives it back to its holder") || gates() != 0 {
		t.Fatalf("an agent that may not close it: %q, %d gates", e, gates())
	}
	if n := count(t, sdb, "SELECT COUNT(*) FROM task_blocks WHERE task_id = 9"); n != 0 {
		t.Fatalf("a refused wait blocked the task: %d", n)
	}
	if r := run("mgr", "task.wait", "m2", landed); r["error"] != nil || gates() != 1 {
		t.Fatalf("a plain wait is anyone's: %v, %d gates", r["error"], gates())
	}
	for i, who := range []string{"w", "boss", "lead"} { // its assignee, its owner, its parent's owner
		if r := run(who, "task.wait", "ok-"+who, landed+`,"then":"done"`); r["error"] != nil || gates() != i+2 {
			t.Fatalf("%s may close it, so may ask: %v, %d gates", who, r["error"], gates())
		}
	}
	if n := count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE owner = 'gate' AND watch_closes = 9"); n != 3 {
		t.Fatalf("%d gates close the task", n)
	}
}
