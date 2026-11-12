package lifecycle

import (
	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
)

// UsageAdapter is the UsagePort the launch and the wake read, over the usage package's status, pick and load.
type UsageAdapter struct {
	Env *usage.Env
	Q   db.Q
}

var _ UsagePort = (*UsageAdapter)(nil)

func (u *UsageAdapter) ShortBlock() int { return usage.ShortBlock }
func (u *UsageAdapter) LongBlock() int  { return usage.LongBlock }

// Read is status.read of the account.
func (u *UsageAdapter) Read(acct string) AccountStatus {
	st := u.Env.Read(acct)
	out := AccountStatus{Exists: st.Exists, Stale: st.Stale(), AgeS: st.AgeS(), ShortTrusted: st.ShortTrusted(), LongTrusted: st.LongTrusted(),
		ShortLabel: st.Short.Label(), LongLabel: st.Long.Label(), ShortReset: st.Short.Reset, LongReset: st.Long.Reset, Unmetered: st.Unmetered,
		Paused: u.Env.PausedWhy(acct) != ""}
	return out
}

// PickAccount is status.pick_account as find_room asks it: a never-run account is a real place to go, and this is the one caller about
// to place an agent, so a carried or idle-floored candidate is asked fresh. A load or a pick that cannot be read picks nobody: fail closed.
func (u *UsageAdapter) PickAccount(candidates []string, agent string) string {
	loads, err := u.Env.ReadAll(u.Q, agent)
	if err != nil {
		return ""
	}
	picked, _, err := u.Env.Pick(u.Q, usage.PickOpts{Candidates: candidates, Bootstrap: true, Load: loads, Agent: agent, LiveRefresh: true})
	if err != nil {
		return ""
	}
	return picked
}

// NotFor is whether no agent of vendor is ever placed on the account (another vendor's, its cycle ended, paused, or unreadable).
func (u *UsageAdapter) NotFor(acct, vendor string) bool { return u.Env.NotFor(acct, vendor) != "" }

// PickReasons is the pick's reasons with no live refresh: a display must not cost a network call. A load that cannot be read gives none.
func (u *UsageAdapter) PickReasons(candidates []string, agent string) []usage.Reason {
	loads, err := u.Env.ReadAll(u.Q, agent)
	if err != nil {
		return nil
	}
	_, reasons, _ := u.Env.Pick(u.Q, usage.PickOpts{Candidates: candidates, Bootstrap: true, Load: loads, Agent: agent})
	return reasons
}

// Admit is the Fits tier of verdict.Admit for the account. A load that cannot be read says nothing is fit.
func (u *UsageAdapter) Admit(acct, agent, vendor string) verdict.Admission {
	loads, err := u.Env.ReadAll(u.Q, agent)
	if err != nil {
		return verdict.Admission{Reasons: []string{"its load cannot be read: " + err.Error()}}
	}
	return u.Env.FitAdmission(acct, vendor, agent, loads[acct])
}

// CapOf is how many agents the account may have mid-turn at once, shrunk where it would be cut off before a reset.
func (u *UsageAdapter) CapOf(acct string) int { return u.Env.CapNow(u.Q, acct) }

// CapUnpaced is CapOf without the pace.
func (u *UsageAdapter) CapUnpaced(acct string) int { return u.Env.CapUnpaced(u.Q, acct) }

// Urgency is usage.Urgency of the account.
func (u *UsageAdapter) Urgency(acct string) (float64, bool) { return usage.Urgency(u.Env.Read(acct)) }

// PaceShare is usage.Prediction.Share of the account with agent started on it, with the launch pace hold on; ok false when unknown or
// the hold is off, which holds nothing.
func (u *UsageAdapter) PaceShare(q db.Q, acct, agent string) (float64, bool) {
	if !usage.LaunchHoldOn(u.Env.CurrentConfig()) {
		return 1, false
	}
	return u.Env.Predict(q, acct, agent, agent).Share()
}

// Pace is the account's weekly pace now.
func (u *UsageAdapter) Pace(acct string) *usage.Pace { return u.Env.PaceNow(acct) }

// Headroom is what the account's weekly bucket still holds.
func (u *UsageAdapter) Headroom(acct string) (float64, bool) { return u.Env.Headroom(acct) }

// ActiveOn is how many agents are mid-turn on the account, agent left out. A count that cannot be read is the cap: no room.
func (u *UsageAdapter) ActiveOn(q db.Q, acct, agent string) int {
	by, err := u.Env.ActiveAgents(q, agent)
	if err != nil {
		return u.CapOf(acct)
	}
	return by[acct]
}

// MidTurn is every agent mid-turn, agent left out; nil when the count cannot be read.
func (u *UsageAdapter) MidTurn(q db.Q, agent string) []string {
	by, err := u.Env.ActiveByAccount(q)
	if err != nil {
		return nil
	}
	var out []string
	for _, names := range by {
		for _, a := range names {
			if a != agent {
				out = append(out, a)
			}
		}
	}
	return out
}

// StatusAdapter is the StatusPort: what the daemon knows of an agent's session, its id and context from its status file.
type StatusAdapter struct {
	Env *usage.Env
	Q   db.Q
}

var _ StatusPort = (*StatusAdapter)(nil)

// ReadForAgent is status.read_for_agent.
func (s *StatusAdapter) ReadForAgent(agent, account string) SessionStatus {
	st := s.Env.ReadForAgent(agent, account)
	out := SessionStatus{SessionID: st.SessionID}
	if st.CtxPct != nil {
		out.CtxPct = *st.CtxPct
	}
	return out
}
