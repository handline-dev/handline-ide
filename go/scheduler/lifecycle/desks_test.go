package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/scheduler/worktreepool"
)

// fakeDesks gives the desk give and, as the pool does, records it on the agent's row.
type fakeDesks struct {
	give, why string
	asked     []string
	q         db.Q
}

func (f *fakeDesks) TakeFrom(agent, repo, _ string) (worktreepool.Taken, string, error) {
	f.asked = append(f.asked, agent+"@"+repo)
	if f.why == "" && f.q != nil {
		if _, err := f.q.Exec("UPDATE agent_state SET worktree = ? WHERE agent = ?", f.give, agent); err != nil {
			return worktreepool.Taken{}, "", err
		}
	}
	return worktreepool.Taken{Path: f.give, How: "free"}, f.why, nil
}

func closedWithDesk(t *testing.T, give string) (*Manager, *fakeTerm, *fakeDesks) {
	t.Helper()
	m, d, tm := newManager(t)
	d.waitingMsg = "Here is your request."
	d.tookPrompt = true
	tm.launchOK = true
	desks := &fakeDesks{give: give, q: m.Q}
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

// noteHandoff is recordingHandoff that moves a walled agent to dest when asked.
type noteHandoff struct {
	*recordingHandoff
	dest string
}

func (h noteHandoff) MovePlan(db.Q, string, string, bool) (string, string) { return h.dest, "" }
func (noteHandoff) BackingOff(db.Q, string) bool                           { return false }

// wallWhileResumable: account "a" is at its wall for a session to resume; a fresh start goes where there is room, as WakeWait does.
type wallWhileResumable struct{ wallLaunch }

func (wallWhileResumable) WakeWait(_ db.Q, _ string, row *db.AgentState) *WakeWait {
	if row.ResumeSession.String == "" {
		return nil
	}
	return &WakeWait{Account: "a", Pct: 99, Window: "5h"}
}

func wakePromptText(t *testing.T, m *Manager) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(m.Home, "wake", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// waitsThenStartsFresh wakes a twice: first while the transcript note is being written, which waits and launches nothing, then once
// it is in, which starts fresh. It returns the second wake's prompt.
func waitsThenStartsFresh(t *testing.T, m *Manager, tm *fakeTerm, h *recordingHandoff) string {
	t.Helper()
	before := len(tm.relaunched)
	ok, said, err := m.WakeAgent("a", WakeOpts{Force: true})
	if err != nil || ok || !strings.Contains(said, "waiting for the handoff note") {
		t.Fatalf("did not wait for the note: %v %q %v", ok, said, err)
	}
	if n := len(tm.relaunched) - before; n > 1 || n == 1 && !strings.Contains(tm.relaunched[before], "--resume") {
		t.Fatalf("a fresh start went before its note: %v", tm.relaunched)
	}
	if rec, _ := db.GetHandoff(m.Q, "a"); rec == "" {
		t.Fatal("no record for the note to go into")
	}
	h.pending = false
	before = len(tm.relaunched)
	if ok, said, err := m.WakeAgent("a", WakeOpts{Force: true}); err != nil || !ok {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if len(tm.relaunched) != before+1 || strings.Contains(tm.relaunched[before], "--resume") {
		t.Fatalf("not one fresh start: %v", tm.relaunched)
	}
	return wakePromptText(t, m)
}

func TestADeskMovedWakeWaitsForTheTranscriptNoteAndSaysTheFolderMoved(t *testing.T) {
	m, tm, _ := closedWithDesk(t, "/desks/desk-2")
	h := &recordingHandoff{fakeHandoff: fakeHandoff{pending: true}, q: m.Q}
	m.Handoff = h
	got := waitsThenStartsFresh(t, m, tm, h)
	if !strings.HasPrefix(got, "Your folder went to another agent, so this is a fresh session in desk-2. Your handoff is in your system prompt above") {
		t.Fatalf("the wake does not say the folder moved: %q", got)
	}
}

func TestADeskMovedWakeThatMovesAccountWaitsForTheNoteAndLetsTheSlotGo(t *testing.T) {
	m, tm, _ := closedWithDesk(t, "/desks/desk-2")
	m.Launch = wallWhileResumable{}
	h := &recordingHandoff{fakeHandoff: fakeHandoff{pending: true}, q: m.Q}
	m.Handoff = noteHandoff{recordingHandoff: h, dest: "b"}
	got := waitsThenStartsFresh(t, m, tm, h)
	if _, dropped, _ := db.LastEventID(m.Q, "a", "hold_dropped"); !dropped {
		t.Fatal("the slot the move reserved was kept through the wait")
	}
	if sw, _ := db.GetSwitch(m.Q, "a"); sw != nil {
		t.Fatalf("the switch to b still shows, though it started where there was room: %+v", sw)
	}
	if strings.Contains(got, "usage limit") || !strings.Contains(got, "fresh session in desk-2") {
		t.Fatalf("the wake does not say the real reason: %q", got)
	}
}

func TestAFailedResumeWaitsForTheTranscriptNoteBeforeItsFreshStart(t *testing.T) {
	m, tm, _ := closedWithDesk(t, "/wt")
	tm.failResume = true
	h := &recordingHandoff{fakeHandoff: fakeHandoff{pending: true}, q: m.Q}
	m.Handoff = h
	got := waitsThenStartsFresh(t, m, tm, h)
	if row, _ := db.GetAgentState(m.Q, "a"); row.ResumeSession.String != "" {
		t.Fatalf("the resume that failed is tried again: %+v", row)
	}
	if !strings.HasPrefix(got, "Your session could not be resumed, so this is a fresh one. Your handoff is in your system prompt above.") {
		t.Fatalf("%q", got)
	}
}

func TestAMoveSaysWhyItMoved(t *testing.T) {
	m, _, _ := newManager(t)
	got, _ := m.WakePrompt("a", false, WakePromptIn{MovedTo: "beta", MoveWhy: "Your prompt cache had expired", Desk: "/d/desk-1"})
	want := "Your folder went to another agent, so this is a fresh session in desk-1. Your prompt cache had expired, so this session was started on beta."
	if !strings.HasPrefix(got, want) {
		t.Fatalf("%q", got)
	}
}

type heldTyping struct{ fakeTyping }

func (heldTyping) TakeTerminal(string) (func(), bool) { return nil, false }

// A closed agent whose folder is about to move is let go of: its session's record is taken unless it has one, and its resume id goes;
// one that is not closed, or whose terminal a wake holds, is not.
func TestLetGoOfResumeTakesTheRecordAndForgetsTheSession(t *testing.T) {
	m, _, _ := newManager(t)
	h := &recordingHandoff{q: m.Q}
	m.Handoff = h
	db.SetTerminal(m.Q, "a", "t1", "/wt")
	db.SetClosed(m.Q, "a", "sess-1")
	m.Typing = heldTyping{}
	if ok, err := m.LetGoOfResume("a"); err != nil || ok {
		t.Fatalf("let go while a wake holds its terminal: %v %v", ok, err)
	}
	m.Typing = fakeTyping{}
	if ok, err := m.LetGoOfResume("a"); err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	row, _ := db.GetAgentState(m.Q, "a")
	if rec, _ := db.GetHandoff(m.Q, "a"); rec != "record of sess-1" || row.ResumeSession.String != "" {
		t.Fatalf("record %q, resume %q", rec, row.ResumeSession.String)
	}
	db.SetClosed(m.Q, "a", "sess-2") // it has a record now: that one is kept
	if ok, _ := m.LetGoOfResume("a"); !ok || len(h.calls) != 1 {
		t.Fatalf("the record it had was taken again: %v", h.calls)
	}
	if ok, _ := m.LetGoOfResume("a"); !ok {
		t.Fatal("an agent with no session to resume holds its folder")
	}
	if _, err := m.Q.Exec("INSERT INTO agent_state(agent, state, updated, resume_session) VALUES('b', 'IDLE', 1, 's')"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.LetGoOfResume("b"); ok {
		t.Fatal("an agent that is not closed was let go of")
	}
}
