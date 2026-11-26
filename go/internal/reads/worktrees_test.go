package reads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	asked := 0
	var answer map[string]any
	var fail error
	e.BranchPR = func(_ context.Context, r, branch string) (map[string]any, error) {
		asked++
		if r != "acme/app" || branch != "story/x" {
			t.Errorf("asked about %s %s", r, branch)
		}
		return answer, fail
	}
	mineRow := func() map[string]any {
		t.Helper()
		got, err := e.worktreesList(nil, map[string]any{"owner": alice})
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)["worktrees"].([]any)[0].(map[string]any)
	}
	answer = map[string]any{"number": 7, "url": "https://github.com/acme/app/pull/7", "verdict": "checks_failing", "failing": 2}
	w := mineRow()
	if w["base"] != "main" || w["commits_ahead"] != 1 || w["files_changed"] != 1 || w["size_bytes"] != int64(2048) {
		t.Fatalf("facts: %v", w)
	}
	if pr, _ := w["pr"].(map[string]any); pr["verdict"] != "checks_failing" || pr["failing"] != 2 || pr["number"] != 7 {
		t.Fatalf("pr: %v", w["pr"])
	}
	answer = nil
	if mineRow()["pr"] == nil || asked != 1 {
		t.Fatalf("a minute's answer is kept: asked %d", asked)
	}
	e.branchPRs = newBranchPRs()
	fail = errors.New("gh: not logged in")
	if w := mineRow(); w["pr"] != nil {
		t.Fatalf("gh failing is pr null, never a guess: %v", w["pr"])
	}
}

