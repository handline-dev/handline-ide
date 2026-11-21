package term_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/term"
)

// fakeRemote is one runner, "box", whose calls are written down.
type fakeRemote struct {
	calls  []string
	screen []string
}

func (f *fakeRemote) log(s string) { f.calls = append(f.calls, s) }

func (f *fakeRemote) Nodes() []string { return []string{"box"} }
func (f *fakeRemote) Open(node, agent, cwd, command string) (string, error) {
	f.log("open " + node + " " + agent + " " + cwd + " " + command)
	return "%7", nil
}
func (f *fakeRemote) Keys(node, handle, keys string, enter bool) error {
	f.log("keys " + node + " " + handle + " " + keys)
	return nil
}
func (f *fakeRemote) Screen(node, handle string, rows int) ([]string, error) {
	f.log("screen " + node + " " + handle)
	return f.screen, nil
}
func (f *fakeRemote) Close(node, handle string) error {
	f.log("close " + node + " " + handle)
	return nil
}
func (f *fakeRemote) Respawn(node, handle string) error {
	f.log("respawn " + node + " " + handle)
	return nil
}
func (f *fakeRemote) Terminals(node string) ([]string, error) {
	return []string{"%7"}, nil
}

// localHost is this machine's host: it lists %1 and writes down what reaches it.
type localHost struct{ fakeHost }

func (h *localHost) ListTerminals() map[string]bool { return map[string]bool{"%1": true} }
func (h *localHost) CreateTerminal(command, cwd, title, agent string) (string, string) {
	h.sent = append(h.sent, "create "+agent)
	return "%1", ""
}
func (h *localHost) CloseTerminal(handle string, _ bool) (bool, string) {
	h.sent = append(h.sent, "close "+handle)
	return true, ""
}
func (h *localHost) RespawnTerminal(handle string) (bool, string) {
	h.sent = append(h.sent, "respawn "+handle)
	return true, ""
}
func (h *localHost) PanePid(string) int { return 99 }

// A runner's handle reaches that runner and never this machine's host; this machine's reaches its host and never a runner. A terminal
// opens where Where says, and the two hosts' terminals are listed together.
func TestMachinesRoutesEachCallByItsHandle(t *testing.T) {
	local := &localHost{fakeHost{screenAt: func(int) []string { return []string{"local"} }}}
	remote := &fakeRemote{screen: []string{"on box"}}
	m := &term.Machines{Local: local, Remote: remote, Where: func(agent string) string {
		if agent == "bob" {
			return "box"
		}
		return ""
	}}
	tm := term.New(m, claudecode.ScreenFor)

	if h, why := tm.CreateTerminal("claude", "/w", "bob", "bob"); h != "box:%7" || why != "" {
		t.Fatalf("bob opened %q %q", h, why)
	}
	if h, _ := tm.CreateTerminal("claude", "/w", "ann", "ann"); h != "%1" {
		t.Fatalf("ann opened %q", h)
	}
	if got := tm.LiveTerminals(); !reflect.DeepEqual(got, map[string]bool{"%1": true, "box:%7": true}) {
		t.Fatalf("listed %v", got)
	}
	if ok, _ := tm.Send("box:%7", "ls", true); !ok {
		t.Fatal("send to box")
	}
	if s := tm.Backend.ReadScreen("box:%7", 40); s == nil || s.Tail[0] != "on box" {
		t.Fatalf("box's screen: %v", s)
	}
	if ok, _ := m.RespawnTerminal("box:%7"); !ok {
		t.Fatal("respawn on box")
	}
	if ok, _ := tm.CloseTerminal("box:%7"); !ok {
		t.Fatal("close on box")
	}
	if pid := tm.PanePid("box:%7"); pid != 0 {
		t.Fatalf("a runner's pane pid is not this machine's to say: %d", pid)
	}
	tm.Send("%1", "pwd", true)
	tm.CloseTerminal("%1")
	if s := tm.Backend.ReadScreen("%1", 40); s.Tail[0] != "local" || tm.PanePid("%1") != 99 {
		t.Fatal("a local handle did not reach the local host")
	}

	want := []string{"open box bob /w claude", "keys box %7 ls", "screen box %7", "respawn box %7", "close box %7"}
	if !reflect.DeepEqual(remote.calls, want) {
		t.Fatalf("box got %q, want %q", remote.calls, want)
	}
	if got := strings.Join(local.sent, ","); got != "create ann,pwd/true,close %1" {
		t.Fatalf("the local host got %q", got)
	}
}

// The vendor's readers run on the master over a runner's screen: claude's folder-trust screen on box is read as one.
func TestTheDriversReadARunnersScreen(t *testing.T) {
	trust := []string{"Quick safety check: Is this a project you created or one you trust?", "", " Accessing workspace:", " /w/bob", "",
		"❯ 1. Yes, I trust this folder", "  2. No, exit"}
	remote := &fakeRemote{screen: trust}
	tm := term.New(&term.Machines{Local: &localHost{}, Remote: remote}, claudecode.ScreenFor)
	if !tm.TrustPromptShowing("box:%7") || tm.TrustPromptFolder("box:%7") != "/w/bob" {
		t.Fatalf("the trust screen on box was not read: %q", remote.calls)
	}
	remote.screen = []string{"$ "}
	if !tm.WaitIdle("box:%7", 3000) {
		t.Fatal("a still shell on box is idle")
	}
}

// A runner that cannot be reached shows no screen and leaves the list of terminals unknown: "could not ask" is never "none there".
func TestARunnerNotReachedCannotSay(t *testing.T) {
	m := &term.Machines{Local: &localHost{}, Remote: downRemote{&fakeRemote{}}}
	if s := m.ReadScreen("box:%7", 40); s != nil {
		t.Fatalf("read %v", s)
	}
	if ok, why := m.Send("box:%7", "x", false); ok || !strings.Contains(why, "not linked") {
		t.Fatalf("send %v %q", ok, why)
	}
	if got := m.ListTerminals(); got != nil {
		t.Fatalf("listed %v: a runner that does not answer leaves the list unknown, not without its terminals", got)
	}
}

type downRemote struct{ *fakeRemote }

var errDown = errors.New("node box is not linked to this master")

func (downRemote) Screen(string, string, int) ([]string, error) { return nil, errDown }
func (downRemote) Keys(string, string, string, bool) error      { return errDown }
func (downRemote) Terminals(string) ([]string, error)           { return nil, errDown }
