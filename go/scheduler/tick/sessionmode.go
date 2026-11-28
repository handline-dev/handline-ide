package tick

import (
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

// ModeApplied is the meta key prefix that keeps the mode a switch restarted a session for: a launch that still runs the other way after it
// is not restarted again for the same mode. agent.open_terminal sets it too, before its own restart, so the two never restart one session.
const ModeApplied = "session_mode_applied:"

// ModePorts are what moving a session between Chat and Terminal asks.
type ModePorts interface {
	// RunsChat is whether the agent's running session is a Chat one; known false when that cannot be told.
	RunsChat(agent string) (chat, known bool)
	// WantsChat is whether it should be (lifecycle's WantsChat); err when that cannot be read.
	WantsChat(agent string) (bool, error)
	CurrentAccount(q db.Q, agent string) string
}

// SessionMode moves a running session to the mode its setting or Open terminal asks, between turns: it is closed at its prompt and resumed
// in the same conversation the other way. Never two processes on one conversation.
type SessionMode struct{ Ports ModePorts }

// Sweep moves every agent whose session runs the other way; one agent's failure is not the tick's.
func (m *SessionMode) Sweep(t *Tick, now int64) error {
	for _, agent := range t.Host.Agents() {
		if err := guard(func() { m.move(t, agent) }); err != nil {
			t.logf("  %s: moving its session between Chat and Terminal failed: %v", agent, err)
		}
	}
	return nil
}

func modeWord(chat bool) string {
	if chat {
		return "chat"
	}
	return "terminal"
}

func (m *SessionMode) move(t *Tick, agent string) {
	row := must(db.GetAgentState(t.Q, agent))
	if row == nil || closedAt(row) || row.State != states.Idle || !t.Host.SessionAlive(agent) {
		return
	}
	chat, known := m.Ports.RunsChat(agent)
	if !known {
		return
	}
	want, err := m.Ports.WantsChat(agent)
	if err != nil {
		return // a read that failed is no reason to move a session
	}
	applied, _ := must2(db.Meta(t.Q, ModeApplied+agent))
	if chat == want {
		if applied != "" {
			check(db.SetMeta(t.Q, ModeApplied+agent, ""))
		}
		return
	}
	if applied == modeWord(want) || !t.Delivery.SessionPresent(agent) || !t.Host.SessionIdle(agent) || t.Delivery.HasBackgroundWork(t.Q, agent) {
		return // a launch that could not give it, or not between turns yet
	}
	st := t.Usage.ReadForAgent(agent, m.Ports.CurrentAccount(t.Q, agent))
	if st.SessionID == "" {
		return // no conversation to resume in the other mode
	}
	ok, said := t.Lifecycle.RestartAgent(t.Q, agent, st, "daemon", nil, "session_mode", false)
	if !ok {
		t.logf("  %s: not moved to %s yet - %s", agent, modeWord(want), said)
		return
	}
	check(db.SetMeta(t.Q, ModeApplied+agent, modeWord(want)))
	t.logf("  %s: moved to %s, resuming its conversation", agent, modeWord(want))
}
