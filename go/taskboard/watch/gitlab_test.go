package watch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeGlab answers `glab api` from the fixtures in testdata/gitlab: merge requests as the GitLab REST API answers them, which glab api
// prints unchanged, taken from public gitlab.com projects and cut to the fields the reader takes. mrs maps an endpoint's merge request
// part to a fixture; logged is the hosts glab is logged in to.
type fakeGlab struct {
	t      *testing.T
	mrs    map[string]string
	logged map[string]bool
	calls  [][]string
}

func (f *fakeGlab) reader() *GitLab {
	return &GitLab{Run: func(_ context.Context, args ...string) ([]byte, error) {
		f.calls = append(f.calls, args)
		switch {
		case args[0] == "version":
			return []byte("glab 1.0"), nil
		case args[0] == "auth":
			if f.logged[args[3]] {
				return nil, nil
			}
			return nil, errors.New("glab auth: " + args[3] + " has not been authenticated with glab")
		case args[0] == "api" && args[1] == "--hostname":
			for part, file := range f.mrs {
				if strings.HasSuffix(args[3], part) {
					b, err := os.ReadFile(filepath.Join("testdata", "gitlab", file))
					if err != nil {
						f.t.Fatal(err)
					}
					return b, nil
				}
			}
			return nil, errors.New("glab api: 404 Not Found (HTTP 404)")
		}
		f.t.Fatalf("unexpected glab %q", args)
		return nil, nil
	}}
}

// A merge request reads into the same Pull as a pull request: its state, its target branch, its head, the commit its merge put on the
// target (the merge commit before the squash commit), its pipeline as one check, and its approvals as the review.
func TestAGitLabMergeRequestReadsIntoThePullModel(t *testing.T) {
	f := &fakeGlab{t: t, mrs: map[string]string{
		"/merge_requests/4589": "mr-merged.json", "/merge_requests/4589/approvals": "approvals-approved.json",
		"/merge_requests/4590": "mr-not-approved.json", "/merge_requests/4590/approvals": "approvals-left.json",
		"/merge_requests/4591": "mr-running.json", "/merge_requests/4591/approvals": "approvals-approved.json",
		"/merge_requests/4584": "mr-changes.json", "/merge_requests/4584/approvals": "approvals-left.json",
	}}
	g := f.reader()
	ctx := context.Background()
	const repo = "gitlab.com/gitlab-org/cli"
	merged, err := g.Pull(ctx, repo, 4589)
	want := &Pull{Repo: repo, Number: 4589, URL: "https://gitlab.com/gitlab-org/cli/-/merge_requests/4589", State: PullMerged, Base: "main",
		HeadSha: "e43fbcf3bdf946abfb4952b7523f403297f74f35", MergeSha: "352c0a797711f2a1a00d940b40fff70eeea248ea", Review: ReviewApproved,
		Checks: []Check{{Name: "pipeline", State: CheckPassed}}}
	if err != nil || !reflect.DeepEqual(merged, want) {
		t.Fatalf("merged: %+v %v", merged, err)
	}
	if f.calls[0][3] != "projects/gitlab-org%2Fcli/merge_requests/4589" {
		t.Fatalf("the project is one escaped part: %q", f.calls[0])
	}
	for n, c := range map[int64]struct {
		state, review string
		check         string
	}{
		4590: {PullOpen, ReviewRequired, CheckPassed},
		4591: {PullOpen, ReviewApproved, CheckWaiting},
		4584: {PullOpen, ReviewChanges, ""},
	} {
		p, err := g.Pull(ctx, repo, n)
		if err != nil || p.State != c.state || p.Review != c.review || (c.check == "") != (len(p.Checks) == 0) || c.check != "" && p.Checks[0].State != c.check {
			t.Errorf("!%d: %+v %v", n, p, err)
		}
	}
	if _, err := g.Pull(ctx, repo, 1); !errors.Is(err, ErrGone) {
		t.Fatalf("a 404 is gone: %v", err)
	}
}

// A squash with no merge commit put the squash commit on the target, and a fast-forward its head; a pipeline maps by how it ended; a
// closed or locked merge request is closed.
func TestAMergeRequestsMergeCommitAndPipeline(t *testing.T) {
	ff := glMR{State: "merged", SHA: "aaa"}
	squash := glMR{State: "merged", SHA: "aaa", SquashSHA: "bbb"}
	if ff.pull("h.x/g/p", nil).MergeSha != "aaa" || squash.pull("h.x/g/p", nil).MergeSha != "bbb" {
		t.Fatal("merge sha")
	}
	if (glMR{State: "locked"}).pull("h.x/g/p", nil).State != PullOpen || (glMR{State: "closed"}).pull("h.x/g/p", nil).State != PullClosed {
		t.Fatal("locked is a merge under way, closed is closed")
	}
	for status, want := range map[string]string{"success": CheckPassed, "skipped": CheckPassed, "failed": CheckFailed, "canceled": CheckFailed,
		"running": CheckWaiting, "pending": CheckWaiting, "created": CheckWaiting, "manual": CheckWaiting} {
		if got := pipelineState(status); got != want {
			t.Errorf("%s: %s", status, got)
		}
	}
}

