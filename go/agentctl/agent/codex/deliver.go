package codex

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/delivery"
)

// Deliver types a message into the codex session's terminal (codex has no inbox) and confirms it once the session's transcript records
// it as a message from the person; one the transcript has not shown within the wait is settled as written, maybe not read. A message
// typed mid-turn is taken by codex as the turn goes on, and shows in the transcript then.
func (Driver) Deliver(env agent.DeliverEnv) agent.Port {
	return &Port{env: env, now: time.Now, poll: time.Second, wait: 10 * time.Minute, receipts: map[string]delivery.Receipt{}, typing: map[string]bool{}}
}

type Port struct {
	env        agent.DeliverEnv
	now        func() time.Time
	poll, wait time.Duration

	mu       sync.Mutex
	receipts map[string]delivery.Receipt // agent+"\x00"+op
	typing   map[string]bool             // ops being typed now
}

func (*Port) Structured(string) bool { return true }

func (p *Port) Live(a string) bool {
	if p.env.Where == nil {
		return false
	}
	_, err := p.env.Where(a)
	return err == nil
}

// Send types text now, a Queue message too, mid-turn or not: codex keeps a line typed during a turn, so nothing is held. An error means
// nothing was typed.
func (p *Port) Send(a, op, text string, _ delivery.Delivery) (delivery.Receipt, error) {
	k := a + "\x00" + op
	p.mu.Lock()
	if r, ok := p.receipts[k]; ok {
		p.mu.Unlock()
		return r, nil
	}
	if p.typing[k] { // the first Send of op is typing it now
		p.mu.Unlock()
		return delivery.Receipt{Op: op, UUID: delivery.UUIDFor(op), State: delivery.Sent}, nil
	}
	p.typing[k] = true
	p.mu.Unlock()
	r, err := p.typeIn(a, op, text)
	p.mu.Lock()
	delete(p.typing, k)
	if err == nil {
		p.receipts[k] = r
	}
	p.mu.Unlock()
	return r, err
}

func (p *Port) typeIn(a, op, text string) (delivery.Receipt, error) {
	if p.env.Where == nil || p.env.TypeIn == nil {
		return delivery.Receipt{}, errors.New(a + ": no terminal to type into")
	}
	pl, err := p.env.Where(a)
	if err != nil {
		return delivery.Receipt{}, err
	}
	file, _ := SessionFile(pl.Config, pl.Cwd, p.now())
	var from int64
	if fi, err := os.Stat(file); err == nil {
		from = fi.Size()
	}
	if err := p.env.TypeIn(a, text); err != nil {
		return delivery.Receipt{}, err
	}
	go p.confirm(a, op, strings.TrimSpace(text), file, from)
	return delivery.Receipt{Op: op, UUID: delivery.UUIDFor(op), State: delivery.Sent}, nil
}

// confirm watches the session's transcript from offset from for the typed text. The file is looked for again on every read: a session
// started since the typing has a newer one, read from its start. It gives up when the session ends.
func (p *Port) confirm(a, op, text, file string, from int64) {
	deadline := p.now().Add(p.wait)
	for p.now().Before(deadline) {
		pl, err := p.env.Where(a)
		if err != nil {
			break
		}
		if now, ok := SessionFile(pl.Config, pl.Cwd, p.now()); ok && now != file {
			file, from = now, 0
		}
		if file != "" {
			var seen bool
			if seen, from = shows(file, from, text); seen {
				if p.env.OnDelivered != nil {
					p.env.OnDelivered(a, op)
				}
				return
			}
		}
		time.Sleep(p.poll)
	}
	if p.env.OnFailed != nil {
		p.env.OnFailed(a, op, "codex's transcript did not show it", true)
	}
}

// shows is whether the whole lines of file after offset from record text as a message the person sent, and the offset read up to.
func shows(file string, from int64, text string) (bool, int64) {
	f, err := os.Open(file)
	if err != nil {
		return false, from
	}
	defer f.Close()
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return false, from
	}
	b, _ := io.ReadAll(f)
	end := bytes.LastIndexByte(b, '\n')
	if end < 0 {
		return false, from
	}
	for _, line := range bytes.Split(b[:end], []byte{'\n'}) {
		if got, ok := userText(line); ok && strings.TrimSpace(got) == text {
			return true, from + int64(end) + 1
		}
	}
	return false, from + int64(end) + 1
}

// Withdraw cannot take back a message: it is typed as it is sent.
func (p *Port) Withdraw(a, op string) (delivery.Receipt, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.receipts[a+"\x00"+op], false, nil
}

// Promote has nothing to promote: nothing is held.
func (p *Port) Promote(a, op string) (delivery.Receipt, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.receipts[a+"\x00"+op], nil
}
