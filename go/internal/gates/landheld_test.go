package gates

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/handline/handline/go/taskboard/watch"
)

// A gate that saw the merge and could never check it does not end as plain "timed out": at its deadline it opens as passed, with the
// merge it saw, and its holder is told the commit and why the task was not closed, on the gate and in a comment on the task. That is
// what the holder needs to close it by hand.
func TestAGateHeldAsPassedOpensAtItsDeadlineWithTheMergeItSaw(t *testing.T) {
	r := newLandRig(t, "main")
	sha := r.merge("main")
	r.fetchErr = errors.New("git fetch: fatal: unable to access the remote: Could not resolve host")
	r.pass(1000)
	r.pass(4940)
	if e, _, _, b := r.task(2); e != "" || b != 1 || len(r.tell.done) != 0 {
		t.Fatalf("before the deadline: T2 %q, %d blockers, told of %v", e, b, r.tell.done)
	}
	r.pass(5000)
	want := "merged " + sha + "; T2 not closed: git fetch: fatal: unable to access the remote: Could not resolve host (tried until the gate's deadline)"
	gate, _, _, _ := r.task(1)
	if e, _, _, b := r.task(2); e != "" || b != 0 || gate != want {
		t.Fatalf("at the deadline: T2 %q, %d blockers, the gate %q", e, b, gate)
	}
	var result string
	if err := r.d.QueryRow("SELECT watch_result FROM tasks WHERE id = 1").Scan(&result); err != nil || result != "passed" {
		t.Fatalf("the gate's result: %q %v", result, err)
	}
	if m := r.comments(); m != "daemon: Gate T1 passed (merged "+sha+") and did not close this task: git fetch: fatal: unable to access the remote: "+
		"Could not resolve host (tried until the gate's deadline)" {
		t.Fatalf("the comment: %s", m)
	}
	if len(r.tell.done) != 1 || r.tell.done[0] != 1 {
		t.Fatalf("told of %v", r.tell.done)
	}
}

// The network that fails the fetch often fails the read of GitHub too. A read that fails while the gate is held does not put its own
// reason over the merge the gate saw: the status keeps saying it passed, and at the deadline the holder still gets the commit.
func TestAReadThatFailsAfterTheMergeWasSeenDoesNotForgetIt(t *testing.T) {
	r := newLandRig(t, "main")
	sha := r.merge("main")
	r.fetchErr = errors.New("git fetch: fatal: Could not resolve host")
	r.pass(1000)
	held := "passed, merged " + sha + "; not closing T2 yet: git fetch: fatal: Could not resolve host"
	if r.watchError() != held {
		t.Fatalf("the gate says %q", r.watchError())
	}
	r.s.reads(func(context.Context, string, string) (watch.Verdict, string, error) {
		return "", "", errors.New("gh pr view: could not resolve host api.github.com")
	})
	r.pass(1060)
	if r.watchError() != held {
		t.Fatalf("after a read that failed the gate says %q", r.watchError())
	}
	r.pass(5000)
	if e, _, _, _ := r.task(1); e != "merged "+sha+"; T2 not closed: git fetch: fatal: Could not resolve host (tried until the gate's deadline)" {
		t.Fatalf("at the deadline the gate opened with %q", e)
	}
}

// A held gate whose task was taken off it, or ended, before the deadline still opens with the merge it saw, and touches no task.
func TestAHeldGateWhoseTaskLeftItOpensAsPassedAndTouchesNothing(t *testing.T) {
	r := newLandRig(t, "main")
	sha := r.merge("main")
	r.fetchErr = errors.New("git fetch timed out after 1m0s")
	r.pass(1000)
	r.exec("DELETE FROM task_blocks WHERE task_id = 2")
	r.pass(5000)
	if e, _, _, _ := r.task(1); e != "merged "+sha || r.comments() != "" {
		t.Fatalf("the gate %q, comments %q", e, r.comments())
	}
}

// A fetch that fails does not stop the look at the clone: when the clone already holds the merge on origin's copy of the default
// branch (its holder fetched it), the task is closed though the daemon's own fetch could not run.
func TestAFetchThatFailsStillClosesFromACloneThatHoldsTheMerge(t *testing.T) {
	r := newLandRig(t, "main")
	sha := r.merge("main")
	r.fetchErr = errors.New("git fetch timed out after 1m0s")
	r.pass(1000)
	if e, _, _, _ := r.task(2); e != "" || !strings.HasSuffix(r.watchError(), "; not closing T2 yet: git fetch timed out after 1m0s") {
		t.Fatalf("the clone has no merge yet: T2 %q, the gate says %q", e, r.watchError())
	}
	gitAt(t, r.repo, "fetch", "-q", "origin", "main")
	r.pass(1060)
	if e, via, by, _ := r.task(2); e != sha || via != "sha" || by != r.bob {
		t.Fatalf("T2 done %q via %q by %q, want the merge commit by its holder", e, via, by)
	}
	if e, _, _, _ := r.task(1); e != "merged "+sha || r.watchError() != "" {
		t.Fatalf("the gate: %q, still saying %q", e, r.watchError())
	}
}
