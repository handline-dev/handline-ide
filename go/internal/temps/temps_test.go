package temps_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	. "github.com/handline/handline/go/internal/temps"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/storetest"
)

func TestMain(m *testing.M) { storetest.Main(m, []string{"HANDLINE_AGENT", "HANDLINE_TERMINAL"}, nil) }

type rig struct {
	t       *testing.T
	home    string
	db      *sql.DB
	env     *Env
	room    string
	mode    string
	present map[string]bool
	idle    map[string]bool
	closed  []string
	handed  []string
	// handOffErr fails the next handoffs.
	handOffErr error
}

func newRig(t *testing.T) *rig {
	t.Helper()
	home := t.TempDir()
	d := teamtest.Seed(t, home, "team", "team.ui", "team.lander")
	teamtest.Set(t, home, "team.ui", "model", "opus", "effort", "high", "sandbox_allow", "pnpm-install")
	r := &rig{t: t, home: home, db: d, present: map[string]bool{}, idle: map[string]bool{}}
	clock := time.Unix(1_000_000, 0)
	r.env = &Env{DB: d, Home: home, Now: func() time.Time { return clock },
		HandOff: func(tid int64, agent, _ string) error {
			r.handed = append(r.handed, agent)
			if r.handOffErr != nil {
				return r.handOffErr
			}
			_, err := d.Exec("UPDATE tasks SET owner = ? WHERE id = ?", agent, tid)
			return err
		},
		Mode:    func() string { return r.mode },
		Room:    func(string) string { return r.room },
		Session: func(a string) (bool, bool) { return r.present[a], r.idle[a] },
		Close: func(a string) (bool, string) {
			r.closed = append(r.closed, a)
			r.present[a] = false
			return true, ""
		}}
	return r
}

func (r *rig) key(name string) string { return teamtest.Key(r.t, r.home, name) }

func (r *rig) must(err error) {
	r.t.Helper()
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) agents() map[string]bool {
	r.t.Helper()
	out := map[string]bool{}
	for _, k := range paths.ListAgents(r.home) {
		n, err := paths.AgentNameIn(r.db, k)
		r.must(err)
		out[n] = true
	}
	return out
}

