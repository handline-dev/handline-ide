package lifecycle

import (
	"fmt"
	"slices"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
)

func moveByKey(agent string) string { return "move_by:" + agent }

// ForceMove is agent.move: a person's move of the agent to dest. An idle agent's live session moves now, through the handoff's own move,
// and a closed agent's next wake starts it there (move_owed). One mid-turn keeps the move, one per agent (a later call replaces its
// target), and RunPendingMove runs it when the turn ends. Nothing is ended unless dest takes one more agent. by is recorded on the event.
// Whether it did or kept it and what it says; a refusal is the first false.
func (v *Mover) ForceMove(agent, dest, by string) (ok bool, said string, err error) {
	defer catch(&err)
	m := v.M
	defer m.lockAgent(agent)()
	row := must(db.GetAgentState(m.Q, agent))
	if why := v.livePhase(row); why != "" {
		return false, why, nil
	}
	if why := v.moveRefused(agent, dest, row); why != "" {
		return false, why, nil
	}
	if isClosed(row) || v.turnOver(agent) {
		ok, said = v.moveIdle(agent, dest, by, row)
		return ok, said, nil
	}
	v.reserve(agent, dest, by, db.SwitchScheduled)
	return true, fmt.Sprintf("%s is mid-turn: it moves to %s when the turn ends (by %s)", addr.Ref(agent), dest, addr.Ref(by)), nil
}

// MoveNow is agent.move_now: the agent's move, to dest or with dest "" the one it keeps, starts now. A turn under way is ended by stop,
// under the agent's lock so it cannot hit the note's turn; RunPendingMove moves it once its handoff note is in. A turn stop cannot end
// keeps the move for the turn's end, where it can still be cancelled.
func (v *Mover) MoveNow(agent, dest, by string, stop func() error) (ok bool, said string, err error) {
	defer catch(&err)
	m := v.M
	defer m.lockAgent(agent)()
	row := must(db.GetAgentState(m.Q, agent))
	s := must(db.LiveSwitch(m.Q, row, m.now()))
	if dest == "" {
		if !s.Pending() {
			return false, agent + " has no move waiting; name the account to move it to", nil
		}
		dest = s.To
	}
	if why := v.livePhase(row); why != "" {
		return false, why, nil
	}
	if why := v.moveRefused(agent, dest, row); why != "" {
		return false, why, nil
	}
	if isClosed(row) {
		ok, said = v.moveIdle(agent, dest, by, row)
		return ok, said, nil
	}
	if m.sessionBusy(agent) {
		if err := stop(); err != nil {
			v.reserve(agent, dest, by, db.SwitchScheduled)
			return true, fmt.Sprintf("%s's turn could not be stopped (%v): it moves to %s when the turn ends (by %s)", addr.Ref(agent), err, dest, addr.Ref(by)), nil
		}
	}
	v.reserve(agent, dest, by, db.SwitchStopping)
	return true, fmt.Sprintf("%s moves to %s once its handoff note is in (by %s)", addr.Ref(agent), dest, addr.Ref(by)), nil
}

// CancelMove is agent.move_cancel: the move the agent keeps for its turn's end is dropped. Too late once move_now has stopped the turn.
func (v *Mover) CancelMove(agent string) (ok bool, said string, err error) {
	defer catch(&err)
	m := v.M
	defer m.lockAgent(agent)()
	s := must(db.LiveSwitch(m.Q, must(db.GetAgentState(m.Q, agent)), m.now()))
	switch {
	case !s.Pending():
		return false, agent + " has no move waiting", nil
	case s.Phase == db.SwitchStopping:
		return false, fmt.Sprintf("too late: %s's turn was stopped for the move to %s and its note is being written", addr.Ref(agent), s.To), nil
	}
	check(db.ClearSwitch(m.Q, agent))
	m.refreshView(agent)
	v.log("  %s: the move to %s is cancelled", agent, s.To)
	return true, fmt.Sprintf("%s stays on %s: the move to %s is cancelled", addr.Ref(agent), s.From, s.To), nil
}

// RunPendingMove runs the person's move the agent keeps once what it waits for is over: the turn's end for a scheduled one; for one
// move_now stopped, the note (NoteHold) and the turn that writes it, or NoteWaitStaleS since the stop, past which it moves on the daemon's
// own record. A target that cannot take it then clears the move and leaves a move_failed line, never retried. Whether it acted.
func (v *Mover) RunPendingMove(agent string) (acted bool, err error) {
	defer catch(&err)
	m := v.M
	if s := must(db.GetSwitch(m.Q, agent)); !s.Pending() {
		return false, nil // the common case, read without the lock a person's move holds through its close and wake
	}
	defer m.lockAgent(agent)()
	row := must(db.GetAgentState(m.Q, agent))
	now := m.now()
	s := must(db.LiveSwitch(m.Q, row, now))
	if !s.Pending() {
		return false, nil
	}
	closed := isClosed(row)
	if !closed {
		switch {
		case !m.sessionAlive(agent):
			return false, nil // a session ending or starting: it is judged when it is one or the other
		case s.Phase == db.SwitchScheduled && !v.turnOver(agent):
			return false, nil
		case s.Phase == db.SwitchStopping && now-s.Since <= db.NoteWaitStaleS && (m.sessionBusy(agent) || v.E.NoteHold(m.Q, agent, now)):
			return false, nil
		}
	}
	by, _ := must2(db.Meta(m.Q, moveByKey(agent)))
	check(db.ClearSwitch(m.Q, agent))
	why := "it broke off; the daemon log says why"
	defer func() {
		if why != "" {
			v.moveFailed(agent, s.To, why)
		}
	}()
	if why = v.moveRefused(agent, s.To, row); why == "" {
		var ok bool
		if ok, why = v.moveIdle(agent, s.To, by, row); ok {
			why = ""
		}
	}
	return true, nil
}

