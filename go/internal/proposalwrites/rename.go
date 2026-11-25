package proposalwrites

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/yamlcompat"
	"github.com/handline/handline/go/protocol/rpc"
)

// renameMethod is agent.rename: the founder renames an agent by the same step an approved rename proposal runs (doRename), and the call
// answers only with how it ended. One with a session running is held as the founder's approved proposal applied now: its turn is
// interrupted, its session closes, the rename runs and the agent starts under its new name; the call waits for that, up to Wait.
func (e *Env) renameMethod(c *rpc.Conn, params map[string]any) (any, error) {
	agent, _ := params["agent"].(string)
	if agent == "" {
		return nil, rpc.BadRequestError("agent is an agent's name", "agent", rpc.InvalidParams)
	}
	to, _ := params["to"].(string)
	to = strings.TrimSpace(to)
	if !slices.Contains(paths.ListAgents(e.Home), agent) {
		err := rpc.NotFoundError("no such agent: " + addr.Ref(agent))
		err.Data = map[string]any{"what": "agent", "name": agent}
		return nil, err
	}
	if from, err := paths.HiredFrom(e.DB, agent); err != nil {
		return nil, err
	} else if from != "" {
		return nil, rpc.Refused(addr.Ref(agent)+" is a seat lent from "+addr.Ref(from)+": rename that agent, not the seat.", "state", "")
	}
	qualified, same, err := e.renameTarget(agent, to)
	if err != nil {
		return nil, err
	}
	if same {
		return e.retired(c, true, fmt.Sprintf("%s is named %s already.", addr.Ref(agent), to))
	}
	live, ok := map[string]bool(nil), false
	if e.Live != nil {
		live, ok = e.Live()
	}
	if !ok {
		return nil, rpc.Refused("could not confirm whether "+addr.Ref(agent)+" has a session running just now - try again in a moment.", "state", "")
	}
	if live[agent] {
		prop, err := newProp("rename", agent, c.Caller.Actor(), "renamed by the founder")
		if err != nil {
			return nil, err
		}
		prop.set("to", qualified)
		prop.set("apply_now", time.Now().Unix()) // its turn is interrupted, as the founder's Apply now does
		if err := e.recordDecision(prop.id(), "approved", prop.subject()); err != nil {
			return nil, err
		}
		got, done := e.await(prop.id())
		defer done()
		if err := e.hold(prop, []string{agent}); err != nil {
			return nil, err
		}
		if o, ended := e.waitHeld(prop.id(), got, c.Done(), e.Wait); ended {
			return e.retired(c, o.ok, o.said)
		}
		return nil, rpc.Refused(addr.Ref(agent)+" did not stop in time, so it was not renamed; rename it again once its turn ends.", "state", "")
	}
	prop := &Prop{pairs: []yamlcompat.Pair{{Key: "kind", Val: "rename"}, {Key: "agent", Val: agent}, {Key: "to", Val: qualified},
		{Key: "id", Val: fmt.Sprintf("%x", time.Now().UnixNano())}}}
	ok, said, err := e.run(prop)
	if err != nil {
		return nil, err
	}
	return e.retired(c, ok, said)
}

// renameTarget is the name agent.rename hands the roster for agent's new word to: workspace.word for an agent, the bare word for a
// workspace's lead (whose name is its workspace's), and whether it is the name the agent has already. Everything the rename would refuse is
// refused here, before a running session is touched: a word that is not a free name, or a lead whose workspace has other agents.
func (e *Env) renameTarget(agent, to string) (qualified string, same bool, err error) {
	_, id, _ := actor.Parse(agent)
	var ws, others int64
	var wsName, name string
	var lead sql.NullInt64
	if err := e.DB.QueryRow(`SELECT w.id, w.name, a.name, w.lead, (SELECT COUNT(*) FROM agents o WHERE o.workspace = w.id AND o.id != a.id)
		FROM agents a JOIN workspaces w ON w.id = a.workspace WHERE a.id = ?`, id).Scan(&ws, &wsName, &name, &lead, &others); err != nil {
		return "", false, err
	}
	isLead := lead.Valid && lead.Int64 == id
	kind, current, qualified := names.Agent, name, wsName+"."+to
	if isLead {
		kind, current, qualified = names.Workspace, wsName, to
		if others > 0 {
			return "", false, rpc.Refused(fmt.Sprintf("%s leads workspace %s, whose name is its own: a rename would rename the workspace, which has %d "+
				"other agent(s).", addr.Ref(agent), wsName, others), "state", "")
		}
	}
	if to == current {
		return qualified, true, nil
	}
	if err := names.Check(e.DB, kind, to, ws); err != nil {
		var bad *names.Refusal
		if errors.As(err, &bad) {
			return "", false, rpc.Refused(bad.Msg, "state", "")
		}
		return "", false, err
	}
	return qualified, false, nil
}
