package tick

import (
	"fmt"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
)

// loginStallWords is the sentence in the detail of the stall raised for an account that cannot be left. The stall's kind is verdict.StuckLogin;
// the words are for people only.
const loginStallWords = "no other account can take it"

// RepeatN is how many StopFailures of one error the rules below do not name, on one account with no Stop between, stall it. The worst
// run of a service-side error on one account seen live is 3 (server_error), and one wake can fail twice in a second.
const RepeatN = 4

// passing are the errors that never stall an account: rate_limit has the limit path, server_error and overloaded hit every account alike, and
// invalid_request and max_output_tokens are the turn's own. Any other error stalls the account once it repeats RepeatN times.
var passing = map[string]bool{"rate_limit": true, "server_error": true, "overloaded": true, "invalid_request": true, "max_output_tokens": true}

// Login is daemon/login_expired.py without the screen, for every error that makes an account unusable: the CLI's own StopFailure
// is the witness, so a structured session needs no terminal. A zero Login does nothing.
type Login struct {
	// Stalled is the account's stall (accounts.login_expired): why, the error that stalled it, and whether it is there.
	Stalled func(q db.Q, acct string) (why, errType string, ok bool)
	// Failed is the StopFailure the agent's last turn ended on at acct that no lift has passed: its error, its details, and its time.
	Failed func(q db.Q, agent, acct string) (errType, details string, at int64, ok bool)
	// Repeats is how many StopFailures on errType acct has had that no lift has passed.
	Repeats func(q db.Q, acct, errType string) int
	// Stall stalls the account and records the event for it, stamped at (the second of the failure, so a login made since ends it);
	// false when the account is already stalled (the first sighting stands).
	Stall func(q db.Q, agent, acct, errType, why string, at int64) bool
	// Ended is why the account's month cycle is over, "" while it is not: its agents leave it as they leave a stalled one.
	Ended func(acct string) string
	// Paused is why the founder paused the account, "" while it is not: its agents leave it as they leave an ended one.
	Paused func(acct string) string
}

// loginExpired is login_expired.check: an agent with a live session on a stalled, ended or paused account leaves it, level-triggered on the
// account's stall or end. True when the agent was dealt with and the tick has nothing more to do for it.
func (t *Tick) loginExpired(agent string, row *db.AgentState, acct string, actions *[]string) bool {
	l := t.Login
	if l.Stalled == nil || acct == "" || row == nil || closedAt(row) || !t.Host.SessionAlive(agent) ||
		(row.State != states.Running && row.State != states.Idle && row.State != states.Queued) {
		return false
	}
	why, errType, stalled := l.Stalled(t.Q, acct)
	ended, paused := "", ""
	if !stalled && l.Ended != nil {
		ended = l.Ended(acct)
	}
	if !stalled && ended == "" && l.Paused != nil {
		paused = l.Paused(acct)
	}
	if !stalled && ended == "" && paused == "" {
		var details string
		var at int64
		var failed bool
		if errType, details, at, failed = l.Failed(t.Q, agent, acct); !failed || passing[errType] {
			return false
		}
		if details != "" {
			details = ": " + details
		}
		why = fmt.Sprintf("%s's turn ended on %s%s", agent, errType, details)
		if !team.Unusable(errType) {
			n := l.Repeats(t.Q, acct, errType)
			if n < RepeatN {
				return false
			}
			why += fmt.Sprintf(" (%d times on %s with no Stop between)", n, acct)
		}
		if l.Stall(t.Q, agent, acct, errType, why, at) {
			t.logf("  %s: STALLED (%s), no launch lands on it", acct, why)
		}
	}
	// A stalled account's turn has already failed, so its agents go at once; an ended one still answers, so a turn in flight finishes first.
	word, kind := team.StallWord(errType), "login"
	if ended != "" {
		word, why, kind = "subscription ended", ended, "ended"
	}
	if paused != "" {
		word, why, kind = "paused", paused, "paused"
	}
	dest, noMove := t.Handoff.MovePlan(t.Q, agent, kind)
	if dest == "" && paused != "" {
		t.noRoom(acct, "paused", noMove)
		return false
	}
	if dest == "" && ended != "" {
		// An ended account that still answers is better than none: the agent stays on it until another has room.
		t.noRoom(acct, "ended", noMove)
		return false
	}
	if dest == "" {
		// Nothing is waiting out a window here, so a retry loop would only fail again: the agent is shown as stopped, and the
		// reconciler's lift releases it once the account's stall ends.
		detail := fmt.Sprintf("%s on %s and %s (%s) - log %s in, or wake it, in the app's Accounts page", word, acct, loginStallWords, noMove, acct)
		check(db.SetStalled(t.Q, agent, string(verdict.StuckLogin), cut(detail, 300), nil))
		t.noRoom(acct, "login", noMove)
		*actions = append(*actions, fmt.Sprintf("%s: STALLED, %s on %s", agent, word, acct))
		t.logf("  %s: %s", agent, detail)
		return true
	}
	ok := t.Handoff.PerformHandoff(t.Q, agent, Trigger{true, kind, fmt.Sprintf("%s on %s: %s", word, acct, why)}, dest)
	suffix := ""
	if !ok {
		suffix = " -> degraded"
	}
	*actions = append(*actions, fmt.Sprintf("%s: handoff (%s)%s", agent, kind, suffix))
	return true
}

// noRoom keeps, on acct's warning to the founder, that no other account can take its agents (kind: login, ended or paused) and why.
func (t *Tick) noRoom(acct, kind, room string) {
	check(db.SetNoRoom(t.Q, acct, db.NoRoom{Since: t.Now(), Kind: kind, Room: room}))
}

// noRoomOver clears each account's no-room row once it is over: the account is no longer stalled, ended or paused, or no open agent is on it.
func (t *Tick) noRoomOver() error {
	l := t.Login
	if l.Stalled == nil {
		return nil
	}
	rows, err := db.NoRooms(t.Q)
	if err != nil {
		return err
	}
	for acct := range rows {
		_, _, stalled := l.Stalled(t.Q, acct)
		held := stalled || (l.Ended != nil && l.Ended(acct) != "") || (l.Paused != nil && l.Paused(acct) != "")
		if held {
			// Only an agent still waiting to move counts: a closed one keeps its account and moves nowhere.
			if err := t.Q.QueryRow("SELECT EXISTS(SELECT 1 FROM agent_state WHERE account = ? AND COALESCE(closed_at, 0) = 0 AND state IN (?, ?, ?, ?))",
				acct, states.Running, states.Idle, states.Queued, states.Stalled).Scan(&held); err != nil {
				return err
			}
		}
		if !held {
			if err := db.ClearNoRoom(t.Q, acct); err != nil {
				return err
			}
			t.logf("  %s: its agents have room again", acct)
		}
	}
	return nil
}

func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
