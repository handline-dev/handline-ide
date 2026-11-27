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
	"time"

	"github.com/handline/handline/go/data/db"
)

// NoCIWait is how long a PR with no checks at all is waited on, from its head commit or the watch's start, whichever is later, before it
// is taken to have no CI.
const NoCIWait = 10 * time.Minute

// GitHub watches a pull request's CI (ci), its close (closed) or its landing (landed), or an issue being finished (done), through the gh
// CLI's own login: handline never sees a token. A target is owner/name#N. It is also the one reader of a pull request (ReadPull, Pull,
// BranchPull), for whoever shows one.
type GitHub struct {
	// Run runs gh with args and answers its standard output; nil runs the gh CLI. Now is the clock; nil is time.Now.
	Run func(ctx context.Context, args ...string) ([]byte, error)
	Now func() time.Time
	// AuthWait is how long Unready lets `gh auth status` take; 0 is the package's AuthWait.
	AuthWait time.Duration

	memo // what was last read, by pull request, by branch and for Unready
}

func (*GitHub) Name() string     { return "GitHub" }
func (*GitHub) Untils() []string { return []string{"ci", "closed", "landed", "done"} }

// PullOf is the pull request watch w is about, as its host names it: the repository (owner/name on GitHub, host/path on GitLab) and the
// number. ok is false for a watch on anything else: an issue, or another vendor's target.
func PullOf(w db.Watch) (repo string, number int64, ok bool) {
	if w.Vendor == "gitlab" {
		return splitLab(w.Target)
	}
	repo, n, ok := splitTarget(w.Target)
	if w.Vendor != "github" || w.Until == "done" || !ok {
		return "", 0, false
	}
	number, err := strconv.ParseInt(n, 10, 64)
	return repo, number, err == nil
}

// Why gh cannot be used on this machine (Unready).
const (
	GhMissing   = "gh_missing"
	GhLoggedOut = "gh_logged_out"
)

// AuthWait is how long `gh auth status` may take when a screen asks why gh could not be used (GitHub.AuthWait, when it is not set).
const AuthWait = 5 * time.Second

// loggedOut is gh saying it has no usable login: "You are not logged into any GitHub hosts", or the hint to run gh auth login that
// it prints for a login that no longer works.
var loggedOut = regexp.MustCompile(`(?i)not logged in|gh auth login`)

// authReason reads what `gh auth status` came to: "" when it worked, GhMissing when there is no gh, GhLoggedOut when gh said it has
// no login. Any other failure (the wait ran out, the network, a GitHub that is down) says nothing of the login: ok is false, and
// nobody is told to log in for it.
func authReason(ctx context.Context, err error) (reason string, ok bool) {
	switch {
	case err == nil:
		return "", true
	case errors.Is(err, exec.ErrNotFound):
		return GhMissing, true
	case ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		return "", false
	case loggedOut.MatchString(err.Error()):
		return GhLoggedOut, true
	}
	return "", false
}

// authWords is a reason of authReason in words.
var authWords = map[string]string{GhMissing: "the gh CLI is not installed", GhLoggedOut: "gh is not logged in: run gh auth login"}

func (g *GitHub) Ready(ctx context.Context) (bool, string) {
	_, err := g.gh(ctx, "auth", "status")
	switch reason, ok := authReason(ctx, err); {
	case !ok:
		return false, "gh auth status did not answer, so whether gh is logged in is not known: " + strings.Join(strings.Fields(err.Error()), " ")
	case reason != "":
		return false, authWords[reason]
	}
	return true, ""
}

