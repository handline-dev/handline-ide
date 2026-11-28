package sessionrpc

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// Open terminal on a Chat agent asks a terminal for its next session: between turns it is resumed there at once, mid-turn once the turn
// ends (the answer says so); a closed agent is started; one in a terminal is no change.
func TestOpenTerminalMovesAChatSessionToItsTerminal(t *testing.T) {
	r, h, _ := typingRig(t)
	r.ctl.ok = true
	open := func(id string) string {
		t.Helper()
		r.ctl.calls = nil
		said, _ := result(t, r.dial(false).call("agent.open_terminal", map[string]any{"agent": "ann", "request_id": rid(id)}))["said"].(string)
		return said
	}
	hold := func() int64 {
		t.Helper()
		v, err := db.TerminalHold(r.e.DB, r.ann)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if said := open("t1"); !strings.Contains(said, "already") || len(r.ctl.calls) != 0 || hold() != db.TerminalHoldNone {
		t.Fatalf("a terminal session: %q %v", said, r.ctl.calls)
	}
	chat := &headlessChat{turn: true}
	r.e.Term.Chat = chat
	if said := open("t2"); !strings.Contains(said, "when its turn ends") || len(r.ctl.calls) != 0 || hold() != db.TerminalHoldAsked {
		t.Fatalf("mid-turn: %q %v", said, r.ctl.calls)
	}
	chat.turn = false
	if said := open("t3"); !strings.Contains(said, "opened") || len(r.ctl.calls) != 1 || r.ctl.calls[0] != "restart "+r.ann+" false false" || len(h.sent) != 0 {
		t.Fatalf("between turns: %q %v, sent %q", said, r.ctl.calls, h.sent)
	}
	r.e.Term.Chat = nil
	if _, err := r.e.DB.Exec("UPDATE agent_state SET closed_at = 5, terminal_hold = 0 WHERE agent = ?", r.ann); err != nil {
		t.Fatal(err)
	}
	if said := open("t4"); !strings.Contains(said, "started") || len(r.ctl.calls) != 1 || r.ctl.calls[0] != "wake "+r.ann || hold() != db.TerminalHoldAsked {
		t.Fatalf("closed: %q %v", said, r.ctl.calls)
	}
}

// agent.restart_here answers with where the agent started, or refuses (gate state) with the reason.
func TestRestartHereAnswersOnceWithWhereOrTheRefusal(t *testing.T) {
	r := newRig(t)
	ok, said := false, "MacBook Air is online and linked: it runs there"
	r.e.RestartHere = func(_ Call, agent string) (bool, string, error) { return ok, said, nil }
	c := r.dial(false)
	if g := refused(c.call("agent.restart_here", map[string]any{"agent": "ann", "request_id": rid("h1")})); g != "state" {
		t.Fatalf("gate %q", g)
	}
	ok, said = true, "started on Mac mini"
	if got := result(t, c.call("agent.restart_here", map[string]any{"agent": "ann", "request_id": rid("h2")})); got["said"] != said {
		t.Fatalf("%v", got)
	}
}
