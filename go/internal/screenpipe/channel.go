package screenpipe

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

// frameWithin is how long an attach waits for a source's first frame before it answers with none.
const frameWithin = 3 * time.Second

// channel is one source being watched: its watchers, the last frame it sent, and the goroutine that sends frames to them. A watcher is
// added in the same step that finds or makes the channel, so the last one leaving and a new one arriving never cross.
type channel struct {
	ready chan struct{} // closed once Start has answered; err is its error
	err   error
	done  chan struct{} // closed when the channel stops
	kick  chan struct{} // a frame waits to be sent

	mu       sync.Mutex
	src      Source
	started  bool // Start succeeded: Stop is the stopper's to call
	stopped  bool
	watchers map[*rpc.Conn]*watcher
	last     *Frame
	seq      int64
	first    chan struct{} // closed at the first frame
}

type watcher struct {
	name string // the caller's name, for the hold that ends when its holder no longer watches
	live bool   // the attach's reply was sent
	busy bool   // a frame is being written to it
	sent int64  // the newest frame it was given
}

// join adds c to k's watchers, making the channel and starting its source when there is none; it answers once the source has started.
func (h *Hub) join(c *rpc.Conn, k key) (*channel, error) {
	h.mu.Lock()
	h.init()
	ch := h.chans[k]
	made := ch == nil
	if made {
		ch = &channel{ready: make(chan struct{}), done: make(chan struct{}), kick: make(chan struct{}, 1),
			watchers: map[*rpc.Conn]*watcher{}, first: make(chan struct{})}
		h.chans[k] = ch
	}
	ch.mu.Lock()
	ch.watchers[c] = &watcher{name: callerName(c)} // a second attach on one connection is the same watcher, answered afresh
	ch.mu.Unlock()
	keys := h.conns[c]
	if keys == nil {
		keys = map[key]bool{}
		h.conns[c] = keys
		c.OnClose(func() { h.hangUp(c) })
	}
	keys[k] = true
	h.mu.Unlock()
	if made {
		h.start(k, ch)
	}
	<-ch.ready
	ch.mu.Lock()
	err, stopped := ch.err, ch.stopped
	ch.mu.Unlock()
	if err == nil && stopped { // it ended or was left before this watcher could be answered
		err = ErrNotOpen
	}
	if err != nil {
		h.leave(c, k)
		return nil, err
	}
	return ch, nil
}

// start opens and starts k's source for the channel just made.
func (h *Hub) start(k key, ch *channel) {
	go h.send(k, ch)
	src, err := h.source(k)
	if err == nil {
		ch.mu.Lock()
		ch.src = src
		ch.mu.Unlock()
		err = src.Start(func(f Frame) { h.frame(ch, f) }, func() { h.ended(k, ch) })
	}
	ch.mu.Lock()
	ch.err = err
	ch.started = err == nil
	stopped := ch.stopped // stopped while it started: the stop could not reach it
	ch.mu.Unlock()
	switch {
	case err != nil:
		h.drop(k, ch)
		ch.stop()
		if errors.Is(err, ErrNotOpen) {
			h.closed(k)
		}
	case stopped:
		src.Stop()
	default:
		h.opened(k)
	}
	close(ch.ready)
}

// frame keeps f as the newest and wakes the sender: a watcher slower than the source misses frames, and holds up nothing else.
func (h *Hub) frame(ch *channel, f Frame) {
	ch.mu.Lock()
	ch.seq++
	ch.last = &f
	if ch.seq == 1 {
		close(ch.first)
	}
	ch.mu.Unlock()
	ch.wake()
}

func (ch *channel) wake() {
	select {
	case ch.kick <- struct{}{}:
	default:
	}
}

// send gives each live watcher the newest frame it lacks, each as a write of its own: a watcher still writing the last one is skipped
// and caught up when it is done, so no watcher waits on another.
func (h *Hub) send(k key, ch *channel) {
	type out struct {
		c *rpc.Conn
		w *watcher
	}
	for {
		select {
		case <-ch.done:
			return
		case <-ch.kick:
		}
		ch.mu.Lock()
		if ch.last == nil {
			ch.mu.Unlock()
			continue
		}
		seq, last := ch.seq, *ch.last
		var to []out
		for c, w := range ch.watchers {
			if w.live && !w.busy && w.sent < seq {
				w.busy, w.sent = true, seq
				to = append(to, out{c, w})
			}
		}
		ch.mu.Unlock()
		if len(to) == 0 {
			continue
		}
		b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "screen_frame", "params": frameParams(k, seq, last)})
		if err != nil {
			continue
		}
		for _, o := range to {
			go func() {
				o.c.SendRaw(b)
				ch.mu.Lock()
				o.w.busy = false
				behind := o.w.sent < ch.seq
				ch.mu.Unlock()
				if behind {
					ch.wake()
				}
			}()
		}
	}
}

// live is the watchers whose attach was answered; under ch.mu.
func (ch *channel) live() []*rpc.Conn {
	var to []*rpc.Conn
	for c, w := range ch.watchers {
		if w.live {
			to = append(to, c)
		}
	}
	return to
}

func frameParams(k key, seq int64, f Frame) map[string]any {
	p := frameWire(f)
	p["agent"], p["source"], p["seq"] = k.agent, k.source, seq
	return p
}

