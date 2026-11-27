package watch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GitLab watches a merge request's close (closed) or its landing (landed) on gitlab.com or a self-hosted GitLab, through glab's own
// login for that host (glab api): handline never sees a token. A target is host/path!N. It is also the reader of a merge request into
// Pull, the same model as GitHub's, so a gate, a card and a branch's verdict read it as they read a pull request.
type GitLab struct {
	// Run runs glab with args and answers its standard output; nil runs the glab CLI. Now is the clock; nil is time.Now.
	Run func(ctx context.Context, args ...string) ([]byte, error)
	Now func() time.Time

	memo
}

func (*GitLab) Name() string     { return "GitLab" }
func (*GitLab) Untils() []string { return []string{"landed", "closed"} }

// Ready is whether glab is installed; whether it is logged in to the target's host is Reads, and a read it cannot make says why.
func (l *GitLab) Ready(ctx context.Context) (bool, string) {
	if _, err := l.glab(ctx, "version"); errors.Is(err, exec.ErrNotFound) {
		return false, "the glab CLI is not installed"
	}
	return true, ""
}

// Reads is whether glab is logged in to host here. A yes or no is kept for PullTTL; a glab that did not answer in time is a yes, not
// kept, so a slow glab does not drop a merge to watch: the read then says what failed.
func (l *GitLab) Reads(ctx context.Context, host string) bool {
	key := "\x00auth\x00" + host
	if k, held := l.memo.held(l.now(), key); held {
		return k.err == nil
	}
	ctx, cancel := context.WithTimeout(ctx, AuthWait)
	defer cancel()
	_, err := l.glab(ctx, "auth", "status", "--hostname", host)
	if ctx.Err() != nil {
		return true
	}
	l.memo.keep(l.now(), key, nil, err)
	return err == nil
}

// Unready says nothing: a host glab cannot read is no reader's (ReaderOf), and that is said where the reader was looked for.
func (*GitLab) Unready() (string, string) { return "", "" }

func (*GitLab) Parse(target, until string) (string, string, error) {
	repo, n, ok := splitLab(target)
	if !ok {
		return "", "", fmt.Errorf("a gitlab target is host/path!N, not %q", target)
	}
	target = strings.ToLower(HostOf(repo)) + repo[len(HostOf(repo)):] + "!" + strconv.FormatInt(n, 10)
	if until == "closed" {
		return target, fmt.Sprintf("MR !%d to close", n), nil
	}
	return target, fmt.Sprintf("MR !%d to land", n), nil
}

func (l *GitLab) Check(ctx context.Context, target, until string, since time.Time) (Verdict, string, error) {
	repo, n, ok := splitLab(target)
	if !ok {
		return "", "", fmt.Errorf("a gitlab target is host/path!N, not %q", target)
	}
	p, err := l.ReadPull(ctx, repo, n)
	switch {
	case errors.Is(err, ErrGone):
		return Gone, err.Error(), nil
	case err != nil:
		return "", "", err
	case until == "landed":
		v, evidence := landedVerdict(p, since)
		return v, evidence, nil
	}
	v, evidence := closedVerdict(p)
	return v, evidence, nil
}

// glMR is a merge request as the GitLab REST API answers it, which `glab api` prints as it is. Only these fields are taken.
type glMR struct {
	IID          int64  `json:"iid"`
	State        string `json:"state"` // opened, closed, locked or merged
	Draft        bool   `json:"draft"`
	TargetBranch string `json:"target_branch"`
	SHA          string `json:"sha"`
	MergeSHA     string `json:"merge_commit_sha"`
	SquashSHA    string `json:"squash_commit_sha"`
	// DetailedStatus is what holds its merge: not_approved and requested_changes are its review's.
	DetailedStatus string    `json:"detailed_merge_status"`
	WebURL         string    `json:"web_url"`
	UpdatedAt      time.Time `json:"updated_at"`
	HeadPipeline   *struct {
		Status string `json:"status"`
	} `json:"head_pipeline"` // only a single merge request's read has it
}

// glApprovals is a merge request's approvals: who approved is only counted.
type glApprovals struct {
	ApprovalsLeft int        `json:"approvals_left"`
	ApprovedBy    []struct{} `json:"approved_by"`
}

// pipelineState is a pipeline's status as a check's: waiting until it ends, passed on success or skipped, failed on failed or canceled.
func pipelineState(status string) string {
	switch status {
	case "success", "skipped":
		return CheckPassed
	case "failed", "canceled":
		return CheckFailed
	}
	return CheckWaiting // created, waiting_for_resource, preparing, pending, running, scheduled, manual
}

