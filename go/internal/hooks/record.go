package hooks

import (
	"database/sql"
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
	Subagent func(agent, session string) (string, error)
	// TurnEnded is told each Stop, StopFailure and SessionEnd once it is applied.
	TurnEnded func(agent, kind string)
}

// SpoolEvery is how often the spool is looked at while the daemon runs, as the Python daemon does each tick.
const SpoolEvery = 2 * time.Second

// Register adds hook.record and subagent.start to the server's table.
func (h *Recorder) Register(s *rpc.Server) {
	s.Register("hook.record", rpc.Method{Handler: h.record, OwnEvent: true})
	s.Register("subagent.start", rpc.Method{Handler: h.subagentStart, OwnEvent: true})
}

// subagentStart is the PreToolUse guard's question before an Agent call: {"deny": why} when one more subagent would take the agent's
// account to a limit before it resets, else the start is recorded (usage.Env.StartSubagent) and {} lets it through.
func (h *Recorder) subagentStart(c *rpc.Conn, params map[string]any) (any, error) {
	agent, ok := params["agent"].(string)
	if !ok || agent == "" {
		return nil, rpc.BadRequestError("agent is a name", "agent", rpc.InvalidParams)
	}
	session, _ := params["session_id"].(string)
	if err := c.RequireSelf(agent); err != nil {
		return nil, err
	}
	if h.Subagent == nil {
		return map[string]any{}, nil
	}
	why, err := h.Subagent(agent, session)
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
	if err := h.apply(agent, kind, data, params["request_id"].(string), at); err != nil {
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
	_, noted := Notified(h.Home, agent)
	if err := Apply(h.DB, h.Home, agent, kind, data, requestID, at); err != nil {
		return err
	}
	if h.Turns != nil {
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
