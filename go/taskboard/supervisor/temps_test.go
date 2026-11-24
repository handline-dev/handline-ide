package supervisor

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
)

// fakeTemps hires a temp as the daemon does (team.HireRow) and gives it a session, or hires none when manual.
type fakeTemps struct {
	r      *rig
	manual bool
	hired  []string
}

func (f *fakeTemps) Hire(agent string) string {
	if f.manual {
		return ""
	}
	temp, err := team.HireRow(f.r.DB, agent, f.r.clock)
	f.r.must(err)
	f.r.agentUp(temp)
	f.hired = append(f.hired, temp)
	return temp
}

func (f *fakeTemps) Waiting(string) bool { return false }

// tempRig is alice mid-turn on T<first>, carol's, with T<second> from carol queued to her behind it.
func tempRig(t *testing.T, manual bool) (r *rig, temps *fakeTemps, first, second int64) {
	r = newRig(t)
	r.bigTeam()
	r.agentUp(r.k("alice"))
	temps = &fakeTemps{r: r, manual: manual}
	r.Temps = temps
	first = r.newTask("alice", "carol", "the screen")
	r.beginOnTop(first)
	r.must(db.SetAgentState(r.DB, r.k("alice"), states.Running, 0, ""))
	second = r.newTask("alice", "carol", "a typo on the settings page")
	return r, temps, first, second
}

func (r *rig) told(agent string, tid int64) bool {
	r.t.Helper()
	var n int
	r.must(r.DB.QueryRow(`SELECT COUNT(*) FROM requests q JOIN task_requests l ON l.request_id = q.id WHERE q.target = ? AND l.task_id = ?`,
		agent, tid).Scan(&n))
	return n > 0
}

// A task queued to an agent mid-turn on another goes to a temp of it, which starts on it, and the agent's turn gets nothing of it.
func TestATaskForAnAgentMidTurnGoesToATempOfIt(t *testing.T) {
	r, temps, first, second := tempRig(t, false)
	typed := len(r.term.typed)
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 1 {
		t.Fatalf("hired %v", temps.hired)
	}
	temp := temps.hired[0]
	if got := r.acting(second); got != temp {
		t.Fatalf("T%d is %s's, want the temp %s's", second, got, temp)
	}
	if cur, ok, _ := db.CurrentTask(r.DB, temp); !ok || cur != second {
		t.Fatalf("the temp holds %d %v", cur, ok)
	}
	if !r.told(temp, second) {
		t.Fatal("the temp was not given the task")
	}
	if r.told(r.k("alice"), second) {
		t.Fatal("alice was told of the task her temp took")
	}
	for _, s := range r.term.typed[typed:] {
		if strings.Contains(s, "typo") && !strings.Contains(s, "temp of") {
			t.Fatalf("typed to alice: %q", s)
		}
	}
	if cur, _, _ := db.CurrentTask(r.DB, r.k("alice")); cur != first {
		t.Fatalf("alice holds %d, not her own T%d", cur, first)
	}
	if from, _ := team.Temps(r.DB); from[temp] != r.k("alice") {
		t.Fatalf("temps %v", from)
	}
}

// Manual, or no room: the task waits for the agent as it always did, and nobody else is given it.
func TestWithNoTempTheTaskWaitsForTheBusyAgent(t *testing.T) {
	r, _, first, second := tempRig(t, true)
	r.must(r.SweepTaskQueue(r.clock))
	if got := r.acting(second); got != r.k("alice") {
		t.Fatalf("T%d went to %s", second, got)
	}
	if cur, _, _ := db.CurrentTask(r.DB, r.k("alice")); cur != first {
		t.Fatalf("alice holds %d", cur)
	}
	if r.told(r.k("alice"), second) {
		t.Fatal("alice was told of a task behind her current one")
	}
}

// An agent not mid-turn hires nobody: one that is free takes the task itself, as before.
func TestAnIdleAgentTakesTheTaskItself(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	r.agentUp(r.k("alice"))
	temps := &fakeTemps{r: r}
	r.Temps = temps
	tid := r.newTask("alice", "carol", "a typo")
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 0 {
		t.Fatalf("hired %v", temps.hired)
	}
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("alice")); !ok || cur != tid {
		t.Fatalf("alice holds %d %v", cur, ok)
	}
}

// One sweep hires one temp: the next task waits until that temp's session is up, so each hire meets the limits on its own.
func TestOneSweepHiresOneTemp(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	third := r.newTask("alice", "carol", "another typo")
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 1 || r.acting(second) != temps.hired[0] || r.acting(third) != r.k("alice") {
		t.Fatalf("hired %v; T%d is %s's, T%d %s's", temps.hired, second, r.acting(second), third, r.acting(third))
	}
}
