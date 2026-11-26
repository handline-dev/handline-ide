package proposalwrites

import (
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

// renameTarget is the name agent.rename hands the roster for agent's new word to, and whether it is the name the agent has already. A
// lead is renamed like any agent; its workspace keeps its name. A word that is not a free name in its workspace is refused here, before a
// running session is touched.
func (e *Env) renameTarget(agent, to string) (qualified string, same bool, err error) {
	_, id, _ := actor.Parse(agent)
	var ws int64
	var name string
	if err := e.DB.QueryRow(`SELECT workspace, name FROM agents WHERE id = ?`, id).Scan(&ws, &name); err != nil {
		return "", false, err
	}
	if to == name {
		return to, true, nil
	}
	if err := names.Check(e.DB, names.Agent, to, ws); err != nil {
		var bad *names.Refusal
		if errors.As(err, &bad) {
			return "", false, rpc.Refused(bad.Msg, "state", "")
		}
		return "", false, err
	}
	return to, false, nil
}
