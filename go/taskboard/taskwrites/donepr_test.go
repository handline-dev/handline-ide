package taskwrites

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

// An agent's done whose words link to one pull request of its own repository's GitHub origin waits for that merge: a gate that closes
// the task opens, and the task stays open. A link to another repository, two links, or an agent with no folder close the task now.
func TestADoneThatLinksItsOwnPullRequestWaitsForItsMerge(t *testing.T) {
	loggedIn(t)
	home, repo := t.TempDir(), t.TempDir()
	git := gitIn(t, repo)
	git("init", "-q")
	git("remote", "add", "origin", "git@github.com:acme/app.git")
	sdb, _, run := reportEnvIn(t, home,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(9, 'ship', 'w', 'boss', 1, 'boss'), "+
			"(10, 'elsewhere', 'w', 'boss', 1, 'boss'), (11, 'two', 'w', 'boss', 1, 'boss'), (12, 'no folder', 'mgr', 'boss', 1, 'boss'), "+
			"(13, 'with a child', 'w', 'boss', 1, 'boss'), (14, 'the child', 'mgr', 'w', 1, 'w')",
		"UPDATE tasks SET parent_task = 13 WHERE id = 14")
	teamtest.Set(t, home, "w", "repo", repo)
	done := func(tid string) sql.NullString {
		t.Helper()
		var ev sql.NullString
		if err := sdb.QueryRow("SELECT done_evidence FROM tasks WHERE id = ?", tid).Scan(&ev); err != nil {
			t.Fatal(err)
		}
		return ev
	}
	s := said(t, run("w", "task.done", "d9", `,"task":9,"evidence":"opened https://github.com/Acme/app/pull/12 for review"`))
	if !strings.Contains(s, "a merge closes T9 with its commit") || done("9").Valid {
		t.Fatalf("%q, done %v", s, done("9"))
	}
	if n := count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE owner = 'gate' AND watch_closes = 9 AND watch_target = 'Acme/app#12' AND watch_until = 'landed'"); n != 1 {
		t.Fatalf("%d gates close T9", n)
	}
	// what would refuse the merge's done refuses this one now: the gate it already waits on, an open child
	if e := errText(run("w", "task.done", "d9b", `,"task":9,"evidence":"https://github.com/acme/app/pull/12"`)); !strings.Contains(e, "task T9 is waiting on T") {
		t.Fatalf("a second done on the same pull request: %q", e)
	}
	if e := errText(run("w", "task.done", "d13", `,"task":13,"evidence":"https://github.com/acme/app/pull/13"`)); !strings.Contains(e, "task T13 still has open child tasks") {
		t.Fatalf("a task with an open child: %q", e)
	}
	if n := count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE owner = 'gate'"); n != 1 {
		t.Fatalf("%d gates", n)
	}
	for _, c := range []struct{ tid, evidence, who string }{
		{"10", "https://github.com/other/app/pull/3", "w"},
		{"11", "https://github.com/acme/app/pull/3 and https://github.com/acme/app/pull/4", "w"},
		{"12", "https://github.com/acme/app/pull/5", "mgr"},
	} {
		said(t, run(c.who, "task.done", "d"+c.tid, `,"task":`+c.tid+`,"evidence":"`+c.evidence+`"`))
		if !done(c.tid).Valid {
			t.Errorf("T%s is not closed now", c.tid)
		}
	}
}
