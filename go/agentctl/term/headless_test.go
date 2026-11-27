package term_test

import (
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/term"
)

type fakeChat struct {
	interrupts, ends, stops int
	state, running          string // state "" is a stream that has not said
	limit                   bool
}

func (c *fakeChat) Headless(h string) bool { return h == "chat" }
func (c *fakeChat) Interrupt(string) error { c.interrupts++; return nil }
func (c *fakeChat) EndInput(string) error  { c.ends++; return nil }
func (c *fakeChat) Idle(string) (bool, bool) {
	return c.state == "idle", c.state != ""
}
func (c *fakeChat) Background(string) (string, bool) { return c.running, c.state != "" }
func (c *fakeChat) AtLimit(string) bool              { return c.limit }
func (c *fakeChat) StopBackground(_ string, ids []string) (int, error) {
	c.stops++
	return 1 + len(ids), nil
}

func TestAHeadlessTerminalIsNeverReadOrTypedAtAndEscInterruptsOnItsStream(t *testing.T) {
	reads := 0
	h := &fakeHost{screenAt: func(int) []string { reads++; return permissionPrompt }}
	tm := newTerm(h)
	c := &fakeChat{}
	tm.Chat = c
	tm.Sleep = func(time.Duration) {}
	if tm.ScreenBlocker("chat") != nil || tm.DialogUp("chat") != nil || tm.TrustPromptShowing("chat") || tm.UsageLimitOpen("chat") {
		t.Fatal("a dialog read on a headless terminal")
	}
	if _, ok := tm.ScreenText("chat"); ok || reads != 0 {
		t.Fatalf("its screen was read %d times", reads)
	}
	if on, known := tm.SessionOnScreen("chat"); !on || !known {
		t.Fatal("its holder is its session")
	}
	if ok, _ := tm.Send("chat", "hello", true); ok || len(h.sent) != 0 {
		t.Fatalf("typed at: %v", h.sent)
	}
	if ok, why := tm.Send("chat", term.Escape, false); !ok || c.interrupts != 1 || len(h.sent) != 0 {
		t.Fatalf("Esc is the stream's interrupt: %v %s", h.sent, why)
	}
	if tm.ScreenBlocker("other") == nil || reads == 0 {
		t.Fatal("another terminal is read as before")
	}
}

// A headless session's turn, background work and usage limit are what its stream says, and its background work stops on the stream.
func TestAHeadlessSessionsStateIsWhatItsStreamSays(t *testing.T) {
	tm := newTerm(&fakeHost{screenAt: func(int) []string { return permissionPrompt }})
	c := &fakeChat{state: "running"}
	tm.Chat = c
	tm.Sleep = func(time.Duration) {}
	if tm.IdleNow("chat") || tm.WaitIdle("chat", 0) {
		t.Fatal("idle mid-turn")
	}
	if shown, ok := tm.TurnShown("chat"); !shown || !ok {
		t.Fatal("the turn is not shown")
	}
	c.state, c.running, c.limit = "idle", "sleep 300", true
	if !tm.IdleNow("chat") || !tm.WaitIdle("chat", 1000) || !tm.IdleWithBackground("chat") || !tm.UsageLimitOpen("chat") {
		t.Fatal("idle with work behind it, at its limit")
	}
	if running, known := tm.BackgroundWork("chat"); running != "sleep 300" || !known {
		t.Fatal(running, known)
	}
	if n, err := tm.StopBackground("chat", nil); n != 1 || err != nil || c.stops != 1 {
		t.Fatal(n, err)
	}
	if _, err := tm.StopBackground("other", nil); err == nil {
		t.Fatal("stopped a terminal's work on a stream")
	}
}

// Closing a headless session's terminal first ends it on its stream: a running turn interrupted, then its input ended.
func TestClosingAHeadlessTerminalEndsItsSessionOnTheStreamFirst(t *testing.T) {
	tm := newTerm(&fakeHost{screenAt: func(int) []string { return nil }})
	c := &fakeChat{state: "running"}
	tm.Chat = c
	tm.Sleep = func(time.Duration) {}
	tm.HeadlessEndWait = 0
	tm.CloseTerminal("chat")
	if c.interrupts != 1 || c.ends != 1 {
		t.Fatalf("interrupts %d, ends %d", c.interrupts, c.ends)
	}
	c.state = "idle"
	tm.CloseTerminal("chat")
	tm.CloseTerminal("other")
	if c.interrupts != 1 || c.ends != 2 {
		t.Fatalf("an idle session is not interrupted: %d, %d", c.interrupts, c.ends)
	}
}

// hookTurns is a hook tracker that says the session is idle with work behind it.
type hookTurns struct{}

func (hookTurns) WaitIdle(string, time.Duration) (bool, bool) { return true, true }
func (hookTurns) Background(string) (string, bool)            { return "sleep 300", true }
func (hookTurns) Settled(string) bool                         { return true }
func (hookTurns) Session(string) (bool, bool)                 { return true, true }

// A stream that has not said yet (a daemon just restarted) leaves the turn and the background work to the hooks.
func TestAHeadlessSessionsHooksAnswerWhileItsStreamHasNotSaid(t *testing.T) {
	tm := newTerm(&fakeHost{screenAt: func(int) []string { return nil }})
	tm.Chat = &fakeChat{}
	tm.Turns = hookTurns{}
	if running, known := tm.BackgroundWork("chat"); running != "sleep 300" || !known {
		t.Fatal(running, known)
	}
	if !tm.IdleWithBackground("chat") || !tm.IdleNow("chat") || !tm.WaitIdle("chat", 0) {
		t.Fatal("the hooks' idle with work behind it")
	}
}
