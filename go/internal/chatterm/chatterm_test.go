package chatterm

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/holder"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/kit/paths"
)

// stamped is a host whose every terminal is stamped for an agent with no holder.
type stamped struct{}

func (stamped) Name() string                                   { return "stamped" }
func (stamped) Available() bool                                { return true }
func (stamped) ListTerminals() map[string]bool                 { return nil }
func (stamped) ReadScreen(string, int) *term.Screen            { return nil }
func (stamped) Send(string, string, bool) (bool, string)       { return false, "" }
func (stamped) WaitIdle(string, int, func([]string) bool) bool { return true }
func (stamped) PaneAgent(string) (string, bool)                { return "a9", true }

// The limit the stream named holds until its window resets.
func TestALimitHoldsUntilItsWindowResets(t *testing.T) {
	for _, c := range []struct {
		l    agent.Limit
		want bool
	}{
		{agent.Limit{Blocked: true, ResetsAt: 200}, true},
		{agent.Limit{Blocked: true, ResetsAt: 100}, false},
		{agent.Limit{Blocked: true}, true},
		{agent.Limit{ResetsAt: 200}, false},
	} {
		if got := atLimit(c.l, 100); got != c.want {
			t.Errorf("%+v: %v", c.l, got)
		}
	}
}

// A runner's terminal is headless while the agent the board names for it runs under a holder here, which the relay serves; a terminal
// of this machine is never looked up that way.
func TestARunnersTerminalIsHeadlessWhileItsAgentsHolderIsServed(t *testing.T) {
	home, err := os.MkdirTemp("", "ct")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	sock := paths.ChatSocket(home, "a7")
	os.MkdirAll(filepath.Dir(sock), 0o700)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	os.WriteFile(holder.PidFile(sock), []byte(strconv.Itoa(os.Getpid())), 0o600)
	asked := []string{}
	c := New(home, term.New(stamped{}, nil), nil, func(h string) string { asked = append(asked, h); return "a7" })
	if !c.Headless("n1:%3") || c.Headless("%3") || len(asked) != 1 {
		t.Fatalf("headless: runner %v, here %v, asked %v", c.Headless("n1:%3"), c.Headless("%3"), asked)
	}
}
