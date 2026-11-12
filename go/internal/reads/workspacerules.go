package reads

import (
	"database/sql"
	"errors"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

// workspaceRules is workspace.rules: the kept rules in force for a workspace (its own beating the machine's), or the machine's alone.
func (e *Env) workspaceRules(c *rpc.Conn, params map[string]any) (any, error) {
	ws := int64(team.MachineRules)
	if who, named := params["workspace"].(string); named {
		q, arg := "SELECT id FROM workspaces WHERE name = ?", any(who)
		if k, n, ok := actor.Parse(who); ok && k == actor.Workspace {
			q, arg = "SELECT id FROM workspaces WHERE id = ?", n
		}
		if err := e.DB.QueryRow(q, arg).Scan(&ws); errors.Is(err, sql.ErrNoRows) {
			nf := rpc.NotFoundError("no workspace " + who)
			nf.Data = map[string]any{"what": "workspace", "name": who}
			return nil, nf
		} else if err != nil {
			return nil, err
		}
	}
	rules, err := team.KeptRules(e.DB, ws)
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, r := range rules {
		out = append(out, map[string]any{"key": r.Key, "value": r.Value, "on": team.RuleOn(r.Value), "source": r.Source, "set_by": r.SetBy, "set_at": r.SetAt})
	}
	return map[string]any{"rules": out}, nil
}
