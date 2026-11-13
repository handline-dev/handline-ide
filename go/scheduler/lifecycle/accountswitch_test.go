package lifecycle

import (
	"slices"
	"testing"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/scheduler/staymove"
)

// watchSwitch records each phase the view is told of, as phase:to.
func watchSwitch(g *termMoveRig) *[]string {
	seen := &[]string{}
	g.m.RefreshView = func(agent string) {
		s, _ := db.GetSwitch(g.m.Q, agent)
		*seen = append(*seen, s.Phase+":"+s.To)
	}
	return seen
}

// switchOf is the row as written.
func switchOf(t *testing.T, g *termMoveRig) db.AccountSwitch {
	t.Helper()
	s, err := db.GetSwitch(g.m.Q, "a")
	if err != nil || s == nil {
		t.Fatalf("no switch: %v", err)
	}
	return *s
}

func TestAMoveShowsEachStepAsItBegins(t *testing.T) {
	for _, kind := range []string{"usage", "usage_7d", states.EarlyUsageTrigger, "forced", LoginTrigger} {
		g := newTermMoveRig(t)
		seen := watchSwitch(g)
		if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: kind, Reason: "wall"}, ""); !ok || err != nil {
			t.Fatalf("%s: %v %v", kind, ok, err)
		}
		if want := []string{"saving:", "switching:beta", "starting:beta"}; !slices.Equal(*seen, want) {
			t.Fatalf("%s: %v, want %v", kind, *seen, want)
		}
		s := switchOf(t, g)
		if s.Cause != kind || s.From != "acct" || s.To != "beta" {
			t.Fatalf("%s: %+v", kind, s)
		}
	}
}

func TestATriggerThatMovesNothingWritesNoSwitch(t *testing.T) {
	g := newTermMoveRig(t)
	seen := watchSwitch(g)
	if _, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, ""); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 0 {
		t.Fatalf("%v", *seen)
	}
}

func TestSavingShowsWhileTheHandoffWaitsAndKeepsItsSince(t *testing.T) {
	cases := []struct {
		name  string
		setup func(g *termMoveRig)
	}{
		{"mid-turn", func(g *termMoveRig) { g.run.status = "busy" }},
		{"note hold", func(g *termMoveRig) { g.v.E.NoteHold = func(db.Q, string, int64) bool { return true } }},
		{"too soon after the last", func(g *termMoveRig) {
			db.StartHandoff(g.m.Q, "h0", "a", "s", "usage", 0)
			db.AdvanceHandoff(g.m.Q, "h0", "released", "done", "")
		}},
	}
	for _, c := range cases {
		g := newTermMoveRig(t)
		seen := watchSwitch(g)
		c.setup(g)
		for range 2 {
			if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, ""); !ok || err != nil {
				t.Fatalf("%s: %v %v", c.name, ok, err)
			}
		}
		if !slices.Equal(*seen, []string{"saving:"}) {
			t.Fatalf("%s: %v", c.name, *seen)
		}
	}
}

func TestOnlyAWallShowsSavingWhenItFiresTooSoon(t *testing.T) {
	g := newTermMoveRig(t)
	seen := watchSwitch(g)
	db.StartHandoff(g.m.Q, "h0", "a", "s", "usage", 0)
	db.AdvanceHandoff(g.m.Q, "h0", "released", "done", "")
	if _, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: states.EarlyUsageTrigger, Reason: "early"}, ""); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 0 {
		t.Fatalf("an early move put off shows nothing, or it flickers each tick: %v", *seen)
	}
}

func TestAStaleSavingStartsAgainWithItsOwnFacts(t *testing.T) {
	g := newTermMoveRig(t)
	db.SetSwitch(g.m.Q, db.AccountSwitch{Agent: "a", Cause: "usage", From: "old", Phase: db.SwitchSaving, Since: 1})
	db.SetClosed(g.m.Q, "a", "")
	g.v.saving("a", "usage_7d")
	if s := switchOf(t, g); s.From != "acct" || s.Since == 1 || s.Cause != "usage_7d" {
		t.Fatalf("%+v", s)
	}
}

