package tick

import (
	"errors"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

type modePorts struct {
	chat, known, want *bool
	err               *error
}

func (m modePorts) RunsChat(string) (bool, bool)     { return *m.chat, *m.known }
func (m modePorts) WantsChat(string) (bool, error)   { return *m.want, *m.err }
func (modePorts) CurrentAccount(db.Q, string) string { return "acct" }

// A session that runs the other way than it should is moved between turns, resuming its conversation; not during a turn, not while
// background work runs, not on a read that failed, and not again for a mode a launch could not give it.
func TestASessionIsMovedBetweenChatAndTerminalBetweenTurns(t *testing.T) {
	tk, w := newWorld(t)
	setState(t, tk, "a", states.Idle, 0)
	w.idle, w.restartOK = true, true
	w.status = Status{SessionID: "s1"}
	chat, known, want := false, true, true
	var readErr error
	m := &SessionMode{Ports: modePorts{&chat, &known, &want, &readErr}}
	sweep := func() bool {
		t.Helper()
		w.calls = nil
		if err := m.Sweep(tk, 0); err != nil {
			t.Fatal(err)
		}
		return w.did("restart a by daemon for  (session_mode)")
	}
	w.bgWork = true
	if sweep() {
		t.Fatal("moved while background work runs")
	}
	w.bgWork, w.idle = false, false
	if sweep() {
		t.Fatal("moved mid-turn")
	}
	w.idle, readErr = true, errors.New("busy")
	if sweep() {
		t.Fatal("moved on a read that failed")
	}
	readErr = nil
	if !sweep() {
		t.Fatalf("not moved: %v", w.calls)
	}
	if sweep() {
		t.Fatal("moved again for a mode the launch did not give")
	}
	chat = true
	if sweep() {
		t.Fatal("moved a session that runs as it should")
	}
	want = false
	if !sweep() {
		t.Fatal("not moved back once the mode changed again")
	}
	known = false
	if sweep() {
		t.Fatal("moved a session whose mode cannot be told")
	}
}