// task is a task the lander queued for agent.
func (r *rig) task(agent string) int64 {
	r.t.Helper()
	lander := r.key("team.lander")
	tid, err := db.NewTask(r.db, "a typo", agent, lander, 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	_, err = db.SetTaskQueued(r.db, tid, lander)
	r.must(err)
	return tid
}

func (r *rig) hire(agent string) (seat string, tid int64) {
	r.t.Helper()
	tid = r.task(agent)
	if seat = r.env.Hire(agent, tid, "note"); seat == "" {
		r.t.Fatal("no seat took it")
	}
	return seat, tid
}

// events is the payloads of agent's events of kind, oldest first, without spaces.
func (r *rig) events(agent, kind string) []string {
	r.t.Helper()
	rows, err := r.db.Query("SELECT payload FROM events WHERE agent = ? AND kind = ? ORDER BY id", agent, kind)
	r.must(err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		r.must(rows.Scan(&p))
		out = append(out, strings.ReplaceAll(p, " ", ""))
	}
	return out
}

// over ends seat's task and lets it go: done, session closed at its prompt, and the sweep after.
func (r *rig) over(seat string, tid int64) {
	r.t.Helper()
	_, err := r.db.Exec("UPDATE tasks SET done_evidence = 'fixed', done_at = 1 WHERE id = ?", tid)
	r.must(err)
	r.idle[seat] = true
	r.must(r.env.Sweep())
	r.must(r.env.Sweep())
}

// A seat is the agent's name and the lowest free number, with the agent's settings; its row names the agent and the task it is at work
// on, and the hire is a line in the agent's chat. A second task while the first seat is at work takes a new seat.
func TestAHireTakesASeatWithTheAgentsSettings(t *testing.T) {
	r := newRig(t)
	ui := r.key("team.ui")
	seat, tid := r.hire(ui)
	if name, _ := paths.AgentNameIn(r.db, seat); name != "team.ui-2" {
		t.Fatalf("named %q", name)
	}
	if from, _ := paths.HiredFrom(r.db, seat); from != ui {
		t.Fatalf("hired from %q", from)
	}
	if at, _ := team.HiredFor(r.db, seat); at != tid || mustOwner(t, r.db, tid) != seat {
		t.Fatalf("at work on T%d, T%d is %s's", at, tid, mustOwner(t, r.db, tid))
	}
	cfg, err := team.NewRoster(r.home).Read(seat)
	r.must(err)
	if cfg["model"] != "opus" || cfg["effort"] != "high" || cfg["sandbox_allow"] != "pnpm-install" {
		t.Fatalf("settings %v", cfg)
	}
	if got := r.events(ui, HiredEvent); len(got) != 1 || got[0] != `{"seat":"`+seat+`","task_id":`+itoa(tid)+`}` {
		t.Fatalf("hire lines in ui's chat: %v", got)
	}
	r.present[seat] = true
	// a seat's next task is a seat of the agent, the next number while the first is at work
	if next, _ := r.hire(seat); next == seat {
		t.Fatal("a seat at work took a second task")
	} else if name, _ := paths.AgentNameIn(r.db, next); name != "team.ui-3" {
		t.Fatalf("second named %q", name)
	} else if from, _ := paths.HiredFrom(r.db, next); from != ui {
		t.Fatalf("second hired from %q", from)
	}
}

// With no room to start one, with temps Manual, or when the handoff fails, no seat is at work and nothing else is left behind.
func TestNoRoomManualOrAFailedHandoffPutsNoSeatToWork(t *testing.T) {
	r := newRig(t)
	ui := r.key("team.ui")
	before := len(r.agents())
	var asked []string
	r.env.Room = func(a string) string { asked = append(asked, a); return r.room }
	tid := r.task(ui)
	r.room = "no account can take one more agent"
	if seat := r.env.Hire(ui, tid, ""); seat != "" {
		t.Fatalf("hired %s with no room", seat)
	}
	r.room, r.mode = "", Manual
	if seat := r.env.Hire(ui, tid, ""); seat != "" {
		t.Fatalf("hired %s on manual", seat)
	}
	if after := len(r.agents()); after != before {
		t.Fatalf("%d agents, were %d", after, before)
	}
	if len(asked) != 1 || asked[0] != ui {
		t.Fatalf("room asked for %v, want the agent the seat would be of", asked)
	}
	r.mode, r.handOffErr = "", errors.New("refused")
	if seat := r.env.Hire(ui, tid, ""); seat != "" {
		t.Fatalf("a failed handoff left %s at work", seat)
	}
	seats, err := team.TempsOf(r.db, ui)
	r.must(err)
	for _, s := range seats {
		if at, _ := team.HiredFor(r.db, s); at != 0 {
			t.Fatalf("%s at work on T%d", s, at)
		}
	}
	if len(r.events(ui, HiredEvent)) != 0 || mustOwner(t, r.db, tid) != ui {
		t.Fatalf("a failed hire left a line or moved T%d", tid)
	}
}

// Its task over, a seat's session is closed at its prompt; then it gives its desk back and sits idle, its row kept: the leave is a line
// in the agent's chat that says how the task ended, a word to it reaches the agent, and the agent's next task takes it, on a fresh
// conversation with no handoff.
func TestASeatStaysIdleOnceItsTaskIsOverAndTheNextHireTakesItFresh(t *testing.T) {
	r := newRig(t)
	ui := r.key("team.ui")
	seat, tid := r.hire(ui)
	r.present[seat] = true
	r.must(r.env.Sweep())
	if at, _ := team.HiredFor(r.db, seat); at != tid {
		t.Fatal("let go while its task is open")
	}
	_, err := r.db.Exec("UPDATE tasks SET done_evidence = 'fixed', done_at = 1 WHERE id = ?", tid)
	r.must(err)
	r.must(r.env.Sweep())
	if len(r.closed) != 0 {
		t.Fatal("closed mid-turn")
	}
	if to, _ := paths.Reaches(r.db, seat); to != seat {
		t.Fatalf("a word to the seat at work reaches %s", to)
	}
	r.idle[seat] = true
	r.must(r.env.Sweep())
	if at, _ := team.HiredFor(r.db, seat); len(r.closed) != 1 || at != tid {
		t.Fatalf("closed %v, at work on T%d; it leaves once its session is gone", r.closed, at)
	}
	r.must(r.env.Sweep())
	if at, _ := team.HiredFor(r.db, seat); at != 0 || !r.agents()["team.ui-2"] {
		t.Fatalf("at work on T%d, or its row went", at)
	}
	if got := r.events(ui, LeftEvent); len(got) != 1 || got[0] != `{"seat":"`+seat+`","task_id":`+itoa(tid)+`,"ended":"done"}` {
		t.Fatalf("leave lines in ui's chat: %v", got)
	}
	if to, _ := paths.Reaches(r.db, seat); to != ui {
		t.Fatalf("a word to the idle seat reaches %s", to)
	}
	if to, _ := paths.Reaches(r.db, ui); to != ui {
		t.Fatalf("a word to the agent reaches %s", to)
	}
	r.must(r.env.Sweep())
	if len(r.events(ui, LeftEvent)) != 1 {
		t.Fatal("an idle seat left again")
	}
	// what its last task left: a conversation to resume and a handoff
	_, err = r.db.Exec("INSERT INTO agent_state(agent, state, updated, resume_session, handoff) VALUES(?, 'IDLE', 1, 's-old', 'old work')", seat)
	r.must(err)
	note := paths.HandoffNoteFile(r.home, seat)
	r.must(os.MkdirAll(filepath.Dir(note), 0o755))
	r.must(os.WriteFile(note, []byte("old work"), 0o644))
	next, tid2 := r.hire(ui)
	if next != seat {
		t.Fatalf("the next task took %s, not the idle seat %s", next, seat)
	}
	row, err := db.GetAgentState(r.db, seat)
	r.must(err)
	if row.ResumeSession.String != "" {
		t.Fatalf("resumes %q", row.ResumeSession.String)
	}
	if h, _ := db.GetHandoff(r.db, seat); h != "" {
		t.Fatalf("carries the handoff %q", h)
	}
	if _, err := os.Stat(note); !os.IsNotExist(err) {
		t.Fatal("its handoff note is still there")
	}
	// with both at work and then both idle, the lowest takes the next
	r.present[seat] = true
	third, tid3 := r.hire(ui)
	r.over(seat, tid2)
	r.over(third, tid3)
	if again, _ := r.hire(ui); again != seat {
		t.Fatalf("took %s, not the lowest idle seat %s", again, seat)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func mustOwner(t *testing.T, q db.Q, tid int64) string {
	t.Helper()
	var owner string
	if err := q.QueryRow("SELECT owner FROM tasks WHERE id = ?", tid).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	return owner
}

// A seat's task with someone else's turn on top (the founder's, a checker's) is still its work: it does not leave.
func TestASeatWhoseTaskWaitsOnATurnStaysAtWork(t *testing.T) {
	r := newRig(t)
	ui, lander := r.key("team.ui"), r.key("team.lander")
	seat, tid := r.hire(ui)
	r.must(db.PushTurn(r.db, tid, db.Turn{Agent: lander, By: seat, Since: 1}))
	r.present[seat] = true
	r.must(r.env.Sweep())
	r.present[seat] = false
	r.must(r.env.Sweep())
	if at, _ := team.HiredFor(r.db, seat); at != tid {
		t.Fatal("left while its task waits on a turn")
	}
}

// A seat that never started gives its task back once the agent is free, and then is idle, its leave saying the task was handed on; one
// that began keeps it.
func TestASeatThatNeverStartedGivesItsTaskBack(t *testing.T) {
	r := newRig(t)
	ui := r.key("team.ui")
	seat, tid := r.hire(ui)
	r.handed = nil
	r.must(r.env.Sweep())
	if len(r.handed) != 1 || r.handed[0] != ui || mustOwner(t, r.db, tid) != ui {
		t.Fatalf("given back to %v", r.handed)
	}
	r.must(r.env.Sweep())
	if at, _ := team.HiredFor(r.db, seat); at != 0 {
		t.Fatal("still at work with nothing to do")
	}
	if got := r.events(ui, LeftEvent); len(got) != 1 || !strings.Contains(got[0], `"ended":"handed_off"`) {
		t.Fatalf("leave lines %v", got)
	}
	began, tid2 := r.hire(ui)
	_, err := r.db.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(1, ?, 'task_begin', json_object('task_id', ?))", began, tid2)
	r.must(err)
	r.handed = nil
	r.must(r.env.Sweep())
	if begun, _ := db.Begun(r.db, tid2); !begun || len(r.handed) != 0 {
		t.Fatalf("begun %v, given back to %v", begun, r.handed)
	}
}

// Waiting is a seat at work with no session yet: another is not hired meanwhile.
func TestASeatAtWorkWithNoSessionIsWaiting(t *testing.T) {
	r := newRig(t)
	ui := r.key("team.ui")
	seat, _ := r.hire(ui)
	if !r.env.Waiting(ui) {
		t.Fatal("a seat at work with no session does not read as waiting")
	}
	r.present[seat] = true
	if r.env.Waiting(ui) {
		t.Fatal("a seat with a session reads as waiting to start")
	}
}

// A task that comes back to an idle seat, reopened, goes to the agent it is of.
func TestATaskReopenedOnAnIdleSeatGoesToItsAgent(t *testing.T) {
	r := newRig(t)
	ui := r.key("team.ui")
	seat, tid := r.hire(ui)
	r.over(seat, tid)
	_, err := r.db.Exec("UPDATE tasks SET done_evidence = NULL, done_at = NULL WHERE id = ?", tid)
	r.must(err)
	r.handed = nil
	r.must(r.env.Sweep())
	if len(r.handed) != 1 || r.handed[0] != ui || mustOwner(t, r.db, tid) != ui {
		t.Fatalf("given to %v", r.handed)
	}
	if at, _ := team.HiredFor(r.db, seat); at != 0 {
		t.Fatal("an idle seat went to work without a hire")
	}
}
