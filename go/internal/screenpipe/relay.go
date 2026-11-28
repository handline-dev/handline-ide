package screenpipe

import (
	"encoding/base64"
	"errors"
	"sync"

	"github.com/handline/handline/go/protocol/rpc"
)

// A runner's agent's source runs on the runner: the master watches it as a stream of the runner's link (runner.screen.attach), as it
// does the agent's terminal, and sends input and the release down the link.

// Stream is nodes.Registry.Stream: a stream of node's link, its notices to on.
type Stream func(node, method string, params map[string]any, on func(map[string]any), ended func()) (reply map[string]any, id string, stop func(), err error)

// Call is nodes.Registry.Call.
type Call func(node, method string, params map[string]any) (map[string]any, error)

// relaySource is a runner's source as the master sees it.
type relaySource struct {
	stream              Stream
	call                Call
	node, agent, source string
	mu                  sync.Mutex
	id                  string
	stop                func()
	stopped             bool
}

// notOpen turns the runner's not_found into ErrNotOpen.
func notOpen(err error) error {
	var e *rpc.Error
	if errors.As(err, &e) && e.Code == "not_found" {
		return ErrNotOpen
	}
	return err
}

func (s *relaySource) Start(frame func(Frame), ended func()) error {
	on := func(p map[string]any) {
		switch p["method"] {
		case "frame":
			if f, ok := frameOf(p["params"]); ok {
				frame(f)
			}
		case "ended":
			ended()
		}
	}
	_, id, stop, err := s.stream(s.node, "runner.screen.attach", map[string]any{"agent": s.agent, "source": s.source}, on, ended)
	if err != nil {
		return notOpen(err)
	}
	s.mu.Lock()
	s.id, s.stop = id, stop
	early := s.stopped
	s.mu.Unlock()
	if early {
		stop()
	}
	return nil
}

func (s *relaySource) Stop() {
	s.mu.Lock()
	stop := s.stop
	s.stop, s.stopped = nil, true
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
}

func (s *relaySource) Input(ev Input) error {
	s.mu.Lock()
	id := s.id
	s.mu.Unlock()
	_, err := s.call(s.node, "runner.screen.input", map[string]any{"stream": id, "event": ev.wire()})
	return err
}

func (s *relaySource) Release() error {
	_, err := s.call(s.node, "runner.screen.release", map[string]any{"agent": s.agent, "source": s.source})
	return notOpen(err)
}

func frameWire(f Frame) map[string]any {
	return map[string]any{"data": base64.StdEncoding.EncodeToString(f.JPEG), "width": f.Width, "height": f.Height}
}

func frameOf(v any) (Frame, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return Frame{}, false
	}
	data, _ := m["data"].(string)
	jpeg, err := base64.StdEncoding.DecodeString(data)
	w, _ := num(m["width"])
	h, _ := num(m["height"])
	return Frame{JPEG: jpeg, Width: int(w), Height: int(h)}, err == nil
}

// Streams is a runner's side: each master stream of one of its sources, by the stream's id.
type Streams struct {
	Open Opener
	mu   sync.Mutex
	m    map[string]Source
}

// Attach starts stream id of the agent's source; each frame goes up with notify.
func (s *Streams) Attach(id, agent, source string, notify func(string, map[string]any) bool) (any, error) {
	if s.Open == nil {
		return nil, rpc.NotFoundError(agent + " has no " + source + " open")
	}
	src, err := s.Open(agent, source)
	if err != nil {
		if errors.Is(err, ErrNotOpen) {
			return nil, rpc.NotFoundError(agent + " has no " + source + " open")
		}
		return nil, err
	}
	frame := func(f Frame) {
		notify("runner.output", map[string]any{"stream": id, "method": "frame", "params": frameWire(f)})
	}
	ended := func() {
		s.mu.Lock()
		delete(s.m, id)
		s.mu.Unlock()
		src.Stop()
		notify("runner.output", map[string]any{"stream": id, "method": "ended"})
	}
	s.mu.Lock() // kept before it starts, so an end that comes at once finds it
	if s.m == nil {
		s.m = map[string]Source{}
	}
	s.m[id] = src
	s.mu.Unlock()
	if err := src.Start(frame, ended); err != nil {
		s.mu.Lock()
		delete(s.m, id)
		s.mu.Unlock()
		return nil, err
	}
	return map[string]any{}, nil
}

// Input is a person's input on stream id.
func (s *Streams) Input(id string, event any) error {
	ev, err := ParseInput(event)
	if err != nil {
		return err
	}
	s.mu.Lock()
	src := s.m[id]
	s.mu.Unlock()
	if src == nil {
		return rpc.NotFoundError("no screen stream " + id)
	}
	return src.Input(ev)
}

// Release releases the agent's source, watched or not.
func (s *Streams) Release(agent, source string) error {
	if s.Open == nil {
		return rpc.NotFoundError(agent + " has no " + source + " open")
	}
	src, err := s.Open(agent, source)
	if errors.Is(err, ErrNotOpen) {
		return rpc.NotFoundError(agent + " has no " + source + " open")
	}
	if err != nil {
		return err
	}
	defer src.Stop()
	return src.Release()
}

// Detach ends stream id; one that is not a screen stream is left alone.
func (s *Streams) Detach(id string) {
	s.mu.Lock()
	src := s.m[id]
	delete(s.m, id)
	s.mu.Unlock()
	if src != nil {
		src.Stop()
	}
}

// DetachAll ends every stream: the link they went up ended.
func (s *Streams) DetachAll() {
	s.mu.Lock()
	all := s.m
	s.m = nil
	s.mu.Unlock()
	for _, src := range all {
		src.Stop()
	}
}
