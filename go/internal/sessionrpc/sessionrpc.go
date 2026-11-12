// Package sessionrpc is the methods that drive an agent's session through the daemon: session.start, wake, stop, restart, park and
// interrupt, and terminal.close (handline/daemon/rpc_session.py, rpc_typing.py's interrupt).
//
// The session methods are thin: validation, the verbs' own state checks, and the request_id effect, over the lifecycle's Controls. A success is recorded under the caller's request_id the moment the action returns, so a retry after a crash
// that lost the reply replays it instead of acting again. A refusal names its gate: terminal_lock when whoever types at a terminal agent
// holds the lock the tick takes, state for every other reason the action gives.
//
// session.input, keys and type and the input holder (typing.go) serve a terminal agent. Dropped with the design: the exit mode that typed
// /exit, and the kill that ended a structured session's process group (a kill is refused). session.interrupt is Esc on the agent's pane.
// A method whose port is nil is not registered, so hello never lists what cannot be served.
package sessionrpc

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/handline/handline/go/kit/addr"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/inputhold"
	"github.com/handline/handline/go/internal/outcome"
	"github.com/handline/handline/go/kit/lock"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// TypingElsewhere is the refusal's reason while whoever types at a terminal agent holds the lock the tick takes.
const TypingElsewhere = "someone else is typing at this agent"

// Call is who is acting, for the lifecycle.
type Call struct {
	Person bool
	By     string // the caller's actor name
}

// Controls is actions/control.py and daemon/lifecycle.py as the session methods use them (the S3 lifecycle). Each returns whether it did,
// and what it says; the words of a refusal are the verb's own.
type Controls interface {
	// Start is control.start_result: give an agent a worktree and start it there. repo "" is the working directory; base is never "".
	Start(c Call, agent, repo, base string) (ok bool, said string, err error)
	// Wake is lifecycle.wake_agent; why is the lifecycle's own words, not yet prefixed with the agent.
	Wake(c Call, agent string) (ok bool, why string, err error)
	// Close is control.close_result, fresh also dropping the session id.
	Close(c Call, agent string, fresh bool) (ok bool, said string, err error)
	// Restart is control.restart_result.
	Restart(c Call, agent string, fresh, force bool) (ok bool, said string, err error)
	// Park is control.park_result: record a handoff, park until the window rolls over.
	Park(c Call, agent string) (ok bool, said string, err error)
	// Compact asks for the handoff a full context runs: note, record, a fresh session. It answers before any of it.
	Compact(c Call, agent string) (ok bool, said string, err error)
}

// Env is what the methods run on.
type Env struct {
	DB       *sql.DB
	Home     string
	Controls Controls
	// Terminal agents only (frozen): the terminal's close, and whether a session still runs in it.
	CloseTerminal  func(handle string) (ok bool, why string)
	SessionPresent func(agent string) (bool, error)
	// InterruptTerminal interrupts a terminal agent's turn (Env.PressEsc of the typing Env); nil serves no session.interrupt.
	InterruptTerminal func(c *rpc.Conn, agent string) error
	// Turns closes a turn an Esc ended on screen (turns.Tracker); nil leaves it to the transcript.
	Turns TurnEnder
	// Fold is control.fold: fold the day's events into the agent's digest, the one model call. nil serves no agent.fold.
	Fold func(agent string) (ok bool, said string)
	// Marker tells whoever watches the agent's terminal that a control call happened (termchan.Env.Marker); nil tells no one.
	Marker func(agent, kind string, actor map[string]any, text string)
	Now    func() time.Time
	// Typing at a terminal: who holds each terminal's input, the host to type at, and why the usage wall stands in the way ("" for none).
	// Without Holds none of input.take, release, session.input, keys and type is served; without Term the three session ones are not.
	Holds *inputhold.Store
	Term  *term.Term
	Wall  func(agent string) string
	// Roster registers the agent of agent.create, and Emit tells the operations topic its progress; with Controls they serve agent.create.
	Roster *team.Roster
	Emit   func(topic, typ string, data any, agent string)
	// Move is handoff.force_move: the agent's session to another account now, a closed agent's next wake there, one mid-turn's when the turn
	// ends; nil serves no agent.move.
	Move func(call Call, agent, account string) (ok bool, said string, err error)
	// MoveNow starts the agent's move, to account or with "" the one it keeps, once its note is in; stop ends a turn under way.
	// CancelMove drops the move it keeps. nil serves no agent.move_now, no agent.move_cancel.
	MoveNow    func(call Call, agent, account string, stop func() error) (ok bool, said string, err error)
	CancelMove func(call Call, agent string) (ok bool, said string, err error)
	// SwitchNow moves an agent waiting at its wall to the account a person chose past the room test, "" for the closest (a switch_offer); nil serves no agent.switch_now.
	SwitchNow func(call Call, agent, account string) (ok bool, said string, err error)
	// ForceStart lets an agent's work the machine's load holds go past the limits once; nil serves no agent.force_start.
	ForceStart func(agent string) (ok bool, said string, err error)
	// Unpass takes back what ForceStart let past, when its start did not come in time; nil takes nothing back.
	Unpass func(agent string)
	// Launch is the launch hl launch execs, built and recorded here; nil serves no session.launch_prepare. A launch that must not go ahead
	// is a *LaunchRefusal.
	Launch func(LaunchRequest) (LaunchResult, error)
	// Wait is how long a call that answers only with its outcome (agent.create, agent.force_start) waits for it; 0 is outcome.Limit.
	Wait time.Duration
}

