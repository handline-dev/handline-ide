//go:build fakes

package fake

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/term"
)

type rig struct {
	t         *testing.T
	tm        term.Fake
	h         string
	mu        sync.Mutex
	delivered []string
}

// newRig starts the fake CLI in a fake pane, as a launch types its line into a fresh shell.
func newRig(t *testing.T, prompt string) *rig {
	a := Agent{}
	argv := append(a.LaunchArgv(a.Binary(), agent.LaunchArgs{}), a.PromptArgv(prompt)...)
	quoted := make([]string, len(argv))
	for i, s := range argv {
		quoted[i] = "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
	}
	r := &rig{t: t, tm: term.Fake{Place: term.Place{Key: t.TempDir()}}}
	h, why := r.tm.CreateTerminal("exec "+strings.Join(quoted, " "), t.TempDir(), "x", "f.one")
	if h == "" {
		t.Fatal(why)
	}
	r.h = h
	t.Cleanup(func() { r.tm.CloseTerminal(h, true) })
	return r
}

func (r *rig) screen() []string { return r.tm.ReadScreen(r.h, 20).Tail }

func (r *rig) waitFor(cond func([]string) bool, what string) {
	r.t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if cond(r.screen()) {
			return
		}
	}
	r.t.Fatalf("%s: %q", what, r.screen())
}

func (r *rig) idle(tail []string) bool {
	return !(Screen{}).TurnOnScreen(tail) && strings.HasSuffix(tail[len(tail)-1], Prompt)
}

func (r *rig) port() *Port {
	return Agent{}.Deliver(agent.DeliverEnv{
		Where: func(string) (agent.Place, error) { return agent.Place{}, nil },
		Typeable: func(string) string {
			if (Screen{}).TurnOnScreen(r.screen()) {
				return "a turn is running"
			}
			return ""
		},
		TypeIn:      func(_, text string) error { r.tm.Send(r.h, text, true); return nil },
		OnDelivered: func(_, op string) { r.mu.Lock(); r.delivered = append(r.delivered, op); r.mu.Unlock() },
	}).(*Port)
}

func TestTheFakeTakesItsLaunchPromptAsAFirstTurnAndShowsTheTurnOnScreen(t *testing.T) {
	r := newRig(t, "work 1")
	r.waitFor(func(tail []string) bool { return (Screen{}).TurnOnScreen(tail) }, "the launch prompt runs as a turn")
	if w, known := (Screen{}).WorkingOnScreen(r.screen()); !w || !known {
		t.Fatal("working is known")
	}
	r.waitFor(r.idle, "the turn ends at the prompt")
	if !strings.Contains(strings.Join(r.screen(), "\n"), "fake reply: work 1") {
		t.Fatalf("%q", r.screen())
	}
}

func TestTheFakePortWritesNowHoldsBehindATurnAndAnswersARepeatWithTheFirstReceipt(t *testing.T) {
	r := newRig(t, "")
	r.waitFor(r.idle, "ready")
	p := r.port()
	if !p.Structured("f.one") || !p.Live("f.one") {
		t.Fatal("a structured, live session")
	}
	rc, err := p.Send("f.one", "op1", "work 1", delivery.Queue)
	if err != nil || rc.State != delivery.Sent || rc.UUID != delivery.UUIDFor("op1") {
		t.Fatalf("idle: written at once: %+v %v", rc, err)
	}
	r.waitFor(func(tail []string) bool { return (Screen{}).TurnOnScreen(tail) }, "the message is a turn")
	if again, _ := p.Send("f.one", "op1", "other", delivery.Queue); again != rc {
		t.Fatalf("a repeat writes nothing: %+v", again)
	}
	held, _ := p.Send("f.one", "op2", "second", delivery.Queue)
	if held.State != delivery.Queued {
		t.Fatalf("held behind the turn: %+v", held)
	}
	gone, _ := p.Send("f.one", "op3", "never", delivery.Queue)
	if w, ok, _ := p.Withdraw("f.one", "op3"); !ok || w.State != delivery.Withdrawn || gone.State != delivery.Queued {
		t.Fatalf("a held message is withdrawn: %+v %v", w, ok)
	}
	if _, ok, _ := p.Withdraw("f.one", "op1"); ok {
		t.Fatal("a written message is not withdrawn")
	}
	now, _ := p.Send("f.one", "op4", "joins", delivery.Now)
	if now.State != delivery.Sent {
		t.Fatalf("now joins the running turn: %+v", now)
	}
	r.waitFor(func(tail []string) bool { return strings.Contains(strings.Join(tail, "\n"), "fake reply: second") }, "the held one is written when the turn ends")
	if s := strings.Join(r.screen(), "\n"); strings.Contains(s, "never") {
		t.Fatalf("the withdrawn one never reached the CLI: %q", s)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.Join(r.delivered, ",") != "op1,op4,op2" {
		t.Fatalf("delivered %v", r.delivered)
	}
}

func TestPromoteWritesAHeldMessageNow(t *testing.T) {
	r := newRig(t, "work 2")
	r.waitFor(func(tail []string) bool { return (Screen{}).TurnOnScreen(tail) }, "a turn")
	p := r.port()
	if rc, _ := p.Send("f.one", "op", "pushed", delivery.Queue); rc.State != delivery.Queued {
		t.Fatalf("%+v", rc)
	}
	if rc, err := p.Promote("f.one", "op"); err != nil || rc.State != delivery.Sent {
		t.Fatalf("%+v %v", rc, err)
	}
	if rc, _ := p.Promote("f.one", "op"); rc.State != delivery.Sent {
		t.Fatalf("promoting a written one answers its receipt: %+v", rc)
	}
	if _, ok := agent.Lookup(Vendor); !ok {
		t.Fatal("the fake registers its account facts")
	}
}
