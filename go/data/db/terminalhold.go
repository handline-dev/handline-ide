package db

import (
	"database/sql"
	"errors"
)

// Open terminal on a Chat agent: its next session runs in a terminal, and that one stays a terminal session until it ends.
const (
	TerminalHoldNone  = 0
	TerminalHoldAsked = 1 // the next session runs in a terminal
	TerminalHoldOn    = 2 // the session running now is that terminal session
)

// TerminalHold is agent's hold, TerminalHoldNone for an agent with no row.
func TerminalHold(q Q, agent string) (int64, error) {
	var v int64
	err := q.QueryRow("SELECT terminal_hold FROM agent_state WHERE agent = ?", agent).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return TerminalHoldNone, nil
	}
	return v, err
}

// SetTerminalHold sets agent's hold.
func SetTerminalHold(q Q, agent string, hold int64) error {
	_, err := q.Exec("INSERT INTO agent_state(agent, state, updated, terminal_hold) VALUES(?,?,?,?) ON CONFLICT(agent) DO UPDATE SET terminal_hold=excluded.terminal_hold",
		agent, "IDLE", Now(), hold)
	return err
}
