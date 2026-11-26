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
