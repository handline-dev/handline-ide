package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/paths"
)

const xcodeLine = `W=/x; xcodebuild test -project $W/a.xcodeproj > $L 2>&1`

// bgWaitRig is a terminal agent at its wall, idle with background work its task waits on, on a clock the test moves.
func bgWaitRig(t *testing.T, waitS string) (*termMoveRig, *int64) {
	g := newTermMoveRig(t)
	clock := int64(1_000_000)
	g.m.Now = func() int64 { return clock }
	if waitS != "" {
		if err := os.WriteFile(paths.ConfigFile(g.m.Home), []byte("move_background_wait_s: "+waitS+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g.run.status, g.run.bgIdle, g.d.owesNothing, g.bg, g.bgLine = "busy", true, false, "xcodebuild test", xcodeLine
	return g, &clock
}

func pastBackgroundEvents(t *testing.T, g *termMoveRig) []string {
	t.Helper()
	ids, err := db.IDs(g.m.Q, "SELECT id FROM events WHERE agent = 'a' AND kind = ?", MovedPastBackgroundKind)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, id := range ids {
		var p string
		g.m.Q.QueryRow("SELECT payload FROM events WHERE id = ?", id).Scan(&p)
		out = append(out, p)
	}
	return out
}

// Off, a wall move waits on background work as long as it runs. On, past the limit it asks for the note as an early move does, with
// the work running, then closes, which ends the work, and starts on the new account, which is told what ran. The switch names the job in
// words; the log keeps its line.
func TestAWallMoveGoesPastBackgroundWorkOnlyPastTheSettingsLimit(t *testing.T) {
	for _, waitS := range []string{"", "0", "600"} {
		g, clock := bgWaitRig(t, waitS)
		var logged []string
		g.m.Log = func(f string, a ...any) { logged = append(logged, f) }
		var asked []string
		g.v.E.NoteHold = func(_ db.Q, _, kind string, _ int64) bool { asked = append(asked, kind); return false }
		wall := HandoffTrigger{Kind: "usage", Reason: "wall"}
		g.v.PerformHandoff("a", wall, "")
		if s, _ := db.GetSwitch(g.m.Q, "a"); s == nil || s.Waits != "background: xcodebuild test" || len(g.sp.startedIn) != 0 {
			t.Fatalf("%q: the wait in words: %+v", waitS, s)
		}
		*clock += 599
		g.v.PerformHandoff("a", wall, "")
		if len(g.sp.startedIn) != 0 {
			t.Fatalf("%q: moved before the limit", waitS)
		}
		*clock += 2
		g.v.PerformHandoff("a", wall, "")
		if waitS != "600" {
			if len(g.sp.startedIn) != 0 || len(asked) != 0 || len(pastBackgroundEvents(t, g)) != 0 {
				t.Fatalf("%q: went past the work with no limit set: asked %v", waitS, asked)
			}
			continue
		}
		if len(g.sp.startedIn) != 1 || g.row(t).Account.String != "beta" {
			t.Fatalf("did not move past the limit: %v %+v", g.sp.startedIn, g.row(t))
		}
		if len(asked) != 1 || asked[0] != states.EarlyUsageTrigger {
			t.Fatalf("the note was asked as %v", asked)
		}
		ev := pastBackgroundEvents(t, g)
		if len(ev) != 1 || !strings.Contains(ev[0], `"jobs": "xcodebuild test"`) || !strings.Contains(ev[0], `"waited_s": 601`) {
			t.Fatalf("event %v", ev)
		}
		prompt, _ := os.ReadFile(filepath.Join(g.m.Home, "wake", "a.txt"))
		if !strings.Contains(string(prompt), "The move stopped the background work your last session still ran ("+xcodeLine+")") {
			t.Fatalf("the next session is not told what ran: %q", prompt)
		}
		if !strings.Contains(strings.Join(logged, "\n"), "moving past it, which ends it") {
			t.Fatalf("no log line: %v", logged)
		}
	}
}

// Past the limit the note's own turn comes between: the move waits for it, and goes once it ends, said once.
func TestAMovePastBackgroundWorkWaitsForTheNotesTurn(t *testing.T) {
	g, clock := bgWaitRig(t, "600")
	hold := true
	g.v.E.NoteHold = func(db.Q, string, string, int64) bool { return hold }
	wall := HandoffTrigger{Kind: states.EarlyUsageTrigger, Reason: "wall"}
	g.v.PerformHandoff("a", wall, "")
	*clock += 700
	g.v.PerformHandoff("a", wall, "")
	if s, _ := db.GetSwitch(g.m.Q, "a"); s == nil || s.Waits != "its handoff note" || len(g.sp.startedIn) != 0 {
		t.Fatalf("not waiting for the note: %+v", s)
	}
	g.run.bgIdle = false // the note's turn
	*clock += 30
	g.v.PerformHandoff("a", wall, "")
	if s, _ := db.GetSwitch(g.m.Q, "a"); s == nil || s.Waits != "the turn to end" {
		t.Fatalf("%+v", s)
	}
	g.run.bgIdle, hold = true, false
	*clock += 30
	g.v.PerformHandoff("a", wall, "")
	if len(g.sp.startedIn) != 1 || len(pastBackgroundEvents(t, g)) != 1 {
		t.Fatalf("started %v, events %v", g.sp.startedIn, pastBackgroundEvents(t, g))
	}
}

// Move now on a wall move waiting on background work moves at once, with no note asked, and says so; with nowhere to go it refuses why.
func TestMoveNowOnAMoveWaitingOnBackgroundWorkMovesAtOnce(t *testing.T) {
	g, _ := bgWaitRig(t, "")
	g.v.E.NoteHold = func(db.Q, string, string, int64) bool { t.Fatal("a person's move asked for a note"); return true }
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, "")
	if s, _ := db.GetSwitch(g.m.Q, "a"); s == nil || s.Phase != db.SwitchSaving {
		t.Fatalf("%+v", s)
	}
	stop := func() error { t.Fatal("an idle agent's turn was stopped"); return nil }
	ok, said, err := g.v.MoveNow("a", "", "p1", stop)
	if !ok || err != nil || !strings.Contains(said, "-> beta") || len(g.sp.startedIn) != 1 || g.row(t).Account.String != "beta" {
		t.Fatalf("%v %q %v, started %v", ok, said, err, g.sp.startedIn)
	}
	if ev := pastBackgroundEvents(t, g); len(ev) != 1 || !strings.Contains(ev[0], `"by": "p1"`) {
		t.Fatalf("event %v", ev)
	}

	g, _ = bgWaitRig(t, "")
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, "")
	g.l.dest, g.l.why = "", "no account has room"
	ok, said, err = g.v.MoveNow("a", "", "p1", stop)
	if ok || err != nil || !strings.Contains(said, "no account has room") || len(g.sp.startedIn) != 0 || len(g.pane.handles) != 0 {
		t.Fatalf("%v %q %v", ok, said, err)
	}
}

// The wait counts on the work running now: a mark left by an earlier stretch of work does not let the move go past new work at once.
func TestTheBackgroundWaitCountsFromTheWorkRunningNow(t *testing.T) {
	g, clock := bgWaitRig(t, "600")
	g.v.E.NoteHold = func(db.Q, string, string, int64) bool { return false }
	wall := HandoffTrigger{Kind: "usage", Reason: "wall"}
	g.v.PerformHandoff("a", wall, "")
	*clock += 3600
	g.d.bgSince = *clock - 60 // that work ended; this stretch began a minute ago
	g.v.PerformHandoff("a", wall, "")
	if len(g.sp.startedIn) != 0 {
		t.Fatal("went past new work on an old mark")
	}
	*clock += 541
	g.v.PerformHandoff("a", wall, "")
	if len(g.sp.startedIn) != 1 {
		t.Fatal("did not go once the new work had run the limit")
	}
}

// Move now goes past the work only while the agent is idle with it: once a turn has begun, nothing is closed under it and the move
// already under way is said.
func TestMoveNowMidTurnIsNoMovePastBackgroundWork(t *testing.T) {
	g, _ := bgWaitRig(t, "")
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, "")
	g.run.bgIdle = false // a turn began since
	stopped := false
	ok, said, err := g.v.MoveNow("a", "beta", "p1", func() error { stopped = true; return nil })
	if ok || err != nil || stopped || len(g.sp.startedIn) != 0 || len(g.pane.handles) != 0 || !strings.Contains(said, "already switching") {
		t.Fatalf("%v %q %v stopped %v started %v", ok, said, err, stopped, g.sp.startedIn)
	}
}
