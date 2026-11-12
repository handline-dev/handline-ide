package db

import (
	"database/sql"

	"github.com/handline/handline/go/data/rowstate"
)

// The phases of an account switch (AgentView.account_switch).
const (
	SwitchSaving    = "saving"    // the record is being taken; where it goes is not decided
	SwitchSwitching = "switching" // the old session is closing for To
	SwitchStarting  = "starting"  // the session on To is launching
	SwitchWaiting   = "waiting"   // no move: it stays parked until From resets
	// A person's move, before it begins: it waits for the turn to end (scheduled), then for the handoff note (stopping).
	SwitchScheduled = "scheduled"
	SwitchStopping  = "stopping"
)

// MoveFailedKind is the event of a person's move that could not run when its turn ended or its note was in: a line in the agent's chat.
const MoveFailedKind = "move_failed"

// Pending is whether the switch is a person's move that has not begun: AgentView.pending_move, not account_switch.
func (s *AccountSwitch) Pending() bool {
	return s != nil && (s.Phase == SwitchScheduled || s.Phase == SwitchStopping)
}

// AccountSwitch is the account switch an agent is in. To is "" until an account is chosen. Since is when this phase began.
type AccountSwitch struct {
	Agent, Cause, From, To, Phase string
	Since                         int64
}

// SetSwitch writes the agent's switch: there is one per agent, and a later one replaces it.
func SetSwitch(q Q, s AccountSwitch) error {
	_, err := q.Exec("INSERT OR REPLACE INTO account_switch(agent, cause, from_account, to_account, phase, since) VALUES(?,?,?,?,?,?)",
		s.Agent, s.Cause, s.From, s.To, s.Phase, s.Since)
	return err
}

// GetSwitch is the row as written, over or not; nil for none.
func GetSwitch(q Q, agent string) (*AccountSwitch, error) {
	var s AccountSwitch
	err := q.QueryRow("SELECT agent, cause, from_account, to_account, phase, since FROM account_switch WHERE agent = ?", agent).
		Scan(&s.Agent, &s.Cause, &s.From, &s.To, &s.Phase, &s.Since)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

// ClearSwitch ends the agent's switch.
func ClearSwitch(q Q, agent string) error {
	_, err := q.Exec("DELETE FROM account_switch WHERE agent = ?", agent)
	return err
}

// ClearSavingSwitch ends the switch only if it has not gone past saving: the trigger that began it no longer fires.
func ClearSavingSwitch(q Q, agent string) error {
	_, err := q.Exec("DELETE FROM account_switch WHERE agent = ? AND phase = ?", agent, SwitchSaving)
	return err
}

// LiveSwitch is the switch the agent is in now, nil when its row is over: a landed move leaves its row, and this is what ends it. A session
// bound since ends a move, and NoteWaitStaleS outlives a daemon that died mid-move; waiting ends with the park; a closed agent is not saving.
// A person's move that has not begun lasts until it runs or is cancelled, a restart and a stall included: it stays in sight to cancel.
func LiveSwitch(q Q, row *AgentState, now int64) (*AccountSwitch, error) {
	if row == nil {
		return nil, nil
	}
	s, err := GetSwitch(q, row.Agent)
	if err != nil || s == nil || (row.State == rowstate.Stalled && !s.Pending()) {
		return nil, err
	}
	switch s.Phase {
	case SwitchScheduled, SwitchStopping:
	case SwitchWaiting:
		if row.State != rowstate.Parked || row.WakeAt.Int64 <= now {
			return nil, nil
		}
	case SwitchSaving:
		if row.ClosedAt.Int64 != 0 {
			return nil, nil
		}
	default:
		if now-s.Since > NoteWaitStaleS {
			return nil, nil
		}
		last, err := LastBoundSessionStart(q, row.Agent)
		if err != nil || last >= s.Since {
			return nil, err
		}
	}
	return s, nil
}
