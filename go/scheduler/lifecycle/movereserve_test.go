package lifecycle

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

// A person's move on a fake clock: scenario rows F1-F3, F5, F6 and the pin refusals D1/D2 of docs/design/account-assignment-scenarios.md.

func newReserveRig(t *testing.T) (*termMoveRig, *int64) {
	t.Helper()
	g := newTermMoveRig(t)
	g.account(t, "beta", true)
	g.account(t, "gamma", true)
	clock := int64(1_800_000_000)
	g.m.Now = func() int64 { return clock }
	return g, &clock
}

func pending(t *testing.T, g *termMoveRig) *db.AccountSwitch {
	t.Helper()
	s, err := db.LiveSwitch(g.m.Q, g.row(t), g.m.now())
	if err != nil {
		t.Fatal(err)
	}
	if !s.Pending() {
		return nil
	}
	return s
}

func lastPayload(t *testing.T, g *termMoveRig, kind string) map[string]any {
	t.Helper()
	var raw string
	if err := g.m.Q.QueryRow("SELECT payload FROM events WHERE agent = 'a' AND kind = ? ORDER BY id DESC LIMIT 1", kind).Scan(&raw); err != nil {
		return nil
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// noStop is a stop for an agent with no turn to end: calling it fails the test.
func noStop(t *testing.T) func() error {
	return func() error { t.Helper(); t.Fatal("a turn was stopped that was not running"); return nil }
}

func runPending(t *testing.T, g *termMoveRig) bool {
	t.Helper()
	acted, err := g.v.RunPendingMove("a")
	if err != nil {
		t.Fatal(err)
	}
	return acted
}

// F1: a move mid-turn is kept, shown, replaced by a later one, cancelled, and run when the turn ends.
func TestF1AMoveMidTurnIsKeptAndRunsWhenTheTurnEnds(t *testing.T) {
	g, _ := newReserveRig(t)
	g.run.status = "busy"
	ok, said, err := g.v.ForceMove("a", "beta", "human")
	if !ok || err != nil || said != "a is mid-turn: it moves to beta when the turn ends (by human)" {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if s := pending(t, g); s == nil || s.Phase != db.SwitchScheduled || s.To != "beta" || s.From != "acct" {
		t.Fatalf("not kept: %+v", s)
	}
	if runPending(t, g) || len(g.pane.handles) != 0 {
		t.Fatalf("ran mid-turn: closed %v", g.pane.handles)
	}
	if ok, _, _ := g.v.ForceMove("a", "gamma", "human"); !ok || pending(t, g).To != "gamma" {
		t.Fatalf("a second call does not replace the target: %+v", pending(t, g))
	}
	if ok, said, _ := g.v.CancelMove("a"); !ok || said != "a stays on acct: the move to gamma is cancelled" || pending(t, g) != nil {
		t.Fatalf("cancel: %v %q %+v", ok, said, pending(t, g))
	}
	if ok, said, _ := g.v.CancelMove("a"); ok || said != "a has no move waiting" {
		t.Fatalf("a second cancel: %v %q", ok, said)
	}
	g.v.ForceMove("a", "beta", "human")
	g.run.status = "idle"
	if !runPending(t, g) {
		t.Fatal("not run at the turn's end")
	}
	if len(g.pane.handles) != 1 || len(g.sp.startedIn) != 1 || g.row(t).Account.String != "beta" || pending(t, g) != nil {
		t.Fatalf("closed %v started %v %+v", g.pane.handles, g.sp.startedIn, g.row(t))
	}
	if p := lastPayload(t, g, "moved_account"); p["trigger"] != "forced" || p["note_written"] != false {
		t.Fatalf("moved_account %v", p)
	}
	if p := lastPayload(t, g, "forced_move"); p["by"] != "human" {
		t.Fatalf("forced_move %v", p)
	}
}

// An idle agent moves at the call, as before; nothing is kept.
func TestAMoveOfAnIdleAgentRunsAtTheCall(t *testing.T) {
	g, _ := newReserveRig(t)
	if ok, said, _ := g.v.ForceMove("a", "beta", "human"); !ok || said != "a moved acct -> beta (forced by human)" || pending(t, g) != nil {
		t.Fatalf("%v %q %+v", ok, said, pending(t, g))
	}
}

// F2: move_now stops the turn, waits for the note and the turn that writes it, then moves; cancel is too late by then.
func TestF2MoveNowWaitsForTheNoteThenMoves(t *testing.T) {
	g, clock := newReserveRig(t)
	g.run.status = "busy"
	stops := 0
	ok, said, err := g.v.MoveNow("a", "beta", "human", func() error { stops++; return nil })
	if !ok || stops != 1 || err != nil || said != "a moves to beta once its handoff note is in (by human)" {
		t.Fatalf("%v %d %q %v", ok, stops, said, err)
	}
	if s := pending(t, g); s == nil || s.Phase != db.SwitchStopping {
		t.Fatalf("%+v", s)
	}
	if ok, said, _ := g.v.CancelMove("a"); ok || said != "too late: a's turn was stopped for the move to beta and its note is being written" {
		t.Fatalf("%v %q", ok, said)
	}
	if ok, _, _ := g.v.ForceMove("a", "gamma", "human"); ok {
		t.Fatal("a second move replaced one that is stopping")
	}
	hold, written := true, false
	g.v.E.NoteHold = func(db.Q, string, int64) bool { return hold }
	g.v.E.NoteWritten = func(string) bool { return written }
	g.run.status = "idle" // the Esc ended the turn; the ask is out
	if runPending(t, g) {
		t.Fatal("moved before the note's turn began")
	}
	hold, g.run.status = false, "busy" // the note's turn
	*clock += 60
	if runPending(t, g) {
		t.Fatal("moved while the note was being written")
	}
	written, g.run.status = true, "idle"
	if !runPending(t, g) || g.row(t).Account.String != "beta" || len(g.sp.startedIn) != 1 {
		t.Fatalf("not moved once the note was in: %+v", g.row(t))
	}
	if p := lastPayload(t, g, "moved_account"); p["note_written"] != true {
		t.Fatalf("moved_account %v", p)
	}
}

// F2: a note that never comes does not hang the move: past the note wait it moves on the daemon's own record and says so.
func TestF2AHangingNoteDoesNotHangTheMove(t *testing.T) {
	g, clock := newReserveRig(t)
	g.v.E.NoteHold = func(db.Q, string, int64) bool { return true }
	if ok, _, _ := g.v.MoveNow("a", "beta", "human", noStop(t)); !ok {
		t.Fatal("not kept")
	}
	*clock += db.NoteWaitStaleS
	if runPending(t, g) {
		t.Fatal("gave up on the note at its limit, not past it")
	}
	*clock++
	if !runPending(t, g) || g.row(t).Account.String != "beta" {
		t.Fatalf("hung on the note: %+v", g.row(t))
	}
	if p := lastPayload(t, g, "moved_account"); p["note_written"] != false {
		t.Fatalf("moved_account %v", p)
	}
}

// move_now without an account runs the move kept for the turn's end; with none kept it is refused.
func TestMoveNowWithoutAnAccountRunsTheKeptOne(t *testing.T) {
	g, _ := newReserveRig(t)
	if ok, said, _ := g.v.MoveNow("a", "", "human", noStop(t)); ok || said != "a has no move waiting; name the account to move it to" {
		t.Fatalf("%v %q", ok, said)
	}
	g.run.status = "busy"
	g.v.ForceMove("a", "gamma", "human")
	if ok, _, _ := g.v.MoveNow("a", "", "human", func() error { return nil }); !ok {
		t.Fatal(ok)
	}
	if s := pending(t, g); s.Phase != db.SwitchStopping || s.To != "gamma" {
		t.Fatalf("%+v", s)
	}
}

// F3: a full target refuses at the call with the room text; one that fills before the turn ends drops the move with a move_failed line,
// and it is not tried again.
func TestF3AFullTargetRefusesOrFailsVisibly(t *testing.T) {
	g, _ := newReserveRig(t)
	g.run.status = "busy"
	g.l.roomOn = "has no free slot"
	if ok, said, _ := g.v.ForceMove("a", "beta", "human"); ok || said != "beta has no free slot - not moving a there" || pending(t, g) != nil {
		t.Fatalf("%v %q", ok, said)
	}
	g.l.roomOn = ""
	g.v.ForceMove("a", "beta", "human")
	g.l.roomOn, g.run.status = "has no free slot", "idle"
	if !runPending(t, g) {
		t.Fatal("a failed move was silent")
	}
	if p := lastPayload(t, g, db.MoveFailedKind); p["to"] != "beta" || p["reason"] != "beta has no free slot - not moving a there" {
		t.Fatalf("move_failed %v", p)
	}
	if pending(t, g) != nil || len(g.pane.handles) != 0 || g.row(t).Account.String == "beta" {
		t.Fatalf("the failed move ended or kept something: %v %+v", g.pane.handles, pending(t, g))
	}
	if runPending(t, g) {
		t.Fatal("a failed move was tried again")
	}
}

// D1/D2: a pin refuses a person's move at the call, and one set while the move waits fails it visibly at the turn's end.
func TestDAPinRefusesAPersonsMove(t *testing.T) {
	g, _ := newReserveRig(t)
	g.run.status = "busy"
	pinned := "is not acct, the account it is pinned to"
	g.l.roomOn = pinned
	if ok, said, _ := g.v.ForceMove("a", "beta", "human"); ok || said != "beta "+pinned+" - not moving a there" {
		t.Fatalf("%v %q", ok, said)
	}
	if ok, said, _ := g.v.MoveNow("a", "beta", "human", noStop(t)); ok || said != "beta "+pinned+" - not moving a there" {
		t.Fatalf("move_now: %v %q", ok, said)
	}
	g.l.roomOn = ""
	g.v.ForceMove("a", "beta", "human")
	g.l.roomOn, g.run.status = pinned, "idle"
	runPending(t, g)
	if p := lastPayload(t, g, db.MoveFailedKind); p["reason"] != "beta "+pinned+" - not moving a there" || g.row(t).Account.String == "beta" {
		t.Fatalf("move_failed %v", p)
	}
}

// F5: a kept move outlives a daemon restart (a new mover over the same store, past every note wait) and runs at the turn's end.
func TestF5AKeptMoveSurvivesARestart(t *testing.T) {
	g, clock := newReserveRig(t)
	g.run.status = "busy"
	g.v.ForceMove("a", "beta", "human")
	*clock += 3 * db.NoteWaitStaleS
	g.m.Locks = &AgentLocks{}
	g.v = newMoveRigOn(g.m, g.l).v
	if s := pending(t, g); s == nil || s.Phase != db.SwitchScheduled || s.To != "beta" {
		t.Fatalf("lost on restart: %+v", s)
	}
	g.run.status = "idle"
	if !runPending(t, g) || g.row(t).Account.String != "beta" {
		t.Fatalf("%+v", g.row(t))
	}
}

// F6: a restart while move_now waits for the note moves the agent once, to one account.
func TestF6ARestartMidMoveNowEndsOnOneAccount(t *testing.T) {
	g, clock := newReserveRig(t)
	g.run.status = "busy"
	g.v.MoveNow("a", "beta", "human", func() error { return nil })
	*clock += db.NoteWaitStaleS + 1
	g.v = newMoveRigOn(g.m, g.l).v
	g.run.status = "idle"
	if !runPending(t, g) || runPending(t, g) {
		t.Fatal("not run exactly once")
	}
	if g.row(t).Account.String != "beta" || len(g.sp.startedIn) != 1 || len(g.pane.handles) != 1 {
		t.Fatalf("%+v started %v closed %v", g.row(t), g.sp.startedIn, g.pane.handles)
	}
}

// Another switch under way refuses a person's move, naming its phase.
func TestAMoveIsRefusedWhileAnotherSwitchIsUnderWay(t *testing.T) {
	g, _ := newReserveRig(t)
	db.SetSwitch(g.m.Q, db.AccountSwitch{Agent: "a", Cause: "usage", From: "acct", To: "gamma", Phase: db.SwitchStarting, Since: g.m.now()})
	if ok, said, _ := g.v.ForceMove("a", "beta", "human"); ok || said != "a is already switching accounts (starting acct -> gamma)" {
		t.Fatalf("%v %q", ok, said)
	}
}

// A turn move_now cannot stop keeps the move for the turn's end, where it can still be cancelled.
func TestMoveNowThatCannotStopTheTurnKeepsTheMove(t *testing.T) {
	g, _ := newReserveRig(t)
	g.run.status = "busy"
	ok, said, _ := g.v.MoveNow("a", "beta", "human", func() error { return errors.New("typing elsewhere") })
	if !ok || said != "a's turn could not be stopped (typing elsewhere): it moves to beta when the turn ends (by human)" {
		t.Fatalf("%v %q", ok, said)
	}
	if s := pending(t, g); s == nil || s.Phase != db.SwitchScheduled {
		t.Fatalf("%+v", s)
	}
	if ok, _, _ := g.v.CancelMove("a"); !ok {
		t.Fatal("not cancellable")
	}
}

// A turn stopped at the usage limit is over for a kept move, though the session reads busy behind the limit's dialog.
func TestAKeptMoveRunsBehindTheLimitDialog(t *testing.T) {
	g, _ := newReserveRig(t)
	g.run.status = "busy"
	g.v.ForceMove("a", "beta", "human")
	g.v.E.LimitDialogOpen = func(string) bool { return true }
	if !runPending(t, g) || g.row(t).Account.String != "beta" {
		t.Fatalf("%+v", g.row(t))
	}
}

// A policy handoff that comes before a kept move takes the agent to the person's account, or says it could not.
func TestAPolicyHandoffGoesWhereTheKeptMoveSays(t *testing.T) {
	g, _ := newReserveRig(t)
	g.run.status = "busy"
	g.v.ForceMove("a", "gamma", "human")
	g.run.status = "idle"
	if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, ""); !ok || err != nil || g.row(t).Account.String != "gamma" {
		t.Fatalf("%v %v %+v", ok, err, g.row(t))
	}
	g, _ = newReserveRig(t)
	g.run.status = "busy"
	g.v.ForceMove("a", "gamma", "human")
	g.run.status, g.l.roomOn = "idle", "has no free slot"
	g.v.PerformHandoff("a", HandoffTrigger{Kind: "usage", Reason: "wall"}, "")
	if p := lastPayload(t, g, db.MoveFailedKind); p["reason"] != "a usage handoff came first and could not take it there" {
		t.Fatalf("lost without a word: %v", p)
	}
}

// A kept move stays in sight through a stall, so it can be cancelled before the agent comes back.
func TestAKeptMoveStaysInSightThroughAStall(t *testing.T) {
	g, _ := newReserveRig(t)
	g.run.status = "busy"
	g.v.ForceMove("a", "beta", "human")
	db.SetAgentState(g.m.Q, "a", states.Stalled, 0, "x")
	if pending(t, g) == nil {
		t.Fatal("hidden by the stall")
	}
	if ok, _, _ := g.v.CancelMove("a"); !ok {
		t.Fatal("not cancellable while stalled")
	}
}
