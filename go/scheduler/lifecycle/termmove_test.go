package lifecycle

import (
	"errors"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/scheduler/settings"
)

// fakeTermRun is the session file of one terminal agent: its pid and what it reports. The session dies with the pane unless survives.
type fakeTermRun struct {
	pid      int
	status   string
	survives bool
	told     []string
	outside  bool  // the session runs in another pane than the one handed in
	started  int64 // the start time of the process now at pid; 0 reads as unknown
	bgIdle   bool  // the screen shows the prompt idle with background work running
}

func (f *fakeTermRun) Session(string) (int, string)       { return f.pid, f.status }
func (f *fakeTermRun) PidGone(int) bool                   { return f.status == "" }
func (f *fakeTermRun) PidStart(int) int64                 { return f.started }
func (f *fakeTermRun) PaneHolds(string, int) (bool, bool) { return !f.outside, true }
func (f *fakeTermRun) IdleWithBackground(string) bool     { return f.bgIdle }
func (f *fakeTermRun) Tell(a, op, text string) error {
	f.told = append(f.told, a+"|"+op+"|"+text)
	return nil
}

// paneTerm closes a pane and, unless the session survives it, ends the session in it.
type paneTerm struct {
	*fakeTerm
	run     *fakeTermRun
	refuse  bool
	handles []string
}

func (p *paneTerm) CloseTerminal(h string) (bool, string) {
	p.handles = append(p.handles, h)
	if p.refuse {
		return false, "the host kept it"
	}
	if !p.run.survives {
		p.run.status = ""
	}
	return true, ""
}

type termMoveRig struct {
	*moveRig
	run  *fakeTermRun
	pane *paneTerm
	sp   *fakeSpawn
	d    *fakeDelivery
}

func newTermMoveRig(t *testing.T) *termMoveRig {
	t.Helper()
	m, d, tm := newManager(t)
	d.waitingMsg, d.tookPrompt, tm.launchOK = "work", true, true
	run := &fakeTermRun{pid: 4242, status: "idle"}
	pane := &paneTerm{fakeTerm: tm, run: run}
	sp := &fakeSpawn{ok: true}
	l := &moveLaunch{dest: "beta"}
	m.Term, m.TermRun, m.Spawn, m.Launch = pane, run, sp, l
	db.SetTerminal(m.Q, "a", "t1", "/wt")
	db.SetResumeSession(m.Q, "a", "sess-1")
	g := newMoveRigOn(m, l)
	return &termMoveRig{moveRig: g, run: run, pane: pane, sp: sp, d: d}
}

func TestATerminalAgentAtTheWallIsClosedByItsPaneAndRelaunchedFreshOnTheOtherAccount(t *testing.T) {
	g := newTermMoveRig(t)
	if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "5h wall"}, ""); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if len(g.pane.handles) != 1 || g.pane.handles[0] != "t1" || len(g.sp.startedIn) != 1 || g.sp.startedIn[0] != "a@/wt" {
		t.Fatalf("closed %v started %v", g.pane.handles, g.sp.startedIn)
	}
	if len(g.pane.relaunched) != 0 {
		t.Fatalf("nothing is typed into the old pane: %v", g.pane.relaunched)
	}
	row := g.row(t)
	if row.Account.String != "beta" || isClosed(row) || row.ResumeSession.String != "" {
		t.Fatalf("%+v", row)
	}
	if _, ok, _ := db.LastEventID(g.m.Q, "a", "terminal_closed"); !ok {
		t.Fatal("no terminal_closed event")
	}
}

func TestATerminalAgentOnAnExpiredLoginIsClosedByItsPaneAndRelaunchedFreshOnTheOtherAccount(t *testing.T) {
	g := newTermMoveRig(t)
	if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: LoginTrigger, Reason: "login expired on alpha"}, ""); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	row := g.row(t)
	if len(g.pane.handles) != 1 || len(g.sp.startedIn) != 1 || row.Account.String != "beta" || isClosed(row) {
		t.Fatalf("closed %v started %v %+v", g.pane.handles, g.sp.startedIn, row)
	}
}

