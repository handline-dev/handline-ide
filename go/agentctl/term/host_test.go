package term_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/term"
)

// hostBackend is a backend with a host: it shows a screen, lists its terminals and records what is typed and respawned.
type hostBackend struct {
	mu      sync.Mutex
	screen  func(sent int) []string
	live    map[string]bool
	sent    []string
	respawn []string
	closed  []string
	waited  int
}

func (h *hostBackend) Name() string                                   { return "fake" }
func (h *hostBackend) Available() bool                                { return true }
func (h *hostBackend) ListTerminals() map[string]bool                 { return h.live }
func (h *hostBackend) WaitIdle(string, int, func([]string) bool) bool { h.waited++; return true }
func (h *hostBackend) ReadScreen(string, int) *term.Screen {
	h.mu.Lock()
	defer h.mu.Unlock()
	return &term.Screen{Tail: h.screen(len(h.sent)), Source: "screen"}
}
func (h *hostBackend) Send(_, text string, enter bool) (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent = append(h.sent, fmt.Sprintf("%s/%v", text, enter))
	return true, ""
}
func (h *hostBackend) CreateTerminal(string, string, string, string) (string, string) {
	return "%1", ""
}
func (h *hostBackend) CloseTerminal(handle string, _ bool) (bool, string) {
	h.closed = append(h.closed, handle)
	return true, ""
}
func (h *hostBackend) RespawnTerminal(handle string) (bool, string) {
	h.respawn = append(h.respawn, handle)
	return true, ""
}

func hostTerm(h *hostBackend) *term.Term {
	t := term.New(h, claudecode.ScreenFor)
	t.Settle = 0
	var now time.Time
	t.Now = func() time.Time { return now }
	t.Sleep = func(d time.Duration) { now = now.Add(d) }
	return t
}

var composer = []string{rule, "│ > │", rule, "  ? for shortcuts"}

func TestASessionIsEndedByRestartingItsShellNotByTypingAtIt(t *testing.T) {
	h := &hostBackend{live: map[string]bool{"%1": true}, screen: func(int) []string { return composer }}
	ok, why := hostTerm(h).CloseSession("%1")
	if !ok || len(h.sent) != 0 || len(h.respawn) != 1 || h.respawn[0] != "%1" {
		t.Fatalf("%v %q sent %q respawn %q", ok, why, h.sent, h.respawn)
	}
	// a terminal that is gone, and a dialog standing on the screen, are left alone
	h = &hostBackend{live: map[string]bool{}, screen: func(int) []string { return composer }}
	if ok, why := hostTerm(h).CloseSession("%1"); ok || !strings.Contains(why, "not alive") || len(h.respawn) != 0 {
		t.Fatalf("%v %q", ok, why)
	}
	h = &hostBackend{live: map[string]bool{"%1": true}, screen: func(int) []string { return permissionPrompt }}
	if ok, why := hostTerm(h).CloseSession("%1"); ok || len(h.respawn) != 0 || len(h.sent) != 0 {
		t.Fatalf("a modal was ended under: %v %q", ok, why)
	}
}

func TestARelaunchTypesTheLineThenItsEnterAndAsksWhetherAnythingStarted(t *testing.T) {
	shell := []string{"~/work $ "}
	h := &hostBackend{live: map[string]bool{"%1": true}, screen: func(int) []string { return shell }}
	ok, why := hostTerm(h).Relaunch("%1", "hl launch alice", func() bool { return true })
	if !ok || strings.Join(h.sent, ",") != "hl launch alice/false,/true" {
		t.Fatalf("%v %q %q", ok, why, h.sent)
	}
	h = &hostBackend{live: map[string]bool{"%1": true}, screen: func(int) []string { return shell }}
	ok, why = hostTerm(h).Relaunch("%1", "hl launch alice", func() bool { return false })
	if ok || !strings.Contains(why, "no session started") || !strings.Contains(why, "the terminal is alive (screen: ~/work $)") {
		t.Fatalf("%v %q", ok, why)
	}
	// the launch is done once its session is drawn: a first turn that keeps the screen busy is not waited out
	turn := append([]string{"✻ Working… (esc to interrupt)"}, composer...)
	h = &hostBackend{live: map[string]bool{"%1": true}, screen: func(n int) []string {
		if n < 2 {
			return shell
		}
		return turn
	}}
	if ok, why := hostTerm(h).Relaunch("%1", "hl launch alice", func() bool { return true }); !ok || h.waited != 0 {
		t.Fatalf("%v %q, waited for idle %d times", ok, why, h.waited)
	}
	// a session already drawn there is not typed at
	h = &hostBackend{live: map[string]bool{"%1": true}, screen: func(int) []string { return composer }}
	if ok, why := hostTerm(h).Relaunch("%1", "hl launch alice", nil); ok || len(h.sent) != 0 || !strings.Contains(why, "a session is on the terminal's screen") {
		t.Fatalf("%v %q %q", ok, why, h.sent)
	}
}

