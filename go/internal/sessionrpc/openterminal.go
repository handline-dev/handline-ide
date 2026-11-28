package sessionrpc

import (
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/tick"
)

// openTerminal is agent.open_terminal: a Chat agent's conversation goes on in its terminal, now when it is between turns, else once its
// turn ends (the session-mode sweep moves it then). That session stays a terminal one until it ends; the next follows the setting. An
// agent with no session is started in its terminal.
func (e *Env) openTerminal(c *rpc.Conn, params map[string]any) (any, error) {
	return e.run(c, "agent.open_terminal", params, func(call Call, agent string) (map[string]any, error) {
		handle, err := db.Terminal(e.DB, agent)
		if err != nil {
			return nil, err
		}
		row, err := db.GetAgentState(e.DB, agent)
		if err != nil {
			return nil, err
		}
		live := handle != "" && row != nil && !(row.ClosedAt.Valid && row.ClosedAt.Int64 != 0)
		headless := live && e.Term.Headless(handle)
		if live && !headless {
			return done(true, agent+": runs in its terminal already", nil)
		}
		if err := db.SetTerminalHold(e.DB, agent, db.TerminalHoldAsked); err != nil {
			return nil, err
		}
		if !headless {
			ok, why, err := e.Controls.Wake(call, agent)
			if err != nil || !ok {
				return done(ok, agent+": its next session runs in its terminal; not started now - "+why, err)
			}
			return done(true, agent+": started in its terminal", nil)
		}
		if !e.Term.IdleNow(handle) || e.Term.IdleWithBackground(handle) {
			return done(true, agent+": opens in its terminal when its turn ends, and any background work with it", nil)
		}
		// Kept as the session-mode pass keeps its own move, so the pass does not restart this session again.
		if err := db.SetMeta(e.DB, tick.ModeApplied+agent, "terminal"); err != nil {
			return nil, err
		}
		ok, said, err := e.Controls.Restart(call, agent, false, false)
		if err != nil || !ok {
			if merr := db.SetMeta(e.DB, tick.ModeApplied+agent, ""); merr != nil { // the pass moves it then
				return nil, merr
			}
			return done(true, agent+": opens in its terminal at its next moment between turns - "+said, err)
		}
		return done(true, agent+": opened in its terminal, in the same conversation", nil)
	})
}