// Register adds the methods the Env has the ports for.
func Register(s *rpc.Server, e *Env) {
	if e.Controls != nil {
		s.Register("session.start", rpc.Method{Handler: e.start})
		s.Register("session.wake", rpc.Method{Handler: e.wake})
		s.Register("session.restart", rpc.Method{Handler: e.restart})
		s.Register("session.compact", rpc.Method{Handler: e.verb("session.compact", e.Controls.Compact)})
		s.Register("session.park", rpc.Method{Handler: e.verb("session.park", e.Controls.Park)})
		s.Register("session.stop", rpc.Method{Handler: e.stop})
	}
	if e.Controls != nil && e.Roster != nil {
		s.Register("agent.create", rpc.Method{Waits: true, Handler: e.create, Refresh: func(_ *rpc.Conn, reply map[string]any) error {
			return e.createOutcome(reply)
		}})
		s.Register("workspace.create", rpc.Method{Waits: true, Handler: e.workspaceCreate, Refresh: func(_ *rpc.Conn, reply map[string]any) error {
			return e.createOutcome(reply)
		}})
	}
	if e.Move != nil {
		s.Register("agent.move", rpc.Method{Handler: e.move})
	}
	if e.MoveNow != nil {
		s.Register("agent.move_now", rpc.Method{Handler: e.moveNow})
	}
	if e.CancelMove != nil {
		s.Register("agent.move_cancel", rpc.Method{Handler: e.moveCancel})
	}
	if e.SwitchNow != nil {
		s.Register("agent.switch_now", rpc.Method{Handler: e.switchNow})
	}
	if e.DB != nil {
		s.Register("agent.unstall", rpc.Method{Handler: e.unstall})
	}
	if e.ForceStart != nil {
		s.Register("agent.force_start", rpc.Method{Waits: true, Handler: e.forceStart})
	}
	if e.Fold != nil {
		s.Register("agent.fold", rpc.Method{Handler: e.fold})
	}
	if e.Launch != nil {
		s.Register("session.launch_prepare", rpc.Method{Verbatim: true, Handler: e.launchPrepare})
	}
	if e.InterruptTerminal != nil {
		s.Register("session.interrupt", rpc.Method{Handler: e.interrupt})
	}
}

func bad(msg, field string) error { return rpc.BadRequestError(msg, field, rpc.InvalidParams) }

func (e *Env) agent(params map[string]any) (string, error) {
	name, _ := params["agent"].(string)
	if name == "" {
		return "", bad("agent is a name", "agent")
	}
	for _, n := range paths.ListAgents(e.Home) {
		if n == name {
			return name, nil
		}
	}
	err := rpc.NotFoundError("no such agent: " + name)
	err.Data = map[string]any{"what": "agent", "name": name}
	return "", err
}