// pull maps a merge request into Pull. Its commit on the target branch is the merge commit, else the squash commit, else, merged with
// neither (a fast-forward), its head. A request for changes carries no time here, so ChangesAt is its last update, which is no earlier:
// one asked before a wait and untouched since still waits. ap is nil for a branch's lookup, and for approvals that could not be read.
func (m glMR) pull(repo string, ap *glApprovals) *Pull {
	p := &Pull{Repo: repo, Number: m.IID, URL: m.WebURL, State: PullOpen, Draft: m.Draft, Base: m.TargetBranch, HeadSha: m.SHA}
	switch m.State {
	case "merged":
		p.State = PullMerged
		p.MergeSha = m.MergeSHA
		if p.MergeSha == "" {
			p.MergeSha = m.SquashSHA
		}
		if p.MergeSha == "" {
			p.MergeSha = m.SHA
		}
	case "closed": // locked is a merge under way, still open
		p.State = PullClosed
	}
	if m.HeadPipeline != nil {
		p.Checks = []Check{{Name: "pipeline", State: pipelineState(m.HeadPipeline.Status)}}
	}
	switch {
	case m.DetailedStatus == "requested_changes":
		p.Review, p.ChangesAt = ReviewChanges, m.UpdatedAt
	case ap != nil && ap.ApprovalsLeft > 0, ap == nil && m.DetailedStatus == "not_approved":
		p.Review = ReviewRequired
	case ap != nil && len(ap.ApprovedBy) > 0:
		p.Review = ReviewApproved
	}
	return p
}

// project is repo (host/path) as glab api takes it: the host, and the path as one escaped part of the endpoint.
func project(repo string) (host, endpoint string) {
	host = HostOf(repo)
	return host, "projects/" + url.PathEscape(repo[len(host)+1:])
}

// ReadPull reads merge request number of repo now and keeps the answer for Pull. One that is not there, or not to be seen by this
// login, is ErrGone.
func (l *GitLab) ReadPull(ctx context.Context, repo string, number int64) (*Pull, error) {
	p, err := l.readPull(ctx, repo, number)
	l.memo.keep(l.now(), pullKey(repo, number), p, err)
	return p, err
}

func (l *GitLab) readPull(ctx context.Context, repo string, number int64) (*Pull, error) {
	host, proj := project(repo)
	mr := proj + "/merge_requests/" + strconv.FormatInt(number, 10)
	var m glMR
	if err := l.api(ctx, host, mr, &m); err != nil {
		return nil, err
	}
	if m.IID == 0 {
		m.IID = number
	}
	// approvals add who approved; a GitLab that does not answer them leaves the review to what holds the merge
	var ap glApprovals
	if err := l.api(ctx, host, mr+"/approvals", &ap); err != nil {
		return m.pull(repo, nil), nil
	}
	return m.pull(repo, &ap), nil
}

// Pull is merge request number of repo as it was last read, if that was within PullTTL, else read now.
func (l *GitLab) Pull(ctx context.Context, repo string, number int64) (*Pull, error) {
	if k, ok := l.memo.held(l.now(), pullKey(repo, number)); ok {
		return k.pull, k.err
	}
	return l.ReadPull(ctx, repo, number)
}

// BranchPull is the merge request of branch in repo: the newest open one, else the newest merged one, else nil; kept for PullTTL.
func (l *GitLab) BranchPull(ctx context.Context, repo, branch string) (*Pull, error) {
	key := repo + "\x00" + branch
	if k, ok := l.memo.held(l.now(), key); ok {
		return k.pull, k.err
	}
	host, proj := project(repo)
	var mrs []glMR
	err := l.api(ctx, host, proj+"/merge_requests?state=all&per_page=10&source_branch="+url.QueryEscape(branch), &mrs)
	var p *Pull
	if err == nil { // the API lists the newest first
	found:
		for _, state := range []string{"opened", "merged"} {
			for _, m := range mrs {
				if m.State == state {
					p = m.pull(repo, nil)
					break found
				}
			}
		}
	}
	l.memo.keep(l.now(), key, p, err)
	return p, err
}

// api reads endpoint of host into out through `glab api`. A 404 is ErrGone: GitLab answers a project this login cannot see the same.
func (l *GitLab) api(ctx context.Context, host, endpoint string, out any) error {
	body, err := l.glab(ctx, "api", "--hostname", host, endpoint)
	if err != nil {
		if s := err.Error(); strings.Contains(s, "404 Not Found") || strings.Contains(s, "HTTP 404") {
			return goneError{err.Error()}
		}
		return err
	}
	return json.Unmarshal(body, out)
}

func (l *GitLab) glab(ctx context.Context, args ...string) ([]byte, error) {
	if l.Run != nil {
		return l.Run(ctx, args...)
	}
	return runCLI(ctx, "glab", args...)
}

func (l *GitLab) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}
