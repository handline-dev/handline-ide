package watch

import (
	"context"
	"strings"
	"testing"
	"time"
)

// reviewList is a list of reviews as gh prints one under key (latestReviews: each reviewer's newest of any kind; reviews: all of them),
// each a state at a time of day.
func reviewList(key string, each ...[2]string) string {
	var out []string
	for _, r := range each {
		out = append(out, `{"author":{"login":"reviewer"},"authorAssociation":"MEMBER","body":"","state":"`+r[0]+`","submittedAt":"2026-10-20T`+r[1]+`Z"}`)
	}
	return `"` + key + `":[` + strings.Join(out, ",") + `]`
}

// Whether changes are requested is GitHub's own decision, not what the newest review of each reviewer happens to be: a reviewer who
// asked for changes and then left a plain comment still holds the pull request, though their latest review is the comment. The time of
// the request is the newest one any list shows; when the decision says changes requested and no request can be seen at all, it is
// taken as made now, so the task goes back and is not left waiting on a pull request that cannot land.
func TestChangesRequestedIsTheHostsDecisionNotTheNewestReview(t *testing.T) {
	since := time.Date(2026, 10, 20, 11, 45, 0, 0, time.UTC)
	ok := checkRun("go", "SUCCESS", "11:50:00")
	decided := `"reviewDecision":"CHANGES_REQUESTED",`
	for _, c := range []struct {
		name, more string
		verdict    Verdict
		evidence   string
	}{
		{"asked after the wait began, then a comment", decided + reviewList("latestReviews", [2]string{"COMMENTED", "11:55:00"}) + "," +
			reviewList("reviews", [2]string{"CHANGES_REQUESTED", "11:50:00"}, [2]string{"COMMENTED", "11:55:00"}), Failed, "changes requested"},
		{"asked before the wait began, then a comment after it", decided + reviewList("latestReviews", [2]string{"COMMENTED", "11:55:00"}) + "," +
			reviewList("reviews", [2]string{"CHANGES_REQUESTED", "11:30:00"}, [2]string{"COMMENTED", "11:55:00"}), Waiting, ""},
		{"the decision with no request to be seen", decided + reviewList("latestReviews", [2]string{"COMMENTED", "11:30:00"}) + "," +
			reviewList("reviews", [2]string{"COMMENTED", "11:30:00"}), Failed, "changes requested"},
		{"the decision with no review listed at all", decided + reviewList("latestReviews") + "," + reviewList("reviews"), Failed, "changes requested"},
		{"an old request an approval replaced is no request now", `"reviewDecision":"APPROVED",` + reviewList("latestReviews", [2]string{"APPROVED", "11:55:00"}) + "," +
			reviewList("reviews", [2]string{"CHANGES_REQUESTED", "11:50:00"}, [2]string{"APPROVED", "11:55:00"}), Waiting, ""},
	} {
		var args []string
		v, e, err := gh(printed("OPEN", c.more, ok), nil, &args).Check(context.Background(), "o/r#7", "landed", since)
		if err != nil || v != c.verdict || e != c.evidence {
			t.Errorf("%s: %s %q %v, want %s %q", c.name, v, e, err, c.verdict, c.evidence)
		}
	}
}

// The reader keeps what it read for a minute and forgets what nobody asked about again: an answer two more minutes past its own is
// dropped when the next one is kept, so a daemon that has seen many branches does not hold them all.
func TestWhatNobodyAsksAboutAgainIsForgotten(t *testing.T) {
	c := &counting{answer: func([]string) (string, error) { return viewed, nil }}
	g := c.reader()
	ctx := context.Background()
	for n := int64(1); n <= 50; n++ {
		if _, err := g.Pull(ctx, "acme/app", n); err != nil {
			t.Fatal(err)
		}
	}
	if len(g.kept) != 50 {
		t.Fatalf("%d kept", len(g.kept))
	}
	c.now = c.now.Add(3*PullTTL + time.Second)
	if _, err := g.Pull(ctx, "acme/app", 51); err != nil || len(g.kept) != 1 {
		t.Fatalf("%v, %d kept after three minutes", err, len(g.kept))
	}
	c.now = c.now.Add(PullTTL + time.Second) // past its minute and not yet dropped: asked again, not served stale
	if _, err := g.Pull(ctx, "acme/app", 51); err != nil || c.asked["pr view"] != 52 {
		t.Fatalf("%v, asked %d times", err, c.asked["pr view"])
	}
}