func flag(params map[string]any, key string) (bool, error) {
	v, present := params[key]
	if !present {
		return false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, bad(key+" is true or false", key)
	}
	return b, nil
}

// refusal is a refusal whose gate is read off its words.
func refusal(why string) error {
	gate := "state"
	if strings.Contains(why, TypingElsewhere) {
		gate = "terminal_lock"
	}
	return rpc.Refused(why, gate, "")
}

func done(ok bool, said string, err error) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, refusal(said)
	}
	return map[string]any{"accepted": true, "said": said}, nil
}

func callOf(c *rpc.Conn) Call { return Call{Person: c.Caller.Allows("member"), By: c.Caller.Actor()} }

// run runs fn(agent) and records its result as the effect of this request_id. With hold the agent's terminal lock is held across it
// (terminal.close); without, the lifecycle takes whatever lock it needs itself.
func (e *Env) run(c *rpc.Conn, name string, params map[string]any, hold bool, fn func(call Call, agent string) (map[string]any, error)) (any, error) {
	agent, err := e.agent(params)
	if err != nil {
		return nil, err
	}
	release, err := e.useWorkspace(agent)
	if err != nil {
		return nil, err
	}
	defer release()
	var result map[string]any
	if hold {
		h := lock.TakeTerminal(e.Home, agent)
		if h == nil {
			return nil, rpc.Refused(agent+": "+TypingElsewhere, "terminal_lock", "")
		}
		result, err = fn(callOf(c), agent)
		h.Release()
	} else {
		result, err = fn(callOf(c), agent)
	}
	if err != nil {
		return nil, err
	}
	if result, err = c.Effect(func(*sql.Tx) (map[string]any, error) { return result, nil }); err != nil {
		return nil, err
	}
	if e.Marker != nil {
		e.Marker(agent, "control", c.Caller.ActorInfo(), name)
	}
	return result, nil
}

func (e *Env) start(c *rpc.Conn, params map[string]any) (any, error) {
	if params["account"] != nil {
		return nil, bad("account is not served yet: a session starts on the account the daemon picks", "account")
	}
	var repo, base string
	for _, k := range []string{"repo", "base"} {
		v := params[k]
		if v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return nil, bad(k+" is a string", k)
		}
		if k == "repo" {
			repo = s
		} else {
			base = s
		}
	}
	return e.run(c, "session.start", params, false, func(call Call, agent string) (map[string]any, error) {
		return done(e.Controls.Start(call, agent, repo, base))
	})
}

func (e *Env) wake(c *rpc.Conn, params map[string]any) (any, error) {
	return e.run(c, "session.wake", params, false, func(call Call, agent string) (map[string]any, error) {
		row, err := db.GetAgentState(e.DB, agent)
		if err != nil {
			return nil, err
		}
		if row != nil && row.State == states.Stalled { // a wake of a stalled agent clears the stall, as the verb does
			cleared, said, err := e.Unstall(agent)
			return done(err == nil && cleared, said, err)
		}
		ok, why, err := e.Controls.Wake(call, agent)
		if err != nil {
			return nil, err
		}
		if ok {
			return done(true, agent+": "+why, nil)
		}
		return done(false, agent+": not woken - "+why, nil)
	})
}

// Unstall clears a stall, the one state the daemon will not leave alone. cleared is false when the agent was not STALLED.
func (e *Env) Unstall(agent string) (cleared bool, said string, err error) {
	row, err := db.GetAgentState(e.DB, agent)
	if err != nil {
		return false, "", err
	}
	if row == nil || row.State != states.Stalled {
		state := "no state"
		if row != nil {
			state = row.State
		}
		return false, fmt.Sprintf("%s: not STALLED (%s)", addr.Ref(agent), state), nil
	}
	if err := db.ClearStall(e.DB, agent, "cleared by a human"); err != nil {
		return false, "", err
	}
	return true, agent + ": STALLED -> QUEUED", nil
}

