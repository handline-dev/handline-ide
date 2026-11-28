package compose

import (
	"fmt"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/sessionrpc"
	"github.com/handline/handline/go/scheduler/settings"
)

// reclaimed starts again each agent the reclaim took back that still holds a task or has requests waiting, when its restart_reclaimed
// switch is on: where placement puts it now, fresh from its handoff (start: lifecycle's StartElsewhere, the runner's rows already gone).
func (p *Parts) reclaimed(reg *nodes.Registry, got []nodes.Reclaimed, start func(agent, why string) (bool, string, error), log func(string, ...any)) {
	for _, r := range got {
		if !p.LC.Manager.Settings.AgentFlag(r.Agent, settings.RestartReclaimedKey, false) {
			continue
		}
		depth, err := db.QueueDepth(p.db, r.Agent)
		if err != nil {
			log("  %s: not started again after the reclaim: %v", r.Agent, err)
			continue
		}
		if r.Task == 0 && depth == 0 {
			continue
		}
		ok, said, err := start(r.Agent, "it is offline")
		switch {
		case err != nil:
			log("  %s: not started again after the reclaim: %v", r.Agent, err)
		case !ok:
			log("  %s: not started again after the reclaim - %s", r.Agent, said)
		default:
			log("  %s: %s after the reclaim", r.Agent, said)
		}
	}
}

// restartHere is agent.restart_here: the agent's machine is silent (offline or not linked), so its rows there are dropped and it starts now
// where placement puts it. Refused while that machine is online and linked.
func (c controls) restartHere(_ sessionrpc.Call, agent string) (bool, string, error) {
	reg := c.nodes
	handle, err := db.Terminal(c.p.db, agent)
	if err != nil {
		return false, "", err
	}
	node, _, onRunner := term.OnRunner(handle)
	if reg == nil || !onRunner {
		return false, "its session is not on another machine", nil
	}
	since, silent := reg.Silent(node)
	if !silent {
		return false, reg.NodeName(node) + " is online and linked: it runs there", nil
	}
	why := fmt.Sprintf("it is silent since %s; started here on a person's press", since.Format("01-02 15:04"))
	return c.lc.Manager.StartElsewhere(agent, why, true, true)
}

// heldOn is AgentView.held_on: the other machine the agent's session is on, since when it is silent, and whether agent.restart_here is
// open now; nil on this machine.
func (p *Parts) heldOn(reg *nodes.Registry, agent string) any {
	if reg == nil {
		return nil
	}
	handle, err := db.Terminal(p.db, agent)
	node, _, onRunner := term.OnRunner(handle)
	if err != nil || !onRunner {
		return nil
	}
	since, silent := reg.Silent(node)
	var at any
	if silent {
		at = since.Unix()
	}
	return map[string]any{"node": node, "machine": reg.NodeName(node), "silent_since": at, "restart_here": silent}
}
