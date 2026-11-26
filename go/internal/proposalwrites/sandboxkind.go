package proposalwrites

import (
	"fmt"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
)

// A sandbox proposal is an agent's ask for one entry of its sandbox_allow list (`hl propose sandbox <kind> [<value>]`): the founder's
// OK adds that entry on the day of the OK and keeps every other.

// sandboxAskProblem is why sets is not one entry agent may ask for, or "".
func (e *Env) sandboxAskProblem(agent string, sets map[string]string) string {
	kind, value := sets["kind"], sets["value"]
	for k := range sets {
		if k != "kind" && k != "value" {
			return "a sandbox ask is one entry: kind and value"
		}
	}
	if why := team.SandboxEntryProblem(kind, value); why != "" {
		return why
	}
	cfg, err := e.roster().Read(agent)
	if err != nil {
		return err.Error()
	}
	if _, added := team.WithSandboxEntry(sandboxText(cfg), kind, value, time.Now()); !added {
		return fmt.Sprintf("%s may already %s", addr.Ref(agent), team.SandboxEntry{Kind: kind, Value: value}.Phrase())
	}
	return ""
}

func sandboxText(cfg team.Config) string {
	v, _ := cfg["sandbox_allow"].(string)
	return v
}

// doSandbox adds the asked entry to the agent's list, and for a temp to the list of the agent it was hired from too, which its next
// temps start with.
func (e *Env) doSandbox(p step) (bool, string, error) {
	agent, v := p.str("agent"), p.sets()
	if why := team.SandboxEntryProblem(v["kind"], v["value"]); why != "" {
		return false, why, nil
	}
	key, err := paths.AgentKeyIn(p.tx, agent)
	if err != nil {
		return false, "", err
	}
	if key == "" {
		return false, addr.Ref(agent) + " is no longer an agent", nil
	}
	targets := []string{agent}
	if from, err := paths.HiredFrom(p.tx, key); err != nil {
		return false, "", err
	} else if from != "" {
		targets = append(targets, from)
	}
	r := e.rosterIn(p)
	for _, a := range targets {
		cfg, err := r.Read(a)
		if err != nil {
			return false, "", err
		}
		next, added := team.WithSandboxEntry(sandboxText(cfg), v["kind"], v["value"], time.Now())
		if !added {
			continue
		}
		if ok, said := r.SaveSettings(a, []team.Change{{Field: "sandbox_allow", Old: sandboxText(cfg), New: next}}, idleClose{e, r}); !ok {
			return false, said, nil
		}
	}
	if err := db.SetMeta(p.tx, db.SettingsByMeta(key), p.str("requested_by")); err != nil { // the restart that applies it names the proposer
		return false, "", err
	}
	return true, fmt.Sprintf("%s may %s from its next fresh session", addr.Ref(agent), team.SandboxEntry{Kind: v["kind"], Value: v["value"]}.Phrase()), nil
}
