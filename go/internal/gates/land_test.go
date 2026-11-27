package gates

import (
	"context"
	"database/sql"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/taskboard/taskresult"
	"github.com/handline/handline/go/taskboard/watch"
)

// landRig is workspace shop with bob, who holds T2 "ship it" and waits with it on gate T1 (a pull request until landed, which closes T2
// when it passes), and rev, who may review. bob works in a clone of an origin whose default branch is named as asked; the origin gets
// its merges from elsewhere, so bob's clone does not have them until the sweep fetches.
type landRig struct {
	t                   *testing.T
	home                string
	d                   *sql.DB
	tell                *doneTeller
	s                   *Sweep
	origin, repo, other string
	bob, rev, lead      string
	verdict             watch.Verdict
	evidence            string
	fetchErr            error
}

func gitAt(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newLandRig(t *testing.T, branch string) *landRig {
	t.Helper()
	r := &landRig{t: t, home: t.TempDir(), tell: &doneTeller{}, verdict: watch.Waiting}
	r.d = teamtest.Seed(t, r.home, "shop", "shop/bob", "shop/rev")
	r.bob, r.rev, r.lead = teamtest.Key(t, r.home, "shop/bob"), teamtest.Key(t, r.home, "shop/rev"), teamtest.Key(t, r.home, "shop")
	root, _ := filepath.EvalSymlinks(t.TempDir())
	r.origin, r.repo, r.other = filepath.Join(root, "origin.git"), filepath.Join(root, "repo"), filepath.Join(root, "other")
	gitAt(t, root, "init", "-q", "--bare", "-b", branch, r.origin)
	gitAt(t, root, "init", "-q", "-b", branch, r.other)
	gitAt(t, r.other, "remote", "add", "origin", r.origin)
	gitAt(t, r.other, "commit", "-q", "--allow-empty", "-m", "start")
	gitAt(t, r.other, "push", "-q", "origin", branch)
	gitAt(t, root, "clone", "-q", r.origin, r.repo)
	teamtest.Set(t, r.home, "shop/bob", "repo", r.repo)
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, expects) VALUES(2, 'ship it', '" + r.bob + "', '" + r.lead + "', 1, '" + r.lead + "', 'report')",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at, watch_vendor, watch_target, watch_until, watch_label, " +
			"watch_closes, timeout_at) VALUES(1, 'T2 waits on PR #7 to land, then is done', 'gate', '" + r.bob + "', 1, '" + r.bob +
			"', 1, 'test', 'o/r#7', 'landed', 'PR #7 to land', 2, 5000)",
		"INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(2, 1, 1)",
	} {
		r.exec(q)
	}
	r.s = r.sweep()
	return r
}

// sweep is a sweep as a daemon that just started has it: nothing remembered from an earlier one.
func (r *landRig) sweep() *Sweep {
	s := &Sweep{DB: r.d, Home: r.home, Tell: r.tell, Fetch: func(repo string) error {
		if r.fetchErr != nil {
			return r.fetchErr
		}
		return FetchBase(repo, "file")
	}}
	s.reads(func(context.Context, string, string) (watch.Verdict, string, error) {
		return r.verdict, r.evidence, nil
	})
	return s
}

func (r *landRig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.d.Exec(q, args...); err != nil {
		r.t.Fatal(err)
	}
}

// merge lands a commit on a branch of the origin, as a merge on the host does, and says the pull request is merged with it.
func (r *landRig) merge(branch string) string {
	r.t.Helper()
	gitAt(r.t, r.other, "checkout", "-q", "-B", branch)
	gitAt(r.t, r.other, "commit", "-q", "--allow-empty", "-m", "the merge")
	gitAt(r.t, r.other, "push", "-q", "origin", branch)
	sha := gitAt(r.t, r.other, "rev-parse", "HEAD")
	r.verdict, r.evidence = watch.Passed, "merged "+sha
	return sha
}

func (r *landRig) pass(now int64) {
	r.t.Helper()
	if err := r.s.Pass(context.Background(), now); err != nil {
		r.t.Fatal(err)
	}
}

