package termchan

// The bytes format of a runner's terminal (docs/several-machines.md): each app client's attach on the master is a stream of its
// own from the runner (runner.attach), which serves it with this same bytes channel (Streams). On the master the client is a BytesClient
// as any other; its resync asks the runner for one in its stream (runner.resync).

import (
	"encoding/base64"
	"sync"

	"github.com/handline/handline/go/protocol/rpc"
)

// Relay is the master's way to its runners' links (go/internal/nodes.Registry).
type Relay interface {
	// Stream is nodes.Registry.Stream.
	Stream(node, method string, params map[string]any, on func(map[string]any), ended func()) (reply map[string]any, id string, stop func(), err error)
	Call(node, method string, params map[string]any) (map[string]any, error)
}

// relay is a bytes channel's runner: the stream each of its clients has.
type relay struct {
	r          Relay
	node, pane string // pane is the runner's own handle

	mu      sync.Mutex
	streams map[*BytesClient]*relayed
}

// relayed is one client's stream: its notices wait in early until the answer has made the client live.
type relayed struct {
	id    string
	stop  func()
	mu    sync.Mutex
	live  bool
	ended bool // the link ended
	early []map[string]any
}

// attach opens c's stream and answers the snapshot the runner took, nil with the error when it could not.
func (rl *relay) attach(ch *BytesChannel, c *BytesClient, history int) (map[string]any, error) {
	s := &relayed{}
	on := func(p map[string]any) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.live {
			s.early = append(s.early, p)
			return
		}
		ch.relayed(c, p)
	}
	ended := func() { // a link that ends before the answer fails the attach instead
		s.mu.Lock()
		live := s.live
		s.ended = true
		s.mu.Unlock()
		if live {
			c.close()
		}
	}
	reply, id, stop, err := rl.r.Stream(rl.node, "runner.attach", map[string]any{"handle": rl.pane, "history": history}, on, ended)
	if err != nil {
		return nil, err
	}
	s.id, s.stop = id, stop
	rl.mu.Lock()
	rl.streams[c] = s
	rl.mu.Unlock()
	c.mu.Lock()
	c.live = true
	c.mu.Unlock()
	s.mu.Lock()
	s.live = true
	for _, p := range s.early {
		ch.relayed(c, p)
	}
	s.early = nil
	gone := s.ended
	s.mu.Unlock()
	if gone {
		c.close()
	}
	return reply, nil
}

func (rl *relay) detach(c *BytesClient) {
	rl.mu.Lock()
	s := rl.streams[c]
	delete(rl.streams, c)
	rl.mu.Unlock()
	if s != nil {
		s.stop()
	}
}

// resync asks the runner for a snapshot in c's stream; c takes no output until it comes.
func (rl *relay) resync(c *BytesClient, reason string) {
	rl.mu.Lock()
	s := rl.streams[c]
	rl.mu.Unlock()
	if s == nil {
		c.mu.Lock()
		c.resyncing = false
		c.mu.Unlock()
		return
	}
	go func() {
		_, err := rl.r.Call(rl.node, "runner.resync", map[string]any{"stream": s.id, "reason": reason})
		rl.mu.Lock()
		open := rl.streams[c] == s
		rl.mu.Unlock()
		if err != nil && open {
			c.close() // no snapshot can come: the client attaches again
		}
	}()
}

// relayed is one notice of c's stream: output to queue, a snapshot in place of what is queued, or the end of the stream.
func (ch *BytesChannel) relayed(c *BytesClient, p map[string]any) {
	method, _ := p["method"].(string)
	params, _ := p["params"].(map[string]any)
	switch method {
	case "output":
		s, _ := params["data"].(string)
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return
		}
		if c.putOutput(data) {
			ch.resync(c, "slow_client")
		}
	case "snapshot":
		snap := map[string]any{}
		for k, v := range params {
			if k != "terminal" && k != "seq" { // the client's own, put in as it is sent
				snap[k] = v
			}
		}
		c.resyncWith(snap)
	case "resync_failed": // as a local one that fails: the client takes output again
		c.mu.Lock()
		c.resyncing = false
		c.mu.Unlock()
	case "ended":
		c.close()
	}
}

