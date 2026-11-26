package taskwrites

import (
	"fmt"
	"os"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
)

// A gate that waits for a pull request to land watches a week when no time box is given, or as long as landed_deadline_s says: a
// review can take days, and a gate that timed out daily would bring its task back each time to no use. Any other gate keeps its day,
// a time box given is kept as given, and a gate opened by task.new follows the same rule.
func TestAGateOnALandingWatchesAWeekOrAsLongAsTheSettingSays(t *testing.T) {
	loggedIn(t)
	home := t.TempDir()
	sdb, _, run := reportEnvIn(t, home, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(9, 'ship', 'w', 'boss', 1, 'boss')")
	n := 0
	box := func(method, params string) int64 {
		t.Helper()
		n++
		r := run("w", method, "d"+string(rune('a'+n)), params)
		if r["error"] != nil {
			t.Fatalf("%s: %v", params, r["error"])
		}
		res := r["result"].(map[string]any)
		gate := res["gate"]
		if gate == nil {
			gate = res["task"]
		}
		return int64(count(t, sdb, fmt.Sprintf("SELECT timeout_at - sent_at FROM tasks WHERE id = %d", int64(gate.(float64)))))
	}
	landed := `,"task":9,"watch":{"vendor":"github","target":"o/r#7","until":"landed"}`
	if got := box("task.wait", landed); got != 604800 {
		t.Fatalf("a landing waits %ds by default", got)
	}
	if got := box("task.wait", `,"task":9,"watch":{"vendor":"github","target":"o/r#7","until":"closed"}`); got != db.GateDeadlineS {
		t.Fatalf("a close waits %ds", got)
	}
	if got := box("task.wait", landed+`,"time_box":"2h"`); got != 7200 {
		t.Fatalf("a time box given: %ds", got)
	}
	if err := os.WriteFile(paths.ConfigFile(home), []byte("landed_deadline_s: 172800\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := box("task.wait", landed); got != 172800 {
		t.Fatalf("by the setting: %ds", got)
	}
	if got := box("task.new", `,"owner":"gate","title":"wait for it to land","watch":{"vendor":"github","target":"o/r#7","until":"landed"}`); got != 172800 {
		t.Fatalf("a gate opened by task.new: %ds", got)
	}
	if got := box("task.wait", `,"task":9,"watch":{"vendor":"github","target":"o/r#7"}`); got != db.GateDeadlineS {
		t.Fatalf("the setting is for landings only: %ds", got)
	}
	for _, bad := range []string{"landed_deadline_s: 0\n", "landed_deadline_s: soon\n", "landed_deadline_s: -5\n"} {
		os.WriteFile(paths.ConfigFile(home), []byte(bad), 0o644)
		if got := box("task.wait", landed); got != team.LandedDeadlineS {
			t.Fatalf("%q: %ds, want the default", bad, got)
		}
	}
}
