package reads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/taskboard/watch"
)

// fakeGh stands in for the gh CLI: it prints what was recorded for a branch's `gh pr list` and a pull request's `gh pr view`, fails
// every read with fail, and answers `gh auth status` as a machine whose gh is logged out or (fail exec.ErrNotFound) not installed.
type fakeGh struct {
	t         *testing.T
	lists     map[string]string // "owner/name branch" -> what gh pr list prints
	views     map[string]string // "owner/name#N" -> what gh pr view prints
	fail      error
	loggedOut bool
	asked     map[string]int
}

func (f *fakeGh) run(_ context.Context, args ...string) ([]byte, error) {
	f.t.Helper()
	if f.asked == nil {
		f.asked = map[string]int{}
	}
	what := strings.Join(args[:2], " ")
	f.asked[what]++
	switch what {
	case "auth status":
		if errors.Is(f.fail, exec.ErrNotFound) {
			return nil, f.fail
		}
		if f.loggedOut {
			return nil, errors.New("gh auth: You are not logged into any GitHub hosts.")
		}
		return nil, nil
	case "pr list":
		if f.fail != nil {
			return nil, f.fail
		}
		if out, ok := f.lists[args[3]+" "+args[5]]; ok {
			return []byte(out), nil
		}
		return []byte("[]"), nil
	case "pr view":
		if f.fail != nil {
			return nil, f.fail
		}
		if out, ok := f.views[args[4]+"#"+args[2]]; ok {
			return []byte(out), nil
		}
		return nil, fmt.Errorf("gh pr: GraphQL: Could not resolve to a PullRequest with the number of %s. (repository.pullRequest)", args[2])
	}
	f.t.Errorf("ran gh %v", args)
	return nil, nil
}

// reader is a reader of pull requests over the fake, with nothing kept from an earlier read.
func (f *fakeGh) reader() *watch.GitHub { return &watch.GitHub{Run: f.run} }

// recordedPull is pull request 7 of acme/app as gh prints one, in a state, with these checks; more is further fields as gh names them
// (`"isDraft":true`), which stand over the ones here.
func recordedPull(state, more string, checks ...string) string {
	merge := "null"
	if state == "MERGED" {
		merge = `{"oid":"984c6570a1b2c3d4e5f60718293a4b5c6d7e8f90"}`
	}
	if more != "" {
		more = "," + more
	}
	return `{"baseRefName":"main","commits":[{"committedDate":"2026-10-20T11:41:00Z","messageHeadline":"Add the badge","oid":"1f0e9d8c7b6a"}],` +
		`"headRefOid":"1f0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e","isDraft":false,"mergeCommit":` + merge + `,"number":7,"reviewDecision":"",` +
		`"state":"` + state + `","statusCheckRollup":[` + strings.Join(checks, ",") + `],"url":"https://github.com/acme/app/pull/7"` + more + `}`
}

// checkRun is a check run of name that started at a time of day and concluded so ("" while it runs); commitStatus a commit status.
func checkRun(name, conclusion, started string) string {
	state := "COMPLETED"
	if conclusion == "" {
		state = "IN_PROGRESS"
	}
	return `{"__typename":"CheckRun","completedAt":"2026-10-20T11:58:00Z","conclusion":"` + conclusion +
		`","detailsUrl":"https://github.com/acme/app/actions/runs/1/job/2","name":"` + name + `","startedAt":"2026-10-20T` + started +
		`Z","status":"` + state + `","workflowName":"ci"}`
}

func commitStatus(context, state string) string {
	return `{"__typename":"StatusContext","context":"` + context + `","startedAt":"2026-10-20T11:50:05Z","state":"` + state +
		`","targetUrl":"https://deploy.example/1"}`
}

// verdictOf is worktrees.list's `pr` for what gh printed of pull request 7.
func verdictOf(t *testing.T, printed string) map[string]any {
	t.Helper()
	gh := &fakeGh{t: t, views: map[string]string{"acme/app#7": printed}}
	p, err := gh.reader().Pull(context.Background(), "acme/app", 7)
	if err != nil {
		t.Fatal(err)
	}
	return pullVerdict(p)
}

