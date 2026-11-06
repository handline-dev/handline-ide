package term_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/term"
)

// fakeTmux puts a `tmux` on PATH that logs its arguments, one call a line, and answers like a server: has-session fails until
// new-session has run, and new-window and new-session answer pane %7.
func fakeTmux(t *testing.T) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls")
	script := `#!/bin/sh
printf '%s\n' "$*" >> ` + log + `
case "$3" in
  has-session) [ -f ` + dir + `/up ] && exit 0; echo "no server running on x" >&2; exit 1;;
  new-session) touch ` + dir + `/up; echo %7;;
  new-window) echo %7;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return log
}

// atH is a tmux host as a program at /h sets it up: server t, its home and server in each window's environment.
var atH = term.Tmux{Place: term.Place{Key: "/h", Socket: "t", Env: []string{"HANDLINE_HOME=/h", "HANDLINE_TMUX_SOCKET=t"}}}

func calls(t *testing.T, log string) []string {
	b, _ := os.ReadFile(log)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func TestATerminalIsOpenedInTheWorktreeAndTheLaunchLineTypedAtItsShell(t *testing.T) {
	log := fakeTmux(t)
	handle, why := atH.CreateTerminal("hl launch alice", "/wt/alice", "alice.x", "alice")
	if handle != "%7" || why != "" {
		t.Fatalf("%q %q", handle, why)
	}
	got := calls(t, log)
	// no server yet: a session is made with a size; the window name has no dots or colons; the home goes in the environment
	want := "-L t new-session -d -s handline -x 200 -y 50 -n alice_x -c /wt/alice -e HANDLINE_HOME=/h -e HANDLINE_TMUX_SOCKET=t -P -F #{pane_id}"
	if got[1] != want {
		t.Fatalf("%q", got)
	}
	if got[len(got)-2] != "-L t send-keys -t %7 -l -- hl launch alice" || got[len(got)-1] != "-L t send-keys -t %7 Enter" {
		t.Fatalf("%q", got)
	}
	if !strings.Contains(strings.Join(got, "\n"), "-L t set-option -p -t %7 @hl_agent alice") {
		t.Fatalf("the pane is stamped with its agent: %q", got)
	}
	// a second terminal is a window of the session that is up
	atH.CreateTerminal("hl launch bob", "/wt/bob", "bob", "bob")
	got = calls(t, log)
	if !strings.HasPrefix(got[len(got)-4], "-L t new-window -t handline: -n bob -c /wt/bob") {
		t.Fatalf("%q", got)
	}
}

func TestAWindowWhoseLaunchLineCouldNotBeTypedIsClosedAgain(t *testing.T) {
	log := fakeTmux(t)
	dir := filepath.Dir(log)
	os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+log+"\ncase \"$3\" in has-session) exit 1;; new-session) echo %7;; send-keys) echo no pane >&2; exit 1;; esac\n"), 0o755)
	handle, why := atH.CreateTerminal("hl launch alice", "", "", "")
	if handle != "" || !strings.Contains(why, "could not type the command") {
		t.Fatalf("%q %q", handle, why)
	}
	if got := calls(t, log); !strings.HasSuffix(got[len(got)-1], "kill-window -t %7") {
		t.Fatalf("%q", got)
	}
	if ok, _ := (term.Tmux{}).CloseTerminal("not a pane", true); ok {
		t.Fatal("only a pane id is closed")
	}
}

// An unstamped pane is not the daemon's to close later, so a window whose stamp failed is closed now and nothing is typed in it.
func TestAWindowWhoseAgentStampFailedIsClosedAgain(t *testing.T) {
	log := fakeTmux(t)
	dir := filepath.Dir(log)
	os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+log+"\ncase \"$3\" in has-session) exit 1;; new-session) echo %7;; set-option) echo no pane >&2; exit 1;; esac\n"), 0o755)
	handle, why := atH.CreateTerminal("hl launch alice", "", "", "alice")
	if handle != "" || !strings.Contains(why, "could not stamp the new window with its agent") {
		t.Fatalf("%q %q", handle, why)
	}
	got := calls(t, log)
	if !strings.HasSuffix(got[len(got)-1], "kill-window -t %7") || strings.Contains(strings.Join(got, "\n"), "send-keys") {
		t.Fatalf("the window is closed and nothing typed in it: %q", got)
	}
}

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
