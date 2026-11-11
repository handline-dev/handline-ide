package reads

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

// Ident is who an agent is, as its view says it: its name within its workspace, the workspace's key and its owner's key.
type Ident struct{ Name, Workspace, Owner string }

// Idents is every agent's Ident by its key.
func Idents(q db.Q) (map[string]Ident, error) {
	rs, err := q.Query("SELECT id, name, workspace, owner FROM agents")
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[string]Ident{}
	for rs.Next() {
		var id, ws int64
		var it Ident
		if err := rs.Scan(&id, &it.Name, &ws, &it.Owner); err != nil {
			return nil, err
		}
		it.Workspace = actor.Key(actor.Workspace, ws)
		out[actor.Key(actor.Agent, id)] = it
	}
	return out, rs.Err()
}

// WithIdent turns the stored view of the agent whose key is key into the wire's: key, and the agent's name, workspace and owner. An
// agent with no row keeps its key as its name.
func WithIdent(view map[string]any, key string, ids map[string]Ident) {
	it, ok := ids[key]
	if !ok {
		it.Name = key
	}
	view["key"], view["name"], view["workspace"], view["owner"] = key, it.Name, it.Workspace, it.Owner
}

// peopleList is people.list: every person, by key.
func (e *Env) peopleList(c *rpc.Conn, params map[string]any) (any, error) {
	rs, err := e.DB.Query("SELECT id, display_name, handle, role FROM people ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	people := []any{}
	for rs.Next() {
		var id int64
		var name, role string
		var handle sql.NullString
		if err := rs.Scan(&id, &name, &handle, &role); err != nil {
			return nil, err
		}
		var h any
		if handle.Valid {
			h = handle.String
		}
		people = append(people, map[string]any{"key": actor.Key(actor.Person, id), "display_name": name, "handle": h, "role": role})
	}
	return map[string]any{"people": people}, rs.Err()
}

// workspacesList is workspaces.list: every workspace, by key, with its lead's key.
func (e *Env) workspacesList(c *rpc.Conn, params map[string]any) (any, error) {
	rs, err := e.DB.Query("SELECT id, name, folder, added_folders, lead FROM workspaces ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []any{}
	for rs.Next() {
		var id int64
		var name, folder, added string
		var lead sql.NullInt64
		if err := rs.Scan(&id, &name, &folder, &added, &lead); err != nil {
			return nil, err
		}
		folders := []string{}
		if err := json.Unmarshal([]byte(added), &folders); err != nil {
			return nil, fmt.Errorf("workspace %s: added_folders: %w", name, err)
		}
		var l any
		if lead.Valid {
			l = actor.Key(actor.Agent, lead.Int64)
		}
		out = append(out, map[string]any{"key": actor.Key(actor.Workspace, id), "name": name, "folder": folder, "added_folders": folders, "lead": l})
	}
	return map[string]any{"workspaces": out}, rs.Err()
}
