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
}

func TestAWakeWithNoDeskToTakeSaysWhyAndLaunchesNothing(t *testing.T) {
	m, tm, desks := closedWithDesk(t, "")
	desks.why = "could not check out a-work"
	ok, said, _ := m.WakeAgent("a", WakeOpts{Force: true})
	if ok || !strings.Contains(said, "no worktree to start in - could not check out a-work") || len(tm.relaunched) != 0 {
		t.Fatalf("%v %q %v", ok, said, tm.relaunched)
	}
}

func wakePromptText(t *testing.T, m *Manager) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(m.Home, "wake", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A failed resume takes the record and starts fresh at once: it never waits for, nor asks for, a note.
func TestAFailedResumeStartsFreshAtOnce(t *testing.T) {
	m, tm, _ := closedWithDesk(t, "/wt")
	tm.failResume = true
	h := &recordingHandoff{fakeHandoff: fakeHandoff{}, q: m.Q}
	m.Handoff = h
	if ok, said, err := m.WakeAgent("a", WakeOpts{Force: true}); err != nil || !ok {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if n := len(tm.relaunched); n != 2 || strings.Contains(tm.relaunched[1], "--resume") || strings.Join(h.calls, ",") != "record" {
		t.Fatalf("a resume then one fresh start: %v, %q", tm.relaunched, h.calls)
	}
	if got := wakePromptText(t, m); !strings.HasPrefix(got, "Your session could not be resumed, so this is a fresh one. Your handoff is in your system prompt above.") {
		t.Fatalf("%q", got)
	}
}

func TestAMoveSaysWhyItMoved(t *testing.T) {
	m, _, _ := newManager(t)
	got, _ := m.WakePrompt("a", false, WakePromptIn{MovedTo: "beta", MoveWhy: "Your prompt cache had expired"})
	want := "Your prompt cache had expired, so this session was started on beta."
	if !strings.HasPrefix(got, want) {
		t.Fatalf("%q", got)
	}
}

// With the worktree deleted while it slept (the setting) and its pane closed, a closed agent is still woken, resuming: the wake makes
// the folder again at its path. A recorded folder the pool does not hold for it is no worktree to wake in.
func TestAClosedAgentWhoseWorktreeWasDeletedWhileItSleptIsWokenAndResumes(t *testing.T) {
	m, _, desks := closedWithDesk(t, "/gone/a-1a2b3c")
	m.Q.Exec("UPDATE agent_state SET terminal = NULL, worktree = '/gone/a-1a2b3c' WHERE agent = 'a'")
	sp := m.Spawn.(*fakeSpawn)
	sp.gone, sp.ok = map[string]bool{"/gone/a-1a2b3c": true}, true
	row, _ := db.GetAgentState(m.Q, "a")
	if m.ResumableWithoutTerminal(row) {
		t.Fatal("a folder the pool does not hold for it")
	}
	m.Q.Exec("INSERT INTO worktree_folders(path, repo, holder, last_used) VALUES('/gone/a-1a2b3c', '/repo', 'a', 0)")
	if !m.ResumableWithoutTerminal(row) {
		t.Fatal("its own folder, deleted while it slept, is not woken")
	}
	ok, said, err := m.WakeAgent("a", WakeOpts{Force: true})
	if err != nil || !ok || len(desks.asked) != 1 || len(sp.cmds) != 1 || !strings.Contains(sp.cmds[0], "--resume sess-1") ||
		sp.startedIn[0] != "a@/gone/a-1a2b3c" {
		t.Fatalf("%v %q %v; asked %v, started %v %v", ok, said, err, desks.asked, sp.startedIn, sp.cmds)
	}
}