func TestAnAgentOnAnEndedAccountFinishesItsTurnAndNoteBeforeItMoves(t *testing.T) {
	g := newTermMoveRig(t)
	ended := HandoffTrigger{Kind: EndedTrigger, Reason: "subscription ended on alpha"}
	g.run.status = "busy"
	if ok, err := g.v.PerformHandoff("a", ended, "beta"); !ok || err != nil || len(g.pane.handles) != 0 {
		t.Fatalf("an ended account still answers, so a live turn is not cut off: %v %v %v", ok, err, g.pane.handles)
	}
	g.run.status = "idle"
	g.v.E.NoteHold = func(db.Q, string, int64) bool { return true }
	if ok, err := g.v.PerformHandoff("a", ended, "beta"); !ok || err != nil || len(g.pane.handles) != 0 {
		t.Fatalf("it writes its note first: %v %v %v", ok, err, g.pane.handles)
	}
	g.v.E.NoteHold = func(db.Q, string, int64) bool { return false }
	if ok, err := g.v.PerformHandoff("a", ended, "beta"); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if row := g.row(t); len(g.pane.handles) != 1 || row.Account.String != "beta" {
		t.Fatalf("closed %v %+v", g.pane.handles, row)
	}
}

func TestATerminalAgentAtAFullContextIsClosedByItsPaneAndParkedForAFreshWake(t *testing.T) {
	g := newTermMoveRig(t)
	db.MarkNoteWait(g.m.Q, "a", "context", g.m.now())
	if ok, _ := g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "190k of 200k"}, ""); !ok {
		t.Fatal("degraded")
	}
	if _, _, waits, _ := db.NoteWait(g.m.Q, "a"); waits {
		t.Fatal("the note wait outlived the handoff")
	}
	row := g.row(t)
	if len(g.pane.handles) != 1 || !isClosed(row) || row.ResumeSession.String != "" || row.Terminal.String != "" || row.State != states.Parked || row.Worktree.String != "/wt" {
		t.Fatalf("%v %+v", g.pane.handles, row)
	}
	if len(g.sp.startedIn) != 0 {
		t.Fatalf("a context handoff starts nothing itself: %v", g.sp.startedIn)
	}
	if ok, said, _ := g.m.WakeAgent("a", WakeOpts{}); !ok || len(g.sp.startedIn) != 1 || len(g.pane.relaunched) != 0 {
		t.Fatalf("%v %q %v", ok, said, g.sp.startedIn)
	}
}

func TestATerminalThatWillNotCloseIsNotRelaunchedOver(t *testing.T) {
	for name, set := range map[string]func(g *termMoveRig){
		"the host keeps the pane":       func(g *termMoveRig) { g.pane.refuse = true },
		"the session outlives its pane": func(g *termMoveRig) { g.run.survives = true },
	} {
		g := newTermMoveRig(t)
		set(g)
		g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "5h wall"}, "")
		row := g.row(t)
		if isClosed(row) || len(g.sp.startedIn) != 0 || row.State != states.Parked || states.ParkReason(row) != verdict.ParkMoving {
			t.Fatalf("%s: %+v started %v", name, row, g.sp.startedIn)
		}
		if _, ok, _ := db.LastEventID(g.m.Q, "a", "close_failed"); !ok {
			t.Fatalf("%s: the failure is not on the record", name)
		}
		if g.run.survives && row.Terminal.String != "" {
			t.Fatalf("%s: the pane is closed and the row still names it", name)
		}
	}
}

func TestATerminalSessionWhoseFileCannotBeReadIsNeverClosed(t *testing.T) {
	g := newTermMoveRig(t)
	g.run.pid, g.run.status = 0, "unknown"
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, "")
	if len(g.pane.handles) != 0 || isClosed(g.row(t)) {
		t.Fatalf("closed %v", g.pane.handles)
	}
}

func TestATerminalAgentMidTurnIsNotHandedOffAndAForcedOneIsNotInterrupted(t *testing.T) {
	g := newTermMoveRig(t)
	for _, status := range []string{"busy", "waiting", "unknown"} {
		g.run.status = status
		if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, ""); !ok || err != nil {
			t.Fatalf("%s: %v %v", status, ok, err)
		}
		if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "forced", Reason: "by hand"}, "beta"); ok || err == nil {
			t.Fatalf("forced %s: a person is told it did not happen: %v %v", status, ok, err)
		}
		if len(g.pane.handles) != 0 || isClosed(g.row(t)) {
			t.Fatalf("%s: closed %v", status, g.pane.handles)
		}
	}
}

