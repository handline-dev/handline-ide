package watch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/db"
)

// NoCIWait is how long a PR with no checks at all is waited on, from its head commit or the watch's start, whichever is later, before it
// is taken to have no CI.
const NoCIWait = 10 * time.Minute

// GitHub watches a pull request's CI (ci) or its close (closed), or an issue being finished (done), through the gh CLI's own login: handline
// never sees a token. A target is owner/name#N. It is also the one reader of a pull request (ReadPull, Pull, BranchPull), for whoever
// shows one.
type GitHub struct {
	// Run runs gh with args and answers its standard output; nil runs the gh CLI. Now is the clock; nil is time.Now.
	Run func(ctx context.Context, args ...string) ([]byte, error)
	Now func() time.Time

	mu   sync.Mutex
	kept map[string]kept // what was last read, by pull request, by branch and for Unready
}

var githubTarget = regexp.MustCompile(`^([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)#([1-9][0-9]*)$`)

func (*GitHub) Untils() []string { return []string{"ci", "closed", "done"} }

// PullOf is the pull request watch w is about, as its host names it: the repository (owner/name) and the number. ok is false for a watch
// on anything else: an issue, or another vendor's target.
func PullOf(w db.Watch) (repo string, number int64, ok bool) {
	m := githubTarget.FindStringSubmatch(w.Target)
	if w.Vendor != "github" || w.Until == "done" || m == nil {
		return "", 0, false
	}
	n, err := strconv.ParseInt(m[2], 10, 64)
	return m[1], n, err == nil
}

// Why gh cannot be used on this machine (Unready).
const (
	GhMissing   = "gh_missing"
	GhLoggedOut = "gh_logged_out"
)

func (g *GitHub) Ready(ctx context.Context) (bool, string) {
	if _, err := g.gh(ctx, "auth", "status"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return false, "the gh CLI is not installed"
		}
		return false, "gh is not logged in: run gh auth login"
	}
	return true, ""
}

// Unready is why gh cannot be used on this machine, as a reason (GhMissing or GhLoggedOut) and Ready's words; both "" when it can. The
// answer is kept for PullTTL, so a screen that shows many pull requests asks once.
func (g *GitHub) Unready(ctx context.Context) (reason, why string) {
	const key = "\x00ready"
	k, ok := g.held(key)
	if !ok {
		_, err := g.gh(ctx, "auth", "status")
		g.keep(key, nil, err)
		k.err = err
	}
	switch {
	case k.err == nil:
		return "", ""
	case errors.Is(k.err, exec.ErrNotFound):
		return GhMissing, "the gh CLI is not installed"
	}
	return GhLoggedOut, "gh is not logged in: run gh auth login"
}

// Parse takes ci and closed to be about a pull request and done about an issue; GitHub numbers them together, so which one #N is shows
// only when it is read, and a number of the other kind reads as gone.
func (*GitHub) Parse(target, until string) (string, string, error) {
	m := githubTarget.FindStringSubmatch(target)
	if m == nil {
		return "", "", fmt.Errorf("a github target is owner/name#N, not %q", target)
	}
	switch until {
	case "ci":
		return target, "PR #" + m[2] + " CI", nil
	case "closed":
		return target, "PR #" + m[2] + " to close", nil
	}
	return target, "issue #" + m[2] + " done", nil
}

func (g *GitHub) Check(ctx context.Context, target, until string, since time.Time) (Verdict, string, error) {
	m := githubTarget.FindStringSubmatch(target)
	if m == nil {
		return "", "", fmt.Errorf("a github target is owner/name#N, not %q", target)
	}
	repo, n := m[1], m[2]
	if until == "done" {
		out, err := g.gh(ctx, "issue", "view", n, "-R", repo, "--json", "state,stateReason")
		if err != nil {
			if gone(err) {
				return Gone, err.Error(), nil
			}
			return "", "", err
		}
		return doneVerdict(out)
	}
	number, _ := strconv.ParseInt(n, 10, 64)
	p, err := g.ReadPull(ctx, repo, number) // a gate reads afresh, and leaves what it read for a card or a branch's verdict
	switch {
	case errors.Is(err, ErrGone):
		return Gone, err.Error(), nil
	case err != nil:
		return "", "", err
	case until == "ci":
		v, evidence := ciVerdict(p, since, g.now())
		return v, evidence, nil
	}
	v, evidence := closedVerdict(p)
	return v, evidence, nil
}

type ghCheck struct {
	Typename string `json:"__typename"`
	Name     string
	// WorkflowName tells apart checks of one name in different workflows; a rerun stays in its workflow.
	WorkflowName string `json:"workflowName"`
	Context      string
	Status       string
	Conclusion   string
	State        string
	StartedAt    string
}

// waiting is a check that has not finished: a check run not COMPLETED, or a commit status PENDING or EXPECTED.
func (c ghCheck) waiting() bool {
	if c.Typename == "StatusContext" {
		return c.State == "PENDING" || c.State == "EXPECTED"
	}
	return c.Status != "COMPLETED"
}

