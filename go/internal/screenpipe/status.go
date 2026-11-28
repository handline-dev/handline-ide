package screenpipe

import (
	"slices"

	"github.com/handline/handline/go/protocol/rpc"
)

// status is screen.status, the agent's own read of its source: who holds it, who watches it and the machine it runs on. It acts on
// nothing: no hold refuses it, and the source is neither opened, marked in use nor shown to watchers.
func (h *Hub) status(c *rpc.Conn, params map[string]any) (any, error) {
	k, err := keyOf(c, params, true)
	if err != nil {
		return nil, err
	}
	var machine any
	if h.Node != nil {
		if node := h.Node(k.agent); node != "" {
			machine = node
			if h.NodeName != nil {
				if n := h.NodeName(node); n != "" {
					machine = n
				}
			}
		}
	}
	holder, names := h.whoHas(k)
	return map[string]any{"holder": holder, "watchers": names, "machine": machine}, nil
}

// whoHas is k's holder (as holder) and the name of each person or device watching it now, once each, sorted: one look, so a take
// between the two is not half seen.
func (h *Hub) whoHas(k key) (map[string]any, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.init()
	holder := map[string]any{"kind": "agent", "name": k.agent, "since": nil}
	if hd := h.holds[k]; hd != nil {
		holder = map[string]any{"kind": hd.kind, "name": hd.name, "since": hd.since.UTC().Format("2006-01-02T15:04:05Z")}
	}
	names := []string{}
	if ch := h.chans[k]; ch != nil {
		ch.mu.Lock() // h.mu before ch.mu, as join and leave take them
		for _, w := range ch.watchers {
			if w.live && w.name != "" && !slices.Contains(names, w.name) {
				names = append(names, w.name)
			}
		}
		ch.mu.Unlock()
	}
	slices.Sort(names)
	return holder, names
}
