package gates

import (
	"testing"

	"github.com/handline/handline/go/kit/actor"
)

// What the gate decided about its task was read before the fetch, and the fetch can take a minute. A task that changed meanwhile is
// not closed on the old reading: the write reads it again, writes nothing when it changed, and the next look decides as it then
// stands, by the same rules as any look. The change is made here while the fetch runs.
func TestATaskThatChangedWhileItsMergeWasCheckedIsNotClosedOnTheOldReading(t *testing.T) {
	turn := "INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(2, 1, ?, ?, 1)"
	for _, c := range []struct {
		name, change string
		args         func(r *landRig) []any
		// what the next look leaves: the gate's evidence after "merged <sha>", the task's evidence ("sha" for the merge commit) and who
		// marked it done
		gate, task string
		by         func(r *landRig) string
	}{
		{"it became a person's turn", turn, func(r *landRig) []any { return []any{actor.OwnerKey, r.bob} },
			"; T2 not closed: it is not an agent's to close now", "", nil},
		{"it became another agent's turn", turn, func(r *landRig) []any { return []any{r.rev, r.bob} },
			"", "sha", func(r *landRig) string { return r.rev }},
		{"it was taken off the gate", "DELETE FROM task_blocks WHERE task_id = 2", nil, "", "", nil},
		{"its holder closed it", "UPDATE tasks SET done_evidence = 'by hand', done_at = 1, done_via = 'words' WHERE id = 2", nil, "", "by hand", nil},
	} {
		r := newLandRig(t, "main")
		sha := r.merge("main")
		fetch, changed := r.s.Fetch, false
		r.s.Fetch = func(repo string) error {
			if !changed {
				changed = true
				var args []any
				if c.args != nil {
					args = c.args(r)
				}
				r.exec(c.change, args...)
			}
			return fetch(repo)
		}
		r.pass(1000)
		if e, _, by, _ := r.task(2); by != "" || (e != "" && e != "by hand") {
			t.Fatalf("%s: T2 closed on the old reading: %q by %q", c.name, e, by)
		}
		if e, _, _, _ := r.task(1); e != "" || len(r.tell.done) != 0 || r.comments() != "" {
			t.Fatalf("%s: the look that found it changed wrote something: the gate %q, told of %v, comments %q", c.name, e, r.tell.done, r.comments())
		}
		r.pass(1060)
		want, by := c.task, ""
		if want == "sha" {
			want, by = sha, c.by(r)
		}
		if e, _, who, _ := r.task(2); e != want || who != by {
			t.Fatalf("%s: T2 after the next look: %q by %q, want %q by %q", c.name, e, who, want, by)
		}
		if e, _, _, _ := r.task(1); e != "merged "+sha+c.gate {
			t.Fatalf("%s: the gate opened with %q", c.name, e)
		}
	}
}
