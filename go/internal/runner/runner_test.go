package runner

import (
	"os"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/protocol/rpc"
)

type fakeTerm struct {
	typed, closed []string
	respawns      int
}

func (f *fakeTerm) CreateTerminal(command, cwd, title, agent string) (string, string) {
	f.typed = append(f.typed, command)
	return "%1", ""
}
func (f *fakeTerm) CloseTerminal(h string, _ bool) (bool, string) {
	f.closed = append(f.closed, h)
	return true, ""
}
func (f *fakeTerm) Send(h, text string, enter bool) (bool, string) {
	f.typed = append(f.typed, text)
	return true, ""
}
func (f *fakeTerm) ReadScreen(h string, n int) *term.Screen {
	return &term.Screen{Tail: []string{"$ " + h}}
}
func (f *fakeTerm) RespawnTerminal(string) (bool, string) { f.respawns++; return true, "" }
func (f *fakeTerm) ListTerminals() map[string]bool        { return map[string]bool{"%1": true} }
func (f *fakeTerm) PanePid(string) int                    { return 4242 + f.respawns } // a respawned pane runs a new shell

type fakeTokens struct{ kept map[string]int }

func (f *fakeTokens) Record(agent string, pid int) bool { f.kept[agent] = pid; return true }
func (f *fakeTokens) Forget(agent string, pid int) bool { delete(f.kept, agent); return true }

type fakeAuth struct{}

func (fakeAuth) AgentOf(int) string       { return "" }
func (fakeAuth) SessionsReadable() error  { return nil }
func (fakeAuth) FounderKeyOK(string) bool { return false }

// The master's calls reach the terminal host; an open records the session as the agent's and keeps it in runner.json; a stop
// forgets it. An agent calling a runner.* method on the local socket is refused: those are the master's, on the link.
func TestTheMastersCallsReachTheTerminalAndOnlyOverTheLink(t *testing.T) {
	home := t.TempDir()
	tm, tok := &fakeTerm{}, &fakeTokens{kept: map[string]int{}}
	r, err := Start(Config{Home: home, Term: tm, Tokens: tok, Auth: fakeAuth{}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, err := r.serve("runner.terminal.open", map[string]any{"agent": "bob", "folder": home, "command": "claude"})
	if err != nil || got.(map[string]any)["handle"] != "%1" || tok.kept["bob"] != 4242 {
		t.Fatalf("open: %v %v %v", got, err, tok.kept)
	}
	if b, _ := os.ReadFile(StateFile(home)); !strings.Contains(string(b), `"bob":{"handle":"%1","pid":4242,"folder":"`+home+`"}`) {
		t.Fatalf("runner.json: %s", b)
	}
	if _, err := r.serve("runner.keys", map[string]any{"handle": "%1", "keys": "ls", "enter": true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.serve("runner.screen", map[string]any{"handle": "%1"}); got.(map[string]any)["rows"].([]string)[0] != "$ %1" {
		t.Fatalf("screen: %v", got)
	}
	if got, _ := r.serve("runner.terminals", nil); len(got.(map[string]any)["handles"].([]string)) != 1 {
		t.Fatalf("terminals: %v", got)
	}
	// A respawn ends the session's shell: the new one is the agent's session in its place.
	if _, err := r.serve("runner.respawn", map[string]any{"handle": "%1"}); err != nil || tok.kept["bob"] != 4243 {
		t.Fatalf("respawn: %v %v", err, tok.kept)
	}
	if b, _ := os.ReadFile(StateFile(home)); !strings.Contains(string(b), `"pid":4243`) {
		t.Fatalf("runner.json after respawn: %s", b)
	}
	if _, err := r.serve("runner.respawn", map[string]any{"handle": "%9"}); err == nil {
		t.Fatal("a terminal the runner did not open was respawned")
	}
	if _, err := r.serve("runner.stop", map[string]any{"agent": "bob", "handle": "%1"}); err != nil || len(tok.kept) != 0 || len(tm.closed) != 1 {
		t.Fatalf("stop: %v %v %v", err, tok.kept, tm.closed)
	}
	if strings.Join(tm.typed, ",") != "claude,ls" {
		t.Fatalf("typed: %v", tm.typed)
	}
	_, err = r.Local().Dispatch(r.Local().AgentCaller("bob"), "hl", "", "runner.terminal.open", map[string]any{"agent": "bob", "folder": home})
	if e, ok := err.(*rpc.Error); !ok || e.Data["gate"] != "link" {
		t.Fatalf("an agent opened a terminal itself: %v", err)
	}
	_, err = r.Local().Dispatch(r.Local().AgentCaller("bob"), "hl", "", "say", nil)
	if e, ok := err.(*rpc.Error); !ok || e.Data["gate"] != "master" {
		t.Fatalf("a call with no link to the master: %v", err)
	}
}
