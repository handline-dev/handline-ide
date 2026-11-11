package team

import (
	"fmt"
	"sort"
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

// Until the who columns hold keys (base-v2 1b), the rest of the code names an agent by its old dotted name: its workspace's name for
// the lead, else workspace.agent. LegacyName, SplitLegacy and paths.LegacyNameSQL are the only places that know that form.

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
	ReportsTo            string // the key of the agent it reports to; "" when it reports to a person or to no one
	Role                 string
}

// Team is the workspaces and their agents, read from the rows.
type Team struct {
	Agents        map[string]Member   // by key (by old name in a FromNames team)
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
	rows, err := q.Query(`SELECT a.id, a.name, w.name, COALESCE(w.lead = a.id, 0), COALESCE(a.reports_to, ''), a.role
		FROM agents a JOIN workspaces w ON w.id = a.workspace ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t := &Team{Agents: map[string]Member{}, Teams: map[string][]string{}, leads: map[string]string{}, MaxChainDepth: DefaultMaxChainDepth}
	for rows.Next() {
		var id int64
		var m Member
		if err := rows.Scan(&id, &m.Name, &m.Workspace, &m.Lead, &m.ReportsTo, &m.Role); err != nil {
			return nil, err
		}
		m.Key = actor.Key(actor.Agent, id)
		if actor.IsPerson(m.ReportsTo) {
			m.ReportsTo = ""
		}
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

// Managers is workspace name -> its lead.
func (t *Team) Managers() map[string]string {
	out := make(map[string]string, len(t.leads))
	for ws, n := range t.leads {
		out[ws] = n
	}
	return out
}

// ManagerOf is the lead of agent's workspace, or "" when it is in none or is that lead itself.
func (t *Team) ManagerOf(agent string) string {
	if lead := t.leads[t.TeamOf(agent)]; lead != agent {
		return lead
	}
	return ""
}

// SupervisorOf is the agent this one reports to: the one its row names, else its workspace's lead; "" for a lead that reports to a
// person.
func (t *Team) SupervisorOf(agent string) string {
	if t == nil {
		return ""
	}
	if m := t.Agents[agent]; m.ReportsTo != "" {
		return m.ReportsTo
	}
	return t.ManagerOf(agent)
}

// LeadOf is who agent answers to; "" for none.
func (t *Team) LeadOf(agent string) string { return t.SupervisorOf(agent) }

// AuditorOf is the agent named auditor in agent's workspace; "" when it has none.
func (t *Team) AuditorOf(agent string) string {
	if t == nil {
		return ""
	}
	for _, a := range t.Teams[t.TeamOf(agent)] {
		if a != agent && t.Agents[a].Name == "auditor" {
			return a
		}
	}
	return ""
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

// FromNames is the Team of agents known only by their old names and whom each reports to (an agent's old name, "" for a person), as
// team.get gives them to hl, which does not open the database.
func FromNames(parents map[string]string) *Team {
	t := &Team{Agents: map[string]Member{}, Teams: map[string][]string{}, leads: map[string]string{}, MaxChainDepth: DefaultMaxChainDepth}
	for n, parent := range parents {
		ws, agent := SplitLegacy(n)
		m := Member{Workspace: ws, Name: agent, Lead: agent == "", ReportsTo: parent}
		t.Agents[n] = m
		t.Teams[ws] = append(t.Teams[ws], n)
		t.order = append(t.order, n)
		if m.Lead {
			t.leads[ws] = n
		}
	}
	sort.Strings(t.order)
	for ws := range t.Teams {
		sort.Strings(t.Teams[ws])
	}
	return t
}
