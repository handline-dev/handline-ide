package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	turnspkg "github.com/handline/handline/go/agentctl/agent/claudecode/turns"
)

// The typing calls' Turns is the turns tracker, so an Esc the screen shows has ended a turn closes it there and in the agent's row.
func TestAnEscTheScreenShowsEndedClosesTheTurnInTheTracker(t *testing.T) {
	dir, err := os.MkdirTemp("", "hl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	tracker := &turnspkg.Tracker{Log: t.Logf}
	d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), SocketFile: filepath.Join(dir, "d.sock"),
		Auth: fakeAuth{key: "k"}, Turns: tracker, Log: t.Logf})
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("this environment will not bind a socket: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, nil, 0o644)
	tracker.Hook("bob", "UserPromptSubmit", map[string]any{"transcript_path": path, "session_id": "s1"}, time.Now().Unix())
	if tracker.State("bob") != turnspkg.Working {
		t.Fatalf("no turn opened: %v", tracker.State("bob"))
	}
	d.db.Exec("INSERT INTO agent_state(agent, state, updated) VALUES('bob', 'RUNNING', 1)")
	turns := d.typing().Turns
	if turns == nil {
		t.Fatal("the typing calls tell nobody that a turn ended")
	}
	turns.InterruptTurn("bob", turns.TurnMark("bob"))
	if got := tracker.State("bob"); got != turnspkg.Idle {
		t.Fatalf("the turn is %v after the Esc ended it", got)
	}
	var state string
	d.db.QueryRow("SELECT state FROM agent_state WHERE agent = 'bob'").Scan(&state)
	if state != "IDLE" {
		t.Fatalf("the agent row is %q", state)
	}
}
