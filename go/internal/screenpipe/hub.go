package screenpipe

import (
	"sync"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

// DefaultInUse is how long a source counts as in use after the agent's last action on it.
const DefaultInUse = 20 * time.Second

// Hub is every source watched or held through this daemon. A hold is kept in memory: a daemon that starts again holds nothing, and the
// person takes it again.
type Hub struct {
	// Open is this machine's sources.
	Open Opener
	// Node is the runner agent runs on, "" for this machine; nil is this machine for everyone.
	Node func(agent string) string
	// Stream and Call reach a runner's sources over its link; nil: there are no runners.
	Stream Stream
	Call   Call
	// Changed is told an agent whose open sources or their use changed, so its view follows; nil tells no one.
	Changed func(agent string)
	// Tell sends the agent one line; nil sends none.
	Tell func(agent, text string)
	// Log is told what went wrong off any caller; nil logs nothing.
	Log func(format string, args ...any)
	// InUse is how long a source is in use after an agent's action; 0 is DefaultInUse.
	InUse time.Duration
	// Now and After are the clock: After runs f once d has passed, and gives back what stops it. nil is the real one.
	Now   func() time.Time
	After func(d time.Duration, f func()) (stop func())

	mu     sync.Mutex
	chans  map[key]*channel
	holds  map[key]*hold
	used   map[key]time.Time
	open   map[key]bool
	timers map[key]func()
	conns  map[*rpc.Conn]map[key]bool
}

type key struct{ agent, source string }

// hold is who took a source; refused is whether the agent was turned away while they held it.
type hold struct {
	kind, name string
	since      time.Time
	refused    bool
}

func (h *Hub) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Hub) after(d time.Duration, f func()) func() {
	if h.After != nil {
		return h.After(d, f)
	}
	t := time.AfterFunc(d, f)
	return func() { t.Stop() }
}

func (h *Hub) inUse() time.Duration {
	if h.InUse > 0 {
		return h.InUse
	}
	return DefaultInUse
}

func (h *Hub) init() {
	if h.chans == nil {
		h.chans, h.holds, h.used, h.open = map[key]*channel{}, map[key]*hold{}, map[key]time.Time{}, map[key]bool{}
		h.timers, h.conns = map[key]func(){}, map[*rpc.Conn]map[key]bool{}
	}
}

// source is k's source wherever its agent runs.
func (h *Hub) source(k key) (Source, error) {
	if h.Node != nil {
		if node := h.Node(k.agent); node != "" {
			if h.Stream == nil || h.Call == nil {
				return nil, ErrNotOpen
			}
			return &relaySource{stream: h.Stream, call: h.Call, node: node, agent: k.agent, source: k.source}, nil
		}
	}
	if h.Open == nil {
		return nil, ErrNotOpen
	}
	return h.Open(k.agent, k.source)
}

// changed tells the view off the caller: an agent's call does not wait for its view to be worked out.
func (h *Hub) changed(agent string) {
	if h.Changed != nil {
		go h.Changed(agent)
	}
}

func (h *Hub) opened(k key) {
	h.mu.Lock()
	h.init()
	was := h.open[k]
	h.open[k] = true
	h.mu.Unlock()
	if !was {
		h.changed(k.agent)
	}
}

// Closed is an agent's source that went away without its own word, as a device shut down at its session's end.
func (h *Hub) Closed(agent, source string) {
	if h != nil {
		h.closed(key{agent, source})
	}
}

func (h *Hub) closed(k key) {
	h.mu.Lock()
	h.init()
	was := h.open[k]
	delete(h.open, k)
	h.mu.Unlock()
	if was {
		h.changed(k.agent)
	}
}

// use marks k in use from now, and tells the view when that use ends.
func (h *Hub) use(k key) {
	h.mu.Lock()
	h.init()
	wasInUse := h.inUseLocked(k)
	h.used[k] = h.now()
	if stop := h.timers[k]; stop != nil {
		stop()
	}
	h.timers[k] = h.after(h.inUse(), func() { h.changed(k.agent) })
	h.mu.Unlock()
	if !wasInUse {
		h.changed(k.agent)
	}
}

func (h *Hub) inUseLocked(k key) bool {
	at, ok := h.used[k]
	return ok && h.now().Before(at.Add(h.inUse()))
}

// Sources is the agent's open sources and whether each is in use (AgentView.screens).
func (h *Hub) Sources(agent string) []any {
	out := []any{}
	if h == nil {
		return out
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.init()
	for _, s := range Sources {
		k := key{agent, s}
		if h.open[k] {
			out = append(out, map[string]any{"source": s, "in_use": h.inUseLocked(k)})
		}
	}
	return out
}
