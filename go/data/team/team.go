package team

import (
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
)

// Routing reads the workspaces and agents tables (docs/design/people-workspaces.md, Addresses). An agent may reach every agent of
// its workspace, its owner and any workspace's lead; the founder may reach anyone. Decided here, with no model involved.

const (
	DefaultMaxChainDepth = 4
	// AskWindowS is how long a refused message sent again unchanged is the same refusal, not a new denied row.
	AskWindowS = 3600
)

// The roster, proposals and session creation name an agent by its dotted name: its workspace's name for the lead, else
// workspace.agent: LegacyName, SplitLegacy and paths.LegacyNameSQL.

// LegacyName is the old dotted name of agent name in workspace ws; lead is whether it is the workspace's lead.
func LegacyName(ws, name string, lead bool) string {
	if lead {
		return ws
	}
	return ws + "." + name
}

// SplitLegacy is the workspace and agent name an old dotted name stands for; agent is "" for a bare workspace name, the lead.
func SplitLegacy(legacy string) (ws, agent string) {
	ws, agent, _ = strings.Cut(legacy, ".")
	return ws, agent
}

// ParentName is the agent a name hangs under: harbor.sea -> harbor; "" for a name with no dot.
func ParentName(agent string) string {
	i := strings.LastIndex(agent, ".")
	if i <= 0 {
		return ""
	}
	return agent[:i]
}

// Decision is what routing says about one request: a link only allows a message.
type Decision struct {
	Allowed    bool
	Reason     string
	ChainDepth int
}

// Member is one agent row as routing reads it.
type Member struct {
	Key, Workspace, Name string
	Lead                 bool
	Role                 string
}

// Team is the workspaces and their agents, read from the rows.
type Team struct {
	Agents        map[string]Member   // by key
	Teams         map[string][]string // workspace name -> its agents, oldest first
	leads         map[string]string   // workspace name -> its lead
	order         []string            // every agent, oldest first
	MaxChainDepth int
}

// LoadTeam is ReadTeam of home's database; a home with none yet has no agents.
func LoadTeam(home string) (*Team, error) {
	d, err := store.SharedIfThere(paths.DBFile(home))
	if d == nil {
		if err != nil {
			return nil, err
		}
		return &Team{Agents: map[string]Member{}, Teams: map[string][]string{}, leads: map[string]string{}, MaxChainDepth: DefaultMaxChainDepth}, nil
	}
	return ReadTeam(d)
}

// ReadTeam reads the workspaces and agents tables.
func ReadTeam(q db.Q) (*Team, error) {
	rows, err := q.Query(`SELECT a.id, a.name, w.name, COALESCE(w.lead = a.id, 0), a.role
		FROM agents a JOIN workspaces w ON w.id = a.workspace ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t := &Team{Agents: map[string]Member{}, Teams: map[string][]string{}, leads: map[string]string{}, MaxChainDepth: DefaultMaxChainDepth}
	for rows.Next() {
		var id int64
		var m Member
		if err := rows.Scan(&id, &m.Name, &m.Workspace, &m.Lead, &m.Role); err != nil {
			return nil, err
		}
		m.Key = actor.Key(actor.Agent, id)
		n := m.Key
		t.Agents[n] = m
		t.Teams[m.Workspace] = append(t.Teams[m.Workspace], n)
		if m.Lead {
			t.leads[m.Workspace] = n
		}
		t.order = append(t.order, n)
	}
	return t, rows.Err()
}

// Members is the agents of a workspace.
func (t *Team) Members(group string) []string { return append([]string(nil), t.Teams[group]...) }

// TeamOf is the workspace an agent is in; "" when it is not an agent.
func (t *Team) TeamOf(agent string) string { return t.Agents[agent].Workspace }

// Known is whether the target is routable: an agent, or the founder.
func (t *Team) Known(agent string) bool {
	_, ok := t.Agents[agent]
	return ok || agent == actor.OwnerKey
}

// ContactOf is the contact of workspace ws, its lead: work from another workspace comes to it. "" when ws has none.
func (t *Team) ContactOf(ws string) string {
	if t == nil {
		return ""
	}
	return t.leads[ws]
}

// Task roles (agents.role): a check agent's tasks gate other work (audits, landings); every other agent does work.
const (
	RoleWork  = "work"
	RoleCheck = "check"
)

// TaskRole is the role of the tasks agent owns.
func (t *Team) TaskRole(agent string) string {
	if t != nil && t.Agents[agent].Role == RoleCheck {
		return RoleCheck
	}
	return RoleWork
}

// Route decides a request.
func (t *Team) Route(requester, target string, chainDepth int) Decision {
	if requester == target {
		return Decision{Reason: "an agent cannot ask itself"}
	}
	if !t.Known(target) {
		return Decision{Reason: fmt.Sprintf("unknown target '%s'", target)}
	}
	if chainDepth >= t.MaxChainDepth {
		return Decision{Reason: fmt.Sprintf("chain depth %d reached the limit %d (loop guard)", chainDepth, t.MaxChainDepth)}
	}
	allow := func(why string) Decision { return Decision{Allowed: true, Reason: why, ChainDepth: chainDepth} }
	to := t.Agents[target]
	switch {
	case requester == actor.OwnerKey:
		return allow("the founder reaches anyone")
	case target == actor.OwnerKey:
		return allow(requester + " may ask its owner")
	case to.Workspace == t.TeamOf(requester):
		return allow(requester + " and " + target + " are in workspace " + to.Workspace)
	case to.Lead:
		return allow(target + " leads workspace " + to.Workspace)
	}
	return Decision{Reason: fmt.Sprintf("denied: %s is in another workspace (%s): send it to that workspace, whose lead hands it on", target, to.Workspace)}
}
