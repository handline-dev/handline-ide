package leases

import (
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

// Register adds lease.take, lease.release and lease.get.
func Register(s *rpc.Server, m *Manager) {
	for _, r := range []struct {
		name   string
		method rpc.Method
	}{
		{"lease.take", rpc.Method{Waits: true, Handler: m.take}},
		{"lease.release", rpc.Method{Handler: m.release}},
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

// take holds the call until a lease is granted: {lease, waited_s}; refused after CeilingS with who holds them.
func (m *Manager) take(c *rpc.Conn, params map[string]any) (any, error) {
	agent, err := agentOf(c, params)
	if err != nil {
		return nil, err
	}
	kind, _ := params["kind"].(string)
	if !Known(kind) {
		return nil, bad("kind is heavy, simulator or browser", "kind")
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
	if b, ok := err.(*Blocked); ok {
		return nil, rpc.Refused(b.Why, "lease_wait", "")
	}
	if err != nil || l == nil {
		return nil, err
	}
	return map[string]any{"lease": l.ID, "waited_s": l.Since - asked}, nil
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
		return nil, bad("kind is heavy, simulator or browser", "kind")
	}
	return map[string]any{"released": m.ReleaseAgent(agent, kind, false, "released")}, nil
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
