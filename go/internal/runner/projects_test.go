package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

// isolate keeps the user's git config out of a test's git, and names who commits.
func isolate(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "t")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t@t")
	}
}

func gitT(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// origin is a bare repo with one commit on main, and a checkout of it that can push there.
func origin(t *testing.T) (bare, src string) {
	dir := t.TempDir()
	bare, src = filepath.Join(dir, "o.git"), filepath.Join(dir, "src")
	gitT(t, "init", "-q", "--bare", "-b", "main", bare)
	gitT(t, "clone", "-q", bare, src)
	gitT(t, "-C", src, "commit", "-q", "--allow-empty", "-m", "first")
	gitT(t, "-C", src, "push", "-q", "origin", "HEAD:main")
	return bare, src
}

func remoteSha(t *testing.T, bare, branch string) string {
	out, _ := exec.Command("git", "-C", bare, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Output()
	return strings.TrimSpace(string(out))
}

func waitPushed(t *testing.T, bare, branch, sha string) {
	t.Helper()
	for end := time.Now().Add(15 * time.Second); remoteSha(t, bare, branch) != sha; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("%s never reached the remote's %s (it has %q)", sha, branch, remoteSha(t, bare, branch))
		}
	}
}

// A project is cloned once and fetched after, the same answer each time. A worktree made from it has the runner's hooks: a commit
// there runs the project's own post-commit hook and then hl from the worktree's top, and once the agent's hook tells the runner, the
// branch is on the remote: again after the remote was away, and never over a branch that moved on it.
func TestAProjectIsClonedOnceAndEachCommitIsPushed(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	called := filepath.Join(home, "hl-called")
	hl := filepath.Join(home, "hl")
	if err := os.WriteFile(hl, []byte("#!/bin/sh\necho \"$PWD $*\" >> "+called+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var logged []string
	log := func(format string, args ...any) {
		mu.Lock()
		logged = append(logged, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	saw := func(what string) bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(strings.Join(logged, "\n"), what)
	}
	r, err := Start(Config{Home: home, Term: &fakeTerm{}, Tokens: &fakeTokens{kept: map[string]int{}}, Auth: fakeAuth{}, HL: hl, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	bare, src := origin(t)
	for range 2 {
		if got, err := r.serve("runner.project.sync", map[string]any{"remote": bare}); err != nil || got.(map[string]any)["state"] != "cloned" {
			t.Fatalf("sync: %v %v", got, err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(home, "runner-repos", ".*")); len(left) != 0 {
		t.Fatalf("a clone's folder was left: %v", left)
	}

	repo := repoDir(home, bare)
	own := filepath.Join(repo, ".git", "hooks", "post-commit")
	if err := os.WriteFile(own, []byte("#!/bin/sh\ntouch "+filepath.Join(home, "own-ran")+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(home, "wt")
	if _, err := r.serve("runner.worktree.make", map[string]any{"remote": bare, "path": wt, "branch": "bob", "base": "main"}); err != nil {
		t.Fatal(err)
	}
	wt, _ = filepath.EvalSymlinks(wt)
	if _, err := r.serve("runner.terminal.open", map[string]any{"agent": "bob", "folder": wt, "command": "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.serve("runner.respawn", map[string]any{"handle": "%1"}); err != nil { // its folder stays the session's
		t.Fatal(err)
	}
	gitT(t, "-C", wt, "commit", "-q", "--allow-empty", "-m", "work")
	if _, err := os.Stat(filepath.Join(home, "own-ran")); err != nil {
		t.Fatal("the project's own post-commit hook did not run")
	}
	if b, _ := os.ReadFile(called); strings.TrimSpace(string(b)) != wt+" hook git-commit" {
		t.Fatalf("the hook called hl as %q", b)
	}
	if out, err := exec.Command("git", "-C", repo, "config", "--get", "core.hooksPath").Output(); err == nil {
		t.Fatalf("the project's own config was changed: core.hooksPath %s", out)
	}

	commitAs := func(agent, folder string) (any, error) {
		return r.Local().Dispatch(r.Local().AgentCaller(agent), "hl", folder, "runner.committed", map[string]any{"folder": folder})
	}
	commit := func(folder string) (any, error) { return commitAs("bob", folder) }
	if _, err := commit(wt); err != nil {
		t.Fatal(err)
	}
	waitPushed(t, bare, "bob", gitT(t, "-C", wt, "rev-parse", "HEAD"))

	if _, err := commit(src); err == nil || err.(*rpc.Error).Data["gate"] != "folder" {
		t.Fatalf("a folder that is not the agent's session's was pushed: %v", err)
	}
	if _, err := commitAs("ann", wt); err == nil || err.(*rpc.Error).Data["gate"] != "folder" {
		t.Fatalf("another agent pushed bob's worktree: %v", err)
	}

	gitT(t, "-C", wt, "commit", "-q", "--amend", "--allow-empty", "-m", "work, amended")
	if _, err := commit(wt); err != nil {
		t.Fatal(err)
	}
	waitPushed(t, bare, "bob", gitT(t, "-C", wt, "rev-parse", "HEAD"))
	if _, err := r.Local().Dispatch(rpc.Caller{Kind: "person", Key: "p1", Role: "owner"}, "hl", wt, "runner.committed", map[string]any{"folder": wt}); err == nil {
		t.Fatal("a caller that is not an agent pushed")
	}

	away := bare + ".away"
	if err := os.Rename(bare, away); err != nil {
		t.Fatal(err)
	}
	gitT(t, "-C", wt, "commit", "-q", "--allow-empty", "-m", "while away")
	if _, err := commit(wt); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(15 * time.Second); !saw("failed, again"); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("the push did not fail while the remote was away")
		}
	}
	if err := os.Rename(away, bare); err != nil {
		t.Fatal(err)
	}
	waitPushed(t, bare, "bob", gitT(t, "-C", wt, "rev-parse", "HEAD"))

	gitT(t, "-C", src, "fetch", "-q", "origin")
	gitT(t, "-C", src, "commit", "-q", "--allow-empty", "-m", "elsewhere")
	gitT(t, "-C", src, "push", "-q", "-f", "origin", "HEAD:bob")
	moved := remoteSha(t, bare, "bob")
	gitT(t, "-C", wt, "commit", "-q", "--allow-empty", "-m", "behind")
	if _, err := commit(wt); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(15 * time.Second); !saw("was rejected") || r.pushing(wt); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("a rejected push is tried again")
		}
	}
	if remoteSha(t, bare, "bob") != moved {
		t.Fatal("the push went over a branch that moved on the remote")
	}
}

func (r *Runner) pushing(folder string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pushers[folder] != nil
}

// A worktree made with no base named starts from the remote's own default branch, whatever it is called: a remote whose work
// merges into master has no main to start from.
func TestAWorktreeWithNoBaseNamedStartsFromTheRemotesDefaultBranch(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	r, err := Start(Config{Home: home, Term: &fakeTerm{}, Tokens: &fakeTokens{kept: map[string]int{}}, Auth: fakeAuth{}, HL: filepath.Join(home, "hl"),
		Log: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	dir := t.TempDir()
	bare, src := filepath.Join(dir, "o.git"), filepath.Join(dir, "src")
	gitT(t, "init", "-q", "--bare", "-b", "master", bare)
	gitT(t, "init", "-q", "-b", "master", src)
	gitT(t, "-C", src, "commit", "-q", "--allow-empty", "-m", "first")
	gitT(t, "-C", src, "push", "-q", bare, "master")
	wt := filepath.Join(home, "wt")
	if _, err := r.serve("runner.worktree.make", map[string]any{"remote": bare, "path": wt, "branch": "bob"}); err != nil {
		t.Fatalf("no base named, in a master repository: %v", err)
	}
	if got, want := gitT(t, "-C", wt, "rev-parse", "HEAD"), gitT(t, "-C", src, "rev-parse", "HEAD"); got != want {
		t.Fatalf("the worktree is at %s, the remote's master at %s", got, want)
	}
	if _, err := r.serve("runner.worktree.make", map[string]any{"remote": bare, "path": filepath.Join(home, "wt2"), "branch": "eve", "base": "-x"}); err == nil {
		t.Fatal("a base that is an option was taken")
	}
}
