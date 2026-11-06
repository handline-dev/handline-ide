package access

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// Emit puts one event on a topic (stream_log), for access.changed.
type Emit func(topic, typ string, data map[string]any) error

// Register adds access.grant, access.revoke and access.list. live gives the agent's running session a folder at once and says whether it
// did (nil never does); a grant it did not give applies from the next session.
func Register(srv *rpc.Server, home string, emit Emit, live func(agent, dir string) bool) {
	srv.Register("access.grant", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		agent, path, err := agentPath(home, p)
		if err != nil {
			return nil, err
		}
		scope, _ := p["scope"].(string)
		if scope != "once" && scope != "always" {
			return nil, rpc.BadRequestError("scope is once or always", "scope", rpc.InvalidParams)
		}
		st, err := os.Stat(path)
		if err != nil {
			e := rpc.NotFoundError("no file or folder at " + path)
			e.Data = map[string]any{"what": "path", "name": path}
			return nil, e
		}
		out, err := changed(c, emit, agent, func(tx *sql.Tx) (map[string]any, error) {
			g, err := Put(tx, agent, path, scope, time.Now().Unix())
			return map[string]any{"accepted": true, "grant": g.Map(), "applies_from": "next_session"}, err
		})
		// Kept first, then typed: a write that fails leaves no folder given without its grant. /add-dir takes a folder only.
		if err != nil || live == nil || !st.IsDir() {
			return out, err
		}
		db := c.Server().DB
		if ok, err := Running(db, agent); err != nil || !ok || !live(agent, path) {
			return out, err
		}
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if err := GivenNow(tx, agent, path); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		out["applies_from"] = "now"
		return out, nil
	}})
	srv.Register("access.revoke", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		agent, path, err := agentPath(home, p)
		if err != nil {
			return nil, err
		}
		return changed(c, emit, agent, func(tx *sql.Tx) (map[string]any, error) {
			removed, err := Remove(tx, agent, path)
			return map[string]any{"accepted": true, "removed": removed}, err
		})
	}})
	srv.Register("access.list", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		agent, err := agentNamed(home, p)
		if err != nil {
			return nil, err
		}
		gs, err := Grants(c.Server().DB, agent)
		if err != nil {
			return nil, err
		}
		return map[string]any{"grants": maps(gs)}, nil
	}})
}

// changed runs a grant change and says the agent's grants after it, as the change left them, on the agents topic.
func changed(c *rpc.Conn, emit Emit, agent string, fn func(tx *sql.Tx) (map[string]any, error)) (map[string]any, error) {
	var gs []Grant
	out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		out, err := fn(tx)
		if err == nil {
			gs, err = Grants(tx, agent)
		}
		return out, err
	})
	if err != nil {
		return nil, err
	}
	return out, emit("agents", "access.changed", map[string]any{"agent": agent, "grants": maps(gs)})
}

func agentNamed(home string, p map[string]any) (string, error) {
	name, _ := p["agent"].(string)
	if name == "" {
		return "", rpc.BadRequestError("agent is an agent's name", "agent", rpc.InvalidParams)
	}
	for _, n := range paths.ListAgents(home) {
		if n == name {
			return name, nil
		}
	}
	e := rpc.NotFoundError("no such agent: " + name)
	e.Data = map[string]any{"what": "agent", "name": name}
	return "", e
}

func agentPath(home string, p map[string]any) (string, string, error) {
	path, _ := p["path"].(string)
	if !strings.HasPrefix(path, "/") || len(path) > 4096 || filepath.Clean(path) != path || textsafe.Oneline(path) != path {
		return "", "", rpc.BadRequestError("path is an absolute path on one line, with no .. part", "path", rpc.InvalidParams)
	}
	agent, err := agentNamed(home, p)
	return agent, path, err
}

// Map is the grant as the contract's Grant.
func (g Grant) Map() map[string]any {
	return map[string]any{"path": g.Path, "scope": g.Scope, "at": time.Unix(g.At, 0).UTC().Format(time.RFC3339)}
}

func maps(gs []Grant) []map[string]any {
	out := make([]map[string]any, len(gs))
	for i, g := range gs {
		out[i] = g.Map()
	}
	return out
}
