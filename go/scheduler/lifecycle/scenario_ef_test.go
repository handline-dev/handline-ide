package lifecycle

import (
	"testing"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team/teamtest"
)

// Rows B4, E3, E7, F5, F6 and F7 of docs/design/account-assignment-scenarios.md on the real Launcher and usage adapter (placeRig), the clock
// moved by hand. E4 is TestAnOpenAgentsMoveDoesNotTakeTheSlotAnOlderClosedWaiterHolds; E6 is in accounts/usage.

// clockAt points the usage reads at *now.
func (r *placeRig) clockAt(now *int64) {
	r.u.Env.Now = func() float64 { return float64(*now) }
}

// B4: alpha at its cap with pinned agents waiting; one alpha agent ends its turn and exactly one waiter starts, the count never past the cap.
func TestB4ATurnEndingOnAFullAccountStartsOneWaiter(t *testing.T) {
	r := newPlaceRig(t, "alpha")
	db.SetAgentState(r.q, "run-alpha-2", states.Running, 0, "")
	db.SetAccount(r.q, "run-alpha-2", "alpha", "")
	for _, a := range []string{"x", "y"} {
		teamtest.Set(t, r.l.Home, a, "account", "alpha")
	}
	if acct, _ := r.l.place("x", false, ""); acct != "" {
		t.Fatalf("placed on a full account: %q", acct)
	}
	db.SetAgentState(r.q, "run-alpha-2", states.Idle, 0, "turn ended")
	got := r.race(t)
	if n := btoi(got["x"] == "alpha") + btoi(got["y"] == "alpha"); n != 1 || got["x"]+got["y"] != "alpha" {
		t.Fatalf("%v - one waiter starts, the other waits for alpha", got)
	}
}

// E3: a wake reservation no launch answers holds its slot through WakeReserveS, the hold usage counts, and the next waiter gets the
// slot at that second.
func TestE3AnUnansweredReservationGoesToTheNextWaiterAtItsHoldsEnd(t *testing.T) {
	r := newPlaceRig(t, "alpha")
	now := usageNow
	r.clockAt(&now)
	if acct, _ := r.l.place("x", false, ""); acct != "alpha" {
		t.Fatalf("%q", acct)
	}
	now = usageNow + usage.WakeReserveS - 1
	if acct, _ := r.l.place("y", false, ""); acct != "" {
		t.Fatalf("took a held slot at +%d s: %q", usage.WakeReserveS-1, acct)
	}
	now = usageNow + usage.WakeReserveS
	if acct, _ := r.l.place("y", false, ""); acct != "alpha" {
		t.Fatalf("the slot did not go to the next waiter at +%d s: %q", usage.WakeReserveS, acct)
	}
}

// E7: a clock stepped back an hour lets no reservation go early, and no count goes negative.
func TestE7AClockSteppedBackReleasesNothingEarly(t *testing.T) {
	r := newPlaceRig(t, "alpha")
	now := usageNow
	r.clockAt(&now)
	r.l.place("x", false, "")
	now = usageNow - 3600
	if acct, _ := r.l.place("y", false, ""); acct != "" {
		t.Fatalf("a held slot went early: %q", acct)
	}
	if n := r.u.ActiveOn(r.q, "alpha", ""); n != 2 {
		t.Fatalf("%d mid-turn on alpha", n)
	}
}

// F5: a restart rebuilds the counts from what the store holds: a new adapter over the same store counts the same, a reservation held
// before the restart still holds, and an agent both running and held is one.
func TestF5ARestartRebuildsTheCountsFromTheStore(t *testing.T) {
	r := newPlaceRig(t, "alpha", "beta")
	r.l.place("x", false, "")
	r.l.place("y", false, "")
	db.SetAgentState(r.q, "y", states.Running, 0, "") // y's session reported; its hold is still there
	db.SetAccount(r.q, "y", "beta", "")
	before := map[string]int{"alpha": r.u.ActiveOn(r.q, "alpha", ""), "beta": r.u.ActiveOn(r.q, "beta", "")}
	fresh := &UsageAdapter{Env: r.u.Env, Q: r.q}
	for a, n := range before {
		if got := fresh.ActiveOn(r.q, a, ""); got != n || n != 2 {
			t.Fatalf("%s: %d before the restart, %d after, want 2 each", a, n, got)
		}
	}
}

// F7: an account that logs out while agents run on it: they are not ended, its count still shows them, and new starts go elsewhere.
func TestF7ALoggedOutAccountKeepsItsAgentsAndTakesNoStarts(t *testing.T) {
	r := newPlaceRig(t, "alpha", "beta")
	r.l.Ports.Accounts = loggedOut{"alpha": true}
	if acct, _ := r.l.place("x", false, ""); acct != "beta" {
		t.Fatalf("a start went to %q", acct)
	}
	if row, _ := db.GetAgentState(r.q, "run-alpha"); row.State != states.Running {
		t.Fatalf("the running agent was touched: %+v", row)
	}
	if n := r.u.ActiveOn(r.q, "alpha", ""); n != 1 {
		t.Fatalf("alpha's count: %d", n)
	}
}

// loggedOut is the accounts port with these accounts logged out.
type loggedOut map[string]bool

func (l loggedOut) MayStart(acct string) bool { return !l[acct] }
func (loggedOut) LoginCommand(acct string) (string, error) {
	return fakeAccounts{}.LoginCommand(acct)
}
func (loggedOut) EnvFrom(env map[string]string, acct, agent string) map[string]string {
	return fakeAccounts{}.EnvFrom(env, acct, agent)
}

// F6: a daemon restarted mid-handoff finishes it from the step it reached: the record is taken, nothing is started, and the agent stays
// on the one account it was on until its wake.
func TestF6ARestartMidHandoffEndsOnOneAccountWithItsRecord(t *testing.T) {
	g := newTermMoveRig(t)
	db.SetAccount(g.m.Q, "a", "acct", "")
	db.StartHandoff(g.m.Q, "h1", "a", "s", "usage", 0)
	h, _ := db.InFlightFor(g.m.Q, "a")
	g.v = newMoveRigOn(g.m, g.l).v // the restarted daemon's mover
	if err := g.v.ResumeHandoff(*h); err != nil {
		t.Fatal(err)
	}
	row := g.row(t)
	if row.Account.String != "acct" || row.State != states.Parked || len(g.sp.startedIn) != 0 {
		t.Fatalf("%+v started %v", row, g.sp.startedIn)
	}
	if left, _ := db.InFlightFor(g.m.Q, "a"); left != nil {
		t.Fatalf("still in flight: %+v", left)
	}
	var step, state string
	if err := g.m.Q.QueryRow("SELECT step, state FROM handoff WHERE id = 'h1'").Scan(&step, &state); err != nil || step != "released" || state != "done" {
		t.Fatalf("the record step did not run to its end: %s %s %v", step, state, err)
	}
}
