package reads

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/protocol/operations"
	"github.com/handline/handline/go/taskboard/watch"
)

func TestAWorktreeLabelStandsInForTheDaemonsWord(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "alice", "")
	wt, _ := filepath.EvalSymlinks(t.TempDir())
	if _, err := e.DB.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES(?, 'idle', 0, ?)", teamtest.Key(t, e.Home, "alice"), wt); err != nil {
		t.Fatal(err)
	}
	status := func() string {
		got, err := e.worktreesList(nil, map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)["worktrees"].([]any)[0].(map[string]any)["status"].(string)
	}
	if s := status(); s != "clean" {
		t.Fatalf("no label: %q", s)
	}
	key, ok, err := e.LabelKey(wt)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if _, err := e.DB.Exec("INSERT INTO meta(name, value, updated) VALUES(?, 'in review', 0)", key); err != nil {
		t.Fatal(err)
	}
	if s := status(); s != "in review" {
		t.Fatalf("labelled: %q", s)
	}
}

func TestARunningOperationNobodyRunsIsReportedFailed(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if _, err := e.DB.Exec("INSERT INTO operations(id, kind, state, steps_done, agent, created_at, updated_at) VALUES('gone', 'agent.create', 'running', 0, 'x', 0, 0), ('here', 'agent.create', 'running', 0, 'y', 0, 0)"); err != nil {
		t.Fatal(err)
	}
	operations.Begin("here")
	defer operations.End("here")
	for id, want := range map[string]string{"gone": "failed", "here": "running"} {
		got, err := e.operationGet(nil, map[string]any{"operation": id})
		if err != nil {
			t.Fatal(err)
		}
		if s := got.(map[string]any)["state"]; s != want {
			t.Fatalf("%s: %v, want %s", id, s, want)
		}
	}
}

func TestTheDiffOfAWholeWorktreeIncludesUntrackedFilesButNotIgnoredOnes(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	prAgent(t, e, "w", "")
	dir, git := agentGit(t, e, "w")
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	diff := func(file any) string {
		t.Helper()
		got, err := e.worktreeDiff(nil, map[string]any{"path": dir, "file": file})
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)["diff"].(string)
	}
	write(".gitignore", "*.log\n")
	write("a.txt", "one\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	write("b.txt", "brand new\n")
	write("noise.log", "ignored\n")
	got := diff(nil)
	if !strings.Contains(got, "+++ b/b.txt") || !strings.Contains(got, "+brand new") || strings.Contains(got, "noise.log") {
		t.Fatalf("the only change is one untracked file:\n%s", got)
	}
	write("a.txt", "two\n")
	if got = diff(nil); !strings.Contains(got, "+two") || !strings.Contains(got, "+++ b/b.txt") {
		t.Fatalf("a tracked change and an untracked file:\n%s", got)
	}
	if one := diff("b.txt"); !strings.Contains(got, one) {
		t.Fatalf("the per-file diff of the untracked file is part of the whole:\n%s", one)
	}
	write("dump.txt", strings.Repeat("x\n", maxDiffChars))
	got = diff(nil)
	if !strings.Contains(got, "(new file, 400000 bytes: not shown)") || strings.Contains(got, "+x\n+x") || !strings.Contains(got, "+++ b/b.txt") {
		t.Fatalf("an untracked file over the cap is listed, not diffed:\n%.300s", got)
	}
}

