// Package chatrelay makes the holder of a Chat session on a runner look like one on the master (docs/several-machines.md): for each
// live holder a runner lists, the master serves the holder's own protocol at the socket the agent's holder would have here, and relays
// each read, write and acknowledgement over the runner's link. Everything that reads a Chat session (its port, its terminal, its rows)
// reads it the same way wherever it runs. While the link is down the relay stays up and its reader waits; once the link is back the
// reader goes on from the last record it was given, so no record is lost or given twice.
package chatrelay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/handline/handline/go/agentctl/holder"
	"github.com/handline/handline/go/kit/paths"
)

// Runners is the master's way to its runners' links (go/internal/nodes.Registry).
type Runners interface {
	Linked() []string
	// SessionsOn is the agents whose sessions are on node, as the master opened each, mapped to its key.
	SessionsOn(node string) (map[string]string, error)
	Stream(node, method string, params map[string]any, on func(map[string]any), ended func()) (reply map[string]any, id string, stop func(), err error)
	Call(node, method string, params map[string]any) (map[string]any, error)
	// Quick is Call with a short wait, for a read made every tick.
	Quick(node, method string, params map[string]any) (map[string]any, error)
}

// Relays keeps a relay for each live holder on a linked runner.
type Relays struct {
	Home    string
	Runners Runners
	Log     func(format string, a ...any)
	// Retry is how long a relay whose link ended waits before it reads again; 0 is a second.
	Retry time.Duration

	mu     sync.Mutex
	relays map[string]*relay // by agent key
}

// Run keeps the relays every tick until ctx ends, then stops them.
func (m *Relays) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		m.Sync()
		select {
		case <-ctx.Done():
			m.mu.Lock()
			all := m.relays
			m.relays = nil
			m.mu.Unlock()
			for _, r := range all {
				r.stop()
			}
			return
		case <-t.C:
		}
	}
}