func TestWithNoMoveAWallWaitsAndAnythingElseEndsTheSwitch(t *testing.T) {
	g := newTermMoveRig(t)
	g.l.dest, g.l.why = "", "no account can take one more agent"
	if _, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, ""); err != nil {
		t.Fatal(err)
	}
	row := g.row(t)
	s := switchOf(t, g)
	if s.Phase != db.SwitchWaiting || s.To != "" || s.From != "acct" || row.State != states.Parked {
		t.Fatalf("%+v %+v", s, row)
	}
	// A park with its wake already due waits for nothing.
	g.v.parkedSwitch("a", "usage", "acct", verdict.ParkWall, g.m.now())
	if s, _ := db.GetSwitch(g.m.Q, "a"); s != nil {
		t.Fatalf("%+v", s)
	}
	// A short retry stays at the step it stopped at; a park that is no wall ends it.
	db.SetSwitch(g.m.Q, db.AccountSwitch{Agent: "a", Cause: "usage", From: "acct", To: "beta", Phase: db.SwitchSwitching, Since: 1})
	g.v.parkedSwitch("a", "usage", "acct", verdict.ParkMoving, g.m.now()+300)
	if s := switchOf(t, g); s.Phase != db.SwitchSwitching {
		t.Fatalf("%+v", s)
	}
	g.v.parkedSwitch("a", states.ByHandMark, "acct", verdict.ParkByHand, g.m.now()+300)
	if s, _ := db.GetSwitch(g.m.Q, "a"); s != nil {
		t.Fatalf("%+v", s)
	}
}

func TestAResumedHandoffAtAWallWaitsToo(t *testing.T) {
	g := newTermMoveRig(t)
	db.StartHandoff(g.m.Q, "h1", "a", "s", "usage", 0)
	h, _ := db.InFlightFor(g.m.Q, "a")
	if err := g.v.ResumeHandoff(*h); err != nil {
		t.Fatal(err)
	}
	if s := switchOf(t, g); s.Phase != db.SwitchWaiting {
		t.Fatalf("%+v", s)
	}
}

func TestAForcedMoveThatDidNotHappenEndsTheSwitch(t *testing.T) {
	g := newTermMoveRig(t)
	g.l.roomOn = "is not a place for it"
	if _, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "forced", Reason: "by hand"}, "beta"); err != nil {
		t.Fatal(err)
	}
	if s, _ := db.GetSwitch(g.m.Q, "a"); s != nil {
		t.Fatalf("%+v", s)
	}
}

// accountSpawn starts nothing on the accounts it names, the way a launch that fails does, and records each account tried.
type accountSpawn struct {
	*fakeSpawn
	l      *moveLaunch
	bad    map[string]bool // the launch is refused
	slow   map[string]bool // no session took it in time, and none was refused
	tried  []string
	onSlow func()
}

func (s *accountSpawn) StartIn(agent, wt, cmd string) (bool, string, error) {
	s.tried = append(s.tried, s.l.lastAccount)
	if s.bad[s.l.lastAccount] {
		db.AddEvent(s.l.q, agent, LaunchRefusedKind, db.KV{K: "why", V: "signed out"})
		return false, s.l.lastAccount + " is signed out", nil
	}
	if s.slow[s.l.lastAccount] {
		if s.onSlow != nil {
			s.onSlow()
		}
		return false, "the launch command was run but no session took its prompt", nil
	}
	return s.fakeSpawn.StartIn(agent, wt, cmd)
}

func refusing(g *termMoveRig, next func(exclude map[string]string) string, bad ...string) *accountSpawn {
	s := &accountSpawn{fakeSpawn: g.sp, l: g.l, bad: map[string]bool{}, slow: map[string]bool{}}
	g.l.q = g.m.Q
	for _, a := range bad {
		s.bad[a] = true
	}
	g.l.next = next
	g.m.Spawn = s
	return s
}

func TestAnAccountASessionCannotStartOnIsLeftOutAndTheNextTriedAtOnce(t *testing.T) {
	g := newTermMoveRig(t)
	seen := watchSwitch(g)
	var left map[string]string
	sp := refusing(g, func(ex map[string]string) string {
		left = ex
		return "gamma"
	}, "beta")
	if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, ""); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	// At a wall each account is tried with the conversation, then fresh (wakeAgent's fallback).
	if !slices.Equal(sp.tried, []string{"beta", "beta", "gamma"}) || left["beta"] == "" {
		t.Fatalf("tried %v left %v", sp.tried, left)
	}
	if _, ok := left["acct"]; !ok {
		t.Fatalf("the account at the wall came back in: %v", left)
	}
	if !slices.Contains(*seen, "starting:gamma") || g.row(t).Account.String != "gamma" {
		t.Fatalf("%v %+v", *seen, g.row(t))
	}
	if len(g.pane.handles) != 1 {
		t.Fatalf("the session was closed more than once: %v", g.pane.handles)
	}
}

