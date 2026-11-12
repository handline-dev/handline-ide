package gitrun

import (
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

func TestPointOriginHeadAtMainNeedsAnOriginMain(t *testing.T) {
	if PointOriginHeadAtMain(repo(t)) {
		t.Fatal("changed with no origin/main")
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
