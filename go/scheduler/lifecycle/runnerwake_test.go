package lifecycle

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

// fakeRunners: leave says what LeaveFor answers for box; wait whether it waits for it.
type fakeRunners struct {
	leave    func() string
	wait     bool
	released []string
}

func (f *fakeRunners) LeaveFor(_, node string) (string, bool) {
	if node != "box" || f.leave == nil {
		return "", false
	}
	why := f.leave()
	return why, why == "" && f.wait
}
func (f *fakeRunners) Release(agent, node, _ string) error {
	f.released = append(f.released, agent+"@"+node)
	return nil
}
func (*fakeRunners) NodeName(node string) string {
	if node == "" {
		return "Mac mini"
	}
	return map[string]string{"box": "MacBook Air"}[node]
}

// closedOnRunner is agent a, closed with its terminal on runner box and a message waiting: the tick wakes it.
func closedOnRunner(t *testing.T) (*Manager, *fakeRunners, *fakeTerm, *fakeSpawn) {
	t.Helper()
	m, d, tm := newManager(t)
	r := &fakeRunners{}
	m.Runners = r
	d.waitingMsg, d.tookPrompt = "hello", true // a start here takes its prompt; the runner's launch never does (fakeTerm)
	db.SetTerminal(m.Q, "a", "box:%3", "/wt")
	db.SetClosed(m.Q, "a", "sess-1")
	db.SetAgentState(m.Q, "a", states.Queued, 0, "")
	db.AddPending(m.Q, "a", "hello", 1)
	row, _ := db.GetAgentState(m.Q, "a")
	if kind, _, err := m.StartOrWakeNeeded("a", row); err != nil || kind != "wake" {
		t.Fatalf("the waiting message wakes it: %q %v", kind, err)
	}
	sp := m.Spawn.(*fakeSpawn)
	sp.ok = true
	return m, r, tm, sp
}

func assertStartedHere(t *testing.T, m *Manager, r *fakeRunners, sp *fakeSpawn, reason string) {
	t.Helper()
	if len(sp.startedIn) != 1 || sp.startedIn[0] != "a@/wt" || strings.Contains(sp.cmds[0], "--resume") {
		t.Fatalf("started fresh where placement puts it: %v %v", sp.startedIn, sp.cmds)
	}
	if len(r.released) != 1 || r.released[0] != "a@box" {
		t.Fatalf("the runner's rows are dropped: %v", r.released)
	}
	if row, _ := db.GetAgentState(m.Q, "a"); row.WakeFailures != 0 || row.State == states.Stalled {
		t.Fatalf("leaving the runner counts toward no stall: %+v", row)
	}
	if _, p, ok, _ := db.LastEvent(m.Q, "a", db.MachineMovedKind); !ok || p["from"] != "MacBook Air" || p["to"] != "Mac mini" || p["reason"] != reason {
		t.Fatalf("its chat says where it started and why: %v", p)
	}
}

// A closed agent whose runner cannot start it (set to Off, away past the wait, none of its accounts there) is started here in the same
// wake, with nothing typed there.
func TestAClosedAgentOnARunnerThatCannotTakeItIsStartedHereInTheSameWake(t *testing.T) {
	for why, reason := range map[string]string{
		"it is set to off":                     "MacBook Air is set to off",
		"it is offline":                        "MacBook Air is offline",
		"none of its accounts can start there": "MacBook Air: none of its accounts can start there",
	} {
		m, r, tm, sp := closedOnRunner(t)
		r.leave = func() string { return why }
		ok, said, err := m.WakeAgent("a", WakeOpts{})
		if err != nil || !ok {
			t.Fatalf("%s: %v %q %v", why, ok, said, err)
		}
		if len(tm.relaunched) != 0 {
			t.Fatalf("%s: typed at the runner's pane: %v", why, tm.relaunched)
		}
		assertStartedHere(t, m, r, sp, reason)
	}
}

// A runner away only a moment (a master that just started, a laptop asleep) is waited for: the conversation is there, and no stall counts.
func TestARunnerAwayAMomentIsWaitedForUncounted(t *testing.T) {
	m, r, tm, sp := closedOnRunner(t)
	r.leave, r.wait = func() string { return "" }, true
	if ok, said, _ := m.WakeAgent("a", WakeOpts{}); ok || !strings.Contains(said, "waiting for MacBook Air") {
		t.Fatalf("%v %q", ok, said)
	}
	row, _ := db.GetAgentState(m.Q, "a")
	if len(tm.relaunched)+len(sp.startedIn)+len(r.released) != 0 || row.WakeFailures != 0 || row.ResumeSession.String != "sess-1" {
		t.Fatalf("nothing launched, released or counted: %v %v %v %+v", tm.relaunched, sp.startedIn, r.released, row)
	}
}

