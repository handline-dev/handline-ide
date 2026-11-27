// Package deskrpc serves the worktree pool's desks: who holds each, who had it last, and its pin, which the founder sets.
package deskrpc

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/worktreeprune"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/worktreepool"
)

// Register serves desks.list and desk.pin.
func Register(s *rpc.Server) {
	s.Register("desks.list", rpc.Method{Handler: desksList, Read: true})
	s.Register("desk.pin", rpc.Method{Handler: deskPin})
}

func desksList(c *rpc.Conn, p map[string]any) (any, error) {
	q := c.Server().DB
	repo, _ := p["repo"].(string)
	folders, err := worktreepool.List(q, repo)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, f := range folders {
		var size, task any
		if v, ok, err := db.Meta(q, worktreeprune.SizePrefix+f.Path); err == nil && ok {
			size, _ = strconv.ParseInt(v, 10, 64)
		}
		if f.Holder != "" {
			var t sql.NullInt64
			if err := q.QueryRow("SELECT current_task FROM agent_state WHERE agent = ?", f.Holder).Scan(&t); err == nil && t.Int64 > 0 {
				task = t.Int64
			}
		}
		out = append(out, map[string]any{"path": f.Path, "name": filepath.Base(f.Path), "repo": f.Repo, "holder": f.Holder,
			"last_agent": f.LastAgent, "last_used": f.LastUsed, "pinned_to": f.PinnedTo, "size_bytes": size, "task": task})
	}
	return map[string]any{"desks": out}, nil
}

// deskPin keeps a desk for one agent or the founder, or lets it go to anyone (to empty). desk is its path or its name (desk-3).
func deskPin(c *rpc.Conn, p map[string]any) (any, error) {
	desk, _ := p["desk"].(string)
	if desk == "" {
		return nil, rpc.BadRequestError("desk is a desk's path or name", "desk", rpc.InvalidParams)
	}
	to, _ := p["to"].(string)
	pin := ""
	switch {
	case to == "":
	case actor.IsPerson(to):
		pin = worktreepool.Founder
	case actor.KindOf(to) == actor.Agent:
		pin = to
	default:
		return nil, rpc.BadRequestError("to names an agent or the founder, or is left out to unpin", "to", rpc.InvalidParams)
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		path, err := worktreepool.Find(tx, desk)
		if err != nil {
			return nil, rpc.BadRequestError(err.Error(), "desk", rpc.InvalidParams)
		}
		if err := worktreepool.Pin(tx, path, pin); err != nil {
			return nil, err
		}
		if pin == "" {
			return said(fmt.Sprintf("%s goes to any agent", filepath.Base(path)), nil), nil
		}
		who := "the founder"
		if pin != worktreepool.Founder {
			who = addr.Ref(pin)
		}
		return said(fmt.Sprintf("%s is kept for %s", filepath.Base(path), who), nil), nil
	})
}

func said(text string, _ map[string]any) map[string]any {
	return map[string]any{"accepted": true, "said": text}
}