// task is how a task ended and by whom: its done evidence, how it was given, who marked it, and its open blockers.
func (r *landRig) task(id int64) (evidence, via, by string, blockers int) {
	r.t.Helper()
	var e, v, who sql.NullString
	if err := r.d.QueryRow("SELECT done_evidence, done_via, (SELECT agent FROM events WHERE kind = 'mark_task_done' AND "+
		"json_extract(payload, '$.task_id') = tasks.id ORDER BY id DESC LIMIT 1) FROM tasks WHERE id = ?", id).Scan(&e, &v, &who); err != nil {
		r.t.Fatal(err)
	}
	bs, err := db.TaskBlockers(r.d, id)
	if err != nil {
		r.t.Fatal(err)
	}
	return e.String, v.String, who.String, len(bs)
}

func (r *landRig) comments() string {
	r.t.Helper()
	rows, err := r.d.Query("SELECT author || ': ' || body FROM task_comments WHERE task_id = 2 ORDER BY id")
	if err != nil {
		r.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}

func (r *landRig) watchError() string {
	r.t.Helper()
	w, err := db.TaskWatch(r.d, 1)
	if err != nil || w == nil {
		r.t.Fatal(w, err)
	}
	return w.Error
}

// A gate asked to close its task does it when the pull request merges: the daemon fetches the default branch of the holder's own
// repository, finds the merge commit on it, and marks the task done in the holder's name with that commit as its evidence, in the write
// that opens the gate. The task's result says the commit is on the default branch. So it is in a repository whose default is master.
func TestAGateThatPassesClosesItsTaskWithTheMergeCommit(t *testing.T) {
	for _, branch := range []string{"main", "master"} {
		r := newLandRig(t, branch)
		r.pass(1000)
		if e, _, _, b := r.task(2); e != "" || b != 1 {
			t.Fatalf("%s: closed while the pull request is open: %q %d", branch, e, b)
		}
		sha := r.merge(branch)
		r.pass(1060)
		if e, via, by, _ := r.task(2); e != sha || via != "sha" || by != r.bob {
			t.Fatalf("%s: T2 done %q via %q by %q, want the merge commit %s by its holder", branch, e, via, by, sha)
		}
		if e, _, by, _ := r.task(1); e != "merged "+sha || by != db.Gate {
			t.Fatalf("%s: the gate: %q by %q", branch, e, by)
		}
		if res, err := taskresult.Get(r.d, 2); err != nil || res.Commit != sha || !res.Landed || res.Branch != branch {
			t.Fatalf("%s: result %+v %v", branch, res, err)
		}
		if len(r.tell.done) != 2 || r.tell.done[0] != 1 || r.tell.done[1] != 2 {
			t.Fatalf("%s: told of %v", branch, r.tell.done)
		}
		if r.comments() != "" {
			t.Fatalf("%s: a close needs no comment: %s", branch, r.comments())
		}
	}
}

// A gate that fails or runs out of time gives its task back as any gate does, whatever it was asked to do on a pass.
func TestAGateThatDoesNotPassClosesNothing(t *testing.T) {
	r := newLandRig(t, "main")
	r.verdict, r.evidence = watch.Failed, "ci failed: build, lint"
	r.pass(1000)
	if e, _, _, b := r.task(2); e != "" || b != 0 {
		t.Fatalf("T2: %q, %d blockers", e, b)
	}
	if e, _, _, _ := r.task(1); e != "ci failed: build, lint" {
		t.Fatalf("the gate: %q", e)
	}
	r = newLandRig(t, "main")
	r.merge("main")
	r.pass(5000) // past its deadline: not read, so the merge is not seen
	if e, _, _, b := r.task(2); e != "" || b != 0 {
		t.Fatalf("T2 past the deadline: %q, %d blockers", e, b)
	}
	if e, _, _, _ := r.task(1); e != "timed out" {
		t.Fatalf("the gate: %q", e)
	}
}

// The close is a done like any other, so the workspace's review rule holds it: a merge with no review from an agent that did not work
// the task does not close it. The gate opens as passed, its evidence and a comment on the task say why, and the task goes back to its
// holder. With that review, the merge closes it.
func TestTheReviewRuleHoldsAGatesCloseAsItHoldsAnyDone(t *testing.T) {
	r := newLandRig(t, "main")
	shop, _, _ := team.WorkspaceID(r.d, "shop")
	if err := team.SetRule(r.d, shop, team.RuleReviewRequired, true, actor.OwnerKey, 1); err != nil {
		t.Fatal(err)
	}
	sha := r.merge("main")
	r.pass(1000)
	e, _, _, blockers := r.task(2)
	gate, _, _, _ := r.task(1)
	if e != "" || blockers != 0 || !strings.HasPrefix(gate, "merged "+sha+"; T2 not closed: task T2 has commits, and its workspace's rule review_required") {
		t.Fatalf("T2 %q (%d blockers), the gate %q", e, blockers, gate)
	}
	if m := r.comments(); !strings.HasPrefix(m, "daemon: Gate T1 passed (merged "+sha+") and did not close this task: task T2 has commits") {
		t.Fatalf("comment: %s", m)
	}
	if len(r.tell.done) != 1 || r.tell.done[0] != 1 {
		t.Fatalf("told of %v", r.tell.done)
	}

	r = newLandRig(t, "main")
	shop, _, _ = team.WorkspaceID(r.d, "shop")
	if err := team.SetRule(r.d, shop, team.RuleReviewRequired, true, actor.OwnerKey, 1); err != nil {
		t.Fatal(err)
	}
	r.exec("INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(2, 1, ?, ?, 1)", r.rev, r.bob)
	res, err := r.d.Exec("INSERT INTO task_comments(task_id, author, body, created_at) VALUES(2, ?, 'looks right', 1)", r.rev)
	if err != nil {
		t.Fatal(err)
	}
	comment, _ := res.LastInsertId()
	if _, _, ok, err := db.PopTurn(r.d, 2, r.rev, db.TurnPass, "looks right", 0, comment); err != nil || !ok {
		t.Fatal(ok, err)
	}
	sha = r.merge("main")
	r.pass(1000)
	if e, via, by, _ := r.task(2); e != sha || via != "sha" || by != r.bob {
		t.Fatalf("reviewed: T2 done %q via %q by %q", e, via, by)
	}
}

// A done the board refuses is not forced: with a sub-task still open, or another task still blocking it, the merged task stays open,
// the refusal is on the gate and on the task, and the task comes back to its holder once nothing else blocks it.
func TestARefusedCloseGivesTheTaskBackWithTheRefusal(t *testing.T) {
	r := newLandRig(t, "main")
	r.exec("INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, parent_task) VALUES(3, 'a part of it', ?, ?, 1, ?, 2)", r.rev, r.bob, r.bob)
	sha := r.merge("main")
	r.pass(1000)
	e, _, _, blockers := r.task(2)
	gate, _, _, _ := r.task(1)
	if e != "" || blockers != 0 || gate != "merged "+sha+"; T2 not closed: task T2 still has open child tasks - it is done once they are closed:" {
		t.Fatalf("an open sub-task: T2 %q (%d blockers), the gate %q", e, blockers, gate)
	}
	if m := r.comments(); !strings.Contains(m, "T3  queued") || !strings.Contains(m, "hl task done T3") {
		t.Fatalf("the comment names the sub-task and what closes it: %s", m)
	}

	r = newLandRig(t, "main")
	r.exec("INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(4, 'the other half', ?, ?, 1, ?)", r.rev, r.lead, r.lead)
	r.exec("INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(2, 4, 1)")
	sha = r.merge("main")
	r.pass(1000)
	e, _, _, blockers = r.task(2)
	gate, _, _, _ = r.task(1)
	if e != "" || blockers != 1 || gate != "merged "+sha+"; T2 not closed: task T2 is waiting on T4 - it is done only once they are" {
		t.Fatalf("another blocker: T2 %q (%d blockers), the gate %q", e, blockers, gate)
	}
}

// A merge commit that cannot be checked is never taken on the gate's word: while the fetch fails the gate stays open, saying it passed
// and what it waits for, the task stays blocked and open, and nothing is closed in words. Each pass tries again, and so does a daemon
// that starts later with nothing remembered: the open gate is all it needs.
func TestAMergeThatCannotBeCheckedYetIsTriedAgainNotClosedInWords(t *testing.T) {
	r := newLandRig(t, "main")
	sha := r.merge("main")
	r.fetchErr = errors.New("git fetch: fatal: unable to access the remote: Could not resolve host")
	for _, now := range []int64{1000, 1060, 1120, 1180} {
		r.pass(now)
		if e, _, _, b := r.task(2); e != "" || b != 1 {
			t.Fatalf("at %d: T2 %q, %d blockers", now, e, b)
		}
		if e, _, _, _ := r.task(1); e != "" {
			t.Fatalf("at %d: the gate opened: %q", now, e)
		}
	}
	if why := r.watchError(); why != "passed, merged "+sha+"; not closing T2 yet: git fetch: fatal: unable to access the remote: Could not resolve host" {
		t.Fatalf("the gate says %q", why)
	}
	if len(r.tell.done) != 0 || r.comments() != "" {
		t.Fatalf("told of %v, comments %q", r.tell.done, r.comments())
	}
	r.fetchErr = nil
	r.s = r.sweep() // the daemon died and started again
	r.pass(1240)
	if e, via, by, _ := r.task(2); e != sha || via != "sha" || by != r.bob {
		t.Fatalf("after the fetch works: T2 done %q via %q by %q", e, via, by)
	}
	if e, _, _, _ := r.task(1); e != "merged "+sha || r.watchError() != "" {
		t.Fatalf("the gate: %q, still saying %q", e, r.watchError())
	}
}

// A merge into a branch that is not the repository's default is no landing: the fetch works and the commit is not on the default
// branch. After a few looks, in case the host said merged a moment early, the task is given back with that said.
func TestAMergeIntoAnotherBranchGivesTheTaskBack(t *testing.T) {
	r := newLandRig(t, "main")
	sha := r.merge("release")
	for i := range UnlandedReads - 1 {
		r.pass(int64(1000 + 60*i))
		if e, _, _, _ := r.task(1); e != "" || r.watchError() != "passed, merged "+sha+"; not closing T2 yet: "+sha[:7]+" is not on origin/main here" {
			t.Fatalf("look %d: the gate %q saying %q", i+1, e, r.watchError())
		}
	}
	r.pass(2000)
	e, _, _, blockers := r.task(2)
	gate, _, _, _ := r.task(1)
	if e != "" || blockers != 0 || !strings.HasPrefix(gate, "merged "+sha+"; T2 not closed: its merge commit "+sha[:7]+" is not on origin/main of ") {
		t.Fatalf("T2 %q (%d blockers), the gate %q", e, blockers, gate)
	}
}

// The gate closes its task only while that task still waits on it and is an agent's to close: one taken off the gate by hand is its
// holder's again, and one that is the founder's turn is given back, not closed under them. A pass that names no commit closes nothing.
func TestAGateClosesOnlyATaskThatStillWaitsOnItAndIsAnAgents(t *testing.T) {
	r := newLandRig(t, "main")
	r.exec("DELETE FROM task_blocks WHERE task_id = 2")
	sha := r.merge("main")
	r.pass(1000)
	if e, _, _, _ := r.task(2); e != "" || r.comments() != "" {
		t.Fatalf("taken off the gate: T2 %q, comments %q", e, r.comments())
	}
	if e, _, _, _ := r.task(1); e != "merged "+sha {
		t.Fatalf("the gate: %q", e)
	}

	r = newLandRig(t, "main")
	r.exec("INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(2, 1, ?, ?, 1)", actor.OwnerKey, r.bob)
	sha = r.merge("main")
	r.pass(1000)
	if e, _, _, _ := r.task(2); e != "" {
		t.Fatalf("the founder's turn: T2 closed %q", e)
	}
	if e, _, _, _ := r.task(1); e != "merged "+sha+"; T2 not closed: it is not an agent's to close now" {
		t.Fatalf("the gate: %q", e)
	}

	r = newLandRig(t, "main")
	r.merge("main")
	r.evidence = "merged"
	r.pass(1000)
	if e, _, _, _ := r.task(2); e != "" {
		t.Fatalf("no commit named: T2 closed %q", e)
	}
	if e, _, _, _ := r.task(1); e != "merged; T2 not closed: the merge names no commit to close it with" {
		t.Fatalf("the gate: %q", e)
	}
}
