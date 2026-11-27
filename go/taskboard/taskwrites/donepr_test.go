package taskwrites

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/taskboard/watch"
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

// A done in a repository on GitLab waits on its merge request as on a pull request, when glab is logged in to that host here. On a host
// no reader reads, a done that links to its request, or to anything on that host, closes now and says the merge is not watched and why.
func TestADoneOnAnotherHostWaitsOnItsMergeRequestOrSaysItIsNotRead(t *testing.T) {
	was := watch.Vendors["gitlab"]
	watch.Vendors["gitlab"] = &watch.GitLab{Run: func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "auth" && args[3] != "gitlab.com" {
			return nil, errors.New("glab auth: not logged in")
		}
		return nil, nil
	}}
	t.Cleanup(func() { watch.Vendors["gitlab"] = was })
	home, lab, corp := t.TempDir(), t.TempDir(), t.TempDir()
	gitIn(t, lab)("init", "-q")
	gitIn(t, lab)("remote", "add", "origin", "git@gitlab.com:group/app.git")
	gitIn(t, corp)("init", "-q")
	gitIn(t, corp)("remote", "add", "origin", "https://git.corp.example/team/app.git")
	sdb, _, run := reportEnvIn(t, home,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(9, 'ship', 'w', 'boss', 1, 'boss'), "+
			"(10, 'corp', 'mgr', 'boss', 1, 'boss'), (11, 'corp, no link', 'mgr', 'boss', 1, 'boss')")
	teamtest.Set(t, home, "w", "repo", lab)
	teamtest.Set(t, home, "mgr", "repo", corp)
	s := said(t, run("w", "task.done", "d9", `,"task":9,"evidence":"opened https://gitlab.com/Group/app/-/merge_requests/12"`))
	if !strings.Contains(s, "a merge closes T9 with its commit") {
		t.Fatal(s)
	}
	if n := count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE owner = 'gate' AND watch_closes = 9 AND watch_vendor = 'gitlab' AND "+
		"watch_target = 'gitlab.com/Group/app!12' AND watch_until = 'landed'"); n != 1 {
		t.Fatalf("%d gates close T9", n)
	}
	for _, c := range []struct{ tid, evidence string }{
		{"10", "opened https://git.corp.example/team/app/-/merge_requests/3"},
		{"11", "opened https://git.corp.example/team/app/pulls/3"},
	} {
		s := said(t, run("mgr", "task.done", "d"+c.tid, `,"task":`+c.tid+`,"evidence":"`+c.evidence+`"`))
		if !strings.Contains(s, "its pull request's merge is not watched (Pull requests on git.corp.example are not read yet; this repository lands by push.)") {
			t.Errorf("T%s: %q", c.tid, s)
		}
		if n := count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE id = "+c.tid+" AND done_evidence IS NOT NULL"); n != 1 {
			t.Errorf("T%s is not closed now", c.tid)
		}
	}
}
