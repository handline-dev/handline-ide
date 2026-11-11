package supervisor

import (
	"testing"

	"github.com/handline/handline/go/data/db"
)

func TestWallReasonCoversALimitAndAMovableParkOnly(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	if w := r.wallReason(r.DB, r.k(("bob")), nil); r.named(w) != "" {
		t.Fatal(w)
	}
	db.SetAgentState(r.DB, r.k(("bob")), "PARKED", r.clock+100, "usage")
	r.DB.Exec(r.sql("INSERT INTO handoff(id, agent, trigger_kind, step, state, updated) VALUES('h', 'bob', 'usage', 'done', 'done', 5)"))
	if w := r.wallReason(r.DB, r.k(("bob")), nil); r.named(w) != "bob is parked until its usage window rolls over" {
		t.Fatal(w)
	}
	r.DB.Exec(r.sql("UPDATE handoff SET trigger_kind = 'by hand'"))
	if w := r.wallReason(r.DB, r.k(("bob")), nil); r.named(w) != "" {
		t.Fatalf("a hand park is not the wall: %q", w)
	}
	r.Waits = fakeWaits{limit: map[string]bool{r.k(("bob")): true}}
	if w := r.wallReason(r.DB, r.k(("bob")), nil); r.named(w) != "bob is at its usage limit" {
		t.Fatal(w)
	}
}

func TestADialogStandingLongEnoughStallsTheAgent(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	if r.stalledOnDialog(r.DB, r.k(("bob")), "a trust dialog") {
		t.Fatal("stalled with no dialog standing")
	}
	db.SetCursor(r.DB, "dialog_since:"+r.k("bob"), r.clock-100)
	if r.stalledOnDialog(r.DB, r.k(("bob")), "a trust dialog") {
		t.Fatal("stalled too soon")
	}
	r.clock += 600
	if !r.stalledOnDialog(r.DB, r.k(("bob")), "a trust dialog") {
		t.Fatal("not stalled")
	}
	if s, _ := db.GetAgentState(r.DB, r.k(("bob"))); s.State != "STALLED" || s.Detail.String != "cannot get past a dialog: a trust dialog" {
		t.Fatalf("%+v", s)
	}
}

func TestPastDialogKeepsWhenThisDialogFirstProvedUnpassable(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.Dialogs = stubDialogs{why: "a dialog", title: "Trust?"}
	if why := r.pastDialog(r.DB, r.k(("bob")), false); why != "a dialog" {
		t.Fatal(why)
	}
	first, _ := db.Cursor(r.DB, "dialog_since:"+r.k("bob"))
	r.clock += 30
	r.pastDialog(r.DB, r.k(("bob")), false)
	if again, _ := db.Cursor(r.DB, "dialog_since:"+r.k("bob")); again != first {
		t.Fatal("the same dialog restarted its clock")
	}
	r.Dialogs = stubDialogs{why: "a dialog", title: "Another?"}
	r.pastDialog(r.DB, r.k(("bob")), false)
	if again, _ := db.Cursor(r.DB, "dialog_since:"+r.k("bob")); again == first {
		t.Fatal("a different dialog kept the old clock")
	}
	r.Dialogs = stubDialogs{}
	if why := r.pastDialog(r.DB, r.k(("bob")), false); why != "" {
		t.Fatal(why)
	}
	if n, _ := db.Cursor(r.DB, "dialog_since:"+r.k("bob")); n != 0 {
		t.Fatal("clock kept after the dialog went")
	}
}

type stubDialogs struct{ why, title string }

func (s stubDialogs) PastDialog(db.Q, string, bool) (string, string) { return s.why, s.title }
func (stubDialogs) Reason(k, d string) string                        { return k + ": " + d }
func (stubDialogs) DecisionAfterS() int64                            { return 600 }

func TestSessionPresenceNeedsAWitnessAndWaitsAreBounded(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	if !r.sessionPresent(r.k("bob")) {
		t.Fatal("the table lists it")
	}
	r.Observer = fakeObserver{map[string]bool{}}
	r.Terminal = noSession{r.term}
	if r.sessionPresent(r.k("bob")) {
		t.Fatal("no witness, still present")
	}
	if r.WaitSessionGone(r.k("bob")) != true {
		t.Fatal("a session out of the table is gone")
	}
	r.Observer = fakeObserver{map[string]bool{r.k(("bob")): true}}
	if r.WaitSessionGone(r.k("bob")) {
		t.Fatal("a session still in the table is gone")
	}
	if r.WaitForSession(r.k("bob"), 1, "") != true {
		t.Fatal("not waited for")
	}
	if r.WaitForSession(r.k("alice"), 1, "") {
		t.Fatal("alice has no session")
	}
	// NoteSeen: a fresh sweep reading wins over a new table read
	r.Observer = fakeObserver{map[string]bool{r.k(("bob")): false}}
	r.NoteSeen(map[string]Seen{r.k(("bob")): {SessionSeen: true, HasPID: true, SessionPID: 7}})
	if yes, known := r.TableListsSession(r.k("bob")); !yes || !known {
		t.Fatal("ignored the sweep's reading")
	}
	r.NoteSeen(map[string]Seen{r.k(("bob")): {SessionSeen: false}})
	if _, known := r.TableListsSession(r.k("bob")); known {
		t.Fatal("an unreadable table is known")
	}
	r.clock += SeenFreshS + 1
	if yes, known := r.TableListsSession(r.k("bob")); yes || !known {
		t.Fatal("a stale sweep reading was kept")
	}
}
