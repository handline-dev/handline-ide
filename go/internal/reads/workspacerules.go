package reads

import (
	"database/sql"
	"errors"
	"io/fs"
	"os"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

// workspaceRules is workspace.rules: the kept rules in force for a workspace (its own beating the machine's), or the machine's alone.
func (e *Env) workspaceRules(c *rpc.Conn, params map[string]any) (any, error) {
	ws := int64(team.MachineRules)
	who, named := params["workspace"].(string)
	if named {
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
	res := map[string]any{"rules": out}
	if named {
		size := int64(0)
		if st, err := os.Stat(team.RulesFile(e.Home, actor.Key(actor.Workspace, ws))); err == nil {
			size = st.Size()
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		res["text_bytes"], res["limit_bytes"] = size, team.RulesTextLimit
	}
	return res, nil
}