func (e *Env) restart(c *rpc.Conn, params map[string]any) (any, error) {
	fresh, err := flag(params, "fresh")
	if err != nil {
		return nil, err
	}
	force, err := flag(params, "force")
	if err != nil {
		return nil, err
	}
	return e.run(c, "session.restart", params, false, func(call Call, agent string) (map[string]any, error) {
		return done(e.Controls.Restart(call, agent, fresh, force))
	})
}

// move is agent.move. A forced move ends a session, which is the founder's call (a person's, from a terminal or the screen), not an agent's.
func (e *Env) move(c *rpc.Conn, params map[string]any) (any, error) {
	if !c.Caller.Allows("member") {
		return nil, rpc.Refused("a forced move ends a session: it is the founder's, from a terminal or the screen, not an agent's", "role", "")
	}
	account, _ := params["account"].(string)
	if account == "" {
		return nil, bad("account is the account to move the agent to", "account")
	}
	return e.run(c, "agent.move", params, false, func(call Call, agent string) (map[string]any, error) {
		return done(e.Move(call, agent, account))
	})
}

// moveNow is agent.move_now: the turn under way is stopped as session.interrupt stops it, and the move runs once the note is in.
func (e *Env) moveNow(c *rpc.Conn, params map[string]any) (any, error) {
	account, _ := params["account"].(string)
	return e.run(c, "agent.move_now", params, false, func(call Call, agent string) (map[string]any, error) {
		return done(e.MoveNow(call, agent, account, func() error {
			if e.InterruptTerminal == nil {
				return errors.New("no terminal to stop it in")
			}
			return e.InterruptTerminal(c, agent)
		}))
	})
}

// moveCancel is agent.move_cancel.
func (e *Env) moveCancel(c *rpc.Conn, params map[string]any) (any, error) {
	return e.run(c, "agent.move_cancel", params, false, func(call Call, agent string) (map[string]any, error) {
		return done(e.CancelMove(call, agent))
	})
}

// switchNow is agent.switch_now. It ends a session, so it is the founder's (x-role).
func (e *Env) switchNow(c *rpc.Conn, params map[string]any) (any, error) {
	account, _ := params["account"].(string)
	return e.run(c, "agent.switch_now", params, false, func(call Call, agent string) (map[string]any, error) {
		return done(e.SwitchNow(call, agent, account))
	})
}

// unstall is agent.unstall: a stalled agent goes to queued (Unstall); one that is not is refused with the action's own words.
func (e *Env) unstall(c *rpc.Conn, params map[string]any) (any, error) {
	return e.run(c, "agent.unstall", params, true, func(_ Call, agent string) (map[string]any, error) {
		cleared, said, err := e.Unstall(agent)
		return done(err == nil && cleared, said, err)
	})
}

// forceStart is agent.force_start: refused with the action's own words unless the load is what holds the agent.
// The call answers once the start it let past has begun (a launch or a wake), or failed; past the limit the pass is taken back and the
// call is refused, so no start comes later that the caller did not hear of.
func (e *Env) forceStart(c *rpc.Conn, params map[string]any) (any, error) {
	return e.run(c, "agent.force_start", params, false, func(_ Call, agent string) (map[string]any, error) {
		var before int64
		if err := e.DB.QueryRow("SELECT COALESCE(MAX(id), 0) FROM events").Scan(&before); err != nil {
			return nil, err
		}
		ok, said, err := e.ForceStart(agent)
		if err != nil || !ok {
			return done(ok, said, err)
		}
		var kind, why string
		ended, err := outcome.Wait(e.Wait, c.Done(), func() (bool, error) {
			err := e.DB.QueryRow("SELECT kind, COALESCE(json_extract(payload, '$.why'), '') FROM events WHERE agent = ? AND id > ? AND "+
				"kind IN ('launching', 'wake', 'wake_failed', 'wake_dropped') ORDER BY id LIMIT 1", agent, before).Scan(&kind, &why)
			if err == sql.ErrNoRows {
				return false, nil
			}
			return err == nil, err
		})
		switch {
		case err != nil:
			return nil, err
		case !ended:
			if e.Unpass != nil {
				e.Unpass(agent)
			}
			return done(false, agent+" did not start in time, so what let it past is taken back and nothing was started", nil)
		case kind == "wake_failed" || kind == "wake_dropped":
			return done(false, agent+": not started - "+why, nil)
		}
		return done(true, strings.Replace(said, "Starting "+agent+" now", agent+" has started", 1), nil)
	})
}

