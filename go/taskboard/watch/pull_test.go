package watch

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

// viewed is what `gh pr view 418 -R acme/app --json <the fields a read asks for>` prints for an open pull request whose build failed and
// was not run again, whose lint failed and passed on a rerun, with a deploy status still pending and a reviewer asking for changes.
const viewed = `{
  "baseRefName": "main",
  "commits": [
    {"authoredDate": "2026-10-20T11:30:00Z", "committedDate": "2026-10-20T11:30:00Z", "messageBody": "", "messageHeadline": "Add the badge", "oid": "0a1b2c3d4e5f60718293a4b5c6d7e8f901234567"},
    {"authoredDate": "2026-10-20T11:40:00Z", "committedDate": "2026-10-20T11:41:00Z", "messageBody": "", "messageHeadline": "Count unread chats", "oid": "1f0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e"}
  ],
  "headRefOid": "1f0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e",
  "isDraft": false,
  "mergeCommit": null,
  "number": 418,
  "reviewDecision": "CHANGES_REQUESTED",
  "state": "OPEN",
  "statusCheckRollup": [
    {"__typename": "CheckRun", "completedAt": "2026-10-20T11:52:10Z", "conclusion": "FAILURE", "detailsUrl": "https://github.com/acme/app/actions/runs/1/job/1", "name": "build", "startedAt": "2026-10-20T11:50:00Z", "status": "COMPLETED", "workflowName": "ci"},
    {"__typename": "CheckRun", "completedAt": "2026-10-20T11:51:30Z", "conclusion": "FAILURE", "detailsUrl": "https://github.com/acme/app/actions/runs/1/job/2", "name": "lint", "startedAt": "2026-10-20T11:50:00Z", "status": "COMPLETED", "workflowName": "ci"},
    {"__typename": "CheckRun", "completedAt": "2026-10-20T11:56:40Z", "conclusion": "SUCCESS", "detailsUrl": "https://github.com/acme/app/actions/runs/1/job/3", "name": "lint", "startedAt": "2026-10-20T11:55:00Z", "status": "COMPLETED", "workflowName": "ci"},
    {"__typename": "CheckRun", "completedAt": "2026-10-20T11:53:00Z", "conclusion": "SKIPPED", "detailsUrl": "https://github.com/acme/app/actions/runs/1/job/4", "name": "docs", "startedAt": "2026-10-20T11:50:00Z", "status": "COMPLETED", "workflowName": "ci"},
    {"__typename": "StatusContext", "context": "deploy/preview", "startedAt": "2026-10-20T11:50:05Z", "state": "PENDING", "targetUrl": "https://deploy.example/1"}
  ],
  "url": "https://github.com/acme/app/pull/418"
}`

// counting answers every run of gh with what answer says, and counts the runs by their first two words.
type counting struct {
	answer func(args []string) (string, error)
	asked  map[string]int
	now    time.Time
}

func (c *counting) reader() *GitHub {
	c.asked, c.now = map[string]int{}, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	return &GitHub{
		Run: func(_ context.Context, args ...string) ([]byte, error) {
			c.asked[strings.Join(args[:2], " ")]++
			out, err := c.answer(args)
			return []byte(out), err
		},
		Now: func() time.Time { return c.now },
	}
}

// One read takes a pull request's facts from what gh prints, and only those: where it stands, the branch it merges into, its commits,
// the review's decision and each check by its latest run.
func TestAPullRequestIsReadFromWhatGhPrints(t *testing.T) {
	c := &counting{answer: func(args []string) (string, error) { return viewed, nil }}
	g := c.reader()
	var ran []string
	run := g.Run
	g.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		ran = args
		return run(ctx, args...)
	}
	p, err := g.ReadPull(context.Background(), "acme/app", 418)
	if err != nil {
		t.Fatal(err)
	}
	want := &Pull{Repo: "acme/app", Number: 418, URL: "https://github.com/acme/app/pull/418", State: PullOpen, Base: "main",
		HeadSha: "1f0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e", Review: ReviewChanges, HeadAt: time.Date(2026, 10, 20, 11, 41, 0, 0, time.UTC),
		Checks: []Check{{"build", CheckFailed}, {"lint", CheckPassed}, {"docs", CheckPassed}, {"deploy/preview", CheckWaiting}}}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("\n got %+v\nwant %+v", p, want)
	}
	if got := strings.Join(ran, " "); got != "pr view 418 -R acme/app --json number,url,state,isDraft,baseRefName,headRefOid,mergeCommit,reviewDecision,statusCheckRollup,commits" {
		t.Fatalf("ran gh %s", got)
	}
	if !reflect.DeepEqual(p.Failed(), []string{"build"}) || p.Count(CheckWaiting) != 1 || p.Count(CheckPassed) != 2 {
		t.Fatalf("%v %d %d", p.Failed(), p.Count(CheckWaiting), p.Count(CheckPassed))
	}
}

