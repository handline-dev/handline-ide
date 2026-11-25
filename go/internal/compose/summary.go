package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/accounts/usagepoll"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
)

// SummaryModel is the smallest model the Claude CLI offers: the handoff note Handline writes is one short read of an excerpt.
const SummaryModel = "haiku"

// summaryRun is handoff.Summarizer's call: one turn with no tools on the Claude account the pick gives room on, from the readings
// already kept (no live refresh off the tick), among the accounts a launch here may use (mayStart). Only Claude sessions have a
// transcript Handline reads.
func summaryRun(home string, mayStart func(agent string) []string, meter *usagepoll.Meter, uenv *usage.Env, q db.Q) func(ctx context.Context, agent, prompt string) (string, error) {
	return func(ctx context.Context, agent, prompt string) (string, error) {
		var claude []string
		for _, a := range mayStart(agent) {
			if team.AccountVendor(home, a) == team.DefaultVendor {
				claude = append(claude, a)
			}
		}
		loads, err := uenv.ReadAll(q, agent)
		if err != nil {
			return "", err
		}
		acct, _, err := uenv.Pick(q, usage.PickOpts{Candidates: claude, Bootstrap: true, Load: loads, Agent: agent})
		if err != nil {
			return "", err
		}
		if acct == "" {
			return "", errors.New("no account has room")
		}
		ok, out, errType := meter.AskModel(ctx, acct, prompt, SummaryModel)
		if !ok {
			if errType != "" {
				out += " (" + errType + ")"
			}
			return "", fmt.Errorf("on %s: %s", acct, out)
		}
		return out, nil
	}
}
