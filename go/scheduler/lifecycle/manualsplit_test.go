package lifecycle

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// manualLaunch is moveLaunch with account_split manual.
type manualLaunch struct{ *moveLaunch }

func (manualLaunch) ManualSplit() bool { return true }

// With account_split manual an agent starts on the account it is on, not where the pick says, and the daemon moves it nowhere; a new
// agent, on none yet, is placed as in auto.
func TestManualSplitKeepsAnAgentOnItsAccount(t *testing.T) {
	r := pinRig(t, "")
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, pick: "b", cap: 9, manual: true}
	if got := r.l.FindRoom("ann"); got != "b" {
		t.Fatalf("a new agent is placed by the pick: %q", got)
	}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "a" }
	if got := r.l.FindRoom("ann"); got != "a" {
		t.Fatalf("its own account, not the pick: %q", got)
	}
	if got := r.l.SlowestPaced(r.l.Q, "ann", nil, "a"); got != "" {
		t.Fatalf("a full account sends it elsewhere: %q", got)
	}
	if got, _ := r.l.RoomNow(r.l.Q, "ann", nil, "a"); got != "b" {
		t.Fatalf("a person's start off its account still goes: %q", got)
	}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "gone" }
	if got := r.l.FindRoom("ann"); got != "b" {
		t.Fatalf("an account it cannot use (logged out) is not kept: %q", got)
	}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "a" }
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, by: map[string]AccountStatus{"a": {Exists: true, Paused: true}}, pick: "b", cap: 9, manual: true}
	if got := r.l.FindRoom("ann"); got != "b" {
		t.Fatalf("a paused account is not kept: %q", got)
	}
	if got := r.l.SlowestPaced(r.l.Q, "ann", nil, "a"); got != "b" {
		t.Fatalf("a full paused account still sends it elsewhere: %q", got)
	}
	walled := 100
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, ShortTrusted: &walled}, pick: "b", manual: true}
	if got := r.l.FindRoom("ann"); got != "" {
		t.Fatalf("at its wall it waits, not moves: %q", got)
	}
}

// At a wall a manual agent stays; a move off an account it cannot use at all (a login gone) still goes.
func TestManualSplitPlansNoMoveAtAWall(t *testing.T) {
	g := newTermMoveRig(t)
	g.m.Launch = manualLaunch{g.l}
	for _, kind := range []string{"usage", "usage_7d", "usage_early"} {
		if dest, why, _ := g.v.MovePlan("a", kind, true); dest != "" || !strings.HasPrefix(why, "account_split is manual") {
			t.Fatalf("%s: %q %q", kind, dest, why)
		}
	}
	if dest, _, _ := g.v.MovePlan("a", LoginTrigger, true); dest != "beta" {
		t.Fatalf("a login gone: %q", dest)
	}
}

// manualHeadroom is headroomLaunch with account_split manual.
type manualHeadroom struct{ headroomLaunch }

func (manualHeadroom) ManualSplit() bool { return true }

// A cold wake past its cache's life is not re-homed with account_split manual: it starts on its own account.
func TestManualSplitKeepsAColdWakeOnItsAccount(t *testing.T) {
	m, d, tm := newManager(t)
	d.waitingMsg, d.tookPrompt, tm.launchOK = "Here is your request.", true, true
	asked := 0
	m.Launch = manualHeadroom{headroomLaunch{asked: &asked}}
	db.SetTerminal(m.Q, "a", "t1", "/wt")
	db.SetAccount(m.Q, "a", "own", "recorded")
	db.SetClosed(m.Q, "a", "sess-1")
	if ok, said, err := m.WakeAgentLocked("a", WakeOpts{Force: true}); err != nil || !ok {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if _, p, moved, _ := db.LastEvent(m.Q, "a", "moved_account"); moved || asked != 0 {
		t.Fatalf("not moved: %v %v, asked %d", moved, p, asked)
	}
}
