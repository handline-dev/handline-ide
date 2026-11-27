package compose

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/supervisor"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// authStatusTimeout is how long `claude auth status` may take.
const authStatusTimeout = 10 * time.Second

// founderAsker is subscriptions.Asker: a decision for the founder, opened by agent.
type founderAsker struct{ env *supervisor.Env }

func (a founderAsker) RaiseDecision(q db.Q, agent, text, reason string) (int64, error) {
	return a.env.RaiseDecision(q, agent, text, reason, "", "", 0, 0)
}

// founderCloser is subscriptions.Tasks: a task marked done as the founder's own card, then told as any done is.
type founderCloser struct {
	env  *supervisor.Env
	home string
}

func (c founderCloser) MarkDoneByFounder(_ db.Q, tid int64, reason string) (bool, string, error) {
	tx, err := c.env.DB.Begin()
	if err != nil {
		return false, "", err
	}
	res, err := taskwrites.MarkDone(tx, c.home, actor.OwnerKey, taskwrites.DoneIn{Task: tid, Words: reason})
	if err != nil {
		tx.Rollback()
		var re *rpc.Error
		if errors.As(err, &re) {
			return false, re.Message, nil
		}
		return false, "", err
	}
	if err := tx.Commit(); err != nil {
		return false, "", err
	}
	c.env.Done(tid, actor.OwnerKey, res.Owner, res.Told, res.Finished)
	return true, res.Said, nil
}

// vendorPlans is subscriptions.Plans: the plan on disk, and `claude auth status --json` for the one asked again, run with cli, the claude
// binary the daemon runs ("claude" when none is set).
type vendorPlans struct {
	home, cli string
	// wait is how long claude may take to answer: authStatusTimeout when 0, no limit when negative (a test's fake, whose first run
	// may wait on the system's scan of a file just written).
	wait time.Duration
}

func (p vendorPlans) Recorded(acct string) (string, bool) {
	return team.Accounts{Home: p.home}.RecordedSubscriptionType(acct)
}

// Refresh reads the account's subscription type anew from the claude CLI: a reading that cannot be taken leaves what is recorded.
func (p vendorPlans) Refresh(acct string) (string, error) {
	recorded, _ := p.Recorded(acct)
	if !(team.Accounts{Home: p.home}).LoggedIn(acct) || team.AccountVendor(p.home, acct) != team.DefaultVendor {
		return recorded, nil
	}
	dir, err := team.AccountConfigDir(p.home, acct)
	if err != nil {
		return recorded, nil
	}
	wait := p.wait
	if wait == 0 {
		wait = authStatusTimeout
	}
	ctx, cancel := context.Background(), context.CancelFunc(func() {})
	if wait > 0 {
		ctx, cancel = context.WithTimeout(ctx, wait)
	}
	defer cancel()
	cli := p.cli
	if cli == "" {
		cli = "claude"
	}
	cmd := exec.CommandContext(ctx, cli, "auth", "status", "--json")
	cmd.Dir = paths.QuietDir()
	cmd.Env = append(os.Environ(), team.ConfigDirEnv(team.DefaultVendor)+"="+dir)
	out, err := cmd.Output()
	var data struct {
		Sub string `json:"subscriptionType"`
	}
	if err != nil || json.Unmarshal(out, &data) != nil || data.Sub == "" {
		return recorded, nil
	}
	f := filepath.Join(paths.AccountDir(p.home, acct), "subscription.json")
	rec, _ := json.Marshal(map[string]any{"subscriptionType": data.Sub, "rateLimitTier": team.RateLimitTier(dir), "recorded_at": time.Now().Unix()})
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return "", err
	}
	return data.Sub, os.WriteFile(f, rec, 0o644)
}