// moveFailed is the chat's line for a person's move that will not happen; it is not tried again.
func (v *Mover) moveFailed(agent, to, why string) {
	m := v.M
	_, _ = db.AddEvent(m.Q, agent, db.MoveFailedKind, db.KV{K: "to", V: to}, db.KV{K: "reason", V: why})
	m.refreshView(agent)
	v.log("  %s: the move to %s did not happen - %s", agent, to, why)
}

// turnOver is whether the agent's turn has ended for a move: idle, or stopped at the usage limit (its dialog up, or quiet since).
func (v *Mover) turnOver(agent string) bool {
	if !v.M.sessionBusy(agent) || v.E.LimitDialogOpen(agent) {
		return true
	}
	at, stopped := v.E.AtLimit(v.M.Q, agent)
	return stopped && v.M.now()-at >= states.LimitSettleS
}

// reserve keeps a person's move of the agent to dest at phase, replacing the one it kept.
func (v *Mover) reserve(agent, dest, by, phase string) {
	m := v.M
	check(db.SetMeta(m.Q, moveByKey(agent), by))
	m.noteSwitch(agent, "forced", m.Agents.CurrentAccount(m.Q, agent), dest, phase)
	v.log("  %s: keeps a move to %s (%s, by %s)", agent, dest, phase, by)
}

// livePhase refuses a person's move while another switch of the agent is under way or move_now has stopped its turn, "" otherwise.
func (v *Mover) livePhase(row *db.AgentState) string {
	m := v.M
	s := must(db.LiveSwitch(m.Q, row, m.now()))
	if s == nil || s.Phase == db.SwitchWaiting || s.Phase == db.SwitchScheduled {
		return ""
	}
	return fmt.Sprintf("%s is already switching accounts (%s %s -> %s)", addr.Ref(row.Agent), s.Phase, s.From, s.To)
}

// moveRefused is why a person's move of the agent to dest cannot happen now, "" when it can: an account that is not there or not logged
// in, the one it is on, a pin, another vendor or no room (forcedPlan), no session, a handoff under way or a stall.
func (v *Mover) moveRefused(agent, dest string, row *db.AgentState) string {
	m := v.M
	accts := team.Accounts{Home: m.Home}
	switch {
	case !slices.Contains(paths.ListAccounts(m.Home), dest) || !accts.LoggedIn(dest):
		return fmt.Sprintf("account '%s' does not exist or is not logged in", dest)
	case m.Agents.CurrentAccount(m.Q, agent) == dest:
		return fmt.Sprintf("%s is already on %s", addr.Ref(agent), dest)
	case row == nil || !(isClosed(row) || m.sessionAlive(agent)):
		return agent + " has no session to move"
	case row.State == states.Handoff || row.State == states.Stalled:
		return fmt.Sprintf("%s is %s", addr.Ref(agent), row.State)
	}
	if room, why := v.forcedPlan(agent, dest); room == "" {
		return fmt.Sprintf("%s - not moving %s there", why, addr.Ref(agent))
	}
	return ""
}

// moveIdle moves an agent that moveRefused lets go, under its lock: a closed one owes the move to its next wake.
func (v *Mover) moveIdle(agent, dest, by string, row *db.AgentState) (bool, string) {
	m := v.M
	here := m.Agents.CurrentAccount(m.Q, agent)
	check(db.ClearSwitch(m.Q, agent)) // a move it kept is this one now
	// A closed agent has nothing to end: the move is owed to the next wake, which starts fresh on dest.
	if isClosed(row) {
		must(db.AddEvent(m.Q, agent, MoveOwedKind, db.KV{K: "to", V: dest}, db.KV{K: "by", V: by}))
		m.refreshView(agent)
		return true, fmt.Sprintf("%s is closed: its next wake starts it on %s (moved by %s)", addr.Ref(agent), dest, addr.Ref(by))
	}
	if _, err := v.performHandoff(agent, HandoffTrigger{Kind: "forced", Reason: fmt.Sprintf("forced by %s: %s -> %s", by, here, dest)}, dest); err != nil {
		return false, err.Error()
	}
	if landed := must(db.GetAgentState(m.Q, agent)); landed == nil || landed.Account.String != dest {
		return false, fmt.Sprintf("%s did not move (mid-turn, or the new session did not start on %s) - it stays on %s", addr.Ref(agent), dest, here)
	}
	must(db.AddEvent(m.Q, agent, "forced_move", db.KV{K: "from", V: here}, db.KV{K: "to", V: dest}, db.KV{K: "by", V: by}))
	return true, fmt.Sprintf("%s moved %s -> %s (forced by %s)", addr.Ref(agent), here, dest, addr.Ref(by))
}
