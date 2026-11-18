// Package deskrpc serves the worktree pool's desks: who holds each, who had it last, and its pin, which the founder sets.
package deskrpc

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/worktreeprune"
	"github.com/handline/handline/go/kit/protected"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/worktreepool"
)

// Register serves desks.list, desk.pin and desks.keep; home holds the protected store desks.keep writes.
func Register(s *rpc.Server, home string) {
	s.Register("desks.list", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) { return desksList(c, p, home, time.Now()) }, Read: true})
	s.Register("desk.pin", rpc.Method{Handler: deskPin})
	s.Register("desks.keep", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) { return desksKeep(c, p, home) }})
}

func desksList(c *rpc.Conn, p map[string]any, home string, now time.Time) (any, error) {
	q := c.Server().DB
	repo, _ := p["repo"].(string)
	folders, err := worktreepool.List(q, repo)
	if err != nil {
		return nil, err
	}
	var kept any
	tonight := map[string]bool{}
	if repo != "" {
		k := worktreepool.GetKept(protected.New(home), repo)
		agents, err := worktreepool.Agents(q, repo)
		if err != nil {
			return nil, err
		}
		auto, ok, err := worktreepool.AutoKept(q, agents, now)
		if err != nil {
			return nil, err
		}
		var autoValue any
		if ok {
			autoValue = auto
		}
		value := any(k.Value)
		if k.Mode == worktreepool.Auto {
			value = autoValue
		}
		kept = map[string]any{"mode": k.Mode, "value": value, "auto_value": autoValue}
		if n, ok := value.(int); ok {
			if tonight, err = nextPrune(q, repo, n, now); err != nil {
				return nil, err
			}
		}
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
			"last_agent": f.LastAgent, "last_used": f.LastUsed, "pinned_to": f.PinnedTo, "size_bytes": size, "task": task,
			"removed_tonight": tonight[f.Path]})
	}
	return map[string]any{"desks": out, "kept": kept}, nil
}

// nextPrune is the desks of repo the next nightly prune would remove if nothing changed until then: free, unpinned, unused for
// worktreepool.UnusedS by then and holding no work, the oldest beyond keep.
func nextPrune(q db.Q, repo string, keep int, now time.Time) (map[string]bool, error) {
	all, err := worktreepool.List(q, repo)
	if err != nil || len(all) <= keep {
		return nil, err
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), worktreepool.PruneHour, 0, 0, 0, now.Location())
	if !at.After(now) {
		at = at.AddDate(0, 0, 1)
	}
	free, err := worktreepool.Removable(q, repo, at.Unix()-worktreepool.UnusedS)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, f := range free {
		if len(out) == len(all)-keep {
			break
		}
		if worktreepool.WhyKept(f.Path) == "" { // the prune passes over a desk that holds work, as here
			out[f.Path] = true
		}
	}
	return out, nil
}

// desksKeep sets how many desks a project keeps: auto, or manual with a value.
func desksKeep(c *rpc.Conn, p map[string]any, home string) (any, error) {
	repo, _ := p["repo"].(string)
	known := false
	for _, r := range worktreeprune.Repos(home) {
		known = known || r == repo
	}
	if !known {
		nf := rpc.NotFoundError(fmt.Sprintf("%q is no repo an agent works in (repos.list)", repo))
		nf.Data = map[string]any{"what": "repo", "name": repo}
		return nil, nf
	}
	mode, _ := p["mode"].(string)
	k := worktreepool.Kept{Mode: mode}
	field := "mode"
	if mode == worktreepool.Manual {
		field = "value"
		switch v := p["value"].(type) {
		case json.Number:
			n, _ := v.Int64() // 2.5 is no whole number: 0, refused below
			k.Value = int(n)
		case float64:
			if v == float64(int(v)) {
				k.Value = int(v)
			}
		}
	}
	return c.Effect(func(*sql.Tx) (map[string]any, error) {
		if err := worktreepool.PutKept(protected.New(home), repo, k); err != nil {
			return nil, rpc.BadRequestError(err.Error(), field, rpc.InvalidParams)
		}
		if k.Mode == worktreepool.Manual {
			return said(fmt.Sprintf("%s keeps %d desks", filepath.Base(repo), k.Value), nil), nil
		}
		return said(filepath.Base(repo)+" keeps its desks by Auto", nil), nil
	})
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
