package watch

import (
	"context"
	"strings"
	"testing"
	"time"
)

// printed is pull request 7 as `gh pr view` prints one, in a state, with these checks (each as gh prints a statusCheckRollup entry);
// more is further fields as gh names them, which stand over the ones here.
func printed(state, more string, checks ...string) string {
	merge := "null"
	if state == "MERGED" {
		merge = `{"oid":"984c6570a1b2c3d4e5f60718293a4b5c6d7e8f90"}`
	}
	if more != "" {
		more = "," + more
	}
	return `{"baseRefName":"main","commits":[{"committedDate":"2026-10-20T11:41:00Z","messageHeadline":"Add the badge","oid":"1f0e9d8c7b6a"}],` +
		`"headRefOid":"1f0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e","isDraft":false,"latestReviews":[],"mergeCommit":` + merge +
		`,"number":7,"reviewDecision":"","state":"` + state + `","statusCheckRollup":[` + strings.Join(checks, ",") +
		`],"url":"https://github.com/o/r/pull/7"` + more + `}`
}

// checkRun is a check run of name that started at a time of day and concluded so ("" while it runs).
func checkRun(name, conclusion, started string) string {
	status := "COMPLETED"
	if conclusion == "" {
		status = "IN_PROGRESS"
	}
	return `{"__typename":"CheckRun","completedAt":"2026-10-20T11:58:00Z","conclusion":"` + conclusion +
		`","detailsUrl":"https://github.com/o/r/actions/runs/1/job/2","name":"` + name + `","startedAt":"2026-10-20T` + started +
		`Z","status":"` + status + `","workflowName":"ci"}`
}

// review is one reviewer's latest review as gh prints it under latestReviews; only its state and time are read.
func review(state, at string) string {
	return `"latestReviews":[{"author":{"login":"reviewer"},"authorAssociation":"MEMBER","body":"","state":"` + state +
		`","submittedAt":"2026-10-20T` + at + `Z"}]`
}

// A watch for landed passes on the merge, with the commit it made, and gives the task back as soon as the pull request needs its
// holder: closed unmerged, a check that stands failed by its latest run (named), or a reviewer asking for changes after the watch
// started. A failed run that was run again says nothing; checks still running, a review not given, a draft and a request for changes
// that was already there when the watch started all wait.
func TestAPullRequestLandsOrNeedsItsHolder(t *testing.T) {
	since := time.Date(2026, 10, 20, 11, 45, 0, 0, time.UTC)
	ok, red := checkRun("go", "SUCCESS", "11:50:00"), checkRun("build", "FAILURE", "11:50:00")
	for _, c := range []struct {
		name, out string
		verdict   Verdict
		evidence  string
	}{
		{"merged", printed("MERGED", "", ok), Passed, "merged 984c6570a1b2c3d4e5f60718293a4b5c6d7e8f90"},
		{"merged with a check that had failed", printed("MERGED", "", red), Passed, "merged 984c6570a1b2c3d4e5f60718293a4b5c6d7e8f90"},
		{"merged, the host naming no commit", printed("MERGED", `"mergeCommit":null`, ok), Passed, "merged"},
		{"closed unmerged", printed("CLOSED", "", ok), Failed, "closed unmerged"},
		{"CI red on the latest run", printed("OPEN", "", ok, red, checkRun("lint", "TIMED_OUT", "11:50:00")), Failed, "ci failed: build, lint"},
		{"red while another check still runs", printed("OPEN", "", red, checkRun("e2e", "", "11:50:00")), Failed, "ci failed: build"},
		{"a failure a passing rerun replaced", printed("OPEN", "", red, checkRun("build", "SUCCESS", "11:55:00"), ok), Waiting, ""},
		{"a failure whose rerun still runs", printed("OPEN", "", red, checkRun("build", "", "11:55:00")), Waiting, ""},
		{"a rerun that failed again", printed("OPEN", "", red, checkRun("build", "FAILURE", "11:55:00")), Failed, "ci failed: build"},
		{"changes requested after the watch started", printed("OPEN", `"reviewDecision":"CHANGES_REQUESTED",`+review("CHANGES_REQUESTED", "11:50:00"), ok), Failed, "changes requested"},
		{"CI red is said before the review", printed("OPEN", `"reviewDecision":"CHANGES_REQUESTED",`+review("CHANGES_REQUESTED", "11:50:00"), red), Failed, "ci failed: build"},
		{"changes requested before the watch started", printed("OPEN", `"reviewDecision":"CHANGES_REQUESTED",`+review("CHANGES_REQUESTED", "11:30:00"), ok), Waiting, ""},
		{"pending: checks running", printed("OPEN", "", ok, checkRun("e2e", "", "11:50:00")), Waiting, ""},
		{"pending: green and waiting for a review", printed("OPEN", `"reviewDecision":"REVIEW_REQUIRED"`, ok), Waiting, ""},
		{"pending: approved and not merged yet", printed("OPEN", `"reviewDecision":"APPROVED",`+review("APPROVED", "11:50:00"), ok), Waiting, ""},
		{"pending: a draft with no checks", printed("OPEN", `"isDraft":true`), Waiting, ""},
	} {
		var args []string
		v, e, err := gh(c.out, nil, &args).Check(context.Background(), "o/r#7", "landed", since)
		if err != nil || v != c.verdict || e != c.evidence {
			t.Errorf("%s: %s %q %v, want %s %q", c.name, v, e, err, c.verdict, c.evidence)
		}
		if got := strings.Join(args, " "); got != "pr view 7 -R o/r --json "+pullFields {
			t.Errorf("ran gh %s", got)
		}
	}
}

// landed is one of github's untils, about a pull request, said as the pull request landing.
func TestLandedIsAWatchOnAPullRequest(t *testing.T) {
	g := &GitHub{}
	if canon, label, err := g.Parse("o/r#7", "landed"); err != nil || canon != "o/r#7" || label != "PR #7 to land" {
		t.Fatalf("%q %q %v", canon, label, err)
	}
	w, err := Parse("github", "o/r#7", "landed")
	if err != nil {
		t.Fatal(err)
	}
	if repo, n, ok := PullOf(w); !ok || repo != "o/r" || n != 7 {
		t.Fatalf("%q %d %v", repo, n, ok)
	}
	if w, _ := Parse("github", "o/r#7", "done"); w.Until != "done" {
		t.Fatal(w)
	} else if _, _, ok := PullOf(w); ok {
		t.Fatal("an issue is no pull request")
	}
}

// A pull request this login cannot see reads as gone for landed too: the gate counts those, and fails after three in a row.
func TestAPullRequestThisLoginCannotSeeIsGoneForLanded(t *testing.T) {
	var args []string
	g := gh("", goneError{"gh pr: GraphQL: Could not resolve to a PullRequest with the number of 7. (repository.pullRequest)"}, &args)
	if v, e, err := g.Check(context.Background(), "o/r#7", "landed", time.Time{}); err != nil || v != Gone || !strings.Contains(e, "Could not resolve") {
		t.Fatalf("%s %q %v", v, e, err)
	}
}
