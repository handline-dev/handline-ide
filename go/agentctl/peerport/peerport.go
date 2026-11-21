// Package peerport is the supervisor's Agents port for a terminal agent: a message goes in through the session's peer inbox, never as
// keystrokes. Send answers Sent at once and delivers in the background, one message at a time per agent and in order; OnDelivered
// (the transcript showed it) or OnFailed (it did not after the resend) says how it went. Now and Queue are the same write: the receiver
// reads a mid-turn message when the turn ends. The founder's own words (Typed) are the exception: they are typed into the terminal,
// and only when nothing on it would take them for an answer.
package peerport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/peerinbox"
)

// Deliverer is what the port needs of the inbox: *peerinbox.Sender.
type Deliverer interface {
	Deliver(ctx context.Context, sess peerinbox.Session, text string, from peerinbox.From, id string) peerinbox.Result
	Watch(ctx context.Context, sess peerinbox.Session, id string, written time.Time) (seen, queued bool)
	Type(ctx context.Context, sess peerinbox.Session, text string, typeIn func(text string) error) peerinbox.Result
}

// Port delivers to terminal agents.
type Port struct {
	// Peer says whether agent is delivered to through its inbox.
	Peer func(agent string) bool
	// Find is the agent's live session record.
	Find   func(agent string) (peerinbox.Session, error)
	Sender Deliverer
	// OnDelivered is called once the transcript shows op. OnFailed is called when it did not, after the resend, or the session has no
	// inbox any more; why carries nothing from the connection or the key, and written says whether the message was written at all.
	OnDelivered func(agent, op string)
	OnFailed    func(agent, op, why string, written bool)
	Ctx         context.Context
	// From says who the message op to agent is from and the permission class attested for it; nil is the daemon, attesting none.
	From func(agent, op string) peerinbox.From
	// Typed says the message op is typed, not sent to the inbox. Typeable is "" when the agent's terminal may be typed at now, else why
	// not; TypeIn types text there and presses Enter.
	Typed    func(agent, op string) bool
	Typeable func(agent string) string
	TypeIn   func(agent, text string) error

	mu    sync.Mutex
	lines map[string]*line
}

type job struct {
	op, text string
	from     peerinbox.From
	typed    bool
}

type line struct {
	jobs    []job
	running bool
}

// Structured says whether agent is delivered to by the send path: here, whether it uses its inbox.
func (p *Port) Structured(agent string) bool { return p.Peer != nil && p.Peer(agent) }

// Live says whether the agent's session has an inbox that can be written to.
func (p *Port) Live(agent string) bool {
	_, err := p.Find(agent)
	return err == nil
}

// Send takes the message and delivers it in the background; that an op goes out once is the ledger's (supervisor sendOnce). An error means nothing was written.
func (p *Port) Send(agent, op, text string, _ delivery.Delivery) (delivery.Receipt, error) {
	if _, err := p.Find(agent); err != nil {
		return delivery.Receipt{}, errors.New(agent + " has no live session inbox: " + err.Error())
	}
	typed := p.Typed != nil && p.Typed(agent, op)
	if typed {
		if why := p.Typeable(agent); why != "" {
			return delivery.Receipt{}, errors.New(agent + ": not typed - " + why)
		}
	}
	var from peerinbox.From
	if p.From != nil {
		from = p.From(agent, op)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lines == nil {
		p.lines = map[string]*line{}
	}
	l := p.lines[agent]
	if l == nil {
		l = &line{}
		p.lines[agent] = l
	}
	l.jobs = append(l.jobs, job{op, text, from, typed})
	if !l.running {
		l.running = true
		go p.drain(agent, l)
	}
	return delivery.Receipt{Op: op, State: delivery.Sent}, nil
}

// Withdraw cannot take back a message the inbox has taken: it may already be written.
func (p *Port) Withdraw(_, op string) (delivery.Receipt, bool, error) {
	return delivery.Receipt{Op: op, State: delivery.Sent}, false, nil
}

// Promote has nothing to promote: a message is written as soon as its turn in the agent's line comes.
func (p *Port) Promote(_, op string) (delivery.Receipt, error) {
	return delivery.Receipt{Op: op, State: delivery.Sent}, nil
}

// Rewatch goes on watching for op, which an earlier daemon recorded as written to agent at written and had not seen in the transcript:
// OnDelivered once the transcript shows it, OnFailed when it neither shows nor was queued within the usual window (queued only, it is left
// as Deliver leaves it). False, and nothing is watched, when op was typed (matched by its words, which only the writer had) or the agent has
// no live session already running at written.
func (p *Port) Rewatch(agent, op string, written time.Time) bool {
	if p.Typed != nil && p.Typed(agent, op) {
		return false
	}
	sess, err := p.Find(agent)
	if err != nil || sess.StartedAt == 0 || sess.StartedAt/1000 > written.Unix() {
		return false
	}
	ctx := p.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		seen, queued := p.Sender.Watch(ctx, sess, MsgID(agent, op), written)
		switch {
		case seen && p.OnDelivered != nil:
			p.OnDelivered(agent, op)
		case !seen && !queued:
			p.failed(agent, op, "not in the receiver's transcript", true)
		}
	}()
	return true
}

// MsgID is the id op is written to agent under: the same in every daemon, so the next one can find it in the transcript.
func MsgID(agent, op string) string {
	sum := sha256.Sum256([]byte(agent + "\x00" + op))
	return hex.EncodeToString(sum[:8])
}

func (p *Port) drain(agent string, l *line) {
	ctx := p.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		p.mu.Lock()
		if len(l.jobs) == 0 {
			l.running = false
			p.mu.Unlock()
			return
		}
		j := l.jobs[0]
		l.jobs = l.jobs[1:]
		p.mu.Unlock()
		p.deliver(ctx, agent, j)
	}
}

func (p *Port) deliver(ctx context.Context, agent string, j job) {
	sess, err := p.Find(agent)
	if err != nil {
		p.failed(agent, j.op, err.Error(), false)
		return
	}
	var res peerinbox.Result
	if j.typed {
		res = p.Sender.Type(ctx, sess, j.text, func(text string) error { return p.TypeIn(agent, text) })
	} else {
		res = p.Sender.Deliver(ctx, sess, j.text, j.from, MsgID(agent, j.op))
	}
	if res.Kind == peerinbox.Delivered {
		if p.OnDelivered != nil {
			p.OnDelivered(agent, j.op)
		}
		return
	}
	if res.Kind == peerinbox.Queued {
		// The receiver has it and shows it when its turn ends: the ledger stays written and the sender is told nothing. The line is
		// not held for it; the confirmation comes when the transcript shows it, and a session that ends first is settled as lost.
		if res.Await != nil && p.OnDelivered != nil {
			go func() {
				if res.Await(ctx) {
					p.OnDelivered(agent, j.op)
				}
			}()
		}
		return
	}
	p.failed(agent, j.op, res.Why, res.Kind != peerinbox.NoSocket && (!j.typed || res.Attempts > 0))
}

func (p *Port) failed(agent, op, why string, written bool) {
	if p.OnFailed != nil {
		p.OnFailed(agent, op, why, written)
	}
}