// Sync starts a relay for each holder a linked runner lists, and stops each one whose agent's session is no longer on its runner, or
// whose linked runner no longer lists it. A runner not linked now keeps the relays of the sessions still on it.
func (m *Relays) Sync() {
	m.mu.Lock()
	nodes := map[string]bool{}
	for _, r := range m.relays {
		nodes[r.node] = false
	}
	m.mu.Unlock()
	for _, n := range m.Runners.Linked() {
		nodes[n] = true
	}
	type seen struct {
		on   map[string]string // the sessions on the node, nil when they could not be read
		live map[string]bool   // the holders it lists, nil when it was not asked or did not say
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	by := map[string]seen{}
	for node, linked := range nodes {
		wg.Add(1)
		go func() { // one runner slow to answer holds up no other
			defer wg.Done()
			var sn seen
			on, err := m.Runners.SessionsOn(node)
			if err != nil {
				m.log("chatrelay: the sessions on %s could not be read: %v", node, err)
			} else {
				sn.on = on
			}
			if linked && len(on) > 0 {
				if got, err := m.Runners.Quick(node, "runner.chat.list", nil); err == nil {
					sn.live = map[string]bool{}
					list, _ := got["holders"].([]any)
					for _, v := range list {
						if s, ok := v.(string); ok {
							sn.live[s] = true
						}
					}
				}
			}
			mu.Lock()
			by[node] = sn
			mu.Unlock()
		}()
	}
	wg.Wait()
	type want struct{ node, name string }
	wanted := map[string]want{} // by key: a session on node whose holder is listed, or that its node cannot say of now
	for node, sn := range by {
		for opened, key := range sn.on {
			name := filepath.Base(paths.ChatSocket("", opened))
			if sn.live == nil || sn.live[name] {
				wanted[key] = want{node, name}
			}
		}
	}
	m.mu.Lock()
	if m.relays == nil {
		m.relays = map[string]*relay{}
	}
	var gone []*relay
	for key, r := range m.relays {
		w, ok := wanted[key]
		if ok && w == (want{r.node, r.name}) || !ok && by[r.node].on == nil {
			continue
		}
		gone = append(gone, r)
		delete(m.relays, key)
	}
	for key, w := range wanted { // a session its node could not list is kept, not started
		if sn := by[w.node]; sn.live == nil && m.relays[key] == nil {
			delete(wanted, key)
		}
	}
	var start []string
	for key, w := range wanted {
		if m.relays[key] == nil {
			start = append(start, key)
			m.relays[key] = &relay{m: m, node: w.node, name: w.name, sock: paths.ChatSocket(m.Home, key)}
		}
	}
	m.mu.Unlock()
	for _, r := range gone {
		r.stop()
	}
	for _, key := range start {
		m.mu.Lock()
		r := m.relays[key]
		m.mu.Unlock()
		if r == nil {
			continue
		}
		if err := r.start(); err != nil {
			m.log("chatrelay: %s's holder on %s is not relayed: %v", key, r.node, err)
			m.mu.Lock()
			if m.relays[key] == r {
				delete(m.relays, key)
			}
			m.mu.Unlock()
		}
	}
}

func (m *Relays) log(format string, a ...any) {
	if m.Log != nil {
		m.Log(format, a...)
	}
}

func (m *Relays) retry() time.Duration {
	if m.Retry > 0 {
		return m.Retry
	}
	return time.Second
}

// relay is one runner holder served at sock.
type relay struct {
	m          *Relays
	node, name string
	sock       string

	ln     net.Listener
	mine   os.FileInfo
	mu     sync.Mutex
	reader net.Conn
	done   chan struct{}
	once   sync.Once
}

func (r *relay) start() error {
	if holder.Alive(r.sock) {
		return errors.New("a holder already serves " + r.sock)
	}
	if err := os.MkdirAll(filepath.Dir(r.sock), 0o700); err != nil {
		return err
	}
	_ = os.Remove(r.sock)
	ln, err := net.Listen("unix", r.sock)
	if err != nil {
		return err
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	r.ln, r.done = ln, make(chan struct{})
	if r.mine, err = os.Stat(r.sock); err != nil {
		ln.Close()
		return err
	}
	if err := os.WriteFile(holder.PidFile(r.sock), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		ln.Close()
		os.Remove(r.sock)
		return err
	}
	go r.serve()
	return nil
}

// stop ends the relay: its reader sees its holder gone.
func (r *relay) stop() {
	r.once.Do(func() {
		if r.done == nil {
			return
		}
		close(r.done)
		r.ln.Close()
		r.mu.Lock()
		if r.reader != nil {
			r.reader.Close()
		}
		r.mu.Unlock()
		if now, err := os.Stat(r.sock); err == nil && r.mine != nil && os.SameFile(r.mine, now) {
			os.Remove(r.sock)
			os.Remove(holder.PidFile(r.sock))
		}
	})
}

func (r *relay) serve() {
	for {
		c, err := r.ln.Accept()
		if err != nil {
			return
		}
		go r.client(c)
	}
}

func (r *relay) client(c net.Conn) {
	br := bufio.NewReader(c)
	var req holder.Request
	line, err := br.ReadBytes('\n')
	if err != nil || json.Unmarshal(line, &req) != nil {
		c.Close()
		return
	}
	switch {
	case req.Read != nil:
		r.read(c, br, *req.Read)
	case req.Write != nil:
		_, err := r.m.Runners.Call(r.node, "runner.chat.write", map[string]any{"holder": r.name, "line": *req.Write})
		answer(c, err)
	case req.Close:
		_, err := r.m.Runners.Call(r.node, "runner.chat.close", map[string]any{"holder": r.name})
		answer(c, err)
	default:
		c.Close()
	}
}

func answer(c net.Conn, err error) {
	defer c.Close()
	var rep holder.Reply
	if err != nil {
		rep.Err = err.Error()
	}
	b, _ := json.Marshal(rep)
	_, _ = c.Write(append(b, '\n'))
}

// read serves the holder's records after from to c, reading them from the runner, until c or the relay ends. A new reader takes the
// place of the one before. A link that ends is read again from the last record c was given.
func (r *relay) read(c net.Conn, br *bufio.Reader, from int64) {
	r.mu.Lock()
	if r.reader != nil {
		r.reader.Close()
	}
	r.reader = c
	r.mu.Unlock()
	defer c.Close()
	over := make(chan struct{})
	defer close(over)
	q := &queue{wake: make(chan struct{}, 1)}
	var streamMu sync.Mutex
	stream := ""
	gone := make(chan struct{})
	var acked atomic.Int64
	ackNow := make(chan struct{}, 1)
	go func() { // acks from c: the last one goes to the stream read now, one call at a time
		defer close(gone)
		for {
			line, err := br.ReadBytes('\n')
			if err != nil {
				c.Close()
				return
			}
			var req holder.Request
			if json.Unmarshal(line, &req) != nil || req.Ack <= acked.Load() {
				continue
			}
			acked.Store(req.Ack)
			select {
			case ackNow <- struct{}{}:
			default:
			}
		}
	}()
	var ackMu sync.Mutex
	var told int64
	ack := func(id string) { // the last ack c sent, to stream id, once
		ackMu.Lock()
		defer ackMu.Unlock()
		if n := acked.Load(); id != "" && n > told {
			if _, err := r.m.Runners.Call(r.node, "runner.chat.ack", map[string]any{"stream": id, "seq": n}); err == nil {
				told = n
			}
		}
	}
	go func() {
		for {
			select {
			case <-ackNow:
			case <-over:
				return
			}
			streamMu.Lock()
			id := stream
			streamMu.Unlock()
			ack(id)
		}
	}()
	go q.drain(c, over)
	sent := from
	var pid int64 // the runner holder's process: another one at its name is a new holder, whose records start over
	for {
		ended := make(chan struct{})
		var endOnce sync.Once
		on := func(p map[string]any) {
			if m, _ := p["method"].(string); m == "ended" {
				endOnce.Do(func() { close(ended) })
				return
			}
			s, _ := p["record"].(string)
			var rec holder.Record
			if json.Unmarshal([]byte(s), &rec) != nil {
				return
			}
			q.push(rec)
		}
		reply, id, stop, err := r.m.Runners.Stream(r.node, "runner.chat.read", map[string]any{"holder": r.name, "from": q.last(sent)}, on,
			func() { endOnce.Do(func() { close(ended) }) })
		if err == nil {
			if got := asInt(reply["pid"]); pid != 0 && got != pid {
				stop()
				return // c ends, as a holder's reader does when its holder goes
			} else if pid == 0 {
				pid = got
			}
			streamMu.Lock()
			stream = id
			streamMu.Unlock()
			select { // an ack c sent while no stream was read goes now
			case ackNow <- struct{}{}:
			default:
			}
			select {
			case <-ended:
			case <-gone:
			case <-r.done:
			}
			select {
			case <-gone: // the reader's last ack (an exit's) goes before the stream ends, so the holder can end
				ack(id)
			default:
			}
			stop()
			streamMu.Lock()
			stream = ""
			streamMu.Unlock()
		}
		select {
		case <-gone:
			return
		case <-r.done:
			return
		case <-time.After(r.m.retry()):
		}
		sent = q.last(sent)
	}
}

// queue is the records on their way to the reader, kept off the link's loop: a slow reader never holds the link up. Only a record past
// the last one given goes, so a record read again after the link came back is not given twice.
type queue struct {
	mu   sync.Mutex
	recs []holder.Record
	max  int64 // the last record's seq queued
	wake chan struct{}
}

func (q *queue) push(rec holder.Record) {
	q.mu.Lock()
	if rec.Seq <= q.max && rec.Holder != "gap" {
		q.mu.Unlock()
		return
	}
	q.max = max(q.max, rec.Seq)
	q.recs = append(q.recs, rec)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// last is the last record queued, or from when none was.
func (q *queue) last(from int64) int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.max == 0 {
		q.max = from
	}
	return q.max
}

func (q *queue) drain(c net.Conn, done <-chan struct{}) {
	for {
		select {
		case <-q.wake:
		case <-done:
			return
		}
		q.mu.Lock()
		out := q.recs
		q.recs = nil
		q.mu.Unlock()
		for _, rec := range out {
			b, _ := json.Marshal(rec)
			if _, err := c.Write(append(b, '\n')); err != nil {
				return
			}
		}
	}
}

func asInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	}
	return 0
}