func notify(to []*rpc.Conn, method string, params map[string]any) {
	if len(to) == 0 {
		return
	}
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return
	}
	for _, c := range to {
		c.SendRaw(b)
	}
}

// stop stops ch once: the sender ends, and the source if it has started (else start stops it when it does).
func (ch *channel) stop() {
	ch.mu.Lock()
	if ch.stopped {
		ch.mu.Unlock()
		return
	}
	ch.stopped = true
	src, started := ch.src, ch.started
	ch.mu.Unlock()
	close(ch.done)
	if started {
		src.Stop()
	}
}

// drop takes ch out of the hub with all its watchers, and says who they were; nil when ch was no longer k's.
func (h *Hub) drop(k key, ch *channel) []*rpc.Conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.chans[k] != ch {
		return nil
	}
	delete(h.chans, k)
	ch.mu.Lock()
	to := make([]*rpc.Conn, 0, len(ch.watchers))
	for c := range ch.watchers {
		to = append(to, c)
		delete(h.conns[c], k)
	}
	ch.mu.Unlock()
	return to
}

// ended is a source that went away while watched: its watchers are told, and it is no longer open. A relayed one can say so twice.
func (h *Hub) ended(k key, ch *channel) {
	to := h.drop(k, ch)
	ch.stop()
	if to == nil {
		h.mu.Lock()
		none := h.chans[k] == nil
		h.mu.Unlock()
		if none { // it died as its last watcher left
			h.closed(k)
		}
		return
	}
	notify(to, "screen_ended", map[string]any{"agent": k.agent, "source": k.source})
	h.closed(k)
	if err := h.giveBack(k); err != nil {
		h.logf("screen: %s's %s ended while held, and what the holder typed could not be cleared: %v", k.agent, k.source, err)
	}
}

// watchers is who watches k now.
func (h *Hub) watchers(k key) []*rpc.Conn {
	h.mu.Lock()
	ch := h.chans[k]
	h.mu.Unlock()
	if ch == nil {
		return nil
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.live()
}

// leave takes c off k's watchers; the last one stops the source, and a holder who no longer watches gives it back.
func (h *Hub) leave(c *rpc.Conn, k key) {
	h.mu.Lock()
	ch := h.chans[k]
	delete(h.conns[c], k)
	var w *watcher
	empty, stillWatches := false, false
	if ch != nil {
		ch.mu.Lock()
		w = ch.watchers[c]
		delete(ch.watchers, c)
		empty = len(ch.watchers) == 0
		for _, o := range ch.watchers {
			if w != nil && o.name == w.name {
				stillWatches = true
			}
		}
		ch.mu.Unlock()
		if empty {
			delete(h.chans, k)
		}
	}
	hd := h.holds[k]
	h.mu.Unlock()
	if w != nil && !stillWatches && hd != nil && hd.name == w.name {
		if err := h.giveBack(k); err != nil {
			h.logf("screen: %s stopped watching %s's %s and it stays held: %v", w.name, k.agent, k.source, err)
		}
	}
	if empty {
		ch.stop()
	}
}

// release clears what a holder typed into k's source: through the watched source, else one opened for it. A source that is not open has
// nothing to clear.
func (h *Hub) release(k key) error {
	h.mu.Lock()
	ch := h.chans[k]
	h.mu.Unlock()
	var src Source
	if ch != nil {
		ch.mu.Lock()
		if ch.started && !ch.stopped {
			src = ch.src
		}
		ch.mu.Unlock()
	}
	if src == nil {
		var err error
		if src, err = h.source(k); errors.Is(err, ErrNotOpen) {
			return nil
		} else if err != nil {
			return err
		}
		defer src.Stop()
	}
	if err := src.Release(); err != nil && !errors.Is(err, ErrNotOpen) {
		return err
	}
	return nil
}

// giveBack ends k's hold once what its holder typed is cleared (a hold that cannot be cleared stays): watchers are told, and an agent
// turned away meanwhile hears it may go on.
func (h *Hub) giveBack(k key) error {
	h.mu.Lock()
	hd := h.holds[k]
	h.mu.Unlock()
	if hd == nil {
		return nil
	}
	if err := h.release(k); err != nil {
		return fmt.Errorf("the password fields %s typed into could not be emptied: %w", hd.name, err)
	}
	h.mu.Lock()
	gone := h.holds[k] == hd
	if gone {
		delete(h.holds, k)
	}
	h.mu.Unlock()
	if !gone { // taken again meanwhile
		return nil
	}
	h.holderChanged(k)
	if hd.refused && h.Tell != nil {
		h.Tell(k.agent, hd.name+" gave the page back")
	}
	return nil
}

// holder is the spec's ScreenHolder of k: the person or device holding it, else the agent.
func (h *Hub) holder(k key) map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	if hd := h.holds[k]; hd != nil {
		return map[string]any{"kind": hd.kind, "name": hd.name, "since": hd.since.UTC().Format("2006-01-02T15:04:05Z")}
	}
	return map[string]any{"kind": "agent", "name": k.agent, "since": nil}
}

func (h *Hub) holderChanged(k key) {
	notify(h.watchers(k), "screen_holder", map[string]any{"agent": k.agent, "source": k.source, "holder": h.holder(k)})
}

func (h *Hub) logf(format string, args ...any) {
	if h.Log != nil {
		h.Log(format, args...)
	}
}
