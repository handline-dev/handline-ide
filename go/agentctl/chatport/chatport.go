// Package chatport is the delivery port of agents whose sessions run in Chat mode, under a holder: every message is a line on the
// session's stream under the uuid of its operation, and the CLI's own receipts for that uuid confirm it. Now is written at once (the CLI
// takes it into the running turn); Queue is held until the turn ends. It also answers what the CLI asks of the host, so it never waits.
// The holder's socket is open to the user the daemon and the agents run as, like everything under the home.
package chatport

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/holder"
)

// NoAsks is why a tool ask is refused while nothing answers asks from the app.
const NoAsks = "Permission asks are not answered in Chat mode yet; work without this tool or ask the person in your reply."

// Port delivers to Chat agents.
type Port struct {
	// Chat is agent's Chat mode, nil when its sessions run in a terminal.
	Chat func(agent string) agent.Chat
	// Socket is where agent's holder serves.
	Socket func(agent string) string
	// OnDelivered: the CLI took op. OnFailed: it refused or dropped it (written), or it could not be written.
	OnDelivered func(agent, op string)
	OnFailed    func(agent, op, why string, written bool)
	// OnSession is told the session id the CLI runs under; OnTurnEnd that a turn ended. Either may be nil.
	OnSession func(agent, id string)
	OnTurnEnd func(agent string, failed bool)
	// Write and Open reach the holder; nil is package holder.
	Write func(socket string, line []byte) error
	Open  func(ctx context.Context, socket string, from int64) (*holder.Reader, error)

	mu     sync.Mutex
	agents map[string]*session
}

type held struct{ op, text string }

type session struct {
	watching bool
	dialing  bool
	ready    bool
	// active are the commands written or started and not yet finished, by uuid: a turn runs while any is.
	active   map[string]bool
	held     []held
	ops      map[string]string // uuid -> op written and not yet confirmed
	receipts map[string]delivery.Receipt
	// What the stream last said: the turn ("" before it said), the background work, and the usage limit.
	state      string
	tasks      []agent.Task
	tasksKnown bool
	limit      agent.Limit
}

func (p *Port) state(a string) *session {
	if p.agents == nil {
		p.agents = map[string]*session{}
	}
	s := p.agents[a]
	if s == nil {
		s = &session{ops: map[string]string{}, receipts: map[string]delivery.Receipt{}, active: map[string]bool{}}
		p.agents[a] = s
	}
	return s
}

func (p *Port) write(socket string, line []byte) error {
	if p.Write != nil {
		return p.Write(socket, line)
	}
	return holder.Write(socket, line)
}

// Structured: a Chat agent is always delivered to through this port.
func (p *Port) Structured(a string) bool { return p.Chat(a) != nil }

// Live says whether the agent's holder answers.
func (p *Port) Live(a string) bool {
	if p.Chat(a) == nil {
		return false
	}
	p.mu.Lock()
	s := p.state(a)
	watching := s.watching
	p.mu.Unlock()
	return watching
}

// Send writes text under op's uuid: Now at once, Queue at once when no turn runs, else held until the turn ends. The same op twice
// writes once. An error means nothing was written.
func (p *Port) Send(a, op, text string, d delivery.Delivery) (delivery.Receipt, error) {
	c := p.Chat(a)
	if c == nil {
		return delivery.Receipt{}, errors.New(a + " does not run in Chat mode")
	}
	uuid := delivery.UUIDFor(op)
	p.mu.Lock()
	s := p.state(a)
	if r, ok := s.receipts[op]; ok {
		p.mu.Unlock()
		return r, nil
	}
	if !s.watching {
		p.mu.Unlock()
		return delivery.Receipt{}, errors.New(a + " has no live Chat session")
	}
	if d == delivery.Queue && len(s.active) > 0 {
		s.held = append(s.held, held{op, text})
		r := delivery.Receipt{Op: op, UUID: uuid, State: delivery.Queued}
		s.receipts[op] = r
		p.mu.Unlock()
		return r, nil
	}
	// Recorded before the write: the CLI's frames for it can come before the write returns, and a second Send of op finds it.
	r := delivery.Receipt{Op: op, UUID: uuid, State: delivery.Sent}
	s.ops[uuid], s.receipts[op], s.active[uuid] = op, r, true
	p.mu.Unlock()
	if err := p.write(p.Socket(a), c.UserLine(uuid, text)); err != nil {
		p.mu.Lock()
		delete(s.ops, uuid)
		delete(s.receipts, op)
		delete(s.active, uuid)
		p.mu.Unlock()
		p.release(a)
		return delivery.Receipt{}, err
	}
	return r, nil
}