// A launch the runner did not take, and the runner cannot start the agent now (its account not logged in there), leaves it: one fresh
// start here in the same wake, uncounted.
func TestALaunchTheRunnerDidNotTakeIsStartedHereInTheSameWake(t *testing.T) {
	m, r, tm, sp := closedOnRunner(t)
	r.leave = func() string {
		if len(tm.relaunched) == 0 {
			return ""
		}
		return "none of its accounts can start there"
	}
	ok, said, err := m.WakeAgent("a", WakeOpts{})
	if err != nil || !ok {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if len(tm.relaunched) != 1 {
		t.Fatalf("one launch typed at the runner first: %v", tm.relaunched)
	}
	assertStartedHere(t, m, r, sp, "MacBook Air: none of its accounts can start there")
}

// A pane gone on a runner that can still start the agent is forgotten and the same attempt runs in a new terminal: its conversation kept.
func TestAGonePaneOnAGoodRunnerIsReopenedAndTheResumeKept(t *testing.T) {
	m, r, tm, sp := closedOnRunner(t)
	tm.notTaken = "could not type into %3: no server running on /tmp/tmux-1/x"
	if ok, said, err := m.WakeAgent("a", WakeOpts{}); err != nil || !ok {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if len(r.released) != 0 || len(sp.startedIn) != 1 || !strings.Contains(sp.cmds[0], "--resume sess-1") {
		t.Fatalf("not released, resumed in a new terminal: %v %v %v", r.released, sp.startedIn, sp.cmds)
	}
	if _, _, ok, _ := db.LastEvent(m.Q, "a", db.MachineMovedKind); ok {
		t.Fatal("it did not move")
	}
}

// An open session on a runner gone silent is not left: its turn may come back, and the reclaim decides.
func TestAnOpenSessionOnASilentRunnerIsNotLeft(t *testing.T) {
	m, _, _ := newManager(t)
	r := &fakeRunners{leave: func() string { return "it is offline" }}
	m.Runners = r
	db.SetTerminal(m.Q, "a", "box:%3", "/wt")
	db.SetAgentState(m.Q, "a", states.Running, 0, "")
	row, _ := db.GetAgentState(m.Q, "a")
	if kind, _, _ := m.StartOrWakeNeeded("a", row); kind != "" {
		t.Fatalf("an open session is neither woken nor started: %q", kind)
	}
	if h, _ := db.Terminal(m.Q, "a"); h != "box:%3" || len(r.released) != 0 {
		t.Fatalf("left: %q %v", h, r.released)
	}
}

// A running agent whose runner went silent starts here at once, fresh, its terminal there forgotten: a person's press drops the runner's
// rows, a reclaim has dropped them already; one whose session is on this machine is refused.
func TestStartElsewhereStartsAnAgentHeldOnASilentRunnerHere(t *testing.T) {
	for _, release := range []bool{true, false} {
		m, r, _, sp := closedOnRunner(t)
		db.ClearClosed(m.Q, "a")
		db.SetAgentState(m.Q, "a", states.Running, 0, "")
		ok, said, err := m.StartElsewhere("a", "it is silent since 11-28 10:00", release, release)
		if err != nil || !ok || said != "started on Mac mini" {
			t.Fatalf("release %v: %v %q %v", release, ok, said, err)
		}
		if len(sp.startedIn) != 1 || strings.Contains(sp.cmds[0], "--resume") {
			t.Fatalf("started fresh here: %v %v", sp.startedIn, sp.cmds)
		}
		if want := map[bool]int{true: 1, false: 0}[release]; len(r.released) != want {
			t.Fatalf("release %v: the runner's rows dropped %v", release, r.released)
		}
		if h, _ := db.Terminal(m.Q, "a"); strings.HasPrefix(h, "box:") {
			t.Fatalf("the runner's terminal is still its own: %q", h)
		}
		if _, p, ok, _ := db.LastEvent(m.Q, "a", db.MachineMovedKind); !ok || p["from"] != "MacBook Air" || p["to"] != "Mac mini" ||
			p["reason"] != "MacBook Air is silent since 11-28 10:00" {
			t.Fatalf("its chat says where it started: %v", p)
		}
		if row, _ := db.GetAgentState(m.Q, "a"); row.State == states.Running && row.Terminal.String == "" {
			t.Fatalf("left running with no session: %+v", row)
		}
	}
	m, _, _, _ := closedOnRunner(t)
	db.SetTerminal(m.Q, "a", "%3", "/wt")
	if ok, said, err := m.StartElsewhere("a", "x", true, true); ok || err != nil || said != "its session is not on another machine" {
		t.Fatalf("on this machine: %v %q %v", ok, said, err)
	}
}
