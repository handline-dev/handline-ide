package term_test

import (
	"testing"

	"github.com/handline/handline/go/agentctl/term"
)

type fakeChat struct{ interrupts, ends int }

func (c *fakeChat) Headless(h string) bool { return h == "chat" }
func (c *fakeChat) Interrupt(string) error { c.interrupts++; return nil }
func (c *fakeChat) EndInput(string) error  { c.ends++; return nil }

func TestAHeadlessTerminalIsNeverReadOrTypedAtAndEscInterruptsOnItsStream(t *testing.T) {
	reads := 0
	h := &fakeHost{screenAt: func(int) []string { reads++; return permissionPrompt }}
	tm := newTerm(h)
	c := &fakeChat{}
	tm.Chat = c
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