// Withdraw takes back a held message; ok is whether it was held (also on a repeat).
func (p *Port) Withdraw(a, op string) (delivery.Receipt, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.state(a)
	for i, h := range s.held {
		if h.op == op {
			s.held = append(s.held[:i], s.held[i+1:]...)
			r := delivery.Receipt{Op: op, UUID: delivery.UUIDFor(op), State: delivery.Withdrawn}
			s.receipts[op] = r
			return r, true, nil
		}
	}
	r := s.receipts[op]
	return r, r.State == delivery.Withdrawn, nil
}

// Promote writes a held message now.
func (p *Port) Promote(a, op string) (delivery.Receipt, error) {
	p.mu.Lock()
	s := p.state(a)
	var text string
	found := false
	for i, h := range s.held {
		if h.op == op {
			text, found = h.text, true
			s.held = append(s.held[:i], s.held[i+1:]...)
			delete(s.receipts, op)
			break
		}
	}
	r := s.receipts[op]
	p.mu.Unlock()
	if !found {
		return r, nil
	}
	got, err := p.Send(a, op, text, delivery.Now)
	if err != nil { // not written: it stays held
		p.mu.Lock()
		s.held = append([]held{{op, text}}, s.held...)
		s.receipts[op] = delivery.Receipt{Op: op, UUID: delivery.UUIDFor(op), State: delivery.Queued}
		p.mu.Unlock()
	}
	return got, err
}

// Rewatch takes op, written by an earlier daemon, back into what the receipts confirm. False when agent is not a Chat agent.
func (p *Port) Rewatch(a, op string, _ time.Time) bool {
	if p.Chat(a) == nil { // Chat is nil once the holder is gone
		return false
	}
	p.mu.Lock()
	p.state(a).ops[delivery.UUIDFor(op)] = op
	p.mu.Unlock()
	return true
}

// Interrupt stops the running turn.
func (p *Port) Interrupt(a string) error {
	c := p.Chat(a)
	if c == nil {
		return errors.New(a + " does not run in Chat mode")
	}
	return p.write(p.Socket(a), c.InterruptLine(fmt.Sprintf("hl_interrupt_%d", time.Now().UnixNano())))
}

// Close ends the session's input: the CLI finishes its turn and exits.
func (p *Port) Close(a string) error {
	if p.Chat(a) == nil {
		return errors.New(a + " does not run in Chat mode")
	}
	return holder.CloseInput(p.Socket(a))
}

// Ready says the session has answered the line that opened it.
func (p *Port) Ready(a string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state(a).ready
}

// Watch reads agent's holder until it ends, unless a reader already does. It returns at once.
func (p *Port) Watch(ctx context.Context, a string) {
	c := p.Chat(a)
	if c == nil {
		return
	}
	p.mu.Lock()
	s := p.state(a)
	if s.watching || s.dialing {
		p.mu.Unlock()
		return
	}
	s.dialing = true
	p.mu.Unlock()
	open := p.Open
	if open == nil {
		open = holder.Open
	}
	r, err := open(ctx, p.Socket(a), 0)
	p.mu.Lock()
	s.dialing = false
	s.watching = err == nil
	p.mu.Unlock()
	if err == nil {
		go p.read(a, c, r)
	}
}

func (p *Port) read(a string, c agent.Chat, r *holder.Reader) {
	defer func() {
		r.Close()
		p.mu.Lock()
		s := p.state(a)
		s.watching, s.ready, s.active = false, false, map[string]bool{}
		s.state, s.tasks, s.tasksKnown, s.limit = "", nil, false, agent.Limit{}
		lost, unsent := s.ops, s.held
		s.ops, s.held, s.receipts = map[string]string{}, nil, map[string]delivery.Receipt{}
		p.mu.Unlock()
		for _, op := range lost {
			p.failed(a, op, "the session ended before it took the message", true)
		}
		for _, h := range unsent {
			p.failed(a, h.op, "the session ended while the message waited for its turn", false)
		}
	}()
	for {
		rec, err := r.Next()
		if err != nil {
			return
		}
		p.handle(a, c, rec)
		_ = r.Ack(rec.Seq)
		if rec.Holder == "exit" {
			return
		}
	}
}

