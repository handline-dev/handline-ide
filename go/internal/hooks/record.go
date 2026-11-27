package hooks

import (
	"database/sql"
	"errors"
	"time"

	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	"github.com/handline/handline/go/protocol/rpc"
)

// Recorder serves hook.record and the spool over one database and home.
type Recorder struct {
	DB   *sql.DB
	Home string
	Log  func(format string, args ...any)
	// Turns, when set, is told each recorded event (the -turns-from-hooks switch).
	Turns *turns.Tracker
	// Nudge is asked before a Stop is applied: the text that keeps the turn going, "" for none.
	Nudge func(agent string, raw map[string]any) string
	// Background is told each PostToolUse once it is applied.
	Background func(agent string, raw map[string]any)
	// Subagent is asked before an Agent call of agent's session: why it may not start, or "" once its start is recorded.
	Subagent func(agent, session string, call SubagentCall) (string, error)
	// TurnEnded is told each Stop, StopFailure and SessionEnd once it is applied.
	TurnEnded func(agent, kind string)
	// Elsewhere is whether agent's session runs on a runner, whose own tracker watches it and tells this daemon its turn (node.turn):
	// Turns is not told its hooks. nil is none.
	Elsewhere func(agent string) bool
	// Forward, on a runner, takes each event in place of the board (DB): it sends it to the master and answers the master's nudge.
	// ErrUnreached is a master that cannot be reached now: the event goes to the spool and is sent again when it can be.
	Forward func(agent, kind string, data map[string]any, requestID string, at int64) (nudge string, err error)
}

// ErrUnreached is Forward's error while the master cannot be reached.
var ErrUnreached = errors.New("the master cannot be reached now")

// SpoolEvery is how often the spool is looked at while the daemon runs.
const SpoolEvery = 2 * time.Second

// Register adds hook.record and subagent.start to the server's table.
func (h *Recorder) Register(s *rpc.Server) {
	s.Register("hook.record", rpc.Method{Handler: h.record, OwnEvent: true})
	s.Register("subagent.start", rpc.Method{Handler: h.subagentStart, OwnEvent: true})
}

// SubagentCall is what the Agent call said of the subagent it starts.
type SubagentCall struct {
	Description, Type string
	Background        bool
}

// subagentStart is the PreToolUse guard's question before an Agent call: {"deny": why} when one more subagent would take the agent's
// account to a limit before it resets, else the start is recorded (usage.Env.StartSubagent) and {} lets it through.
func (h *Recorder) subagentStart(c *rpc.Conn, params map[string]any) (any, error) {
	agent, ok := params["agent"].(string)
	if !ok || agent == "" {
		return nil, rpc.BadRequestError("agent is a name", "agent", rpc.InvalidParams)
	}
	session, _ := params["session_id"].(string)
	var m SubagentCall
	m.Description, _ = params["description"].(string)
	m.Type, _ = params["subagent_type"].(string)
	m.Background, _ = params["run_in_background"].(bool)
	if err := c.RequireSelf(agent); err != nil {
		return nil, err
	}
	if h.Subagent == nil {
		return map[string]any{}, nil
	}
	why, err := h.Subagent(agent, session, m)
	if err != nil {
		return nil, err
	}
	if why == "" {
		return map[string]any{}, nil
	}
	if h.Log != nil {
		h.Log("%s: subagent refused: %s", agent, why)
	}
	return map[string]any{"deny": why}, nil
}

// Record is hook.record: a runner, which serves no write itself, calls it for the hook.record its agents send.
func (h *Recorder) Record(c *rpc.Conn, params map[string]any) (any, error) {
	return h.record(c, params)
}

func (h *Recorder) record(c *rpc.Conn, params map[string]any) (any, error) {
	agent, ok := params["agent"].(string)
	if !ok || agent == "" {
		return nil, rpc.BadRequestError("agent is a name", "agent", rpc.InvalidParams)
	}
	kind, ok := params["kind"].(string)
	if !ok || kind == "" {
		return nil, rpc.BadRequestError("kind is a hook event name", "kind", rpc.InvalidParams)
	}
	data, ok := params["data"].(map[string]any)
	if !ok {
		return nil, rpc.BadRequestError("data is the hook's own object", "data", rpc.InvalidParams)
	}
	at, ok := rpc.AsInt(params["at"])
	if !ok {
		return nil, rpc.BadRequestError("at is when the hook ran, in epoch seconds", "at", rpc.InvalidParams)
	}
	if err := c.RequireSelf(agent); err != nil {
		return nil, err
	}
	if !safeName(agent) {
		return nil, rpc.BadRequestError("agent is not a name this daemon keeps files for", "agent", rpc.InvalidParams)
	}
	nudge := ""
	if kind == "Stop" && h.Nudge != nil { // before the record, so the last Stop recorded is the turn before
		nudge = h.Nudge(agent, data)
	}
	rid, _ := params["request_id"].(string)
	if rid == "" { // the server checks it for a write it serves; a runner's comes through its fallback
		return nil, rpc.BadRequestError("request_id is a string", "request_id", rpc.InvalidParams)
	}
	if h.Forward != nil {
		if h.Turns != nil {
			h.Turns.Hook(agent, kind, data, at)
		}
		if spoolWaiting(h.Home) { // after what waits there, in order
			return map[string]any{}, AppendSpool(h.Home, agent, kind, data, rid, at)
		}
		var err error
		if nudge, err = h.Forward(agent, kind, data, rid, at); errors.Is(err, ErrUnreached) {
			return map[string]any{}, AppendSpool(h.Home, agent, kind, data, rid, at)
		} else if err != nil {
			return nil, err
		}
	} else if err := h.apply(agent, kind, data, rid, at); err != nil {
		return nil, err
	}
	if kind == "PostToolUse" && h.Background != nil {
		h.Background(agent, data)
	}
	if nudge != "" {
		return map[string]any{"nudge": nudge}, nil
	}
	return map[string]any{}, nil
}

func (h *Recorder) apply(agent, kind string, data map[string]any, requestID string, at int64) error {
	if h.Forward != nil { // a spooled line; one hl spooled itself the tracker has not heard (a heard one moves nothing again)
		if h.Turns != nil {
			h.Turns.Hook(agent, kind, data, at)
		}
		_, err := h.Forward(agent, kind, data, requestID, at)
		if err != nil && !errors.Is(err, ErrUnreached) { // the master refused it: sending it again changes nothing
			h.Log("  spool: the master refused %s's %s: %v", agent, kind, err)
			return nil
		}
		return err
	}
	_, noted := Notified(h.Home, agent)
	if err := Apply(h.DB, h.Home, agent, kind, data, requestID, at); err != nil {
		return err
	}
	if h.Turns != nil && (h.Elsewhere == nil || !h.Elsewhere(agent)) {
		h.Turns.Hook(agent, kind, data, at)
		// A Notification raises the needs-you card and the call run after a person answered takes it away, neither moving the turn.
		if _, still := Notified(h.Home, agent); still != noted {
			h.Turns.Refresh(agent)
		}
	}
	if h.TurnEnded != nil && (kind == "Stop" || kind == "StopFailure" || kind == "SessionEnd") {
		h.TurnEnded(agent, kind)
	}
	return nil
}