// recorded is one pull request as `gh pr list --json` printed it, with these checks (each a recorded statusCheckRollup entry).
func recorded(t *testing.T, checks ...string) ghPull {
	t.Helper()
	var p ghPull
	if err := json.Unmarshal([]byte(`{"number":7,"state":"OPEN","statusCheckRollup":[`+strings.Join(checks, ",")+`]}`), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// checkRun is a check run of name that started at a time of day and concluded so ("" while it runs); commitStatus a commit status.
func checkRun(name, conclusion, started string) string {
	state := "COMPLETED"
	if conclusion == "" {
		state = "IN_PROGRESS"
	}
	return `{"__typename":"CheckRun","name":"` + name + `","workflowName":"ci","status":"` + state + `","conclusion":"` + conclusion +
		`","startedAt":"2026-10-20T` + started + `Z"}`
}

func commitStatus(context, state string) string {
	return `{"__typename":"StatusContext","context":"` + context + `","state":"` + state + `"}`
}

func TestAPullRequestsVerdictIsWhatHoldsIt(t *testing.T) {
	t.Parallel()
	failing := func(conclusions ...string) watch.Rollup {
		var checks []string
		for i, c := range conclusions {
			checks = append(checks, checkRun(fmt.Sprintf("check-%d", i), c, "11:00:00"))
		}
		return recorded(t, checks...).Checks
	}
	for _, c := range []struct {
		name string
		pr   ghPull
		want string
	}{
		{"merged", ghPull{State: "MERGED", IsDraft: true}, "merged"},
		{"draft", ghPull{State: "OPEN", IsDraft: true}, "draft"},
		{"checks fail before a review is asked", ghPull{State: "OPEN", ReviewDecision: "CHANGES_REQUESTED", Checks: failing("FAILURE")}, "checks_failing"},
		{"changes", ghPull{State: "OPEN", ReviewDecision: "CHANGES_REQUESTED"}, "changes_requested"},
		{"review required", ghPull{State: "OPEN", ReviewDecision: "REVIEW_REQUIRED"}, "waiting_review"},
		{"approved", ghPull{State: "OPEN", ReviewDecision: "APPROVED"}, "ready"},
		{"no review rule", ghPull{State: "OPEN"}, "ready"},
	} {
		if got := pullVerdict(c.pr)["verdict"]; got != c.want {
			t.Errorf("%s: %v, want %s", c.name, got, c.want)
		}
	}
	got := pullVerdict(ghPull{State: "OPEN", Checks: failing("FAILURE", "TIMED_OUT", "SUCCESS")})
	if got["failing"] != 2 {
		t.Fatalf("failing count: %v", got)
	}
	if _, has := pullVerdict(ghPull{State: "OPEN"})["failing"]; has {
		t.Fatal("failing is only on checks_failing")
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
	e.BranchPR = func(_ context.Context, repo, branch string) (map[string]any, error) {
		t.Errorf("asked about %s %s", repo, branch)
		return nil, nil
	}
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

func TestAStatusContextOrAStartupFailureCountsAsFailing(t *testing.T) {
	t.Parallel()
	pr := recorded(t, commitStatus("deploy", "FAILURE"), commitStatus("lint", "ERROR"), checkRun("build", "STARTUP_FAILURE", "11:00:00"), commitStatus("docs", "SUCCESS"),
		checkRun("go", "SUCCESS", "11:00:00"))
	if got := pullVerdict(pr); got["verdict"] != "checks_failing" || got["failing"] != 3 {
		t.Fatalf("%v", got)
	}
}

// A check that failed and was run again leaves its failed run in what gh prints. A branch's verdict counts each check by its latest
// run, as a gate does: the passing rerun clears it, a rerun still going is not a failure yet, and a rerun that failed is one failure.
func TestARunThatARerunReplacedDoesNotCountAsFailing(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		checks  []string
		verdict string
		failing any
	}{
		{"failed, then passed on a rerun", []string{checkRun("build", "FAILURE", "11:50:00"), checkRun("build", "SUCCESS", "11:55:00"), checkRun("go", "SUCCESS", "11:50:00")}, "ready", nil},
		{"whatever order gh lists them in", []string{checkRun("build", "SUCCESS", "11:55:00"), checkRun("build", "FAILURE", "11:50:00")}, "ready", nil},
		{"the rerun is still going", []string{checkRun("build", "FAILURE", "11:50:00"), checkRun("build", "", "11:55:00")}, "ready", nil},
		{"the rerun failed too", []string{checkRun("build", "FAILURE", "11:50:00"), checkRun("build", "FAILURE", "11:55:00")}, "checks_failing", 1},
		{"a commit status set again", []string{commitStatus("deploy", "FAILURE"), commitStatus("deploy", "SUCCESS")}, "ready", nil},
		{"cancelled and not run again", []string{checkRun("build", "CANCELLED", "11:50:00")}, "checks_failing", 1},
	} {
		got := pullVerdict(recorded(t, c.checks...))
		if got["verdict"] != c.verdict || got["failing"] != c.failing {
			t.Errorf("%s: %v, want %s %v", c.name, got, c.verdict, c.failing)
		}
	}
}

func TestTheOpenPullRequestOfABranchStandsInFrontOfAMergedOne(t *testing.T) {
	t.Parallel()
	pulls := []ghPull{{Number: 3, State: "MERGED"}, {Number: 2, State: "CLOSED"}, {Number: 1, State: "OPEN"}}
	if p, ok := chosenPull(pulls); !ok || p.Number != 1 {
		t.Fatalf("%v %v", p, ok)
	}
	if p, ok := chosenPull(pulls[:2]); !ok || p.Number != 3 {
		t.Fatalf("a merged one when none is open: %v %v", p, ok)
	}
	if _, ok := chosenPull(pulls[1:2]); ok {
		t.Fatal("a closed one is none")
	}
}

func TestABranchesPullRequestIsKeptAMinuteAndThenAskedAgain(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c := newBranchPRs()
	c.now = func() time.Time { return now }
	c.put("k", map[string]any{"number": 1})
	if _, hit := c.get("k"); !hit {
		t.Fatal("kept")
	}
	now = now.Add(branchPRTTL + time.Second)
	if _, hit := c.get("k"); hit {
		t.Fatal("kept past its minute")
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
