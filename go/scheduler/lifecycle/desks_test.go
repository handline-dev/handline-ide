package lifecycle

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/scheduler/worktreepool"
)

type fakeDesks struct {
	give, why string
	asked     []string
}

func (f *fakeDesks) Take(agent, repo string) (worktreepool.Taken, string, error) {
	f.asked = append(f.asked, agent+"@"+repo)
	return worktreepool.Taken{Path: f.give, How: "free"}, f.why, nil
}

func closedWithDesk(t *testing.T, give string) (*Manager, *fakeTerm, *fakeDesks) {
	t.Helper()
	m, d, tm := newManager(t)
	d.waitingMsg = "Here is your request."
	d.tookPrompt = true
	tm.launchOK = true
	desks := &fakeDesks{give: give}
	m.Desks, m.Agents = desks, fakeAgents{repo: "/repo"}
	db.SetTerminal(m.Q, "a", "t1", "/wt")
	db.SetClosed(m.Q, "a", "sess-1")
	db.AddEvent(m.Q, "a", "Stop")
	return m, tm, desks
}

func TestAWakeBackInItsOwnDeskResumesItsSession(t *testing.T) {
	m, tm, desks := closedWithDesk(t, "/wt")
	ok, said, err := m.WakeAgent("a", WakeOpts{Force: true})
	if err != nil || !ok {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if len(desks.asked) != 1 || desks.asked[0] != "a@/repo" || len(tm.relaunched) != 1 || !strings.HasSuffix(tm.relaunched[0], "-- --resume sess-1") {
		t.Fatalf("asked %v, relaunched %v", desks.asked, tm.relaunched)
	}
	if _, moved, _ := db.LastEventID(m.Q, "a", "desk_changed"); moved {
		t.Fatal("its own desk is no change")
	}
}

func TestAWakeWhoseDeskWentToAnotherStartsFreshInTheDeskItGets(t *testing.T) {
	m, tm, _ := closedWithDesk(t, "/desk-2")
	ok, said, err := m.WakeAgent("a", WakeOpts{Force: true})
	if err != nil || !ok {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if len(tm.relaunched) != 1 || strings.Contains(tm.relaunched[0], "--resume") {
		t.Fatalf("a session kept under another folder cannot be resumed: %v", tm.relaunched)
	}
	if _, moved, _ := db.LastEventID(m.Q, "a", "desk_changed"); !moved {
		t.Fatal("the change of desk is recorded")
	}
	row, _ := db.GetAgentState(m.Q, "a")
	if row.ResumeSession.String != "" {
		t.Fatalf("the resume id is forgotten: %+v", row)
	}
}

func TestAWakeWithNoDeskToTakeSaysWhyAndLaunchesNothing(t *testing.T) {
	m, tm, desks := closedWithDesk(t, "")
	desks.why = "could not check out a-work"
	ok, said, _ := m.WakeAgent("a", WakeOpts{Force: true})
	if ok || !strings.Contains(said, "no desk to start in - could not check out a-work") || len(tm.relaunched) != 0 {
		t.Fatalf("%v %q %v", ok, said, tm.relaunched)
	}
}
