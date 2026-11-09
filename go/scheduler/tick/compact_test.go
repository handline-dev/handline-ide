package tick

import (
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

func startAt(t *testing.T, tk *Tick, agent string, at int64, resumed bool) {
	t.Helper()
	r := 0
	if resumed {
		r = 1
	}
	if _, err := tk.Q.Exec("INSERT INTO session_starts(agent, ts, session_id, cause, resumed, text, prompt) VALUES(?, ?, 's', 'wake', ?, '', '')", agent, at, r); err != nil {
		t.Fatal(err)
	}
}

func askCompact(t *testing.T, tk *Tick, agent string, at int64) {
	t.Helper()
	if _, err := tk.Q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(?, ?, 'compact_asked', json_object('by', 'founder'))", at, agent); err != nil {
		t.Fatal(err)
	}
}

func TestACompactAskIsHandedOffAsAFullContextUntilAHandoffOrANewSessionFollows(t *testing.T) {
	tk, w := newWorld(t)
	w.performOK = true
	startAt(t, tk, "a", 8_000, false)
	askCompact(t, tk, "a", 9_000)
	// Claude Code's own compaction and a resume start no fresh session of the daemon's.
	if _, err := tk.Q.Exec("INSERT INTO events(ts, agent, kind) VALUES(9100, 'a', 'SessionStart')"); err != nil {
		t.Fatal(err)
	}
	startAt(t, tk, "a", 9_200, true)
	if actions := supervise(tk, "a"); !w.did("handoff a context") || !has(actions, "a: handoff (context)") {
		t.Fatalf("%v %v", w.calls, actions)
	}
	if got := compactAsked(tk.Q, "a"); got != "founder" {
		t.Fatalf("the ask stands until a handoff is recorded: %q", got)
	}
}

func TestACompactAskIsSpentByAHandoffOrASessionAfterIt(t *testing.T) {
	for name, mutate := range map[string]func(tk *Tick){
		"a fresh session began after it": func(tk *Tick) { startAt(t, tk, "a", 9_000, false) },
		"a note already awaited goes on as its own kind": func(tk *Tick) {
			if err := db.MarkNoteWait(tk.Q, "a", states.EarlyUsageTrigger, 9_900); err != nil {
				t.Fatal(err)
			}
		},
		"a handoff followed it": func(tk *Tick) {
			if err := startedHandoff(tk, "a", 9_001); err != nil {
				t.Fatal(err)
			}
		},
	} {
		tk, w := newWorld(t)
		w.performOK = true
		askCompact(t, tk, "a", 9_000)
		mutate(tk)
		supervise(tk, "a")
		if w.did("handoff a context") {
			t.Errorf("%s: handed off", name)
		}
	}
	tk, _ := newWorld(t)
	if err := startedHandoff(tk, "a", 9_000); err != nil {
		t.Fatal(err)
	}
	askCompact(t, tk, "a", 9_000)
	if compactAsked(tk.Q, "a") == "" {
		t.Error("a handoff in the same second as the ask came before it")
	}
	if compactAsked(tk.Q, "b") != "" {
		t.Error("another agent's ask")
	}
}

func TestAnIdleCloseWaitsForACompactAsk(t *testing.T) {
	tk, w := newWorld(t)
	w.shouldClose, w.closeOK = true, true
	askCompact(t, tk, "a", 9_000)
	supervise(tk, "a")
	if w.did("close a") {
		t.Fatalf("closed before the compact: %v", w.calls)
	}
}

func TestSavingEndsWhenTheWallItSavedForIsGoneButNotWhileANoteIsAwaited(t *testing.T) {
	saving := db.AccountSwitch{Agent: "a", Cause: "usage", From: "alpha", Phase: db.SwitchSaving, Since: 9_000}
	tk, _ := newWorld(t)
	if err := db.SetSwitch(tk.Q, saving); err != nil {
		t.Fatal(err)
	}
	supervise(tk, "a")
	if s, _ := db.GetSwitch(tk.Q, "a"); s != nil {
		t.Fatalf("nothing fires now: %+v", s)
	}
	tk, w := newWorld(t)
	w.hold = true
	db.SetSwitch(tk.Q, saving)
	if err := db.MarkNoteWait(tk.Q, "a", states.EarlyUsageTrigger, tk.Now()); err != nil {
		t.Fatal(err)
	}
	supervise(tk, "a")
	if s, _ := db.GetSwitch(tk.Q, "a"); s == nil {
		t.Fatal("a note is awaited: the handoff is still on")
	}
}
