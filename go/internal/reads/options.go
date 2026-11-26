package reads

import (
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// knownModels is agent_settings.known_models: the families usage/prices.py prices by name, which the claude CLI takes as they are.
var knownModels = []string{"fable", "mythos", "opus", "sonnet", "haiku"}

var validEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// optionsRole is rpc_options._role_line: the first line of ROLE.md that is not its own title, as one line of at most 200 characters.
func (e *Env) optionsRole(name string) string {
	b, err := team.AgentInstruction(e.DB, name, db.PartRole)
	if err != nil {
		return ""
	}
	var lines []string
	for _, ln := range strings.Split(b, "\n") {
		if strings.TrimSpace(ln) != "" {
			lines = append(lines, strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(ln), "#")))
		}
	}
	if e.DB != nil {
		if _, n, err := paths.AgentNamed(e.DB, name); err == nil && n != "" {
			name = n // the title names the agent, not its key
		}
	}
	pick := ""
	if len(lines) > 1 && strings.HasPrefix(strings.ToLower(lines[0]), strings.ToLower(name)) {
		pick = lines[1]
	} else if len(lines) > 0 {
		pick = lines[0]
	}
	if r := []rune(pick); len(r) > 200 {
		pick = string(r[:200])
	}
	return pick
}

// defaultNames is the name an agent made with no name typed would get in each workspace (names.Plain), by workspace key.
func (e *Env) defaultNames() (map[string]any, error) {
	out := map[string]any{}
	if e.DB == nil {
		return out, nil
	}
	rows, err := e.DB.Query("SELECT id FROM workspaces ORDER BY id")
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		n, err := names.Free(e.DB, names.Agent, id, names.Plain...)
		if err != nil {
			return nil, err
		}
		out[actor.Key(actor.Workspace, id)] = n
	}
	return out, nil
}

// agentOptions is agent.options: what the new-agent form offers.
func (e *Env) agentOptions(_ *rpc.Conn, _ map[string]any) (any, error) {
	roles := []any{}
	for _, n := range paths.ListAgents(e.Home) {
		roles = append(roles, map[string]any{"name": n, "description": e.optionsRole(n)})
	}
	accts := []any{}
	for _, a := range paths.ListAccounts(e.Home) {
		in, known := e.loginState(a)
		accts = append(accts, map[string]any{"name": a, "login_state": loginName(in, known)})
	}
	defaults, err := e.defaultNames()
	if err != nil {
		return nil, err
	}
	return map[string]any{"repos": e.repos(), "roles": roles, "models": strs(knownModels), "efforts": strs(validEfforts),
		"accounts": accts, "default_names": defaults}, nil
}

func strs(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func loginName(in, known bool) string {
	switch {
	case !known:
		return "unknown"
	case in:
		return "ok"
	}
	return "needs_login"
}