func TestATerminalClearThatBackgroundWorkHoldsOffIsToldThroughItsSessionAndSettledWhenTheWorkEnds(t *testing.T) {
	g := newTermMoveRig(t)
	g.bg = "npm test"
	if ok, _ := g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, ""); !ok {
		t.Fatal("degraded")
	}
	if len(g.run.told) != 1 || !strings.Contains(g.run.told[0], "will be cleared in 5 minutes") || len(g.pane.handles) != 0 {
		t.Fatalf("told %v closed %v", g.run.told, g.pane.handles)
	}
	if owed, _ := g.v.SettleOwedClear("a"); !owed || len(g.pane.handles) != 0 || len(g.run.told) != 1 {
		t.Fatalf("owed %v closed %v", owed, g.pane.handles)
	}
	g.bg = ""
	if owed, _ := g.v.SettleOwedClear("a"); !owed || len(g.pane.handles) != 1 {
		t.Fatalf("owed %v closed %v", owed, g.pane.handles)
	}
	if row := g.row(t); row.State != states.Parked || !isClosed(row) {
		t.Fatalf("%+v", row)
	}
}

// With context_clear_past_background on, a context clear meets a turn that has ended with only background work running as between turns:
// told, then cut off when its wait is up. Off, mid-turn, or for a usage move, it is deferred as before.
func TestAClearPastBackgroundWorkGoesOnlyWithTheSettingAndOnlyWhenTheTurnHasEnded(t *testing.T) {
	for _, c := range []struct {
		name         string
		kind         string
		on, bgIdle   bool
		toldAndClose bool
	}{{"on, idle with background", "context", true, true, true}, {"off", "context", false, true, false},
		{"on, mid-turn", "context", true, false, false}, {"on, a usage move", "usage", true, true, false}} {
		g := newTermMoveRig(t)
		if err := g.m.Settings.SetFlag(settings.ClearPastBackgroundKey, c.on); err != nil {
			t.Fatal(err)
		}
		g.run.status, g.run.bgIdle, g.bg = "busy", c.bgIdle, "vite"
		if ok, _ := g.v.PerformHandoff("a", HandoffTrigger{Kind: c.kind, Reason: "full"}, ""); !ok {
			t.Fatalf("%s: degraded", c.name)
		}
		if told := len(g.run.told) == 1 && strings.Contains(g.run.told[0], "will be cleared in 5 minutes"); told != c.toldAndClose || len(g.pane.handles) != 0 {
			t.Fatalf("%s: told %v closed %v", c.name, g.run.told, g.pane.handles)
		}
		if !c.toldAndClose {
			if g.v.clearOwed("a") != 0 {
				t.Fatalf("%s: a deferred clear is not owed", c.name)
			}
			continue
		}
		if owed, _ := g.v.SettleOwedClear("a"); !owed || len(g.pane.handles) != 0 {
			t.Fatalf("%s: closed before its time: %v", c.name, g.pane.handles)
		}
		db.SetCursor(g.m.Q, "clear_owed:a", g.m.now()-ClearWaitS-1)
		if owed, _ := g.v.SettleOwedClear("a"); !owed || len(g.pane.handles) != 1 {
			t.Fatalf("%s: not closed when its time was up: %v", c.name, g.pane.handles)
		}
	}
}

// A clear owed by a login move that found no room is not a context clear: the setting does not cut its background work off early.
func TestAClearPastBackgroundWorkIsNotAClearOwedByAMoveWithNoRoom(t *testing.T) {
	g := newTermMoveRig(t)
	if err := g.m.Settings.SetFlag(settings.ClearPastBackgroundKey, true); err != nil {
		t.Fatal(err)
	}
	g.l.dest = "" // no account has room
	g.run.status, g.run.bgIdle, g.bg = "busy", true, "vite"
	g.v.PerformHandoff("a", HandoffTrigger{Kind: LoginTrigger, Reason: "expired"}, "")
	if g.v.clearOwed("a") == 0 {
		t.Fatal("the clear is not owed")
	}
	db.SetCursor(g.m.Q, "clear_owed:a", g.m.now()-ClearWaitS-1)
	if owed, _ := g.v.SettleOwedClear("a"); !owed || len(g.pane.handles) != 0 {
		t.Fatalf("closed mid-turn: %v", g.pane.handles)
	}
}

func TestATerminalNoticeThatFailsStillLeavesTheClearOwed(t *testing.T) {
	g := newTermMoveRig(t)
	g.bg = "npm test"
	g.m.TermRun = failingTell{g.run}
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, "")
	if g.v.clearOwed("a") == 0 {
		t.Fatal("the clear is not owed")
	}
}

type failingTell struct{ *fakeTermRun }

func (failingTell) Tell(string, string, string) error { return errors.New("no socket") }