func TestAPullRequestsVerdictIsWhatHoldsIt(t *testing.T) {
	t.Parallel()
	failed := checkRun("build", "FAILURE", "11:50:00")
	for _, c := range []struct{ name, printed, want string }{
		{"merged", recordedPull("MERGED", `"isDraft":true`), "merged"},
		{"draft", recordedPull("OPEN", `"isDraft":true`, failed), "draft"},
		{"checks fail before a review is asked", recordedPull("OPEN", `"reviewDecision":"CHANGES_REQUESTED"`, failed), "checks_failing"},
		{"changes", recordedPull("OPEN", `"reviewDecision":"CHANGES_REQUESTED"`), "changes_requested"},
		{"review required", recordedPull("OPEN", `"reviewDecision":"REVIEW_REQUIRED"`), "waiting_review"},
		{"approved", recordedPull("OPEN", `"reviewDecision":"APPROVED"`), "ready"},
		{"no review rule", recordedPull("OPEN", ""), "ready"},
	} {
		if got := verdictOf(t, c.printed)["verdict"]; got != c.want {
			t.Errorf("%s: %v, want %s", c.name, got, c.want)
		}
	}
	got := verdictOf(t, recordedPull("OPEN", "", failed, checkRun("lint", "TIMED_OUT", "11:50:00"), checkRun("go", "SUCCESS", "11:50:00")))
	if got["failing"] != 2 || got["number"] != int64(7) || got["url"] != "https://github.com/acme/app/pull/7" {
		t.Fatalf("failing count: %v", got)
	}
	if _, has := verdictOf(t, recordedPull("OPEN", ""))["failing"]; has {
		t.Fatal("failing is only on checks_failing")
	}
}

func TestAStatusContextOrAStartupFailureCountsAsFailing(t *testing.T) {
	t.Parallel()
	got := verdictOf(t, recordedPull("OPEN", "", commitStatus("deploy", "FAILURE"), commitStatus("lint", "ERROR"),
		checkRun("build", "STARTUP_FAILURE", "11:00:00"), commitStatus("docs", "SUCCESS"), checkRun("go", "SUCCESS", "11:00:00")))
	if got["verdict"] != "checks_failing" || got["failing"] != 3 {
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
		got := verdictOf(t, recordedPull("OPEN", "", c.checks...))
		if got["verdict"] != c.verdict || got["failing"] != c.failing {
			t.Errorf("%s: %v, want %s %v", c.name, got, c.verdict, c.failing)
		}
	}
}

// taskPRRead is task.pr of task id.
func taskPRRead(t *testing.T, e *Env, id int64) map[string]any {
	t.Helper()
	got, err := e.taskPR(nil, map[string]any{"task": json.Number(itoa(id))})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := got.(map[string]any)
	return m
}

// A task's card reads its pull request as GitHub has it now: where it stands, the checks that failed by their latest run, the review's
// decision and the commits; a task that names none has no card, and one that is not there is not_found.
func TestATasksPullRequestIsReadAsGitHubHasItNow(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr, pr_repo) VALUES(1, 'ship', 'bob', 'p1', 1, 'p1', 7, 'acme/app')")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(2, 'no pull request', 'bob', 'p1', 1, 'p1')")
	gh := &fakeGh{t: t, views: map[string]string{"acme/app#7": recordedPull("OPEN", `"reviewDecision":"CHANGES_REQUESTED"`,
		checkRun("build", "FAILURE", "11:50:00"), checkRun("lint", "FAILURE", "11:50:00"), checkRun("lint", "SUCCESS", "11:55:00"),
		checkRun("go", "SUCCESS", "11:50:00"), commitStatus("deploy", "PENDING"))}}
	e.GitHub = gh.reader()
	want := map[string]any{"number": int64(7), "repo": "acme/app", "url": "https://github.com/acme/app/pull/7", "state": "open", "draft": false,
		"base": "main", "review": "changes_requested", "failing": []string{"build"}, "waiting": 1, "passing": 2,
		"head_sha": "1f0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e", "merge_sha": nil, "closes_task": false, "unavailable": nil, "host": "GitHub"}
	if got := taskPRRead(t, e, 1); !reflect.DeepEqual(got, want) {
		t.Fatalf("\n got %v\nwant %v", got, want)
	}
	// its merge closes the task while the task waits on an open gate asked to close it, and not after the gate opened or the wait was undone
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at, watch_vendor, watch_target, watch_until, watch_label, "+
		"watch_closes) VALUES(5, 'T1 waits on PR #7 to land, then is done', 'gate', 'bob', 1, 'bob', 2, 'github', 'acme/app#7', 'landed', 'PR #7 to land', 1)")
	if taskPRRead(t, e, 1)["closes_task"] != false {
		t.Fatal("a gate the task does not wait on closes nothing")
	}
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(1, 5, 1)")
	if taskPRRead(t, e, 1)["closes_task"] != true {
		t.Fatal("the task waits on a gate that closes it")
	}
	run(t, e, "UPDATE tasks SET done_evidence = 'ci failed: build', done_at = 3, watch_result = 'failed' WHERE id = 5")
	if taskPRRead(t, e, 1)["closes_task"] != false {
		t.Fatal("a gate that opened closes nothing any more")
	}
	taskPRRead(t, e, 1)
	if gh.asked["pr view"] != 1 {
		t.Fatalf("a read is kept a minute: gh was asked %d times", gh.asked["pr view"])
	}
	gh.views["acme/app#7"], e.GitHub = recordedPull("MERGED", `"reviewDecision":"APPROVED"`, checkRun("build", "SUCCESS", "11:50:00")), gh.reader()
	if got := taskPRRead(t, e, 1); got["state"] != "merged" || got["merge_sha"] != "984c6570a1b2c3d4e5f60718293a4b5c6d7e8f90" || got["review"] != "approved" ||
		len(got["failing"].([]string)) != 0 || got["passing"] != 1 {
		t.Fatalf("merged: %v", got)
	}
	if got, err := e.taskPR(nil, map[string]any{"task": json.Number("2")}); err != nil || got != nil {
		t.Fatalf("a task with no pull request: %v %v", got, err)
	}
	if _, err := e.taskPR(nil, map[string]any{"task": json.Number("8")}); err == nil || !strings.Contains(err.Error(), "no task T8") {
		t.Fatalf("%v", err)
	}
	if _, err := e.taskPR(nil, map[string]any{}); err == nil {
		t.Fatal("task is required")
	}
}