func TestWaitingForTheTrustPromptToGoGivesUpAfterFiveSeconds(t *testing.T) {
	trust := []string{"Quick safety check: Is this a project you created or one you trust?"}
	h := &hostBackend{live: map[string]bool{"%1": true}, screen: func(n int) []string {
		if n >= 0 {
			return trust
		}
		return composer
	}}
	if hostTerm(h).WaitForTrustPrompt("%1", false) {
		t.Fatal("it never went")
	}
	if !hostTerm(h).WaitForTrustPrompt("%1", true) {
		t.Fatal("it was showing")
	}
}

// Claude Code's first-run steps as a stuck agent's terminal drew them (Claude Code 2.1.331).
var (
	themeStep = []string{" Let's get started.", " Choose the text style that looks best with your terminal", " To change this later, run /theme",
		" ❯ ✔ Auto (match terminal)", "     Dark mode", "  Syntax theme: Monokai Extended (ctrl+t to disable)"}
	securityStep = []string{" Security notes:", " 1. Claude can make mistakes", "    You're responsible for Claude's actions and should always",
		"    review them, especially when running code.", " 2. Due to prompt injection risks, only use it with code you trust",
		"    Learn more: https://code.claude.com/docs/en/security", " Press Enter to continue"}
)

func TestClaudesFirstRunStepsAreReadAndThemeInASessionIsNot(t *testing.T) {
	for _, s := range [][]string{themeStep, securityStep} {
		if !(claudecode.Screen{}).FirstRunScreen(s) {
			t.Errorf("not read as first-run: %q", s)
		}
	}
	themeInSession := themeStep[1:]                                                                  // /theme in a session draws the picker with no "Let's get started."
	shown := append(append([]string{}, securityStep...), "  shown by a program, with more below it") // the words, not the step
	for _, s := range [][]string{themeInSession, shown, composer, {"~/work $ "}} {
		if (claudecode.Screen{}).FirstRunScreen(s) {
			t.Errorf("read as first-run: %q", s)
		}
	}
}

// A program that started before its config said setup was done sits on a first-run step with no session under it. A wake restarts
// the terminal's shell and launches there, instead of typing the line into the step, which would only move it to the next one.
func TestARelaunchOnAFirstRunStepRestartsTheShellAndLaunchesInIt(t *testing.T) {
	for _, step := range [][]string{themeStep, securityStep} {
		h := &hostBackend{live: map[string]bool{"%1": true}}
		h.screen = func(int) []string {
			if len(h.respawn) == 0 {
				return step
			}
			return []string{"~/work $ "}
		}
		ok, why := hostTerm(h).Relaunch("%1", "hl launch alice", func() bool { return true })
		if !ok || strings.Join(h.respawn, ",") != "%1" || strings.Join(h.sent, ",") != "hl launch alice/false,/true" {
			t.Fatalf("%v %q respawn %q sent %q", ok, why, h.respawn, h.sent)
		}
	}
	// at a shell, nothing is restarted: with nothing above its prompt, or with the step's words printed above it (a file shown, a
	// program that exited)
	for _, scr := range [][]string{{"~/work $ "}, append(append([]string{}, securityStep...), "~/work $ ")} {
		h := &hostBackend{live: map[string]bool{"%1": true}, screen: func(int) []string { return scr }}
		if hostTerm(h).Relaunch("%1", "hl launch alice", func() bool { return true }); len(h.respawn) != 0 {
			t.Fatalf("a shell was restarted: %q", h.respawn)
		}
	}
}
