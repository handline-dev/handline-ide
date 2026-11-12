package leases

import (
	"errors"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

// Register adds lease.take, lease.release, lease.extend and lease.get.
func Register(s *rpc.Server, m *Manager) {
	for _, r := range []struct {
		name   string
		method rpc.Method
	}{
		{"lease.take", rpc.Method{Waits: true, Handler: m.take}},
		{"lease.release", rpc.Method{Handler: m.release}},
		{"lease.extend", rpc.Method{Handler: m.extend}},
		{"lease.get", rpc.Method{Read: true, Handler: m.get}},
	} {
		s.Register(r.name, r.method)
	}
}

func bad(msg, field string) error { return rpc.BadRequestError(msg, field, rpc.InvalidParams) }

func agentOf(c *rpc.Conn, params map[string]any) (string, error) {
	agent, _ := params["agent"].(string)
	if agent == "" {
		return "", bad("agent is a name", "agent")
	}
	return agent, c.RequireSelf(agent)
}

// take answers a granted lease: {lease, waited_s, port for a port}; refused with lease_wait, lease_order or, for a call held open, lease_ceiling.
func (m *Manager) take(c *rpc.Conn, params map[string]any) (any, error) {
	agent, err := agentOf(c, params)
	if err != nil {
		return nil, err
	}
	kind, _ := params["kind"].(string)
	if !Known(kind) {
		return nil, bad("kind is heavy, simulator, browser or port", "kind")
	}
	hold, _ := params["hold"].(bool)
	pid := 0
	if !hold {
		if pid = c.Pid; pid == 0 {
			return nil, bad("a command's lease is taken over the local socket, by the process the command runs under", "hold")
		}
	}
	asked := m.now()
	l, err := m.Take(c.Done(), kind, agent, hold, pid, CeilingS*time.Second)
	if r, ok := err.(*Refusal); ok {
		return nil, rpc.Refused(r.Why, "lease_ceiling", "")
	}
	if o, ok := err.(*OutOfOrder); ok {
		return nil, rpc.Refused(o.Why, "lease_order", "")
	}
	if b, ok := err.(*Blocked); ok {
		return nil, rpc.Refused(b.Why, "lease_wait", "")
	}
	if err != nil || l == nil {
		return nil, err
	}
	out := map[string]any{"lease": l.ID, "waited_s": max(l.Since-asked, 0)}
	if l.Kind == Port {
		out["port"] = l.Port
	}
	return out, nil
}

// release ends one lease of the caller, or with no lease every one of kind (every kind when absent): {released: n}.
func (m *Manager) release(c *rpc.Conn, params map[string]any) (any, error) {
	agent, err := agentOf(c, params)
	if err != nil {
		return nil, err
	}
	if id, ok := rpc.AsInt(params["lease"]); ok {
		n := 0
		if m.Release(id, agent) {
			n = 1
		}
		return map[string]any{"released": n}, nil
	}
	kind, _ := params["kind"].(string)
	if kind != "" && !Known(kind) {
		return nil, bad("kind is heavy, simulator, browser or port", "kind")
	}
	return map[string]any{"released": m.ReleaseAgent(agent, kind, false, "released")}, nil
}

// extend adds a period to each of the caller's leases of kind (every kind when absent): {extended: [{lease, kind, max_hold_at}]}.
func (m *Manager) extend(c *rpc.Conn, params map[string]any) (any, error) {
	agent, err := agentOf(c, params)
	if err != nil {
		return nil, err
	}
	kind, _ := params["kind"].(string)
	if kind != "" && !Known(kind) {
		return nil, bad("kind is heavy, simulator, browser or port", "kind")
	}
	ls, err := m.Extend(agent, kind)
	var r *Refusal
	if errors.As(err, &r) {
		return nil, rpc.Refused(r.Why, "", "")
	}
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, l := range ls {
		out = append(out, map[string]any{"lease": l.ID, "kind": l.Kind, "max_hold_at": stamp(l.MaxHoldAt)})
	}
	return map[string]any{"extended": out}, nil
}

func (m *Manager) get(_ *rpc.Conn, _ map[string]any) (any, error) {
	pools := []any{}
	for _, p := range m.View() {
		held := []any{}
		for _, l := range p.Held {
			pids := []any{}
			for _, pid := range l.Pids {
				pids = append(pids, pid)
			}
			row := map[string]any{"lease": l.ID, "agent": l.Agent, "since": stamp(l.Since), "hold": l.Hold, "pids": pids}
			if l.Kind == Port {
				row["port"] = l.Port
			}
			if l.Reserved() {
				row["reserved_for"], row["until"] = l.Task, stamp(l.Until)
			}
			held = append(held, row)
		}
		queue := []any{}
		for _, w := range p.Queue {
			row := map[string]any{"agent": w.Agent, "since": stamp(w.Since), "position": w.Position, "says": w.Says}
			if w.Task != 0 {
				row["task"] = w.Task
			}
			queue = append(queue, row)
		}
		pools = append(pools, map[string]any{"kind": p.Kind, "max": p.Max, "held": held, "queue": queue})
	}
	return map[string]any{"kinds": pools}, nil
}

// stamp is the spec's Timestamp: whole seconds, UTC.
func stamp(at int64) string { return time.Unix(at, 0).UTC().Format("2006-01-02T15:04:05Z") }
