package runs

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/db"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/leases"
)

// Register serves run.list, run.set and run.remove on book; view is the leases now (leases.Manager.View), for who holds and who waits.
func Register(s *rpc.Server, book *Book, view func() []leases.Pool) {
	s.Register("run.list", rpc.Method{Read: true, Handler: func(c *rpc.Conn, p map[string]any) (any, error) { return list(c, p, book, view) }})
	s.Register("run.set", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) { return set(c, p, book) }})
	s.Register("run.remove", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) { return remove(c, p, book) }})
}

// bookTTL is how long Book trusts what it read; run.set and run.remove read again at once.
const bookTTL = 10 * time.Second

// Book is the runs as the lease manager and the hook read them, read again after a write or bookTTL. A read that fails keeps the last
// good list, so a busy database never lifts a limit.
type Book struct {
	DB  db.Q
	Log func(string, ...any)
	Now func() time.Time

	mu   sync.Mutex
	runs []Run
	at   time.Time
}

func (b *Book) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// All is every run, from the last read when it is fresh.
func (b *Book) All() []Run {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.at.IsZero() && b.now().Sub(b.at) < bookTTL {
		return b.runs
	}
	rs, err := All(b.DB)
	if err != nil {
		if b.Log != nil {
			b.Log("run: reading the shared runs: %v (the last read stands)", err)
		}
		return b.runs
	}
	b.runs, b.at = rs, b.now()
	return rs
}

// Changed makes the next All read again.
func (b *Book) Changed() {
	b.mu.Lock()
	b.at = time.Time{}
	b.mu.Unlock()
}

// Shared is the runs as lease kinds and their limits (leases.Manager.Shared).
func (b *Book) Shared() []leases.Shared {
	all := b.All()
	out := make([]leases.Shared, 0, len(all))
	for _, r := range all {
		out = append(out, leases.Shared{Kind: r.Key(), AtOnce: r.AtOnce})
	}
	return out
}

// Named is the lease kind of the run agent knows as name: This Mac's or its workspace's (leases.Manager.Named).
func (b *Book) Named(agent, name string) (string, bool) {
	for _, r := range For(b.All(), WorkspaceOf(b.DB, agent)) {
		if r.Name == name {
			return r.Key(), true
		}
	}
	return "", false
}

func bad(msg, field string) error { return rpc.BadRequestError(msg, field, rpc.InvalidParams) }

// list is {runs: [...]}: every run, or with agent the ones that agent takes, each with who holds a turn and who waits.
func list(c *rpc.Conn, p map[string]any, book *Book, view func() []leases.Pool) (any, error) {
	all := book.All()
	if agent, _ := p["agent"].(string); agent != "" {
		all = For(all, WorkspaceOf(c.Server().DB, agent))
	}
	pools := map[string]leases.Pool{}
	if view != nil && len(all) > 0 {
		for _, pl := range view() {
			pools[pl.Kind] = pl
		}
	}
	out := []any{}
	for _, r := range all {
		held, waiting := []any{}, []any{}
		for _, l := range pools[r.Key()].Held {
			held = append(held, map[string]any{"agent": l.Agent, "since": stamp(l.Since), "hold": l.Hold})
		}
		for _, w := range pools[r.Key()].Queue {
			row := map[string]any{"agent": w.Agent, "since": stamp(w.Since), "position": w.Position}
			if w.Task != 0 {
				row["task"] = w.Task
			}
			waiting = append(waiting, row)
		}
		cmds := make([]any, 0, len(r.Cmds))
		for _, c := range r.Cmds {
			cmds = append(cmds, c)
		}
		out = append(out, map[string]any{"kind": r.Key(), "name": r.Name, "scope": r.Scope(), "cmds": cmds, "at_once": r.AtOnce,
			"why": r.Why, "set_by": r.SetBy, "set_at": stamp(r.SetAt), "held": held, "waiting": waiting})
	}
	presets := make([]any, 0, len(Presets))
	for _, p := range Presets {
		presets = append(presets, p)
	}
	return map[string]any{"runs": out, "presets": presets}, nil
}

