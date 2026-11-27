package watch

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"
)

// PullTTL is how long one read of a pull request serves the readers that can wait: a task's card and a branch's verdict. A gate reads
// afresh on each of its passes and leaves what it read for them, so one pull request is never asked about twice in the same minute
// on their account. A read gh could not answer is kept the same way: a gh that is down is asked once a minute, not once a read.
const PullTTL = time.Minute

// Where a pull request, its review and each of its checks stand.
const (
	PullOpen, PullMerged, PullClosed              = "open", "merged", "closed"
	ReviewApproved, ReviewChanges, ReviewRequired = "approved", "changes_requested", "review_required"
	CheckPassed, CheckFailed, CheckWaiting        = "passed", "failed", "waiting"
)

// ErrGone is a pull request the host says is not there, or that this machine's login may not see: the host answers both the same.
var ErrGone = errors.New("gone")

type goneError struct{ why string }

func (e goneError) Error() string   { return e.why }
func (e goneError) Is(t error) bool { return t == ErrGone }

// Check is where one check of a pull request stands, by its latest run: a run that a rerun replaced is not in it.
type Check struct{ Name, State string }

// Pull is one pull request as its host has it now. It is the one read of a pull request: a gate's verdict, a branch's verdict in
// worktrees.list and a task's card are all made from it, so they cannot disagree, and whatever follows a pull request later reads the
// same. Only these fields are taken from what gh prints.
type Pull struct {
	Repo   string // owner/name
	Number int64
	URL    string
	State  string // PullOpen, PullMerged or PullClosed
	Draft  bool
	Base   string // the branch it merges into
	// HeadSha is its newest commit; MergeSha the commit its merge made, "" until it is merged or when the host names none.
	HeadSha, MergeSha string
	Review            string  // ReviewApproved, ReviewChanges or ReviewRequired; "" when no review is asked for
	Checks            []Check // in the order the checks first appear
	// HeadAt is when its newest commit was made, and ChangesAt the newest request for changes among the reviews the read shows (zero
	// when it shows none; it does not say that changes are requested now, which is Review). Both are zero when the read did not ask
	// (a branch's lookup does not).
	HeadAt, ChangesAt time.Time
}

// Failed names the checks that stand failed: the latest run of each has finished and is not ok.
func (p *Pull) Failed() []string {
	var names []string
	for _, c := range p.Checks {
		if c.State == CheckFailed {
			names = append(names, c.Name)
		}
	}
	return names
}

// Count is how many of its checks stand in state.
func (p *Pull) Count(state string) int {
	n := 0
	for _, c := range p.Checks {
		if c.State == state {
			n++
		}
	}
	return n
}

// The fields a read asks gh for. A branch's lookup lists up to ten pull requests, so it leaves out their commits and reviews.
const (
	branchPullFields = "number,url,state,isDraft,baseRefName,headRefOid,mergeCommit,reviewDecision,statusCheckRollup"
	pullFields       = branchPullFields + ",commits,latestReviews,reviews"
)

// ghPull is one pull request as `gh pr view --json` and `gh pr list --json` print it.
type ghPull struct {
	Number         int64
	URL            string
	State          string
	IsDraft        bool
	BaseRefName    string
	HeadRefOid     string
	MergeCommit    *struct{ Oid string }
	ReviewDecision string
	Rollup         Rollup `json:"statusCheckRollup"`
	Commits        []struct{ CommittedDate time.Time }
	// Reviews: only what each decided and when is taken, never who wrote it. LatestReviews is each reviewer's newest review of any
	// kind, so a reviewer who asked for changes and then left a plain comment shows there as the comment; Reviews has them all.
	LatestReviews, Reviews []ghReview
}

type ghReview struct {
	State       string
	SubmittedAt time.Time
}

func (g ghPull) pull(repo string) *Pull {
	p := &Pull{Repo: repo, Number: g.Number, URL: g.URL, State: PullOpen, Draft: g.IsDraft, Base: g.BaseRefName, HeadSha: g.HeadRefOid,
		Checks: g.Rollup.Checks()}
	switch g.State {
	case "MERGED":
		p.State = PullMerged
	case "CLOSED":
		p.State = PullClosed
	}
	if g.MergeCommit != nil {
		p.MergeSha = g.MergeCommit.Oid
	}
	switch g.ReviewDecision {
	case "APPROVED":
		p.Review = ReviewApproved
	case "CHANGES_REQUESTED":
		p.Review = ReviewChanges
	case "REVIEW_REQUIRED":
		p.Review = ReviewRequired
	}
	if n := len(g.Commits); n > 0 {
		p.HeadAt = g.Commits[n-1].CommittedDate
	}
	// The newest request for changes that can be seen, in either list. Whether changes are requested at all is the host's decision
	// (Review), never this: a request an approval has since replaced is still in the list.
	for _, r := range append(append([]ghReview{}, g.LatestReviews...), g.Reviews...) {
		if r.State == "CHANGES_REQUESTED" && r.SubmittedAt.After(p.ChangesAt) {
			p.ChangesAt = r.SubmittedAt
		}
	}
	return p
}