// Unready is why gh cannot be used on this machine, as a reason (GhMissing or GhLoggedOut) and Ready's words; both "" when it can, and
// when `gh auth status` could not say (it timed out, GitHub is slow or down), which is no reason to tell anyone to log in. It asks with
// a short wait of its own, since it is asked after a read that may have used up its caller's. An answer that says something is kept
// for PullTTL, so a screen that shows many pull requests asks once; one that says nothing is not kept.
func (g *GitHub) Unready() (reason, why string) {
	const key = "\x00ready"
	if k, held := g.held(key); held {
		reason, _ = authReason(context.Background(), k.err)
		return reason, authWords[reason]
	}
	wait := g.AuthWait
	if wait == 0 {
		wait = AuthWait
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	_, err := g.gh(ctx, "auth", "status")
	reason, ok := authReason(ctx, err)
	if !ok {
		return "", ""
	}
	g.keep(key, nil, err)
	return reason, authWords[reason]
}

// Parse takes ci, closed and landed to be about a pull request and done about an issue; GitHub numbers them together, so which one #N
// is shows only when it is read, and a number of the other kind reads as gone.
func (*GitHub) Parse(target, until string) (string, string, error) {
	_, n, ok := splitTarget(target)
	if !ok {
		return "", "", fmt.Errorf("a github target is owner/name#N, not %q", target)
	}
	switch until {
	case "ci":
		return target, "PR #" + n + " CI", nil
	case "closed":
		return target, "PR #" + n + " to close", nil
	case "landed":
		return target, "PR #" + n + " to land", nil
	}
	return target, "issue #" + n + " done", nil
}

func (g *GitHub) Check(ctx context.Context, target, until string, since time.Time) (Verdict, string, error) {
	repo, n, ok := splitTarget(target)
	if !ok {
		return "", "", fmt.Errorf("a github target is owner/name#N, not %q", target)
	}
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
	case until == "landed":
		v, evidence := landedVerdict(p, since)
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

// landedVerdict is a pull request landing, or needing its holder before it can. Merged is passed, with the commit the merge made;
// closed unmerged is failed. While it is open, a check that stands failed by its latest run is failed, the checks named, and so is a
// reviewer asking for changes after the watch started: each is work for whoever holds the task, so the task goes back to them. A
// request for changes made before the watch started is not news to them (they waited knowing it, and GitHub keeps that decision until
// the reviewer looks again), so it waits like a review not yet given, a check still running or a draft. Whether changes are requested
// is the host's own decision (Review); when it says so and no request's time can be seen, it is taken as made now: failed, not waited on.
func landedVerdict(p *Pull, since time.Time) (Verdict, string) {
	if v, evidence := closedVerdict(p); v != Waiting {
		return v, evidence
	}
	if failed := p.Failed(); len(failed) > 0 {
		return Failed, "ci failed: " + strings.Join(failed, ", ")
	}
	if p.Review == ReviewChanges && (p.ChangesAt.IsZero() || p.ChangesAt.After(since)) {
		return Failed, "changes requested"
	}
	return Waiting, ""
}

// Lands is whether watch w passes only on a merge, so that its pass can close a task with the commit that merge made (task.wait then
// done): a pull request or merge request waited on until landed.
func Lands(w db.Watch) bool {
	return (w.Vendor == "github" || w.Vendor == "gitlab") && w.Until == "landed"
}

var mergedSha = regexp.MustCompile(`^merged ([0-9a-f]{7,64})$`)

// MergedSha is the commit the evidence of a pass says the merge made ("merged <sha>"), "" when it names none.
func MergedSha(evidence string) string {
	if m := mergedSha.FindStringSubmatch(evidence); m != nil {
		return m[1]
	}
	return ""
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
	return runCLI(ctx, "gh", args...)
}

// runCLI runs a host's CLI and answers its standard output; a failure says the CLI's own words, and a run stopped at its time limit
// says that, not the signal that ended it.
func runCLI(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second // a helper it started must not hold the read open past its end
	out, err := cmd.Output()
	if err != nil && ctx.Err() != nil {
		return nil, fmt.Errorf("%s %s: %w", name, args[0], ctx.Err())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(exit.Stderr) > 0 {
		return nil, fmt.Errorf("%s %s: %s", name, args[0], strings.TrimSpace(string(exit.Stderr)))
	}
	return out, err
}

func (g *GitHub) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}