// started orders runs of one check. A run still waiting that has no real start time yet (queued, or just rerun) is the newest of all.
func (c ghCheck) started() string {
	if c.waiting() && (c.StartedAt == "" || strings.HasPrefix(c.StartedAt, "0001-")) {
		return "\xff"
	}
	return c.StartedAt
}

// name is what people call the check: a check run's name, a commit status's context.
func (c ghCheck) name() string {
	if c.Typename == "StatusContext" {
		return c.Context
	}
	return c.Name
}

// ok is a finished check that holds nothing up: a check run that concluded SUCCESS, NEUTRAL or SKIPPED, a commit status SUCCESS.
func (c ghCheck) ok() bool {
	if c.Typename == "StatusContext" {
		return c.State == "SUCCESS"
	}
	return c.Conclusion == "SUCCESS" || c.Conclusion == "NEUTRAL" || c.Conclusion == "SKIPPED"
}

// Rollup is a pull request's checks as gh gives them (statusCheckRollup): every run of every check, a rerun beside the run it replaced.
type Rollup []ghCheck

// Checks is where each check stands, in the order the checks first appear: by its latest run, waiting until that has finished, then
// passed when it is ok and failed when it is not. A run that a rerun replaced says nothing. Every reader of a pull request's checks
// reads them through this, so a gate and a branch's verdict cannot disagree.
func (r Rollup) Checks() []Check {
	var out []Check
	for _, c := range latest(r) {
		state := CheckFailed
		switch {
		case c.waiting():
			state = CheckWaiting
		case c.ok():
			state = CheckPassed
		}
		out = append(out, Check{Name: c.name(), State: state})
	}
	return out
}

// latest keeps one entry per check, the one that started last (a later entry on a tie), in the order the checks first appear. A check that was
// cancelled or failed and run again leaves its old run in the rollup; only the newest says where the check stands.
func latest(rollup Rollup) []ghCheck {
	var out []ghCheck
	at := map[string]int{}
	for _, c := range rollup {
		key := c.Typename + "\x00" + c.WorkflowName + "\x00" + c.Name + c.Context
		i, seen := at[key]
		switch {
		case !seen:
			at[key] = len(out)
			out = append(out, c)
		case c.started() >= out[i].started():
			out[i] = c
		}
	}
	return out
}

// ciVerdict is a pull request's CI. A check run is waiting until it completes and ok when it concludes SUCCESS, NEUTRAL or SKIPPED; a
// commit status is waiting while PENDING or EXPECTED and ok when SUCCESS. Only the latest run of each check counts. Every check ok is
// passed and any other end is failed, once none waits. A PR that closes while its CI still waits is failed: that CI will not finish. A
// PR with no checks NoCIWait after its head commit and the watch's start has no CI, and is passed.
func ciVerdict(p *Pull, since, now time.Time) (Verdict, string) {
	closed := p.State != PullOpen
	if len(p.Checks) == 0 {
		switch {
		case closed:
			return Failed, "closed before CI"
		case !p.HeadAt.IsZero():
			if p.HeadAt.After(since) {
				since = p.HeadAt
			}
			if now.Sub(since) >= NoCIWait {
				return Passed, "no CI on this PR"
			}
		}
		return Waiting, ""
	}
	if p.Count(CheckWaiting) > 0 {
		if closed {
			return Failed, "closed before CI"
		}
		return Waiting, ""
	}
	if failed := p.Failed(); len(failed) > 0 {
		return Failed, "ci failed: " + strings.Join(failed, ", ")
	}
	return Passed, "ci passed"
}

// merged is the evidence of a merge: the commit it made, when the host names it.
func merged(p *Pull) string {
	if p.MergeSha == "" {
		return "merged"
	}
	return "merged " + p.MergeSha
}

func closedVerdict(p *Pull) (Verdict, string) {
	switch p.State {
	case PullMerged:
		return Passed, merged(p)
	case PullClosed:
		return Failed, "closed unmerged"
	}
	return Waiting, ""
}

// doneVerdict is an issue closed as completed (passed) or for any other reason, such as not planned (failed).
func doneVerdict(out []byte) (Verdict, string, error) {
	var is struct{ State, StateReason string }
	if err := json.Unmarshal(out, &is); err != nil {
		return "", "", err
	}
	if is.State != "CLOSED" {
		return Waiting, "", nil
	}
	if is.StateReason == "COMPLETED" || is.StateReason == "" {
		return Passed, "closed as completed", nil
	}
	return Failed, "closed as " + strings.ToLower(strings.ReplaceAll(is.StateReason, "_", " ")), nil
}

// gone is a gh error that says the repo or the number is not there, or not to be seen by this login: GitHub answers both the same.
func gone(err error) bool {
	s := err.Error()
	return strings.Contains(s, "Could not resolve to a") || strings.Contains(s, "HTTP 404")
}

func (g *GitHub) gh(ctx context.Context, args ...string) ([]byte, error) {
	if g.Run != nil {
		return g.Run(ctx, args...)
	}
	out, err := exec.CommandContext(ctx, "gh", args...).Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(exit.Stderr) > 0 {
		return nil, fmt.Errorf("gh %s: %s", args[0], strings.TrimSpace(string(exit.Stderr)))
	}
	return out, err
}

func (g *GitHub) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}