func (p *Port) handle(a string, c agent.Chat, rec holder.Record) {
	if rec.Holder == "ready" {
		p.mu.Lock()
		p.state(a).ready = true
		p.mu.Unlock()
		return
	}
	if len(rec.Line) == 0 {
		return
	}
	ev := c.Read(rec.Line)
	switch ev.Kind {
	case agent.EventInit:
		if p.OnSession != nil && ev.SessionID != "" {
			p.OnSession(a, ev.SessionID)
		}
	case agent.EventCommand:
		switch ev.State {
		case "queued", "started":
			p.setActive(a, ev.UUID, true)
			p.confirm(a, ev.UUID)
		case "completed":
			p.confirm(a, ev.UUID)
			p.finished(a, ev.UUID)
		case "cancelled", "discarded", "refused":
			if op := p.take(a, ev.UUID); op != "" {
				p.failed(a, op, "the session "+ev.State+" it", true)
			}
			p.finished(a, ev.UUID)
		}
	case agent.EventEcho:
		p.confirm(a, ev.UUID)
	case agent.EventResult:
		if p.OnTurnEnd != nil {
			p.OnTurnEnd(a, ev.IsError)
		}
		p.release(a)
	case agent.EventState:
		p.mu.Lock()
		p.state(a).state = ev.State
		p.mu.Unlock()
	case agent.EventBackground:
		p.mu.Lock()
		s := p.state(a)
		s.tasks, s.tasksKnown = ev.Tasks, true
		p.mu.Unlock()
	case agent.EventLimit:
		p.mu.Lock()
		p.state(a).limit = ev.Limit
		p.mu.Unlock()
	case agent.EventAsk:
		_ = p.write(p.Socket(a), c.Refuse(ev, NoAsks))
	case agent.EventRequest:
		_ = p.write(p.Socket(a), c.Unserved(ev))
	}
}

func (p *Port) setActive(a, uuid string, on bool) {
	if uuid == "" {
		return
	}
	p.mu.Lock()
	if on {
		p.state(a).active[uuid] = true
	} else {
		delete(p.state(a).active, uuid)
	}
	p.mu.Unlock()
}

// finished is a command's end: once none is left running, the held messages go.
func (p *Port) finished(a, uuid string) {
	p.setActive(a, uuid, false)
	p.release(a)
}

func (p *Port) take(a, uuid string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.state(a)
	op := s.ops[uuid]
	delete(s.ops, uuid)
	if op != "" {
		r := s.receipts[op]
		r.State = delivery.Confirmed
		s.receipts[op] = r
	}
	return op
}

func (p *Port) confirm(a, uuid string) {
	if op := p.take(a, uuid); op != "" && p.OnDelivered != nil {
		p.OnDelivered(a, op)
	}
}

func (p *Port) failed(a, op, why string, written bool) {
	if p.OnFailed != nil {
		p.OnFailed(a, op, why, written)
	}
}

// release lets the held messages go, in order, once no command runs: the first starts the next turn, and those after it wait in the
// CLI's own queue behind it. A result with commands still running (one that joined the turn finishes later) releases nothing.
func (p *Port) release(a string) {
	p.mu.Lock()
	s := p.state(a)
	if len(s.active) > 0 {
		p.mu.Unlock()
		return
	}
	next := s.held
	s.held = nil
	for _, h := range next {
		delete(s.receipts, h.op)
	}
	p.mu.Unlock()
	for _, h := range next {
		if _, err := p.Send(a, h.op, h.text, delivery.Now); err != nil {
			p.failed(a, h.op, err.Error(), false)
		}
	}
}

// Run watches each agent names lists whose holder is up, every tick, until ctx ends: a session just launched and one a restarted
// daemon finds are read the same way.
func (p *Port) Run(ctx context.Context, names func() []string, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		for _, a := range names() {
			if !p.Live(a) {
				p.Watch(ctx, a)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Idle is whether agent's session is between turns, as its stream last said; known is false while it has not said.
func (p *Port) Idle(a string) (idle, known bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.peek(a).state
	return st == "idle", st != ""
}

// peek is a's state without keeping one for an agent never seen.
func (p *Port) peek(a string) session {
	if s := p.agents[a]; s != nil {
		return *s
	}
	return session{}
}

// Background is the work agent's session still runs behind its turn; known is false while its stream has not listed it.
func (p *Port) Background(a string) ([]agent.Task, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.peek(a)
	return slices.Clone(s.tasks), s.tasksKnown
}

// Limit is the usage limit agent's session last heard of.
func (p *Port) Limit(a string) agent.Limit {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peek(a).limit
}

// StopBackground asks the session to stop each piece of background work it listed, or while it has listed none, each of ids; n is how
// many were asked.
func (p *Port) StopBackground(a string, ids []string) (n int, err error) {
	c := p.Chat(a)
	if c == nil {
		return 0, errors.New(a + " does not run in Chat mode")
	}
	if tasks, known := p.Background(a); known {
		ids = ids[:0:0]
		for _, t := range tasks {
			ids = append(ids, t.ID)
		}
	}
	stamp := time.Now().UnixNano()
	for i, id := range ids {
		if err := p.write(p.Socket(a), c.StopTaskLine(fmt.Sprintf("hl_stop_%d_%d", stamp, i), id)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