func TestAStartThatMayYetComeUpIsNotFollowedByALaunchOnAnotherAccount(t *testing.T) {
	g := newTermMoveRig(t)
	sp := refusing(g, func(map[string]string) string { return "gamma" })
	sp.slow["beta"] = true
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, "")
	if !slices.Equal(sp.tried, []string{"beta", "beta"}) {
		t.Fatalf("%v", sp.tried)
	}
}

func TestTheLoopStopsAtAStall(t *testing.T) {
	g := newTermMoveRig(t)
	db.BumpCounter(g.m.Q, "a", "wake_failures", states.MaxFailedAttempts-1)
	sp := refusing(g, func(map[string]string) string { return "gamma" }, "beta", "gamma")
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, "")
	if !slices.Equal(sp.tried, []string{"beta", "beta"}) {
		t.Fatalf("launched past the stall: %v", sp.tried)
	}
	if row := g.row(t); row.State != states.Stalled {
		t.Fatalf("the stall was parked over: %+v", row)
	}
	if h, _ := db.InFlightFor(g.m.Q, "a"); h != nil {
		t.Fatalf("the handoff was left in flight: %+v", h)
	}
}

func TestWhenEveryAccountFailsItRetriesSoon(t *testing.T) {
	g := newTermMoveRig(t)
	sp := refusing(g, func(ex map[string]string) string {
		if _, ok := ex["gamma"]; ok {
			return ""
		}
		return "gamma"
	}, "beta", "gamma")
	if _, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, ""); err != nil {
		t.Fatal(err)
	}
	row := g.row(t)
	if !slices.Equal(sp.tried, []string{"beta", "beta", "gamma", "gamma"}) {
		t.Fatalf("%v", sp.tried)
	}
	if states.ParkReason(row) != verdict.ParkMoving || row.State != states.Parked {
		t.Fatalf("%+v", row)
	}
}

func TestAPersonsChosenAccountHasNoNext(t *testing.T) {
	g := newTermMoveRig(t)
	sp := refusing(g, func(map[string]string) string { return "gamma" }, "beta")
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "forced", Reason: "by hand"}, "beta")
	if !slices.Equal(sp.tried, []string{"beta"}) {
		t.Fatalf("%v", sp.tried)
	}
}

// waitingRig has the agent parked at its wall with its session closed.
func waitingRig(t *testing.T) *termMoveRig {
	t.Helper()
	g := newTermMoveRig(t)
	g.l.dest, g.l.why = "", "no account can take one more agent"
	if _, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, ""); err != nil {
		t.Fatal(err)
	}
	if s := switchOf(t, g); s.Phase != db.SwitchWaiting {
		t.Fatalf("%+v", s)
	}
	return g
}

func TestAWaitWithNoRoomOffersTheClosestAccount(t *testing.T) {
	g := waitingRig(t)
	g.l.closest = "beta"
	to, since, err := g.v.SwitchOffer("a")
	if err != nil || to != "beta" || since == 0 {
		t.Fatalf("%q %d %v", to, since, err)
	}
	if _, ok := g.l.excludedClosest["acct"]; !ok {
		t.Fatalf("the offer may name the account at the wall: %v", g.l.excludedClosest)
	}
}

func TestAWaitOffersNothingWhenItStaysByChoiceHasRoomOrIsNotWaiting(t *testing.T) {
	g := waitingRig(t)
	g.l.closest = "beta"
	g.v.E.StayOrMove = func(staymove.Situation) (bool, string) { return false, "resets soon" }
	if to, _, _ := g.v.SwitchOffer("a"); to != "" {
		t.Fatalf("it stays by choice, offered %q", to)
	}
	g = waitingRig(t)
	g.l.closest, g.l.dest = "beta", "gamma"
	if to, _, _ := g.v.SwitchOffer("a"); to != "" {
		t.Fatalf("an account has room, offered %q", to)
	}
	g = waitingRig(t)
	g.l.closest = "beta"
	db.ClearSwitch(g.m.Q, "a")
	if to, _, _ := g.v.SwitchOffer("a"); to != "" {
		t.Fatalf("not waiting, offered %q", to)
	}
}