// kept is one answer held for PullTTL: a pull request, none (a branch that has no pull request), or the error of the read.
type kept struct {
	pull  *Pull
	err   error
	until time.Time
}

// memo holds what a reader last read, by pull request, by branch and by login asked about.
type memo struct {
	mu   sync.Mutex
	kept map[string]kept
}

func (m *memo) held(now time.Time, key string) (kept, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.kept[key]
	return k, ok && now.Before(k.until)
}

func (m *memo) keep(now time.Time, key string, p *Pull, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kept == nil {
		m.kept = map[string]kept{}
	}
	// What nobody asked about again goes: an answer two more lives past its own is dropped, so the map holds only the pull requests and
	// branches still being looked at.
	for k, old := range m.kept {
		if now.Sub(old.until) > 2*PullTTL {
			delete(m.kept, k)
		}
	}
	m.kept[key] = kept{pull: p, err: err, until: now.Add(PullTTL)}
}

func (g *GitHub) held(key string) (kept, bool)        { return g.memo.held(g.now(), key) }
func (g *GitHub) keep(key string, p *Pull, err error) { g.memo.keep(g.now(), key, p, err) }

// pullKey is where a pull request's read is kept; a branch's answer is kept under its repository and name, which hold no "\x01".
func pullKey(repo string, number int64) string { return repo + "\x01" + strconv.FormatInt(number, 10) }

// ReadPull reads pull request number of repo (owner/name) from GitHub now, through this machine's gh login, and keeps the answer for
// Pull. A pull request that is not there, or not to be seen by this login, is ErrGone.
func (g *GitHub) ReadPull(ctx context.Context, repo string, number int64) (*Pull, error) {
	p, err := g.readPull(ctx, repo, number)
	g.keep(pullKey(repo, number), p, err)
	return p, err
}

func (g *GitHub) readPull(ctx context.Context, repo string, number int64) (*Pull, error) {
	out, err := g.gh(ctx, "pr", "view", strconv.FormatInt(number, 10), "-R", repo, "--json", pullFields)
	if err != nil {
		if gone(err) {
			return nil, goneError{err.Error()}
		}
		return nil, err
	}
	var read ghPull
	if err := json.Unmarshal(out, &read); err != nil {
		return nil, err
	}
	if read.Number == 0 {
		read.Number = number
	}
	return read.pull(repo), nil
}

// Pull is pull request number of repo as it was last read, if that was within PullTTL, else read now.
func (g *GitHub) Pull(ctx context.Context, repo string, number int64) (*Pull, error) {
	if k, ok := g.held(pullKey(repo, number)); ok {
		return k.pull, k.err
	}
	return g.ReadPull(ctx, repo, number)
}

// BranchPull is the pull request of branch in repo: the newest open one, else the newest merged one, else nil. It is kept for PullTTL,
// and what it read serves Pull for that pull request too.
func (g *GitHub) BranchPull(ctx context.Context, repo, branch string) (*Pull, error) {
	key := repo + "\x00" + branch
	if k, ok := g.held(key); ok {
		return k.pull, k.err
	}
	p, err := g.branchPull(ctx, repo, branch)
	g.keep(key, p, err)
	if p != nil {
		g.keep(pullKey(repo, p.Number), p, nil)
	}
	return p, err
}

func (g *GitHub) branchPull(ctx context.Context, repo, branch string) (*Pull, error) {
	out, err := g.gh(ctx, "pr", "list", "-R", repo, "--head", branch, "--state", "all", "--limit", "10", "--json", branchPullFields)
	if err != nil {
		return nil, err
	}
	var pulls []ghPull
	if err := json.Unmarshal(out, &pulls); err != nil {
		return nil, err
	}
	// gh lists the newest first
	for _, state := range []string{"OPEN", "MERGED"} {
		for _, p := range pulls {
			if p.State == state {
				return p.pull(repo), nil
			}
		}
	}
	return nil, nil
}
