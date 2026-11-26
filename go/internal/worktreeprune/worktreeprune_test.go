package worktreeprune

import (
	"github.com/handline/handline/go/kit/store"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/gitrun"
)

// repo is a repository with main and one worktree per case, named for it.
func repo(t *testing.T) (string, map[string]string) {
	t.Helper()
	return repoOn(t, "main")
}

// repoOn is repo with branch for main.
func repoOn(t *testing.T, branch string) (string, map[string]string) {
	t.Helper()
	root := Real(t.TempDir())
	main := filepath.Join(root, "main")
	git := func(dir string, args ...string) {
		t.Helper()
		if r := gitrun.Run(dir, args...); r.RC != 0 {
			t.Fatalf("git %v: %s", args, r.Stderr)
		}
	}
	os.MkdirAll(main, 0o755)
	git(main, "init", "-q", "-b", branch)
	git(main, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "start")
	wts := map[string]string{}
	for _, name := range []string{"merged", "ahead", "untracked", "locked", "agents"} {
		wts[name] = filepath.Join(root, name)
		git(main, "worktree", "add", "-q", "-b", name, wts[name])
	}
	git(wts["ahead"], "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "unmerged work")
	os.WriteFile(filepath.Join(wts["untracked"], "new.txt"), []byte("x"), 0o644)
	return main, wts
}

func TestEachWorktreeIsKeptForItsReasonAndOnlyADoneOneGoes(t *testing.T) {
	main, wts := repo(t)
	gitrun.Run(main, "worktree", "lock", wts["locked"])
	agents := map[string]bool{Real(wts["agents"]): true}
	got := map[string]string{}
	for _, wt := range List(main) {
		got[filepath.Base(wt.Path)] = WhyKept(wt, agents, main)
	}
	for name, want := range map[string]string{
		"main":      "this is run from it",
		"merged":    "",
		"ahead":     "ahead is not merged into main",
		"untracked": "uncommitted or untracked files",
		"locked":    "locked",
		"agents":    "an agent works in it",
	} {
		if got[name] != want {
			t.Errorf("%s: %q, want %q", name, got[name], want)
		}
	}
	if ok, why := Remove(wts["merged"]); !ok {
		t.Fatal(why)
	}
	if _, err := os.Stat(wts["merged"]); !os.IsNotExist(err) {
		t.Fatal("the worktree is still there")
	}
	if ok, _ := Remove(wts["untracked"]); ok {
		t.Fatal("a worktree with an untracked file was removed")
	}
}

func TestHoldsWorkIsTheTwoReasonsAPersonMustJudge(t *testing.T) {
	for why, want := range map[string]bool{"ahead is not merged into main": true, "uncommitted or untracked files": true, "locked": false, "an agent works in it": false, "main": false, "": false} {
		if HoldsWork(why) != want {
			t.Errorf("%q: %v", why, !want)
		}
	}
}

func TestRescueCommitsWhatIsLeftOnABranchThenRemovesTheWorktree(t *testing.T) {
	main, wts := repo(t)
	day := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	branch, err := Rescue(wts["untracked"], "ann", day)
	if err != nil || branch != "rescue/ann-2026-10-26" {
		t.Fatalf("%q %v", branch, err)
	}
	if _, err := os.Stat(wts["untracked"]); !os.IsNotExist(err) {
		t.Fatal("the worktree is still there")
	}
	if r := gitrun.Run(main, "show", branch+":new.txt"); r.RC != 0 || r.Stdout != "x" {
		t.Fatalf("the file is not on the rescue branch: %+v", r)
	}
	// A second rescue the same day takes the next name.
	gitrun.Run(main, "worktree", "add", "-q", "-b", "again", filepath.Join(filepath.Dir(main), "again"))
	if b, err := Rescue(filepath.Join(filepath.Dir(main), "again"), "ann", day); err != nil || b != "rescue/ann-2026-10-26-2" {
		t.Fatalf("%q %v", b, err)
	}
}

func TestRemoveForceTakesADirtyWorktreeAndKeepsItsBranch(t *testing.T) {
	main, wts := repo(t)
	if ok, why := RemoveForce(wts["untracked"]); !ok {
		t.Fatal(why)
	}
	if r := gitrun.Run(main, "rev-parse", "--verify", "untracked"); r.RC != 0 {
		t.Fatal("the branch went with it")
	}
}

// A repository whose default branch is not main is measured against that branch, as origin's HEAD names it.
func TestUnsavedCountsAgainstTheRepositorysDefaultBranch(t *testing.T) {
	main, wts := repoOn(t, "trunk")
	gitrun.Run(main, "update-ref", "refs/remotes/origin/trunk", "trunk")
	gitrun.Run(main, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	if b := gitrun.DefaultBranch(wts["ahead"]); b != "trunk" {
		t.Fatalf("default branch %q", b)
	}
	for name, want := range map[string][3]any{"merged": {true, 0, 0}, "ahead": {false, 0, 1}} {
		merged, files, commits := Unsaved(wts[name])
		if got := [3]any{merged, files, commits}; got != want {
			t.Errorf("%s: merged, files, commits %v, want %v", name, got, want)
		}
	}
}

// In a repository whose only branch beside the work is master, a worktree merged into master is let go and one ahead of it is kept,
// named against master: nothing there is "not merged into main" for want of a main.
func TestAMasterRepositorysWorktreesAreJudgedAgainstMaster(t *testing.T) {
	main, wts := repoOn(t, "master")
	got := map[string]Worktree{}
	for _, wt := range List(main) {
		got[filepath.Base(wt.Path)] = wt
	}
	for name, want := range map[string]string{
		"main":      "master", // the default branch's own checkout stays, whatever the branch is called
		"merged":    "",
		"ahead":     "ahead is not merged into master",
		"untracked": "uncommitted or untracked files",
	} {
		if why := WhyKept(got[name], nil, ""); why != want {
			t.Errorf("%s: %q, want %q", name, why, want)
		}
	}
	if !HoldsWork("ahead is not merged into master") || HoldsWork("master") {
		t.Fatal("work is held by a branch not merged, whatever it is not merged into, and not by the default branch's checkout")
	}
	if merged, files, commits := Unsaved(wts["ahead"]); merged || files != 0 || commits != 1 {
		t.Fatalf("ahead of master: merged, files, commits %v %d %d", merged, files, commits)
	}
	gitrun.Run(wts["ahead"], "checkout", "-q", "--detach")
	if why := DetachedWork(wts["ahead"]); why != "HEAD is not merged into master" {
		t.Fatalf("a detached HEAD ahead of master: %q", why)
	}
	gitrun.Run(wts["merged"], "checkout", "-q", "--detach")
	if why := DetachedWork(wts["merged"]); why != "" {
		t.Fatalf("a detached HEAD on master: %q", why)
	}
	if ok, why := Remove(wts["merged"]); !ok {
		t.Fatal(why)
	}
	if DeleteMergedBranch(main, "master") || gitrun.Run(main, "rev-parse", "-q", "--verify", "refs/heads/master").RC != 0 {
		t.Fatal("the default branch was deleted")
	}
	if !DeleteMergedBranch(main, "merged") {
		t.Fatal("a branch merged into master was not deleted")
	}
}

// A branch merged on the remote is on origin's copy of the default branch while the local branch stays behind, since nothing moves
// it: that worktree is done with too, and counts no commit ahead.
func TestAWorktreeMergedOnOriginIsDoneWithWhileTheLocalBranchIsBehind(t *testing.T) {
	for _, branch := range []string{"main", "master"} {
		main, wts := repoOn(t, branch)
		gitrun.Run(main, "update-ref", "refs/remotes/origin/"+branch, "refs/heads/ahead")
		if gitrun.Run(main, "merge-base", "--is-ancestor", "ahead", branch).RC == 0 {
			t.Fatal("the local branch has the work: the test proves nothing")
		}
		wt, _ := Find(List(main), wts["ahead"])
		if why := WhyKept(wt, nil, ""); why != "" {
			t.Errorf("%s: merged on origin/%s, kept: %q", branch, branch, why)
		}
		if merged, files, commits := Unsaved(wts["ahead"]); !merged || files != 0 || commits != 0 {
			t.Errorf("%s: merged, files, commits %v %d %d", branch, merged, files, commits)
		}
		// and a commit on the local branch alone, which no push has moved to origin's copy, is not merged yet
		gitrun.Run(main, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "not pushed")
		gitrun.Run(wts["merged"], "merge", "-q", "--ff-only", branch)
		merged, _ := Find(List(main), wts["merged"])
		if why := WhyKept(merged, nil, ""); why != "merged is not merged into "+branch {
			t.Errorf("%s: on the local %s only: %q", branch, branch, why)
		}
	}
}

// Size is bytes, at least what the files hold.
func TestSizeIsTheBytesUnderThePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), make([]byte, 64<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := Size(dir); err != nil || n < 64<<10 || n > 1<<20 {
		t.Fatalf("size %d %v, want 64 KiB or a little more", n, err)
	}
}

func TestEveryDeskIsKeptWhoeverIsSkipped(t *testing.T) {
	d, err := store.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	desk := t.TempDir()
	d.Exec("INSERT INTO worktree_folders(path, repo, last_used) VALUES(?, '/r', 1)", desk)
	for _, skip := range []string{"", "a1"} {
		if got, err := AgentWorktrees(d, skip); err != nil || !got[Real(desk)] {
			t.Fatalf("skip %q: %v %v", skip, got, err)
		}
	}
}