func (e *Env) fold(c *rpc.Conn, params map[string]any) (any, error) {
	return e.run(c, "agent.fold", params, false, func(_ Call, agent string) (map[string]any, error) {
		ok, said := e.Fold(agent)
		return done(ok, said, nil)
	})
}

// verb is a session method that takes only the agent and answers with act's words.
func (e *Env) verb(name string, act func(Call, string) (bool, string, error)) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, params map[string]any) (any, error) {
		return e.run(c, name, params, false, func(call Call, agent string) (map[string]any, error) { return done(act(call, agent)) })
	}
}

// stop: close and exit both end the session by closing the CLI's stdin (the CLI exits when its input ends, so nothing is typed);
// kill ends the process group at once, and is for a member or above.
func (e *Env) stop(c *rpc.Conn, params map[string]any) (any, error) {
	mode, _ := params["mode"].(string)
	fresh, err := flag(params, "fresh")
	if err != nil {
		return nil, err
	}
	if mode != "exit" && mode != "close" && mode != "kill" {
		return nil, bad("mode is exit, close or kill", "mode")
	}
	if fresh && mode != "close" {
		return nil, bad("fresh is for close only", "fresh")
	}
	if mode == "kill" {
		if !c.Caller.Allows("member") {
			return nil, rpc.Refused(fmt.Sprintf("session.stop kill is for a member or above; this caller is %s", c.Caller.Role), "role", "")
		}
		return e.run(c, "session.stop", params, false, func(_ Call, agent string) (map[string]any, error) {
			return nil, rpc.Refused(agent+" has no structured session to kill", "state", "")
		})
	}
	return e.run(c, "session.stop", params, false, func(call Call, agent string) (map[string]any, error) {
		return done(e.Controls.Close(call, agent, fresh))
	})
}

// TurnEnder is the turns tracker's side of an interrupt: a mark of the turn before the Esc, and closing that turn.
type TurnEnder interface {
	TurnMark(agent string) int
	InterruptTurn(agent string, mark int)
}

// interrupt stops the running turn by Esc on the agent's pane.
func (e *Env) interrupt(c *rpc.Conn, params map[string]any) (any, error) {
	return e.run(c, "session.interrupt", params, false, func(_ Call, agent string) (map[string]any, error) {
		if err := e.InterruptTerminal(c, agent); err != nil {
			return nil, err
		}
		return done(true, agent+": interrupted", nil)
	})
}

func (e *Env) terminalClose(c *rpc.Conn, params map[string]any) (any, error) {
	return e.run(c, "terminal.close", params, true, func(_ Call, agent string) (map[string]any, error) {
		if err := e.holderGate(c, agent); err != nil {
			return nil, err
		}
		row, err := db.GetAgentState(e.DB, agent)
		if err != nil {
			return nil, err
		}
		if row == nil || row.Terminal.String == "" {
			return nil, rpc.Refused(agent+" has no terminal", "state", "")
		}
		handle := row.Terminal.String
		present, err := e.SessionPresent(agent)
		if err != nil {
			return nil, err
		}
		if row.ClosedAt.Int64 == 0 || present {
			return nil, rpc.Refused(agent+" has a session running; stop it first", "state", "")
		}
		if ok, why := e.CloseTerminal(handle); !ok {
			return nil, rpc.Refused(fmt.Sprintf("%s: its terminal would not close: %s", addr.Ref(agent), why), "state", "")
		}
		if err := db.SetTerminal(e.DB, agent, "", row.Worktree.String); err != nil {
			return nil, err
		}
		if _, err := db.AddEvent(e.DB, agent, "terminal_closed", db.KV{K: "handle", V: handle}); err != nil {
			return nil, err
		}
		if err := e.Holds.Release(agent); err != nil { // a hold on a terminal that is gone is over
			return nil, err
		}
		return map[string]any{"accepted": true, "said": agent + ": terminal closed"}, nil
	})
}
