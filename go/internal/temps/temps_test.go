package temps_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/proposalwrites"
	. "github.com/handline/handline/go/internal/temps"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
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
}

func newRig(t *testing.T) *rig {
	t.Helper()
	home := t.TempDir()
	d := teamtest.Seed(t, home, "team", "team.ui", "team.lander")
	teamtest.Set(t, home, "team.ui", "model", "opus", "effort", "high", "sandbox_allow", "pnpm-install")
	r := &rig{t: t, home: home, db: d, present: map[string]bool{}, idle: map[string]bool{}}
	clock := time.Unix(1_000_000, 0)
	retire := &proposalwrites.Env{DB: d, Home: home}
	r.env = &Env{DB: d, Now: func() time.Time { return clock }, Dismiss: retire.Dismiss,
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

// A temp is the agent's name and the lowest number no agent or temp had, with the agent's settings, and its row names the agent.
func TestAHireCopiesTheAgentsSettingsUnderANumberedName(t *testing.T) {
	r := newRig(t)
	ui := r.key("team.ui")
	temp := r.env.Hire(ui)
	if temp == "" {
		t.Fatal("no temp hired")
	}
	if name, _ := paths.AgentNameIn(r.db, temp); name != "team.ui-2" {
		t.Fatalf("named %q", name)
	}
	if from, _ := paths.HiredFrom(r.db, temp); from != ui {
		t.Fatalf("hired from %q", from)
	}
	cfg, err := team.NewRoster(r.home).Read(temp)
	r.must(err)
	if cfg["model"] != "opus" || cfg["effort"] != "high" || cfg["sandbox_allow"] != "pnpm-install" {
		t.Fatalf("settings %v", cfg)
	}
	var n int
	r.must(r.db.QueryRow("SELECT COUNT(*) FROM events WHERE agent = ? AND kind = ?", temp, HiredEvent).Scan(&n))
	if n != 1 {
		t.Fatalf("%d hire events", n)
	}
	// a temp of a temp is the agent's next one
	if next := r.env.Hire(temp); next == "" {
		t.Fatal("no second temp")
	} else if name, _ := paths.AgentNameIn(r.db, next); name != "team.ui-3" {
		t.Fatalf("second named %q", name)
	} else if from, _ := paths.HiredFrom(r.db, next); from != ui {
		t.Fatalf("second hired from %q", from)
	}
}

// With no room to start one, or with temps Manual, no temp is hired and nothing is left behind.
func TestNoRoomOrManualHiresNobody(t *testing.T) {
	r := newRig(t)
	before := len(r.agents())
	var asked []string
	r.env.Room = func(a string) string { asked = append(asked, a); return r.room }
	seq := func() (n int64) {
		r.must(r.db.QueryRow("SELECT COALESCE(MAX(seq), 0) FROM sqlite_sequence WHERE name = 'agents'").Scan(&n))
		return n
	}
	ids := seq()
	r.room = "no account can take one more agent"
	if temp := r.env.Hire(r.key("team.ui")); temp != "" {
		t.Fatalf("hired %s with no room", temp)
	}
	r.room, r.mode = "", Manual
	if temp := r.env.Hire(r.key("team.ui")); temp != "" {
		t.Fatalf("hired %s on manual", temp)
	}
	if after := len(r.agents()); after != before || seq() != ids {
		t.Fatalf("%d agents, were %d; a row was written and taken back", after, before)
	}
	if len(asked) != 1 || asked[0] != r.key("team.ui") {
		t.Fatalf("room asked for %v, want the agent the temp would be hired from", asked)
	}
}

// Its task over, the temp's session is closed at its prompt; then it leaves. What still names it (the task's owner, a handoff back
// by its name) reaches the agent it was hired from, and its name is never another temp's.
func TestATempLeavesOnceItsTaskIsOverAndWhatNamesItReachesTheAgent(t *testing.T) {
	r := newRig(t)
	ui, lander := r.key("team.ui"), r.key("team.lander")
	temp := r.env.Hire(ui)
	tid, err := db.NewTask(r.db, "a typo", temp, lander, 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	_, err = db.SetTaskQueued(r.db, tid, lander)
	r.must(err)
	r.present[temp], r.idle[temp] = true, false

	r.must(r.env.Sweep())
	if !r.agents()["team.ui-2"] {
		t.Fatal("left while its task is open")
	}
	_, err = r.db.Exec("UPDATE tasks SET done_evidence = 'fixed', done_at = 1 WHERE id = ?", tid)
	r.must(err)
	r.must(r.env.Sweep())
	if len(r.closed) != 0 || !r.agents()["team.ui-2"] {
		t.Fatal("left mid-turn")
	}
	r.idle[temp] = true
	r.must(r.env.Sweep())
	if len(r.closed) != 1 || !r.agents()["team.ui-2"] {
		t.Fatalf("closed %v; it leaves once its session is gone", r.closed)
	}
	r.must(r.env.Sweep())
	if r.agents()["team.ui-2"] {
		t.Fatal("still on the roster")
	}
	if _, err := os.Stat(filepath.Join(r.home, "agents", temp)); err == nil {
		t.Fatal("its folder was not archived")
	}
	if owner := mustOwner(t, r.db, tid); owner != ui {
		t.Fatalf("T%d is %s's", tid, owner)
	}
	_, lid, _ := actor.Parse(lander)
	var ws int64
	r.must(r.db.QueryRow("SELECT workspace FROM agents WHERE id = ?", lid).Scan(&ws))
	for _, word := range []string{"ui-2", "team/ui-2", temp} {
		if got, err := addr.Resolve(r.db, addr.Who{Key: lander, Workspace: ws}, word); err != nil || got != ui {
			t.Fatalf("%s resolves to %q %v", word, got, err)
		}
	}
	if next := r.env.Hire(ui); next == "" {
		t.Fatal("no next temp")
	} else if name, _ := paths.AgentNameIn(r.db, next); name != "team.ui-3" {
		t.Fatalf("the next temp is %q", name)
	}
	var n int
	r.must(r.db.QueryRow("SELECT COUNT(*) FROM events WHERE agent = ? AND kind = ?", temp, LeftEvent).Scan(&n))
	if n != 1 {
		t.Fatalf("%d leave events", n)
	}
}

func mustOwner(t *testing.T, q db.Q, tid int64) string {
	t.Helper()
	var owner string
	if err := q.QueryRow("SELECT owner FROM tasks WHERE id = ?", tid).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	return owner
}

// A task of the temp's with someone else's turn on top (the founder's, a checker's) is still its work: it does not leave.
func TestATempWhoseTaskWaitsOnATurnStays(t *testing.T) {
	r := newRig(t)
	ui, lander := r.key("team.ui"), r.key("team.lander")
	temp := r.env.Hire(ui)
	tid, err := db.NewTask(r.db, "a typo", temp, lander, 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	_, err = db.SetTaskQueued(r.db, tid, lander)
	r.must(err)
	r.must(db.PushTurn(r.db, tid, db.Turn{Agent: lander, By: temp, Since: 1}))
	r.must(r.env.Sweep())
	r.must(r.env.Sweep())
	if !r.agents()["team.ui-2"] {
		t.Fatal("left while its task waits on a turn")
	}
}

// A temp that never started gives its task back once the agent it was hired from is free, and then leaves; one that began keeps it.
func TestATempThatNeverStartedGivesItsTaskBack(t *testing.T) {
	r := newRig(t)
	ui, lander := r.key("team.ui"), r.key("team.lander")
	var back []string
	r.env.HandBack = func(tid int64, agent, _ string) error {
		back = append(back, agent)
		_, err := r.db.Exec("UPDATE tasks SET owner = ? WHERE id = ?", agent, tid)
		return err
	}
	temp := r.env.Hire(ui)
	tid, err := db.NewTask(r.db, "a typo", temp, lander, 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	_, err = db.SetTaskQueued(r.db, tid, lander)
	r.must(err)
	r.must(r.env.Sweep())
	if len(back) != 1 || back[0] != ui {
		t.Fatalf("given back to %v", back)
	}
	r.must(r.env.Sweep())
	if r.agents()["team.ui-2"] {
		t.Fatal("stayed with nothing to do")
	}
}

// An agent with a temp at work is not removed: its temps would be left with nobody to report to.
func TestAnAgentWithATempIsNotRetired(t *testing.T) {
	r := newRig(t)
	ui := r.key("team.ui")
	temp := r.env.Hire(ui)
	r.present[temp] = true
	if r.env.Waiting(ui) {
		t.Fatal("a temp with a session reads as waiting to start")
	}
	retire := &proposalwrites.Env{DB: r.db, Home: r.home}
	refused, err := retire.Dismiss(ui)
	r.must(err)
	if refused == "" || !r.agents()["team.ui"] {
		t.Fatalf("ui retired with its temp at work: %q", refused)
	}
}
