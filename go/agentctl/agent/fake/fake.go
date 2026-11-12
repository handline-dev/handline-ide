//go:build fakes

// Package fake is the agent CLI of the fakes build (vendor "fake"): a scripted sh session that does only what agent.Agent and agent.Port
// ask, so the conformance run can play the fixtures that need an agent without Claude Code. A message is a turn: the screen says
// "fake working: <text>" until the reply, then shows the prompt again. "work <n>" keeps the turn running n seconds.
package fake

import (
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/term"
)

// Vendor is the vendor an agent's row names to run on the fake.
const Vendor = "fake"

// Prompt is the line the fake shows when no turn runs; Working starts the line of a turn in progress.
const (
	Prompt  = "fake> "
	Working = "fake working: "
)

// script is the fake CLI. $1 is the launch prompt, which it takes as a first message.
const script = `turn() { printf 'fake working: %s\n' "$1"; case "$1" in "work "*) sleep "${1#work }";; esac; printf 'fake reply: %s\n' "$1"; }
[ -n "$1" ] && turn "$1"
printf 'fake> '
while IFS= read -r line; do turn "$line"; printf 'fake> '; done`

type Agent struct{}

func init() {
	agent.Register(Agent{})
	agent.RegisterAgent(Vendor, func(agent.Host) agent.Agent { return Agent{} })
}

var _ agent.Agent = Agent{}

func (Agent) Name() string                                { return Vendor }
func (Agent) ConfigDirName() string                       { return ".fake" }
func (Agent) ConfigDirEnv() string                        { return "HANDLINE_FAKE_CONFIG_DIR" }
func (Agent) LoginArgv() []string                         { return []string{"true"} }
func (Agent) LogoutArgv() []string                        { return []string{"true"} }
func (Agent) WebLoginArgv() ([]string, bool)              { return nil, false }
func (Agent) CredentialPresent(string) (bool, bool)       { return true, true }
func (Agent) CredentialModified(string) *float64          { return nil }
func (Agent) IsResume([]string) bool                      { return false }
func (Agent) Binary() string                              { return "/bin/sh" }
func (Agent) Screen() term.Driver                         { return Screen{} }
func (Agent) Hooks() agent.Hooks                          { return nil }
func (Agent) AutoModeArgv() []string                      { return nil }
func (Agent) PermissionModeArgv(string) []string          { return nil }
func (Agent) LaunchEnv(_, _, _ string) map[string]string  { return map[string]string{} }
func (Agent) PromptArgv(prompt string) []string           { return []string{prompt} }
func (Agent) TranscriptDir(string, string) (string, bool) { return "", false }

// LaunchArgv runs the script; what follows it (PromptArgv) is its $1.
func (Agent) LaunchArgv(exe string, _ agent.LaunchArgs) []string {
	return []string{exe, "-c", script, Vendor}
}

// Screen is the fake's wording: a turn is a last line that starts with Working, and nothing on it is a dialog or a question.
type Screen struct{ term.Blind }

func (Screen) Name() string { return Vendor }

func (Screen) WorkingOnScreen(tail []string) (bool, bool) {
	for i := len(tail) - 1; i >= 0; i-- {
		// a pipe echoes no newline after the typed line, so a turn starts on the prompt's line
		if l := strings.TrimPrefix(strings.TrimRight(tail[i], " "), Prompt); l != "" {
			return strings.HasPrefix(l, Working), true
		}
	}
	return false, false
}

func (s Screen) TurnOnScreen(tail []string) bool { w, _ := s.WorkingOnScreen(tail); return w }

// Deliver types a message into the fake's terminal (it has no inbox) and settles it at once: the fake reads every line it is typed,
// so a write is a delivery. A message sent with Queue while a turn runs is held, and written when the turn ends or it is promoted.
func (Agent) Deliver(env agent.DeliverEnv) agent.Port {
	return &Port{env: env, receipts: map[string]delivery.Receipt{}, held: map[string]string{}, poll: 100 * time.Millisecond}
}

type Port struct {
	env      agent.DeliverEnv
	poll     time.Duration
	mu       sync.Mutex
	receipts map[string]delivery.Receipt // agent+"\x00"+op
	held     map[string]string           // agent+"\x00"+op -> text
}

func key(agent, op string) string { return agent + "\x00" + op }

func (*Port) Structured(string) bool { return true }

func (p *Port) Live(a string) bool {
	if p.env.Where == nil {
		return false
	}
	_, err := p.env.Where(a)
	return err == nil
}

// busy is whether a turn runs now: the terminal is not typeable while one does.
func (p *Port) busy(a string) bool { return p.env.Typeable != nil && p.env.Typeable(a) != "" }

func (p *Port) Send(a, op, text string, d delivery.Delivery) (delivery.Receipt, error) {
	p.mu.Lock()
	if r, ok := p.receipts[key(a, op)]; ok {
		p.mu.Unlock()
		return r, nil
	}
	if d != delivery.Now && p.busy(a) {
		r := delivery.Receipt{Op: op, UUID: delivery.UUIDFor(op), State: delivery.Queued}
		p.receipts[key(a, op)], p.held[key(a, op)] = r, text
		p.mu.Unlock()
		go p.writeWhenIdle(a, op)
		return r, nil
	}
	p.mu.Unlock()
	return p.write(a, op, text)
}

func (p *Port) write(a, op, text string) (delivery.Receipt, error) {
	if err := p.env.TypeIn(a, text); err != nil {
		return delivery.Receipt{}, err
	}
	r := delivery.Receipt{Op: op, UUID: delivery.UUIDFor(op), State: delivery.Sent}
	p.mu.Lock()
	p.receipts[key(a, op)] = r
	delete(p.held, key(a, op))
	p.mu.Unlock()
	if p.env.OnDelivered != nil {
		p.env.OnDelivered(a, op)
	}
	return r, nil
}

func (p *Port) writeWhenIdle(a, op string) {
	for {
		time.Sleep(p.poll)
		p.mu.Lock()
		text, ok := p.held[key(a, op)]
		p.mu.Unlock()
		if !ok {
			return // withdrawn or promoted
		}
		if !p.busy(a) {
			if _, err := p.write(a, op, text); err != nil && p.env.OnFailed != nil {
				p.env.OnFailed(a, op, err.Error(), false)
			}
			return
		}
	}
}

func (p *Port) Withdraw(a, op string) (delivery.Receipt, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, known := p.receipts[key(a, op)]
	if _, held := p.held[key(a, op)]; held || known && r.State == delivery.Withdrawn {
		r = delivery.Receipt{Op: op, UUID: delivery.UUIDFor(op), State: delivery.Withdrawn}
		p.receipts[key(a, op)] = r
		delete(p.held, key(a, op))
		return r, true, nil
	}
	return r, false, nil
}

func (p *Port) Promote(a, op string) (delivery.Receipt, error) {
	p.mu.Lock()
	text, held := p.held[key(a, op)]
	r := p.receipts[key(a, op)]
	p.mu.Unlock()
	if !held {
		return r, nil
	}
	return p.write(a, op, text)
}
