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