// What the card cannot read it says, never an empty card: gh not installed or logged out on this machine before anything else, since a
// person can fix that; a pull request this login cannot see; any other failure in gh's own words. The link out stays.
func TestATasksPullRequestThatCannotBeReadSaysWhy(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr, pr_repo) VALUES(1, 'ship', 'bob', 'p1', 1, 'p1', 8, 'acme/app')")
	gh := &fakeGh{t: t}
	for _, c := range []struct {
		name      string
		fail      error
		loggedOut bool
		reason    string
		text      string
	}{
		{"no such pull request", nil, false, "gone", "this machine's gh login cannot see it"},
		{"a failed read", errors.New("gh pr: HTTP 502\n  Bad Gateway"), false, "failed", "gh pr: HTTP 502 Bad Gateway"},
		{"logged out", errors.New("gh pr: To get started with GitHub CLI, please run: gh auth login"), true, "gh_logged_out", "gh is not logged in"},
		{"not installed", exec.ErrNotFound, false, "gh_missing", "the gh CLI is not installed"},
	} {
		gh.fail, gh.loggedOut, e.GitHub = c.fail, c.loggedOut, gh.reader()
		got := taskPRRead(t, e, 1)
		why, _ := got["unavailable"].(map[string]any)
		if why["reason"] != c.reason || !strings.Contains(fmt.Sprint(why["text"]), c.text) || got["state"] != nil ||
			got["url"] != "https://github.com/acme/app/pull/8" {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

// A pull request named by its number alone is read in the repository its holder works in, as that folder's origin names it; with no
// such folder on this machine, or an origin that is not on GitHub, the card says the repository is not known and asks no one.
func TestABareNumberIsReadInTheRepositoryItsHolderWorksIn(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	git := gitIn(t)
	repo := filepath.Join(root, "repo")
	os.MkdirAll(repo, 0o755)
	git(repo, "init", "-q", "-b", "main")
	git(repo, "remote", "add", "origin", "https://github.com/acme/app.git")
	addAgent(t, e, "alice", "")
	addAgent(t, e, "bob", "")
	teamtest.Set(t, e.Home, "alice", "repo", repo)
	alice, bob := teamtest.Key(t, e.Home, "alice"), teamtest.Key(t, e.Home, "bob")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr) VALUES(1, 'ship', '"+alice+"', 'p1', 1, 'p1', 7)")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr) VALUES(2, 'ship', '"+bob+"', 'p1', 1, 'p1', 7)")
	gh := &fakeGh{t: t, views: map[string]string{"acme/app#7": recordedPull("OPEN", "")}}
	e.GitHub = gh.reader()
	if got := taskPRRead(t, e, 1); got["repo"] != "acme/app" || got["state"] != "open" || got["unavailable"] != nil {
		t.Fatalf("%v", got)
	}
	got := taskPRRead(t, e, 2)
	why, _ := got["unavailable"].(map[string]any)
	if why["reason"] != "no_repo" || !strings.Contains(fmt.Sprint(why["text"]), "hl task edit T2 --pr 7 --repo owner/name") || got["repo"] != nil ||
		got["url"] != nil || gh.asked["pr view"] != 1 {
		t.Fatalf("%v (gh asked %v)", got, gh.asked)
	}
}