// scope is the workspace a write names: This Mac for scope mac; for scope workspace, an agent's own, or the one the founder names.
func scope(c *rpc.Conn, p map[string]any) (int64, error) {
	switch s, _ := p["scope"].(string); s {
	case Mac:
		return MacWS, nil
	case "workspace":
	default:
		return 0, bad("scope is mac or workspace", "scope")
	}
	if c.Caller.Kind == "agent" {
		ws := WorkspaceOf(c.Server().DB, c.Caller.Key)
		if ws == MacWS {
			return 0, bad("you are in no workspace; set it for This Mac (scope mac)", "scope")
		}
		return ws, nil
	}
	w, _ := p["workspace"].(string)
	if k, id, ok := actor.Parse(w); ok && k == actor.Workspace && id > 0 {
		return id, nil
	}
	return 0, bad("workspace is the workspace's key (w3) with scope workspace", "workspace")
}

// set keeps a run, in place of the one of its name in its scope, at once.
func set(c *rpc.Conn, p map[string]any, book *Book) (any, error) {
	ws, err := scope(c, p)
	if err != nil {
		return nil, err
	}
	r := Run{Workspace: ws, AtOnce: 1, SetBy: c.Caller.Actor(), SetAt: time.Now().Unix()}
	r.Name, _ = p["name"].(string)
	r.Why, _ = p["why"].(string)
	r.Why = strings.TrimSpace(r.Why)
	if r.Why == "" && c.Caller.Kind == "agent" {
		return nil, bad("why says what the run keeps from running at once, for the founder and the agents it makes wait", "why")
	}
	if v, ok := p["at_once"]; ok && v != nil {
		n, ok := rpc.AsInt(v)
		if !ok {
			return nil, bad("at_once is a whole number from 1", "at_once")
		}
		r.AtOnce = int(n)
	}
	cmds, _ := p["cmds"].([]any)
	for _, x := range cmds {
		s, ok := x.(string)
		if !ok {
			return nil, bad("cmds is a list of commands", "cmds")
		}
		r.Cmds = append(r.Cmds, s)
	}
	if why, field := Check(&r); why != "" {
		return nil, bad(why, field)
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		all, err := All(tx)
		if err != nil {
			return nil, err
		}
		if why := Clash(all, r); why != "" {
			return nil, bad(why, "name")
		}
		if err := Set(tx, r); err != nil {
			return nil, err
		}
		c.AfterReply(book.Changed)
		c.Server().Log("run: %s set %s: %s, %d at once (%s)", r.SetBy, r.Key(), strings.Join(r.Cmds, ", "), r.AtOnce, r.Why)
		return said(fmt.Sprintf("%s now takes turns, %d at once, for %s: %s", r.Name, r.AtOnce, where(r), strings.Join(r.Cmds, ", "))), nil
	})
}

// remove drops a run; its turns held now run to their end.
func remove(c *rpc.Conn, p map[string]any, book *Book) (any, error) {
	ws, err := scope(c, p)
	if err != nil {
		return nil, err
	}
	name, _ := p["name"].(string)
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		gone, err := Remove(tx, ws, name)
		if err != nil {
			return nil, err
		}
		r := Run{Name: name, Workspace: ws}
		c.AfterReply(book.Changed)
		if !gone {
			nf := rpc.NotFoundError(fmt.Sprintf("%s has no shared run %s (hl use list)", where(r), name))
			nf.Data = map[string]any{"what": "run", "name": name}
			return nil, nf
		}
		c.Server().Log("run: %s removed %s", c.Caller.Actor(), r.Key())
		return said(fmt.Sprintf("%s no longer takes turns for %s", name, where(r))), nil
	})
}

// where is the run's scope in words.
func where(r Run) string {
	if r.Workspace == MacWS {
		return "This Mac"
	}
	return "workspace " + r.Scope()
}

func said(text string) map[string]any { return map[string]any{"accepted": true, "said": text} }

// stamp is the spec's Timestamp: whole seconds, UTC.
func stamp(at int64) string { return time.Unix(at, 0).UTC().Format("2006-01-02T15:04:05Z") }