func TestASessionThatOutlivedItsPaneIsRecordedClosedOnceItsPidIsGoneAndThenRelaunchedFresh(t *testing.T) {
	g := newTermMoveRig(t)
	g.run.survives = true
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, "")
	if row := g.row(t); isClosed(row) || row.Terminal.String != "" {
		t.Fatalf("%+v", row)
	}
	// still alive: every tick looks, nothing is closed and nothing is started over it
	for i := 0; i < 3; i++ {
		g.v.SettleOwedClear("a")
	}
	if isClosed(g.row(t)) || len(g.sp.startedIn) != 0 {
		t.Fatal("closed or relaunched over a live session")
	}
	g.run.status = "" // the orphan exits
	g.v.SettleOwedClear("a")
	row := g.row(t)
	if !isClosed(row) || row.ResumeSession.String != "" {
		t.Fatalf("%+v", row)
	}
	g.m.Q.Exec("UPDATE agent_state SET wake_at = ? WHERE agent = 'a'", g.m.now()) // the park's backoff has run out
	if ok, said, _ := g.m.WakeAgent("a", WakeOpts{}); !ok || len(g.sp.startedIn) != 1 {
		t.Fatalf("%v %q %v", ok, said, g.sp.startedIn)
	}
}

func TestAReusedPidReadsAsTheSessionHavingEnded(t *testing.T) {
	g := newTermMoveRig(t)
	g.run.survives, g.run.started = true, 100
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, "")
	g.v.SettleOwedClear("a")
	if isClosed(g.row(t)) {
		t.Fatal("closed over a live session")
	}
	g.run.started = 200 // another process has the number now; the status still reads running
	g.v.SettleOwedClear("a")
	if !isClosed(g.row(t)) {
		t.Fatalf("%+v", g.row(t))
	}
}

func TestASessionStillThereAfterTheBoundIsReportedOnceAndNeverStartedOver(t *testing.T) {
	g := newTermMoveRig(t)
	g.run.survives = true
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, "")
	db.SetCursor(g.m.Q, orphanKey("a")+":since", g.m.now()-OrphanGiveUpS-1)
	for i := 0; i < 3; i++ {
		g.v.SettleOwedClear("a")
	}
	var n int
	g.m.Q.QueryRow("SELECT COUNT(*) FROM events WHERE agent = 'a' AND kind = 'needs_you'").Scan(&n)
	if n != 1 || isClosed(g.row(t)) || len(g.sp.startedIn) != 0 {
		t.Fatalf("needs_you %d, closed %v, started %v", n, isClosed(g.row(t)), g.sp.startedIn)
	}
	g.run.status = "" // a person ended it after all
	g.v.SettleOwedClear("a")
	if !isClosed(g.row(t)) {
		t.Fatal("not recorded closed once it ended")
	}
}

func TestAnOwedClearWhoseOrphanExitsEndsLikeANormalClear(t *testing.T) {
	g := newTermMoveRig(t)
	g.bg = "npm test"
	g.run.survives = true
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, "")
	db.SetCursor(g.m.Q, "clear_owed:a", g.m.now()-ClearWaitS-1)
	g.v.SettleOwedClear("a") // times out: the pane closes, the session stays
	if row := g.row(t); isClosed(row) || row.State == states.Parked {
		t.Fatalf("%+v", row)
	}
	g.v.SettleOwedClear("a") // still there: nothing is closed twice
	g.run.status = ""
	g.v.SettleOwedClear("a")
	row := g.row(t)
	if !isClosed(row) || row.State != states.Parked || row.WakeAt.Int64 == 0 || row.WakeAt.Int64 > g.m.now() {
		t.Fatalf("%+v", row)
	}
	if h, _ := db.GetHandoff(g.m.Q, "a"); !strings.Contains(h, "Cut off by the clear") || !strings.Contains(h, "npm test") {
		t.Fatalf("record %q", h)
	}
	if g.v.clearOwed("a") != 0 {
		t.Fatal("still owed")
	}
}

func TestASecondSessionOutsideTheClosedPaneIsReportedAndDoesNotFailTheClose(t *testing.T) {
	g := newTermMoveRig(t)
	g.run.survives, g.run.outside = true, true
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, "")
	row := g.row(t)
	if !isClosed(row) || row.Terminal.String != "" {
		t.Fatalf("the pane's own end is proved by the pane: %+v", row)
	}
	if _, ok, _ := db.LastEventID(g.m.Q, "a", "needs_you"); !ok {
		t.Fatal("the stray session is not on the record")
	}
	if _, ok, _ := db.LastEventID(g.m.Q, "a", "close_failed"); ok {
		t.Fatal("a stray session was counted as a failed close")
	}
}