func TestSwitchNowStartsAWaitingAgentOnTheChosenAccountPastTheFit(t *testing.T) {
	g := waitingRig(t)
	seen := watchSwitch(g)
	// What a wake would be held back by, with no account fitting: only the person's choice goes past it.
	g.l.wait = &WakeWait{Account: "acct", Window: "5h"}
	ok, said, err := g.v.SwitchNow("a", "beta", "human")
	if !ok || err != nil {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if g.l.lastAccount != "beta" {
		t.Fatalf("launched on %q", g.l.lastAccount)
	}
	if want := []string{"switching:beta", "starting:beta"}; !slices.Equal(*seen, want) {
		t.Fatalf("%v, want %v", *seen, want)
	}
	if row := g.row(t); row.Account.String != "beta" {
		t.Fatalf("%+v", row)
	}
	if at, ok, _ := db.LastEventID(g.m.Q, "a", "moved_account"); !ok || at == 0 {
		t.Fatal("no moved_account")
	}
}

func TestALimitDialogOnScreenEndsTheTurnSoTheMoveDoesNotDefer(t *testing.T) {
	g := newTermMoveRig(t)
	g.run.status = "waiting"
	g.v.E.LimitDialogOpen = func(string) bool { return true }
	if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, ""); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if g.row(t).Account.String != "beta" {
		t.Fatalf("it deferred behind the dialog: %+v", g.row(t))
	}
	g = newTermMoveRig(t)
	g.run.status = "busy"
	g.v.E.LimitDialogOpen = func(string) bool { return true }
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "forced", Reason: "by hand"}, "beta") // a person's move waits for the turn, which the dialog ended
	if g.row(t).Account.String != "beta" {
		t.Fatalf("a person's move deferred behind the dialog: %+v", g.row(t))
	}
	g = newTermMoveRig(t)
	g.run.status = "busy"
	g.v.E.LimitDialogOpen = func(string) bool { return true }
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, "")
	if len(g.pane.handles) != 0 {
		t.Fatal("the dialog excuses only a wall and a person's move")
	}
}