// One read of a pull request serves everyone: what a gate just read is what the card shows, with no second call, and a gate reads
// afresh each time whatever a card read before it.
func TestAGatesReadOfAPullRequestServesTheCard(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr, pr_repo) VALUES(1, 'ship', 'bob', 'p1', 1, 'p1', 7, 'acme/app')")
	gh := &fakeGh{t: t, views: map[string]string{"acme/app#7": recordedPull("OPEN", "", checkRun("build", "FAILURE", "11:50:00"))}}
	e.GitHub = gh.reader()
	since := time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC)
	if v, evidence, err := e.GitHub.Check(context.Background(), "acme/app#7", "ci", since); err != nil || v != watch.Failed || evidence != "ci failed: build" {
		t.Fatalf("%s %q %v", v, evidence, err)
	}
	if got := taskPRRead(t, e, 1); !reflect.DeepEqual(got["failing"], []string{"build"}) || gh.asked["pr view"] != 1 {
		t.Fatalf("%v, gh asked %d times", got["failing"], gh.asked["pr view"])
	}
	if _, _, err := e.GitHub.Check(context.Background(), "acme/app#7", "ci", since); err != nil || gh.asked["pr view"] != 2 {
		t.Fatalf("a gate reads afresh: %v, gh asked %d times", err, gh.asked["pr view"])
	}
}

// A pull request in a repository on another host is read by that host's reader: a GitLab host glab is logged in to reads as a merge
// request, its page and host named; a host no reader reads says so plainly, with no read and no link, and the branch shows no chip.
func TestAPullRequestOnAnotherHostIsReadByItsHostOrSaidUnread(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	git := gitIn(t)
	addAgent(t, e, "alice", "")
	for i, origin := range []string{"git@gitlab.com:group/app.git", "https://git.corp.example/team/app.git"} {
		repo := filepath.Join(root, fmt.Sprint(i))
		os.MkdirAll(repo, 0o755)
		git(repo, "init", "-q", "-b", "main")
		git(repo, "remote", "add", "origin", origin)
		if i == 0 {
			teamtest.Set(t, e.Home, "alice", "repo", repo)
		}
	}
	alice := teamtest.Key(t, e.Home, "alice")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr) VALUES(1, 'ship', '"+alice+"', 'p1', 1, 'p1', 4)")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr, pr_repo) VALUES(2, 'ship', 'bob', 'p1', 1, 'p1', 4, 'git.corp.example/team/app')")
	var asked []string
	e.GitLab = &watch.GitLab{Run: func(_ context.Context, args ...string) ([]byte, error) {
		asked = append(asked, strings.Join(args, " "))
		switch {
		case args[0] == "auth" && args[3] == "gitlab.com":
			return nil, nil
		case args[0] == "auth":
			return nil, errors.New("glab auth: not logged in")
		case strings.HasSuffix(args[3], "/approvals"):
			return []byte(`{"approvals_left":1,"approved_by":[]}`), nil
		}
		return []byte(`{"iid":4,"state":"opened","target_branch":"main","sha":"abc","detailed_merge_status":"not_approved",` +
			`"web_url":"https://gitlab.com/group/app/-/merge_requests/4","head_pipeline":{"status":"failed"}}`), nil
	}}
	got := taskPRRead(t, e, 1)
	if got["repo"] != "gitlab.com/group/app" || got["host"] != "GitLab" || got["state"] != "open" || got["review"] != "review_required" ||
		!reflect.DeepEqual(got["failing"], []string{"pipeline"}) || got["url"] != "https://gitlab.com/group/app/-/merge_requests/4" || got["unavailable"] != nil {
		t.Fatalf("%v", got)
	}
	got = taskPRRead(t, e, 2)
	why, _ := got["unavailable"].(map[string]any)
	if why["reason"] != "unread_host" || why["text"] != "Pull requests on git.corp.example are not read yet; this repository lands by push." ||
		got["host"] != "git.corp.example" || got["url"] != nil || got["state"] != nil {
		t.Fatalf("%v", got)
	}
	for _, a := range asked {
		if strings.HasPrefix(a, "api --hostname git.corp.example") {
			t.Fatalf("an unread host is not asked: %q", asked)
		}
	}
}