// A gate on a merge request lands as a pull request does: merged passes with the commit on the target, a failed pipeline or a request
// for changes gives it back, an approval still owed waits, closed unmerged fails.
func TestAGateOnAMergeRequestLands(t *testing.T) {
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for file, c := range map[string]struct {
		v        Verdict
		evidence string
	}{
		"mr-merged.json":       {Passed, "merged 352c0a797711f2a1a00d940b40fff70eeea248ea"},
		"mr-not-approved.json": {Waiting, ""},
		"mr-changes.json":      {Failed, "changes requested"},
	} {
		f := &fakeGlab{t: t, mrs: map[string]string{"/merge_requests/9": file, "/approvals": "approvals-left.json"}}
		v, evidence, err := f.reader().Check(context.Background(), "gitlab.com/gitlab-org/cli!9", "landed", since)
		if err != nil || v != c.v || evidence != c.evidence {
			t.Errorf("%s: %s %q %v", file, v, evidence, err)
		}
	}
	// a request for changes untouched since before the wait is not news: it waits, as on GitHub
	f := &fakeGlab{t: t, mrs: map[string]string{"/merge_requests/9": "mr-changes.json"}}
	later := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	if v, _, err := f.reader().Check(context.Background(), "gitlab.com/gitlab-org/cli!9", "landed", later); err != nil || v != Waiting {
		t.Fatalf("an old request for changes: %s %v", v, err)
	}
	// approvals that cannot be read leave the review to what holds the merge, and do not fail the read
	f = &fakeGlab{t: t, mrs: map[string]string{"/merge_requests/9": "mr-not-approved.json"}}
	if p, err := f.reader().Pull(context.Background(), "gitlab.com/gitlab-org/cli", 9); err != nil || p.Review != ReviewRequired {
		t.Fatalf("no approvals: %+v %v", p, err)
	}
	w, err := Landing("GitLab.com/gitlab-org/cli!9")
	if err != nil || w.Vendor != "gitlab" || w.Target != "gitlab.com/gitlab-org/cli!9" || w.Label != "MR !9 to land" || !Lands(w) {
		t.Fatalf("%+v %v", w, err)
	}
	if repo, n, ok := PullOf(w); !ok || repo != "gitlab.com/gitlab-org/cli" || n != 9 {
		t.Fatalf("%s %d", repo, n)
	}
	if _, err := Parse("gitlab", "gitlab-org/cli!9", "landed"); err == nil {
		t.Fatal("a target names its host")
	}
}

// A branch's merge request is its newest open one, else its newest merged one: a closed one is passed over. Its review is read from
// what holds its merge, with no approvals asked.
func TestABranchsMergeRequestIsItsNewestOpenElseMerged(t *testing.T) {
	f := &fakeGlab{t: t, mrs: map[string]string{"source_branch=8770-df-fallback-git-docs": "mr-list.json"}}
	p, err := f.reader().BranchPull(context.Background(), "gitlab.com/gitlab-org/cli", "8770-df-fallback-git-docs")
	if err != nil || p == nil || p.Number != 4589 || p.State != PullMerged || len(f.calls) != 1 {
		t.Fatalf("%+v %v (%d calls)", p, err, len(f.calls))
	}
}

// The reader of a repository is chosen from its host: gh for github.com, glab for a host glab is logged in to here (gitlab.com or a
// self-hosted one), and none for any other, nor for a GitLab host glab is not logged in to. What glab answered is kept.
func TestTheReaderIsChosenFromTheHost(t *testing.T) {
	gh := &GitHub{}
	f := &fakeGlab{t: t, logged: map[string]bool{"gitlab.com": true, "git.corp.example": true}}
	gl := f.reader()
	ctx := context.Background()
	for repo, want := range map[string]Reader{
		"acme/app":                     gh,
		"gitlab.com/group/app":         gl,
		"git.corp.example/team/s/app":  gl,
		"gitlab.other.example/g/app":   nil,
		"bitbucket.org/team/app":       nil,
		"github.example.com/acme/app":  nil,
		"not a repository":             nil,
		"gitlab.com/../app":            nil,
		"github.com/acme/app/one-more": nil,
	} {
		if got := ReaderOf(ctx, repo, gh, gl); got != want {
			t.Errorf("%s: %v", repo, got)
		}
	}
	asked := len(f.calls)
	ReaderOf(ctx, "gitlab.com/group/app", gh, gl)
	if len(f.calls) != asked {
		t.Fatal("glab's answer for a host is kept")
	}
	slow := &GitLab{Run: func(ctx context.Context, _ ...string) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }}
	if ReaderOf(ctx, "gitlab.com/group/app", gh, slow) == nil || len(slow.kept) != 0 {
		t.Fatal("a glab that does not answer in time reads, and its silence is not kept")
	}
	if ReaderOf(ctx, "acme/app", nil, gl) != nil || ReaderOf(ctx, "gitlab.com/group/app", gh, nil) != nil {
		t.Fatal("no reader is a nil Reader")
	}
	if got := UnreadHost("bitbucket.org"); got != "Pull requests on bitbucket.org are not read yet; this repository lands by push." {
		t.Fatal(got)
	}
}