// A path with letters above ASCII is written as it is in the diff's own lines, tracked or not: git's default puts it in quotes with
// octal escapes, and a reader of the headers then misses the file.
func TestTheDiffWritesANonASCIIPathAsItIs(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	prAgent(t, e, "wq", "")
	dir, git := agentGit(t, e, "wq")
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("café.txt", "one\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	write("café.txt", "two\n")
	write("naïve file.txt", "new\n")
	for _, file := range []any{nil, "café.txt", "naïve file.txt"} {
		got, err := e.worktreeDiff(nil, map[string]any{"path": dir, "file": file})
		if err != nil {
			t.Fatal(err)
		}
		d := got.(map[string]any)["diff"].(string)
		if strings.Contains(d, "\\3") || strings.Contains(d, `"a/`) {
			t.Fatalf("file %v: a path is quoted:\n%s", file, d)
		}
		if file != "naïve file.txt" && !strings.Contains(d, "diff --git a/café.txt b/café.txt\n") {
			t.Fatalf("file %v: the tracked file's header:\n%s", file, d)
		}
		if file != "café.txt" && !strings.Contains(d, "+++ b/naïve file.txt") {
			t.Fatalf("file %v: the untracked file's header:\n%s", file, d)
		}
	}
}

// Every worktree, an agent's too, carries its base, commits not on it, changed files, measured size and the pull request of its branch;
// a gh that cannot answer gives pr null, and the answer is kept a minute.
func TestEveryWorktreeCarriesItsFactsAndItsPullRequest(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	repo, mine := filepath.Join(root, "repo"), filepath.Join(root, "mine")
	git := gitIn(t)
	os.MkdirAll(repo, 0o755)
	git(repo, "init", "-q", "-b", "main")
	git(repo, "remote", "add", "origin", "git@github.com:acme/app.git")
	git(repo, "commit", "-q", "--allow-empty", "-m", "start")
	git(repo, "worktree", "add", "-q", "-b", "story/x", mine)
	git(mine, "commit", "-q", "--allow-empty", "-m", "one")
	os.WriteFile(filepath.Join(mine, "new.txt"), []byte("x"), 0o644)
	addAgent(t, e, "alice", "")
	teamtest.Set(t, e.Home, "alice", "repo", repo)
	alice := teamtest.Key(t, e.Home, "alice")
	if _, err := e.DB.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES('"+alice+"', 'idle', 0, ?)", mine); err != nil {
		t.Fatal(err)
	}
	e.DB.Exec("INSERT INTO meta(name, value, updated) VALUES(?, '2048', 0)", "worktree_size:"+mine)
	gh := &fakeGh{t: t, lists: map[string]string{"acme/app story/x": `[` + recordedPull("OPEN", "",
		checkRun("build", "FAILURE", "11:50:00"), checkRun("lint", "TIMED_OUT", "11:50:00"), checkRun("go", "SUCCESS", "11:50:00")) + `]`}}
	e.GitHub = gh.reader()
	list := func() (row, unavailable map[string]any) {
		t.Helper()
		got, err := e.worktreesList(nil, map[string]any{"owner": alice})
		if err != nil {
			t.Fatal(err)
		}
		res := got.(map[string]any)
		unavailable, _ = res["prs_unavailable"].(map[string]any)
		return res["worktrees"].([]any)[0].(map[string]any), unavailable
	}
	w, unavailable := list()
	if w["base"] != "main" || w["commits_ahead"] != 1 || w["files_changed"] != 1 || w["size_bytes"] != int64(2048) {
		t.Fatalf("facts: %v", w)
	}
	if pr, _ := w["pr"].(map[string]any); pr["verdict"] != "checks_failing" || pr["failing"] != 2 || pr["number"] != int64(7) ||
		pr["url"] != "https://github.com/acme/app/pull/7" || unavailable != nil {
		t.Fatalf("pr: %v %v", w["pr"], unavailable)
	}
	gh.lists = nil
	if w, _ := list(); w["pr"] == nil || gh.asked["pr list"] != 1 {
		t.Fatalf("a minute's answer is kept: asked %v", gh.asked)
	}
	// gh cannot answer: pr null, never a guess, and the read says why when it is this machine's gh that is not set up
	gh.fail, e.GitHub = errors.New("gh pr: HTTP 502"), gh.reader()
	if w, unavailable := list(); w["pr"] != nil || unavailable != nil {
		t.Fatalf("a read that failed with gh set up: %v %v", w["pr"], unavailable)
	}
	gh.loggedOut, e.GitHub = true, gh.reader()
	if w, unavailable := list(); w["pr"] != nil || unavailable["reason"] != "gh_logged_out" || unavailable["text"] != "gh is not logged in: run gh auth login" {
		t.Fatalf("logged out: %v %v", w["pr"], unavailable)
	}
	gh.fail, e.GitHub = exec.ErrNotFound, gh.reader()
	if _, unavailable := list(); unavailable["reason"] != "gh_missing" {
		t.Fatalf("no gh: %v", unavailable)
	}
}

// gitIn runs git in a directory as a test author.
func gitIn(t *testing.T) func(dir string, args ...string) {
	return func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}

