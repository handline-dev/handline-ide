package lifecycle

import (
	"strings"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
)

// leftRunner is a runner a wake left (node, why), or one it waits for (wait): away, but not long enough to give up its conversation.
type leftRunner struct {
	node, why string
	wait      bool
}

// leaveRunner drops a closed agent's terminal from the master's records when it is on a runner Mac a wake should start it elsewhere than
// now (RunnersPort.LeaveFor), so the wake places it again: its session there is released as a reclaim releases it. The zero value when its
// terminal is not on a runner, the runner can start it, or the release failed (the next wake asks again); wait when the runner is away
// but not yet past the time it is waited for.
func (m *Manager) leaveRunner(agent, wt string) leftRunner {
	if m.Runners == nil {
		return leftRunner{}
	}
	handle := must(db.Terminal(m.Q, agent))
	node, _, ok := term.OnRunner(handle)
	if !ok {
		return leftRunner{}
	}
	why, wait := m.Runners.LeaveFor(agent, node)
	if why == "" {
		return leftRunner{node: node, wait: wait}
	}
	if err := m.Runners.Release(agent, node, why); err != nil {
		m.Log("  %s: %v; it stays on %s", agent, err, m.Runners.NodeName(node))
		return leftRunner{}
	}
	check(db.SetTerminal(m.Q, agent, "", wt))
	check(db.CloseAgentTerminal(m.Q, handle))
	check(db.ClearNode(m.Q, agent))
	must(db.AddEvent(m.Q, agent, "terminal_gone", db.KV{K: "handle", V: handle}, db.KV{K: "why", V: why}))
	m.Log("  %s: left its terminal %s on %s (%s); placed again", agent, handle, m.Runners.NodeName(node), why)
	return leftRunner{node: node, why: why}
}

// dropGonePane forgets a runner terminal a launch found gone (its pane or the runner's tmux server), so the next attempt opens a new one
// where placement puts it; the session stays placed on the runner, its conversation there.
func (m *Manager) dropGonePane(agent, wt, why string) bool {
	handle := must(db.Terminal(m.Q, agent))
	if _, _, ok := term.OnRunner(handle); !ok || !(strings.Contains(why, "is not alive") || strings.Contains(why, "no server running")) {
		return false
	}
	check(db.SetTerminal(m.Q, agent, "", wt))
	check(db.CloseAgentTerminal(m.Q, handle))
	must(db.AddEvent(m.Q, agent, "terminal_gone", db.KV{K: "handle", V: handle}, db.KV{K: "why", V: clip(why, 160)}))
	return true
}

// waitingForRunner is a wake's answer while the agent's runner is away: it counts toward no stall, and the next tick asks again.
func (m *Manager) waitingForRunner(agent string, left leftRunner) string {
	why := "waiting for " + m.Runners.NodeName(left.node) + " to come back, which has its conversation"
	m.Log("  %s: %s", agent, why)
	return why
}

// startedElsewhere records where a wake that left a runner started the agent, for its chat; nothing when it is back on that runner.
func (m *Manager) startedElsewhere(agent string, left leftRunner) {
	from := m.Runners.NodeName(left.node)
	to := m.Runners.NodeName(must(db.AgentNode(m.Q, agent)))
	if from == to {
		return
	}
	reason := from + " " + strings.TrimPrefix(left.why, "it ")
	if !strings.HasPrefix(left.why, "it ") {
		reason = from + ": " + left.why
	}
	must(db.AddEvent(m.Q, agent, db.MachineMovedKind, db.KV{K: "from", V: from}, db.KV{K: "to", V: to}, db.KV{K: "reason", V: reason}))
	m.refreshView(agent)
}

// StartElsewhere starts agent now where placement puts it, its session left on the runner Mac it was on: that machine has gone silent.
// release also drops the runner's rows for it (a person's press, person); a reclaim has dropped them already. It starts fresh from its
// handoff, as a lost session does: its conversation stays on that machine. The answer says where it started, or why it did not.
func (m *Manager) StartElsewhere(agent, why string, release, person bool) (ok bool, said string, err error) {
	defer catch(&err)
	if m.Runners == nil {
		return false, "its session is not on another machine", nil
	}
	unlock, mine := m.Typing.TakeTerminal(agent)
	if !mine {
		return false, TypingElsewhere + "; not starting twice", nil
	}
	defer unlock()
	handle := must(db.Terminal(m.Q, agent)) // read under the lock: a wake that held it may have moved the agent already
	node, _, onRunner := term.OnRunner(handle)
	if !onRunner {
		return false, "its session is not on another machine", nil
	}
	if release {
		if err := m.Runners.Release(agent, node, why); err != nil {
			return false, "", err
		}
	}
	row := must(db.GetAgentState(m.Q, agent))
	wt := ""
	if row != nil {
		wt = m.keptWorktree(row)
	}
	check(db.SetTerminal(m.Q, agent, "", wt))
	check(db.CloseAgentTerminal(m.Q, handle))
	check(db.ClearNode(m.Q, agent))
	must(db.AddEvent(m.Q, agent, "terminal_gone", db.KV{K: "handle", V: handle}, db.KV{K: "why", V: why}))
	if !isClosed(row) {
		m.recordClosed(agent, "", SessionStatus{}) // its conversation is on that machine: nothing to resume here
	}
	ok, said = m.wakeAgent(agent, WakeOpts{Person: person, HeldWork: true, Cause: "machine_offline"})
	if !ok {
		return false, said, nil
	}
	m.startedElsewhere(agent, leftRunner{node: node, why: why})
	return true, "started on " + m.Runners.NodeName(must(db.AgentNode(m.Q, agent))), nil
}