// Streams is the bytes attaches a runner serves its master, by stream id: each a client of this machine's bytes channel whose notices
// go to the master.
type Streams struct {
	link func() *ControlLink
	hub  *BytesHub

	mu   sync.Mutex
	m    map[string]*stream
	gone map[string]bool // ids detached before their attach was kept: the master gave up on it
}

type stream struct {
	ch *BytesChannel
	c  *BytesClient
}

// NewStreams serves streams from the terminals link() reaches.
func NewStreams(link func() *ControlLink) *Streams {
	return &Streams{link: link, hub: NewBytesHub(), m: map[string]*stream{}, gone: map[string]bool{}}
}

// Attach starts stream id on handle and answers its snapshot, with history lines of scrollback. notify sends one notice up the link;
// false when it could not.
func (s *Streams) Attach(id, handle string, history int, notify func(method string, params map[string]any) bool) (map[string]any, error) {
	if id == "" || handle == "" {
		return nil, rpc.BadRequestError("stream and handle are strings", "stream", rpc.InvalidParams)
	}
	link := s.link()
	if !link.Start() {
		return nil, rpc.Refused("the terminal host could not be reached", "state", "")
	}
	send := func(msg map[string]any) bool {
		return notify("runner.output", map[string]any{"stream": id, "method": msg["method"], "params": msg["params"]})
	}
	ended := func() { // the control link ended: the master's client attaches again
		notify("runner.output", map[string]any{"stream": id, "method": "ended"})
		s.detach(id, false)
	}
	c := NewBytesClient(send, ended)
	c.resyncFailed = func() { notify("runner.output", map[string]any{"stream": id, "method": "resync_failed"}) }
	ch := s.hub.Channel(handle, link)
	s.mu.Lock()
	if s.gone[id] {
		delete(s.gone, id)
		s.mu.Unlock()
		return nil, rpc.Refused("stream "+id+" was detached", "state", "")
	}
	old := s.m[id]
	s.m[id] = &stream{ch, c}
	s.mu.Unlock()
	if old != nil {
		old.ch.Detach(old.c)
	}
	snap := ch.Attach(c, history)
	if snap == nil {
		s.detach(id, false)
		return nil, rpc.Refused("the terminal's screen could not be read", "state", "")
	}
	out := snap.encode()
	out["seq"], out["history"] = c.Seq, snap.history()
	ch.Begin(c)
	return out, nil
}

// Detach ends stream id, the master's call.
func (s *Streams) Detach(id string) { s.detach(id, true) }

// detach ends stream id; asked, a detach that comes before its attach is kept, so that attach is refused.
func (s *Streams) detach(id string, asked bool) {
	s.mu.Lock()
	st := s.m[id]
	delete(s.m, id)
	if st == nil && asked {
		s.gone[id] = true
	}
	s.mu.Unlock()
	if st != nil {
		st.ch.Detach(st.c)
	}
}

// DetachAll ends every stream: the link they went up ended.
func (s *Streams) DetachAll() {
	s.mu.Lock()
	all := s.m
	s.m = map[string]*stream{}
	s.mu.Unlock()
	for _, st := range all {
		st.ch.Detach(st.c)
	}
}

// Resync drops what stream id has not sent and sends a snapshot in its place.
func (s *Streams) Resync(id, reason string) error {
	s.mu.Lock()
	st := s.m[id]
	s.mu.Unlock()
	if st == nil {
		return rpc.NotFoundError("no stream " + id)
	}
	if reason != "resize" {
		reason = "slow_client"
	}
	st.ch.resync(st.c, reason)
	return nil
}

// History is the scrollback page of handle (HistoryPage).
func (s *Streams) History(handle string, before, limit int) (map[string]any, error) {
	link := s.link()
	if !link.Start() {
		return nil, rpc.Refused("the terminal host could not be reached", "state", "")
	}
	page := HistoryPage(link, handle, before, limit)
	if page == nil {
		return nil, rpc.Refused("the terminal's scrollback could not be read", "state", "")
	}
	return page, nil
}
