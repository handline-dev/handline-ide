package reads

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/taskboard/watch"
)

// A GitHub that is slow or down is not a gh that is logged out. When the read of a pull request runs out of its time and `gh auth
// status` does not answer either, the card says the read did not come back, and a branch list says nothing of gh; nobody is told to
// run gh auth login. Once gh answers again the next ask is a fresh one: nothing was kept as logged out.
func TestASlowGitHubIsNotSaidToBeLoggedOut(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, pr, pr_repo) VALUES(1, 'ship', 'bob', 'p1', 1, 'p1', 7, 'acme/app')")
	slow, auths := true, 0
	e.GitHub = &watch.GitHub{AuthWait: 30 * time.Millisecond, Run: func(ctx context.Context, args ...string) ([]byte, error) {
		switch {
		case args[0] == "auth":
			auths++
			if slow {
				<-ctx.Done() // waits on a GitHub that does not answer, until its own short wait is over
				return nil, ctx.Err()
			}
			return nil, nil
		case slow:
			return nil, fmt.Errorf("gh pr: %w", context.DeadlineExceeded) // the read used up its caller's time
		}
		return []byte(recordedPull("OPEN", "")), nil
	}}
	start := time.Now()
	got := taskPRRead(t, e, 1)
	why, _ := got["unavailable"].(map[string]any)
	if why["reason"] != "failed" || why["text"] != "GitHub did not answer within 20s." || strings.Contains(fmt.Sprint(why), "log") {
		t.Fatalf("%v", got)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("the card waited %s on gh auth status", took)
	}
	if reason, _ := e.GitHub.Unready(); reason != "" || auths != 2 {
		t.Fatalf("not kept as logged out: %q, gh auth status asked %d times", reason, auths)
	}
	slow = false
	if reason, why := e.GitHub.Unready(); reason != "" || why != "" || auths != 3 {
		t.Fatalf("gh answers again: %q %q, asked %d times", reason, why, auths)
	}
}