// Pull serves what was read within the last minute and reads again after it; ReadPull always reads, and what it read is what Pull
// then serves. A read that failed is kept the same way, so a gh that is down is asked once a minute.
func TestAReadOfAPullRequestIsKeptAMinute(t *testing.T) {
	var fail error
	c := &counting{answer: func([]string) (string, error) { return viewed, fail }}
	g := c.reader()
	ctx := context.Background()
	for range 3 {
		if p, err := g.Pull(ctx, "acme/app", 418); err != nil || p.Number != 418 {
			t.Fatal(p, err)
		}
	}
	if c.asked["pr view"] != 1 {
		t.Fatalf("asked %d times within a minute", c.asked["pr view"])
	}
	if _, err := g.ReadPull(ctx, "acme/app", 418); err != nil || c.asked["pr view"] != 2 {
		t.Fatalf("ReadPull reads now: %v, asked %d", err, c.asked["pr view"])
	}
	c.now = c.now.Add(PullTTL + time.Second)
	fail = errors.New("gh pr: HTTP 502")
	for range 2 {
		if _, err := g.Pull(ctx, "acme/app", 418); err == nil {
			t.Fatal("a failed read is an error")
		}
	}
	if c.asked["pr view"] != 3 {
		t.Fatalf("a failed read is kept too: asked %d", c.asked["pr view"])
	}
	c.now, fail = c.now.Add(PullTTL+time.Second), nil
	if p, err := g.Pull(ctx, "acme/app", 418); err != nil || p == nil || c.asked["pr view"] != 4 {
		t.Fatalf("%v %v, asked %d", p, err, c.asked["pr view"])
	}
	if _, err := g.Pull(ctx, "acme/app", 419); err != nil || c.asked["pr view"] != 5 {
		t.Fatalf("another pull request is another read: %v, asked %d", err, c.asked["pr view"])
	}
}

// A pull request gh cannot find, or that this login may not see, is ErrGone; anything else gh says is an error of the read.
func TestAPullRequestThatIsNotThereIsGone(t *testing.T) {
	for _, c := range []struct {
		said string
		gone bool
	}{
		{"gh pr: GraphQL: Could not resolve to a PullRequest with the number of 418. (repository.pullRequest)", true},
		{"gh pr: GraphQL: Could not resolve to a Repository with the name 'acme/app'. (repository)", true},
		{"gh pr: HTTP 404: Not Found (https://api.github.com/graphql)", true},
		{"gh pr: HTTP 502: Bad Gateway", false},
		{"gh pr: To get started with GitHub CLI, please run:  gh auth login", false},
	} {
		g := (&counting{answer: func([]string) (string, error) { return "", errors.New(c.said) }}).reader()
		_, err := g.ReadPull(context.Background(), "acme/app", 418)
		if err == nil || errors.Is(err, ErrGone) != c.gone || err.Error() != c.said {
			t.Errorf("%s: %v", c.said, err)
		}
	}
}

// A branch's pull request is the newest open one, else the newest merged one, else none; a closed one is none. The answer is kept a
// minute, none included, and the pull request it found is not asked about again by its number.
func TestABranchsPullRequestIsItsOpenOneElseItsMergedOne(t *testing.T) {
	one := func(number, state string) string {
		return `{"baseRefName":"main","headRefOid":"1f0e9d8c","isDraft":false,"mergeCommit":null,"number":` + number +
			`,"reviewDecision":"","state":"` + state + `","statusCheckRollup":[],"url":"https://github.com/acme/app/pull/` + number + `"}`
	}
	listed := ""
	c := &counting{answer: func(args []string) (string, error) { return "[" + listed + "]", nil }}
	ctx := context.Background()
	for _, k := range []struct {
		name, listed string
		want         int64
	}{
		{"an open one stands in front of a newer merged one", one("3", "MERGED") + "," + one("2", "CLOSED") + "," + one("1", "OPEN"), 1},
		{"a merged one when none is open", one("3", "MERGED") + "," + one("2", "CLOSED"), 3},
		{"a closed one is none", one("2", "CLOSED"), 0},
		{"no pull request", "", 0},
	} {
		listed = k.listed
		g := c.reader()
		var got int64
		for range 2 {
			p, err := g.BranchPull(ctx, "acme/app", "story/x")
			if err != nil {
				t.Fatal(err)
			}
			if got = 0; p != nil {
				got = p.Number
			}
		}
		if got != k.want || c.asked["pr list"] != 1 {
			t.Errorf("%s: #%d, asked %d times", k.name, got, c.asked["pr list"])
		}
		if k.want != 0 {
			if p, err := g.Pull(ctx, "acme/app", k.want); err != nil || p.Number != k.want || c.asked["pr view"] != 0 {
				t.Errorf("%s: the pull request it found is asked about again: %v %v", k.name, p, err)
			}
		}
	}
	if got := c.asked; !reflect.DeepEqual(got, map[string]int{"pr list": 1}) {
		t.Fatalf("%v", got)
	}
}

// Why gh cannot be used on this machine is asked once a minute: not installed, or not logged in.
func TestWhyGhCannotBeUsedIsAskedOnceAMinute(t *testing.T) {
	var fail error
	c := &counting{answer: func([]string) (string, error) { return "", fail }}
	g := c.reader()
	ctx := context.Background()
	if reason, why := g.Unready(ctx); reason != "" || why != "" {
		t.Fatal(reason, why)
	}
	fail = exec.ErrNotFound
	if reason, _ := g.Unready(ctx); reason != "" || c.asked["auth status"] != 1 {
		t.Fatalf("kept a minute: %q, asked %d", reason, c.asked["auth status"])
	}
	c.now = c.now.Add(PullTTL + time.Second)
	if reason, why := g.Unready(ctx); reason != GhMissing || why != "the gh CLI is not installed" {
		t.Fatal(reason, why)
	}
	c.now, fail = c.now.Add(PullTTL+time.Second), errors.New("gh auth: You are not logged into any GitHub hosts.")
	if reason, why := g.Unready(ctx); reason != GhLoggedOut || why != "gh is not logged in: run gh auth login" || c.asked["auth status"] != 3 {
		t.Fatal(reason, why, c.asked)
	}
}
