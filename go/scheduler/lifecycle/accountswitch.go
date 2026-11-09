package lifecycle

import (
	"fmt"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

// LaunchRefusedKind is the event of a launch the daemon refused, which says the account or the agent cannot start and that no session is coming.
const LaunchRefusedKind = "launch_refused"

// noteSwitch writes a step of the agent's account switch and shows it at once: the steps of a move pass inside one tick.
func (m *Manager) noteSwitch(agent, cause, from, to, phase string) {
	check(db.SetSwitch(m.Q, db.AccountSwitch{Agent: agent, Cause: cause, From: from, To: to, Phase: phase, Since: m.now()}))
	m.refreshView(agent)
}

func (m *Manager) refreshView(agent string) {
	if m.RefreshView != nil {
		m.RefreshView(agent)
	}
}

// saving begins the switch a movable trigger starts. A trigger fires every tick until it is dealt with, so one under way keeps its step and since.
func (v *Mover) saving(agent, kind string) {
	if !MovableTrigger(kind) {
		return
	}
	m := v.M
	live := must(db.LiveSwitch(m.Q, must(db.GetAgentState(m.Q, agent)), m.now()))
	if live != nil && live.Phase != db.SwitchWaiting {
		return
	}
	m.noteSwitch(agent, kind, m.Agents.CurrentAccount(m.Q, agent), "", db.SwitchSaving)
}

// parkedSwitch is what a park does to the switch: a wait for the wall, none for a park that is no wall, and a move's short retry keeps the
// step it stopped at.
func (v *Mover) parkedSwitch(agent, kind, from string, reason verdict.ParkReason, wake int64) {
	m := v.M
	switch {
	case reason == verdict.ParkWall && wake > m.now():
		m.noteSwitch(agent, kind, from, "", db.SwitchWaiting)
	case reason != verdict.ParkMoving:
		check(db.ClearSwitch(m.Q, agent))
	}
}

// SwitchOffer is the account to offer a person for an agent parked at its wall, and since when it waits: the policy wanted a move and no
// account has room, so the closest is offered. "" when it is not waiting, stays by choice, is pinned, or nothing could take it.
func (v *Mover) SwitchOffer(agent string) (to string, since int64, err error) {
	defer catch(&err)
	m := v.M
	row := must(db.GetAgentState(m.Q, agent))
	wait := must(db.LiveSwitch(m.Q, row, m.now()))
	if wait == nil || wait.Phase != db.SwitchWaiting {
		return "", 0, nil
	}
	if _, _, roomless := v.plan(agent, wait.Cause, false); !roomless {
		return "", 0, nil
	}
	return m.Launch.ClosestRoom(m.Q, agent, row, v.moveExclude(agent, wait.From)), wait.Since, nil
}

// SwitchNow is a person's answer to the offer: the agent waiting at its wall starts on dest, or with dest "" on the closest account, now,
// whatever the room test says of it; a wall, a full account and a pin still refuse. A start that fails leaves it waiting, on a fresh session
// if the old one had to close. by is recorded on the log line.
func (v *Mover) SwitchNow(agent, dest, by string) (ok bool, said string, err error) {
	defer catch(&err)
	m := v.M
	release, mine := m.Typing.TakeTerminal(agent)
	if !mine {
		return false, TypingElsewhere + "; not switching", nil
	}
	defer release()
	defer m.lockAgent(agent)()
	row := must(db.GetAgentState(m.Q, agent))
	wait := must(db.LiveSwitch(m.Q, row, m.now()))
	if wait == nil || wait.Phase != db.SwitchWaiting {
		return false, agent + " is not waiting for an account", nil
	}
	if dest == "" {
		if dest = m.Launch.ClosestRoom(m.Q, agent, row, v.moveExclude(agent, wait.From)); dest == "" {
			return false, "no account can take " + agent + " now", nil
		}
	}
	if dest == wait.From {
		return false, fmt.Sprintf("%s is already on %s", agent, dest), nil
	}
	if why := m.Launch.RoomPastFit(m.Q, agent, row, dest); why != "" {
		return false, fmt.Sprintf("%s %s - not moving %s there", dest, why, agent), nil
	}
	alive := m.sessionAlive(agent)
	if !alive && !isClosed(row) {
		return false, agent + "'s session is ending; try again in a moment", nil
	}
	if alive && m.sessionBusy(agent) && !v.E.LimitDialogOpen(agent) {
		return false, agent + " is mid-turn", nil
	}
	started := false
	defer func() { // a panic on the way leaves it waiting too; a stall or a session that came up late stands
		if now := must(db.GetAgentState(m.Q, agent)); !started && now.State != states.Stalled && now.State != states.Running {
			check(db.SetParked(m.Q, agent, states.Parked, row.WakeAt.Int64, row.ParkReason.String, row.Detail.String))
			check(db.SetSwitch(m.Q, *wait))
			m.refreshView(agent)
		}
	}()
	m.noteSwitch(agent, wait.Cause, wait.From, dest, db.SwitchSwitching)
	if alive {
		if closed, why := v.closeFresh(agent); !closed {
			return false, fmt.Sprintf("%s stays on %s: %s", agent, wait.From, why), nil
		}
	}
	reason := "switched now by " + by
	landed, why := v.land(agent, HandoffTrigger{Kind: wait.Cause, Reason: reason}, wait.From, dest, reason, true)
	if !landed {
		return false, fmt.Sprintf("%s could not start on %s: %s", agent, dest, clip(why, 120)), nil
	}
	started = true
	return true, fmt.Sprintf("%s moved %s -> %s (%s)", agent, wait.From, dest, reason), nil
}