func TestSwitchNowWithNoAccountTakesTheClosest(t *testing.T) {
	g := waitingRig(t)
	g.l.closest = "gamma"
	if ok, said, err := g.v.SwitchNow("a", "", "human"); !ok || err != nil {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if g.l.lastAccount != "gamma" {
		t.Fatalf("%q", g.l.lastAccount)
	}
	g = waitingRig(t)
	if ok, said, _ := g.v.SwitchNow("a", "", "human"); ok || said == "" {
		t.Fatalf("nothing to take: %v %q", ok, said)
	}
}

func TestSwitchNowRefusesItsOwnAccountAnEndingSessionAndATurn(t *testing.T) {
	g := waitingRig(t)
	if ok, _, _ := g.v.SwitchNow("a", "acct", "human"); ok {
		t.Fatal("moved onto the account it is on")
	}
	// Parked, its session gone and the row not closed yet: the sweep has not said so.
	g = waitingRig(t)
	db.ClearClosed(g.m.Q, "a")
	before := len(g.sp.startedIn)
	if ok, _, _ := g.v.SwitchNow("a", "beta", "human"); ok || len(g.sp.startedIn) != before {
		t.Fatalf("%v %v", ok, g.sp.startedIn)
	}
	if s := switchOf(t, g); s.Phase != db.SwitchWaiting {
		t.Fatalf("%+v", s)
	}
}

// liveWaitingRig has the agent waiting with a session up: a park whose close failed.
func liveWaitingRig(t *testing.T) *termMoveRig {
	t.Helper()
	g := waitingRig(t)
	db.ClearClosed(g.m.Q, "a")
	db.SetTerminal(g.m.Q, "a", "t1", "/wt")
	g.run.status = "idle"
	return g
}

func TestSwitchNowOnALiveSessionClosesItOnlyAtTheDialog(t *testing.T) {
	live := func() *termMoveRig {
		g := liveWaitingRig(t)
		g.run.status = "busy"
		return g
	}
	g := live()
	closes := len(g.pane.handles)
	if ok, said, _ := g.v.SwitchNow("a", "beta", "human"); ok || len(g.pane.handles) != closes {
		t.Fatalf("a running turn was cut off: %v %q", ok, said)
	}
	if s := switchOf(t, g); s.Phase != db.SwitchWaiting {
		t.Fatalf("%+v", s)
	}
	g = live()
	g.v.E.LimitDialogOpen = func(string) bool { return true }
	if ok, said, err := g.v.SwitchNow("a", "beta", "human"); !ok || err != nil || len(g.pane.handles) != closes+1 {
		t.Fatalf("%v %q %v %v", ok, said, err, g.pane.handles)
	}
}

func TestSwitchNowThatCannotCloseLeavesItWaiting(t *testing.T) {
	g := liveWaitingRig(t)
	g.pane.refuse = true
	before := switchOf(t, g)
	closes := len(g.pane.handles)
	if ok, said, _ := g.v.SwitchNow("a", "beta", "human"); ok || len(g.pane.handles) != closes+1 {
		t.Fatalf("%v %q %v", ok, said, g.pane.handles)
	}
	if s := switchOf(t, g); s != before || g.row(t).State != states.Parked {
		t.Fatalf("%+v %+v", s, g.row(t))
	}
}

func TestATickMoveThatCannotCloseKeepsTheWait(t *testing.T) {
	g := liveWaitingRig(t)
	g.pane.refuse = true
	g.l.dest = "beta"
	before := switchOf(t, g)
	closes := len(g.pane.handles)
	if moved, err := g.v.MoveToRoom("a", HandoffTrigger{Kind: "usage", Reason: "wall"}); moved || err != nil || len(g.pane.handles) != closes+1 {
		t.Fatalf("%v %v %v", moved, err, g.pane.handles)
	}
	if s := switchOf(t, g); s != before {
		t.Fatalf("%+v", s)
	}
}

func TestSavingDoesNotPutAMoveUnderWayBack(t *testing.T) {
	g := newTermMoveRig(t)
	db.SetSwitch(g.m.Q, db.AccountSwitch{Agent: "a", Cause: "usage", From: "acct", To: "beta", Phase: db.SwitchStarting, Since: g.m.now()})
	g.v.saving("a", "usage")
	if s := switchOf(t, g); s.Phase != db.SwitchStarting {
		t.Fatalf("%+v", s)
	}
}

func TestSwitchNowRefusesWhatTheRoomTestDoesNotExcuse(t *testing.T) {
	g := waitingRig(t)
	g.l.pastFit = "is at a limit or has no free slot"
	ok, said, err := g.v.SwitchNow("a", "beta", "human")
	if ok || err != nil || said == "" {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if s := switchOf(t, g); s.Phase != db.SwitchWaiting {
		t.Fatalf("a refusal changed the wait: %+v", s)
	}
	g = newTermMoveRig(t)
	if ok, _, _ := g.v.SwitchNow("a", "beta", "human"); ok {
		t.Fatal("an agent that is not waiting was switched")
	}
}

func TestASwitchNowThatCannotStartLeavesItWaitingAsItWas(t *testing.T) {
	g := waitingRig(t)
	before, was := switchOf(t, g), g.row(t)
	refusing(g, nil, "beta")
	ok, said, err := g.v.SwitchNow("a", "beta", "human")
	if ok || err != nil || said == "" {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	row := g.row(t)
	if s := switchOf(t, g); s != before || row.State != states.Parked || row.WakeAt != was.WakeAt || row.ParkReason != was.ParkReason || row.Detail != was.Detail {
		t.Fatalf("%+v %+v", s, row)
	}
}

func TestASwitchNowLeavesASessionThatCameUpLate(t *testing.T) {
	g := waitingRig(t)
	sp := refusing(g, nil)
	sp.slow["beta"] = true
	sp.onSlow = func() { db.ClearClosed(g.m.Q, "a"); db.SetAgentState(g.m.Q, "a", states.Running, 0, "") }
	g.v.SwitchNow("a", "beta", "human")
	if row := g.row(t); row.State != states.Running {
		t.Fatalf("%+v", row)
	}
}

func TestSavingReplacesAWaitThatHasEnded(t *testing.T) {
	g := waitingRig(t)
	g.v.saving("a", "usage")
	if s := switchOf(t, g); s.Phase != db.SwitchSaving {
		t.Fatalf("a wall hit again starts its own switch: %+v", s)
	}
}

func TestASwitchNowThatStallsTheAgentLeavesTheStall(t *testing.T) {
	g := waitingRig(t)
	refusing(g, nil, "beta")
	db.BumpCounter(g.m.Q, "a", "wake_failures", states.MaxFailedAttempts-1)
	if ok, _, _ := g.v.SwitchNow("a", "beta", "human"); ok {
		t.Fatal("it started")
	}
	if row := g.row(t); row.State != states.Stalled {
		t.Fatalf("the stall was overwritten: %+v", row)
	}
}
