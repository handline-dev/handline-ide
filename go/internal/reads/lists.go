package reads

import (
	"database/sql"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/protocol/rpc"
)

// instructionInfo is a team.get role_file or base_file: no path (the text is in the database) and when its version in force was written,
// null for none.
func (e *Env) instructionInfo(key, part string) map[string]any {
	var updated any
	if in, err := db.CurrentInstruction(e.DB, key, part); err == nil && in != nil {
		updated = ts(in.At)
	}
	return map[string]any{"path": "", "updated": updated}
}

// teamGet is team.get: the agents of the caller's workspace (every agent for a person outside every workspace folder: the app), each
// with its files, the people, and the other workspaces. Each name is the caller's word for that key (addr.Name).
func (e *Env) teamGet(c *rpc.Conn, params map[string]any) (any, error) {
	r, err := addr.Of(e.DB, c.Caller.Key, c.Cwd())
	if err != nil {
		return nil, err
	}
	all := r.Workspace == 0 && actor.KindOf(r.Key) != actor.Agent
	rows, err := e.DB.Query("SELECT id FROM agents WHERE ? OR workspace = ? ORDER BY workspace, name", all, r.Workspace)
	if err != nil {
		return nil, err
	}
	var keys []string
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, actor.Key(actor.Agent, id))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	agents := []any{}
	for _, k := range keys {
		// reports_to stays null until clients stop reading it: agents are peers
		agents = append(agents, map[string]any{"key": k, "name": addr.Name(e.DB, r, k), "role": e.roleLine(k), "reports_to": nil,
			"role_file": e.instructionInfo(k, db.PartRole), "base_file": e.instructionInfo(k, db.PartBase)})
	}
	people := []any{}
	ids, err := e.DB.Query("SELECT id FROM people ORDER BY id")
	if err != nil {
		return nil, err
	}
	for ids.Next() {
		var id int64
		if err := ids.Scan(&id); err != nil {
			ids.Close()
			return nil, err
		}
		k := actor.Key(actor.Person, id)
		people = append(people, map[string]any{"key": k, "name": addr.Name(e.DB, r, k)})
	}
	ids.Close()
	if err := ids.Err(); err != nil {
		return nil, err
	}
	workspaces := []any{}
	if err := eachRow(e.DB, "SELECT id, COALESCE(lead, 0) FROM workspaces WHERE id != ? ORDER BY name", func(id, lead int64) {
		var l any
		if lead != 0 {
			l = actor.Key(actor.Agent, lead)
		}
		k := actor.Key(actor.Workspace, id)
		workspaces = append(workspaces, map[string]any{"key": k, "name": addr.Name(e.DB, r, k), "lead": l})
	}, r.Workspace); err != nil {
		return nil, err
	}
	return map[string]any{"agents": agents, "people": people, "workspaces": workspaces}, nil
}

// eachRow calls f with each row of query's two integer columns.
func eachRow(q *sql.DB, query string, f func(a, b int64), args ...any) error {
	rows, err := q.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return err
		}
		f(a, b)
	}
	return rows.Err()
}