// The size is read under the worktree's real path, as housekeeping keeps it, though the agent records a path through a link.
// A worktree on its base, a detached one and one whose origin is not on GitHub are never asked about.
func TestWorktreesWithNoPullRequestToAskAboutAreNeverAsked(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	e.GitHub = &watch.GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
		t.Errorf("ran gh %v", args)
		return []byte("[]"), nil
	}}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	git := gitIn(t)
	repo, other, detached := filepath.Join(root, "repo"), filepath.Join(root, "other"), filepath.Join(root, "detached")
	for _, r := range []struct{ dir, origin string }{{repo, "git@github.com:acme/app.git"}, {other, "git@gitlab.com:acme/app.git"}} {
		os.MkdirAll(r.dir, 0o755)
		git(r.dir, "init", "-q", "-b", "main")
		git(r.dir, "remote", "add", "origin", r.origin)
		git(r.dir, "commit", "-q", "--allow-empty", "-m", "start")
	}
	git(repo, "worktree", "add", "-q", "--detach", detached)
	git(other, "checkout", "-q", "-b", "feature")
	link := filepath.Join(root, "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	for agent, path := range map[string]string{"onbase": link + "/", "detached": detached, "gitlab": other} {
		addAgent(t, e, agent, "")
		if _, err := e.DB.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES(?, 'idle', 0, ?)", teamtest.Key(t, e.Home, agent), path); err != nil {
			t.Fatal(err)
		}
	}
	e.DB.Exec("INSERT INTO meta(name, value, updated) VALUES(?, '4096', 0)", "worktree_size:"+repo)
	got, err := e.worktreesList(nil, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	rows := got.(map[string]any)["worktrees"].([]any)
	if len(rows) != 3 {
		t.Fatalf("%d rows", len(rows))
	}
	for _, r := range rows {
		w := r.(map[string]any)
		if w["pr"] != nil {
			t.Errorf("%v: pr %v", w["branch"], w["pr"])
		}
		if w["owner"] == "onbase" && w["size_bytes"] != int64(4096) {
			t.Errorf("a size kept under the real path is read for a linked one: %v", w["size_bytes"])
		}
	}
}

func TestTheDiffAgainstBaseShowsTheBranchsCommitsAndTheUncommittedWork(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	prAgent(t, e, "wb", "")
	row, _ := queryOne(e.DB, "SELECT worktree FROM agent_state WHERE agent = ?", teamtest.Key(t, e.Home, "wb"))
	dir := row.str("worktree")
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt")
	git("add", ".")
	git("commit", "-q", "-m", "base")
	base := git("rev-parse", "--abbrev-ref", "HEAD")
	git("checkout", "-q", "-b", "feat")
	write("committed.txt")
	git("add", ".")
	git("commit", "-q", "-m", "on the branch")
	write("loose.txt")
	diff := func(p map[string]any) map[string]any {
		t.Helper()
		p["path"] = dir
		got, err := e.worktreeDiff(nil, p)
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)
	}
	head := diff(map[string]any{})
	if d := head["diff"].(string); strings.Contains(d, "committed.txt") || !strings.Contains(d, "loose.txt") || head["against"] != "head" {
		t.Fatalf("absent against is the uncommitted work only:\n%v", head)
	}
	got := diff(map[string]any{"against": "base"})
	d := got["diff"].(string)
	if !strings.Contains(d, "committed.txt") || !strings.Contains(d, "loose.txt") || got["against"] != "base" || got["base_ref"] != base {
		t.Fatalf("against base: the branch's commits and the loose file, base_ref %q:\n%v", base, got)
	}
	if _, err := e.worktreeDiff(nil, map[string]any{"path": dir, "against": "main~1"}); err == nil {
		t.Fatal("another value is bad_request")
	}
}

// agentGit is the worktree of agent name (prAgent's) and a git that runs in it, failing the test on an error.
func agentGit(t *testing.T, e *Env, name string) (string, func(args ...string)) {
	t.Helper()
	row, _ := queryOne(e.DB, "SELECT worktree FROM agent_state WHERE agent = ?", teamtest.Key(t, e.Home, name))
	dir := row.str("worktree")
	return dir, func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}
