package taskwrites

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/taskboard/taskresult"
)

// baseRig is a home whose agent w holds tasks 1 to 6, each expecting a sha: done marks one done as w with evidence in repo, and
// result is what that done kept.
func baseRig(t *testing.T) (done func(task int, evidence, repo, more string) map[string]any, result func(task int) taskresult.Result) {
	t.Helper()
	done, result, _ = baseRigDirect(t)
	return done, result
}

// baseRigDirect is baseRig with direct, which hands in to MarkDone as w in a transaction of its own, as a caller other than task.done does.
func baseRigDirect(t *testing.T) (done func(task int, evidence, repo, more string) map[string]any, result func(task int) taskresult.Result, direct func(in DoneIn) error) {
	t.Helper()
	d, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	home := t.TempDir()
	teamtest.Seed(t, home, "w", "boss")
	for id := 1; id <= 6; id++ {
		if _, err := d.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at, expects) VALUES(?, 'needs sha', 'w', 'boss', 1, 'sha')", id); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeTeller{}
	done = func(task int, evidence, repo, more string) map[string]any {
		return call(t, writesServer("w", d, home, f), fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.done","params":{"task":%d,"request_id":"b%d-%s","evidence":%q,"repo":%q%s}}`,
			task, task, evidence, evidence, repo, more))[0]
	}
	result = func(task int) taskresult.Result {
		r, err := taskresult.Get(d, int64(task))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	direct = func(in DoneIn) error {
		t.Helper()
		tx, err := d.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := MarkDone(tx, home, "w", in); err != nil {
			tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return done, result, direct
}

// originAndClone is a bare repository whose default branch is def, holding one commit on it and on each of beside, and a clone of it.
func originAndClone(t *testing.T, def string, beside ...string) (bare, clone string) {
	t.Helper()
	root := t.TempDir()
	bare, clone = filepath.Join(root, "origin.git"), filepath.Join(root, "clone")
	seed := filepath.Join(root, "seed")
	git := gitIn(t, root)
	git("init", "-q", "--bare", "-b", def, bare)
	git("init", "-q", "-b", def, seed)
	gitIn(t, seed)("commit", "-q", "--allow-empty", "-m", "base")
	push := []string{"push", "-q", bare, def}
	for _, b := range beside {
		push = append(push, def+":"+b)
	}
	gitIn(t, seed)(push...)
	git("clone", "-q", bare, clone)
	return bare, clone
}

// A sha is checked against the repository's own default branch, whatever it is called: master where that is the only branch, and
// develop where the remote says so with a main beside it. The refusal names that branch.
func TestDoneTakesAShaOnTheRepositorysDefaultBranchWhateverItsName(t *testing.T) {
	t.Parallel()
	done, result := baseRig(t)
	accepted := func(why string, r map[string]any) {
		t.Helper()
		if r["result"] == nil {
			t.Fatalf("%s: refused: %v", why, r)
		}
	}

	master := t.TempDir()
	git := gitIn(t, master)
	git("init", "-q", "-b", "master")
	git("commit", "-q", "--allow-empty", "-m", "on master")
	on := git("rev-parse", "HEAD")
	git("checkout", "-q", "-b", "side")
	git("commit", "-q", "--allow-empty", "-m", "off master")
	off := git("rev-parse", "HEAD")
	if got := errText(done(1, off, master, "")); got != off+" is not an ancestor of master" {
		t.Fatalf("a sha off master: %q", got)
	}
	accepted("a sha on master", done(1, on, master, ""))
	if got := result(1); got.Commit != on || !got.Landed || got.Branch != "master" {
		t.Fatalf("a sha on master: %+v", got)
	}
	// the commit a done names beside its words is marked by the same branch
	accepted("words with a commit on master", done(2, "built", master, fmt.Sprintf(`,"commit":%q`, on)))
	accepted("words with a commit off master", done(3, "built", master, fmt.Sprintf(`,"commit":%q`, off)))
	if result(2).Commit != on || !result(2).Landed || result(3).Commit != off || result(3).Landed || result(3).Branch != "master" {
		t.Fatalf("on master %+v, off master %+v", result(2), result(3))
	}

	_, develop := originAndClone(t, "develop", "main")
	git = gitIn(t, develop)
	git("commit", "-q", "--allow-empty", "-m", "on develop")
	onDevelop := git("rev-parse", "HEAD")
	// on the clone's own develop and not pushed: origin's develop is what is asked, and the refusal says so
	if got := errText(done(4, onDevelop, develop, "")); got != onDevelop+" is not an ancestor of develop (checked origin/develop, which a push there moves)"+fetchHint {
		t.Fatalf("a sha on the local develop only: %q", got)
	}
	git("push", "-q", "origin", "develop")
	git("checkout", "-q", "-b", "hotfix", "origin/main")
	git("commit", "-q", "--allow-empty", "-m", "on main only")
	onMainOnly := git("rev-parse", "HEAD")
	git("push", "-q", "origin", "HEAD:main")
	if got := errText(done(4, onMainOnly, develop, "")); got != onMainOnly+" is not an ancestor of develop (checked origin/develop, which a push there moves)"+fetchHint {
		t.Fatalf("a sha on main, where work merges into develop: %q", got)
	}
	accepted("a sha on develop", done(4, onDevelop, develop, ""))
}

// fetchHint ends the refusal of a sha in a repository that has an origin: the done fetches nothing, so it says who can.
const fetchHint = "; if it was merged on origin, run `git fetch origin` and try again"

// checkedMaster is what the refusal says of a sha origin's master does not reach.
const checkedMaster = " is not an ancestor of master (checked origin/master, which a push there moves)" + fetchHint

// A sha merged on the remote is on origin's copy of the default branch and not on the local branch, which nothing moves. The done
// reads origin's copy as the clone last heard it, from its own fetch or push, and fetches nothing itself: a merge the clone has not
// fetched is refused, and the refusal says to fetch.
func TestDoneTakesAShaOnOriginsCopyAsTheCloneKnowsItAndFetchesNothing(t *testing.T) {
	t.Parallel()
	done, result := baseRig(t)
	bare, clone := originAndClone(t, "master")
	git := gitIn(t, clone)
	work := func(name string) string {
		t.Helper()
		git("checkout", "-q", "-b", name, "origin/master")
		git("commit", "-q", "--allow-empty", "-m", name)
		git("push", "-q", "origin", name)
		return git("rev-parse", "HEAD")
	}
	behind := func(sha string) {
		t.Helper()
		if gitIn(t, clone)("rev-list", "--count", "master.."+sha) == "0" {
			t.Fatal("the local master has the commit: the test proves nothing")
		}
	}

	// merged on the remote by someone else: refused until the clone fetches, then taken
	first := work("first")
	gitIn(t, bare)("update-ref", "refs/heads/master", first)
	if got := errText(done(1, first, clone, "")); got != first+checkedMaster {
		t.Fatalf("merged on the remote, not fetched here: %q", got)
	}
	if got := git("rev-parse", "origin/master"); got == first {
		t.Fatal("the done fetched")
	}
	if result(1).Commit != "" {
		t.Fatal("the refusal kept a commit")
	}
	git("fetch", "-q", "origin")
	if r := done(1, first, clone, ""); r["result"] == nil {
		t.Fatalf("merged on the remote and fetched: %v", r)
	}
	behind(first)
	if got := result(1); got.Commit != first || !got.Landed {
		t.Fatalf("%+v", got)
	}

	// landed by the clone's own push, which moves origin's copy with no fetch
	git("checkout", "-q", "-b", "second", "origin/master")
	git("commit", "-q", "--allow-empty", "-m", "second")
	second := git("rev-parse", "HEAD")
	git("push", "-q", "origin", "HEAD:master")
	if r := done(2, second, clone, ""); r["result"] == nil {
		t.Fatalf("pushed to origin's master from here: %v", r)
	}
	behind(second)

	// a sha the clone does not hold at all, as a squash made on the remote is, gets the same hint
	if got := errText(done(3, "abcdef1234", clone, "")); got != "abcdef1234 is not an ancestor of master (no such commit in this repo)"+fetchHint {
		t.Fatalf("a sha the clone has not got: %q", got)
	}
	// never merged anywhere: refused, the hint still there since the done cannot tell
	third := work("third")
	if got := errText(done(3, third, clone, "")); got != third+checkedMaster {
		t.Fatalf("never merged: %q", got)
	}
}

// What git says of a done's sha and commit is asked before the write and carried into it (DoneIn.Looked): the transaction runs no git,
// shown here by a repository that is gone by the time of the write. A caller that did not ask first has MarkDone ask, as it always
// did, and an answer is never used for a sha it was not asked about.
func TestADonesShasAreLookedUpBeforeTheWriteAndCarriedIntoIt(t *testing.T) {
	t.Parallel()
	_, result, direct := baseRigDirect(t)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git := gitIn(t, repo)
	git("init", "-q", "-b", "master")
	git("commit", "-q", "--allow-empty", "-m", "on master")
	on := git("rev-parse", "HEAD")
	git("checkout", "-q", "-b", "side")
	git("commit", "-q", "--allow-empty", "-m", "off master")
	off := git("rev-parse", "HEAD")
	refused := func(why string, err error, want string) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: %v, want %q", why, err, want)
		}
	}

	// not looked up first: MarkDone asks git itself, with the same answers
	refused("a sha off master, not looked up first", direct(DoneIn{Task: 1, Sha: off, Repo: repo}), off+" is not an ancestor of master")
	if err := direct(DoneIn{Task: 1, Sha: on[:12], Repo: repo}); err != nil || result(1).Commit != on || !result(1).Landed {
		t.Fatalf("a sha on master, not looked up first: %v %+v", err, result(1))
	}

	// looked up first: the write needs no repository
	sha := DoneIn{Task: 2, Sha: on[:12], Repo: repo}.Looked()
	commit := DoneIn{Task: 3, Words: "built", Commit: off[:12], Repo: repo}.Looked()
	late := DoneIn{Task: 4, Sha: off, Repo: repo}.Looked()
	other := DoneIn{Task: 5, Sha: on, Repo: repo}.Looked()
	if err := os.Rename(repo, repo+".gone"); err != nil {
		t.Fatal(err)
	}
	if err := direct(sha); err != nil || result(2).Commit != on || !result(2).Landed {
		t.Fatalf("a sha looked up before the write: %v %+v", err, result(2))
	}
	if err := direct(commit); err != nil || result(3).Commit != off || result(3).Landed {
		t.Fatalf("a commit looked up before the write: %v %+v", err, result(3))
	}
	refused("a refusal looked up before the write", direct(late), off+" is not an ancestor of master")
	// the answer was for another sha: it is asked again, of a repository that is no longer there and so names no branch but the guess
	other.Sha = off
	refused("an answer for another sha", direct(other), off+" is not an ancestor of main (")
	if result(5).Commit != "" {
		t.Fatal("a sha was taken on the answer to another")
	}
}
