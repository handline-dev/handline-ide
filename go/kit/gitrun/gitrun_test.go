package gitrun

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "x"}} {
		if r := Run(dir, args...); r.RC != 0 {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	return dir
}

func TestRunHandsBackWhatGitSaid(t *testing.T) {
	dir := repo(t)
	if r := Run(dir, "rev-parse", "--abbrev-ref", "HEAD"); r.RC != 0 || strings.TrimSpace(r.Stdout) != "main" {
		t.Fatalf("%+v", r)
	}
	if r := Run(dir, "no-such-command"); r.RC == 0 || r.RC == -1 || r.Stderr == "" {
		t.Fatalf("%+v", r)
	}
	if r := Run(t.TempDir()+"/missing", "status"); r.RC != -1 || r.Stderr == "" {
		t.Fatalf("git could not be run: %+v", r)
	}
	if r := RunTimeout(dir, time.Nanosecond, "status"); r.RC != -1 {
		t.Fatalf("past its limit: %+v", r)
	}
}

// tgit runs git in dir as a test author, and answers what it printed.
func tgit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	r := Run(dir, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
	if r.RC != 0 {
		t.Fatalf("git %v in %s: %+v", args, dir, r)
	}
	return strings.TrimSpace(r.Stdout)
}

// remote is a bare repository whose default branch is def, with one commit on each of branches (def first), and a clone of it.
func remote(t *testing.T, def string, branches ...string) (bare, clone string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	bare, seed, clone := filepath.Join(root, "origin.git"), filepath.Join(root, "seed"), filepath.Join(root, "clone")
	tgit(t, root, "init", "-q", "--bare", "-b", def, bare)
	tgit(t, root, "init", "-q", "-b", def, seed)
	tgit(t, seed, "commit", "-q", "--allow-empty", "-m", "base")
	push := []string{"push", "-q", bare, def}
	for _, b := range branches {
		push = append(push, def+":"+b)
	}
	tgit(t, seed, push...)
	tgit(t, root, "clone", "-q", bare, clone)
	return bare, clone
}

func originHeadOf(dir string) string {
	return strings.TrimSpace(Run(dir, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD").Stdout)
}

// origin/HEAD is the repository's own word for the branch its work merges into, so a launch leaves one that names a branch alone and
// names one only when there is none: a default of develop beside a main stays develop.
func TestOriginHeadIsNamedOnlyWhenItNamesNoBranchOfOrigin(t *testing.T) {
	if NameOriginHead(repo(t)) {
		t.Fatal("named with no origin")
	}
	_, develop := remote(t, "develop", "main")
	if got := originHeadOf(develop); got != "origin/develop" {
		t.Fatalf("a clone's origin/HEAD: %q", got)
	}
	if NameOriginHead(develop) || originHeadOf(develop) != "origin/develop" || DefaultBranch(develop) != "develop" {
		t.Fatalf("origin/HEAD was moved off the default branch: %q, default %q", originHeadOf(develop), DefaultBranch(develop))
	}
	if b := BaseOf(develop); b.Branch != "develop" || fmt.Sprint(b.Refs) != "[origin/develop develop]" {
		t.Fatalf("the base beside a main: %+v", b)
	}

	// a repository made here and pushed has no origin/HEAD: it is named, main as before and master where that is the branch
	for _, def := range []string{"main", "master"} {
		bare, clone := remote(t, def)
		tgit(t, clone, "remote", "set-head", "origin", "--delete")
		if got := originHeadOf(clone); got != "" {
			t.Fatalf("%s: origin/HEAD still set: %q", def, got)
		}
		if !NameOriginHead(clone) || originHeadOf(clone) != "origin/"+def {
			t.Fatalf("%s: an unset origin/HEAD was not named: %q", def, originHeadOf(clone))
		}
		if NameOriginHead(clone) {
			t.Fatalf("%s: named twice", def)
		}

		// the remote drops the branch origin/HEAD names: git keeps the name, which then says nothing
		tgit(t, clone, "push", "-q", "origin", def+":trunk")
		tgit(t, clone, "remote", "set-head", "origin", "trunk")
		tgit(t, bare, "branch", "-D", "trunk")
		tgit(t, clone, "fetch", "-q", "--prune", "origin")
		if got := originHeadOf(clone); got != "origin/trunk" {
			t.Fatalf("%s: the name of a branch that is gone: %q", def, got)
		}
		if b := DefaultBranch(clone); b != def {
			t.Fatalf("%s: the default branch read from an origin/HEAD that names no branch: %q", def, b)
		}
		if !NameOriginHead(clone) || originHeadOf(clone) != "origin/"+def {
			t.Fatalf("%s: an origin/HEAD that names no branch was not named again: %q", def, originHeadOf(clone))
		}
	}
}

// What has landed is on origin's copy of the default branch when the clone has one, whatever the branch is called: a commit merged on
// the remote counts once it is fetched, with the local branch still behind, and one on the local branch alone does not until it is
// pushed. With no origin the local branch decides.
func TestTheBaseIsOriginsCopyOfTheDefaultBranchThenTheLocalOne(t *testing.T) {
	// no origin: the local branch alone
	local := repo(t)
	head := tgit(t, local, "rev-parse", "HEAD")
	b := BaseOf(local)
	if b.Branch != "main" || fmt.Sprint(b.Refs) != "[main]" || b.Ref() != "main" {
		t.Fatalf("a main repository with no origin: %+v", b)
	}
	if ok, why := b.Reaches(local, head); !ok || why != "" {
		t.Fatalf("main's own head: %v %q", ok, why)
	}
	tgit(t, local, "checkout", "-q", "-b", "side")
	tgit(t, local, "commit", "-q", "--allow-empty", "-m", "off")
	if ok, why := b.Reaches(local, "HEAD"); ok || why != "" {
		t.Fatalf("a commit that is not merged: %v %q", ok, why)
	}
	if HasOrigin(local) {
		t.Fatal("an origin where none is named")
	}
	// the branch is asked by its whole name: a tag called main, left behind, does not stand in for it
	tgit(t, local, "checkout", "-q", "main")
	tgit(t, local, "tag", "main", "HEAD")
	tgit(t, local, "commit", "-q", "--allow-empty", "-m", "next")
	if ok, why := b.Reaches(local, "HEAD"); !ok {
		t.Fatalf("a commit on main, with a tag of its name behind it: %q", why)
	}

	bare, clone := remote(t, "master")
	if !HasOrigin(clone) {
		t.Fatal("a clone names no origin")
	}
	b = BaseOf(clone)
	if b.Branch != "master" || fmt.Sprint(b.Refs) != "[origin/master master]" || b.Ref() != "origin/master" {
		t.Fatalf("a clone of master: %+v", b)
	}
	// work made here on a branch, merged on the remote by someone else
	tgit(t, clone, "checkout", "-q", "-b", "work")
	tgit(t, clone, "commit", "-q", "--allow-empty", "-m", "mine")
	mine := tgit(t, clone, "rev-parse", "HEAD")
	tgit(t, clone, "push", "-q", "origin", "work")
	if ok, _ := b.Reaches(clone, mine); ok {
		t.Fatal("a commit on a branch counted as landed")
	}
	if revs, ok := b.NotIn("HEAD"); !ok || tgit(t, clone, append([]string{"rev-list", "--count"}, revs...)...) != "1" {
		t.Fatalf("the commits not landed: %v %v", revs, ok)
	}
	tgit(t, bare, "update-ref", "refs/heads/master", mine)
	if ok, _ := b.Reaches(clone, mine); ok {
		t.Fatal("landed on the remote, but nothing was fetched")
	}
	tgit(t, clone, "fetch", "-q", "origin") // the clone's own fetch: nothing here fetches for it
	if ok, why := b.Reaches(clone, mine); !ok {
		t.Fatalf("merged on origin/master, local master behind: %q", why)
	}
	if Run(clone, "merge-base", "--is-ancestor", mine, "master").RC == 0 {
		t.Fatal("the local master moved: the test proves nothing")
	}
	if revs, _ := b.NotIn("HEAD"); tgit(t, clone, append([]string{"rev-list", "--count"}, revs...)...) != "0" {
		t.Fatal("a commit on origin/master still counts as not landed")
	}
	// a commit on the local branch that is not pushed has not landed: a push moves origin's copy, and that is what is asked
	tgit(t, clone, "checkout", "-q", "master")
	tgit(t, clone, "merge", "-q", "--ff-only", "origin/master")
	tgit(t, clone, "commit", "-q", "--allow-empty", "-m", "local only")
	if ok, why := b.Reaches(clone, "HEAD"); ok || why != "" {
		t.Fatalf("a commit on the local master only: %v %q", ok, why)
	}
	tgit(t, clone, "push", "-q", "origin", "master")
	if ok, why := b.Reaches(clone, "HEAD"); !ok {
		t.Fatalf("pushed to origin's master: %q", why)
	}

	// neither ref: nothing to measure against
	empty := t.TempDir()
	tgit(t, empty, "init", "-q", "-b", "trunk")
	if b := BaseOf(empty); b.Ref() != "" || len(b.Refs) != 0 {
		t.Fatalf("a repository with no commit: %+v", b)
	} else if ok, why := b.Reaches(empty, "HEAD"); ok || why != "this repo has no trunk branch" {
		t.Fatalf("%v %q", ok, why)
	} else if _, ok := b.NotIn("HEAD"); ok {
		t.Fatal("commits measured against no ref")
	}
}

func TestDefaultBranchIsReadFromTheRepository(t *testing.T) {
	git := func(dir string, args ...string) {
		t.Helper()
		if r := Run(dir, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...); r.RC != 0 {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	if b := DefaultBranch(repo(t)); b != "main" {
		t.Fatalf("a main repository: %q", b)
	}
	master := t.TempDir()
	git(master, "init", "-q", "-b", "master")
	if b := DefaultBranch(master); b != "master" {
		t.Fatalf("a master repository, before its first commit: %q", b)
	}
	git(master, "commit", "-q", "--allow-empty", "-m", "x")
	// a clone sits on a feature branch: origin's HEAD still names master
	clone := t.TempDir()
	git(clone, "clone", "-q", master, ".")
	git(clone, "checkout", "-q", "-b", "feature")
	if b := DefaultBranch(clone); b != "master" {
		t.Fatalf("a clone of master on a feature branch: %q", b)
	}
	// no main or master and no origin: the main worktree's branch, also when asked from a linked worktree
	trunk := t.TempDir()
	git(trunk, "init", "-q", "-b", "trunk")
	git(trunk, "commit", "-q", "--allow-empty", "-m", "x")
	linked := filepath.Join(t.TempDir(), "wt")
	git(trunk, "worktree", "add", "-q", "-b", "feature", linked)
	if b := DefaultBranch(linked); b != "trunk" {
		t.Fatalf("a linked worktree of trunk: %q", b)
	}
	// main present beside master, with no origin HEAD to say which
	git(master, "branch", "main")
	if b := DefaultBranch(master); b != "main" {
		t.Fatalf("main beside master: %q", b)
	}
}

// Main is origin's main when there is one: a commit pushed there is on main while the clone's own main is behind, a commit only on a
// branch is not; with no remote the local main decides.
func TestOnMainReadsOriginsMainWhenThereIsOne(t *testing.T) {
	git := func(dir string, args ...string) string {
		t.Helper()
		r := Run(dir, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if r.RC != 0 {
			t.Fatalf("%v: %+v", args, r)
		}
		return strings.TrimSpace(r.Stdout)
	}
	local := repo(t)
	git(local, "checkout", "-q", "-b", "work")
	git(local, "commit", "-q", "--allow-empty", "-m", "w")
	work := git(local, "rev-parse", "HEAD")
	if MainRef(local) != "refs/heads/main" || OnMain(local, work).RC != 1 || OnMain(local, "main").RC != 0 {
		t.Fatalf("no remote: the local main decides")
	}
	bare := t.TempDir()
	git(bare, "init", "-q", "--bare", "-b", "main")
	clone := t.TempDir()
	git(clone, "clone", "-q", bare, ".")
	git(clone, "commit", "-q", "--allow-empty", "-m", "first")
	git(clone, "push", "-q", "origin", "HEAD:main")
	git(clone, "checkout", "-q", "-b", "landing")
	git(clone, "commit", "-q", "--allow-empty", "-m", "landed")
	landed := git(clone, "rev-parse", "HEAD")
	git(clone, "push", "-q", "origin", "HEAD:main")
	git(clone, "commit", "-q", "--allow-empty", "-m", "branch only")
	branchOnly := git(clone, "rev-parse", "HEAD")
	if git(clone, "rev-parse", "main") == landed {
		t.Fatal("the local main moved with the push")
	}
	if MainRef(clone) != "refs/remotes/origin/main" {
		t.Fatalf("with an origin: %q", MainRef(clone))
	}
	if r := OnMain(clone, landed); r.RC != 0 {
		t.Fatalf("pushed to origin's main, local main behind: %+v", r)
	}
	if r := OnMain(clone, branchOnly); r.RC != 1 {
		t.Fatalf("only on a branch: %+v", r)
	}
	// on the clone's own main but never pushed: not on main
	git(clone, "checkout", "-q", "main")
	git(clone, "commit", "-q", "--allow-empty", "-m", "unpushed")
	if r := OnMain(clone, "HEAD"); r.RC != 1 {
		t.Fatalf("on the local main only: %+v", r)
	}
	// asked from a linked worktree, the same
	linked := filepath.Join(t.TempDir(), "wt")
	git(clone, "worktree", "add", "-q", "-b", "other", linked, landed)
	if r := OnMain(linked, landed); r.RC != 0 || MainRef(linked) != "refs/remotes/origin/main" {
		t.Fatalf("from a linked worktree: %+v", r)
	}
	// an origin with no main: the local main decides
	git(local, "remote", "add", "origin", bare)
	if MainRef(local) != "refs/heads/main" {
		t.Fatalf("an origin with no main fetched: %q", MainRef(local))
	}
}

// A task's files are those of the commits naming it on any branch: a file deleted by each of them is deleted, one added again is not, and a
// commit naming another task, or naming this one outside its last parenthesis, is left out.
func TestTaskFilesAreThoseOfTheCommitsNamingTheTask(t *testing.T) {
	dir := repo(t)
	commit := func(msg string, steps ...string) {
		t.Helper()
		for _, s := range steps {
			op, p, _ := strings.Cut(s, " ")
			if op == "rm" {
				Run(dir, "rm", "-q", p)
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, p), []byte(msg), 0o644); err != nil {
				t.Fatal(err)
			}
			Run(dir, "add", p)
		}
		if r := Run(dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", msg); r.RC != 0 {
			t.Fatalf("%s: %+v", msg, r)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	commit("base", "add old.go", "add gone.go", "add back.go")
	Run(dir, "checkout", "-q", "-b", "work")
	commit("one (T7)", "add apps/a.tsx", "rm gone.go", "rm back.go")
	commit("two (T7)", "add back.go")
	commit("other (T8)", "add apps/x.md")
	commit("T7 named but not last (T9)", "add old.go")
	got, err := TaskFiles(dir, 7)
	if err != nil {
		t.Fatal(err)
	}
	want := []TaskFile{{"apps/a.tsx", false}, {"back.go", false}, {"gone.go", true}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("files %v, want %v", got, want)
	}
}
