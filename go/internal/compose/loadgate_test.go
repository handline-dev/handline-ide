package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/internal/housekeeping"
	"github.com/handline/handline/go/scheduler/pause"
	"github.com/handline/handline/go/scheduler/sysres"
	"github.com/handline/handline/go/taskboard/supervisor"
)

func TestForceStartPassesOnlyWorkTheLoadHolds(t *testing.T) {
	machine := &sysres.Machine{Cores: 4, Load1: 2, RAMTotal: 100, RAMFree: 50}
	p, home := rigWith(t, func(c *Config) { c.Machine = func() *sysres.Machine { return machine } })
	lc := p.Lifecycle(cfgOf(p), LifecycleConfig{Env: &supervisor.Env{DB: p.db, Home: home, Log: func(string, ...any) {}}})
	c := controls{p: p, lc: lc}
	os.WriteFile(filepath.Join(home, "config.yaml"), []byte("load_per_core_pct: 150\n"), 0o644)
	db.SetAgentState(p.db, "alice", states.Running, 0, "") // one mid-turn, so a start may be held
	p.db.Exec("UPDATE agent_state SET account = 'a' WHERE agent = 'alice'")
	db.AddPending(p.db, "bob", "job", 1)
	if ok, said, _ := c.ForceStart("bob"); ok || said != "Nothing holds bob back: it starts by itself." {
		t.Fatalf("under the limits: %v %q", ok, said)
	}
	machine.Load1 = 8 // 2.00 per core
	if lc.Gate.Held("bob") == "" {
		t.Fatal("over the load limit and not held")
	}
	if ok, said, _ := c.ForceStart("carol"); ok || said != "Nothing to start: carol has no work waiting." {
		t.Fatalf("nothing waits for carol: %v %q", ok, said)
	}
	ok, said, err := c.ForceStart("bob")
	if err != nil || !ok || said != "Starting bob now, even though the computer is busy right now." {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if lc.Gate.Held("bob") != "" || lc.Gate.Held("carol") == "" {
		t.Fatal("the pass is bob's alone")
	}
	var n int
	p.db.QueryRow("SELECT COUNT(*) FROM events WHERE agent = 'bob' AND kind = 'load_limit_passed'").Scan(&n)
	if n != 1 {
		t.Fatalf("the force start is not on the timeline: %d", n)
	}
}

func TestTheDiskWarningFloorStaysWhileTheHoldIsOff(t *testing.T) {
	if got := diskWarnFloor(0); got != housekeeping.DiskLow {
		t.Errorf("hold off: warns under %d, want %d", got, housekeeping.DiskLow)
	}
	if got := diskWarnFloor(20); got != 20*housekeeping.GB {
		t.Errorf("hold set: warns under %d", got)
	}
}

// The founder's pause, kept in the database, holds starts and the supervisor's messages through the gate, and a force start does not pass it.
func TestThePauseHoldsThroughTheGateAndAForceStartDoesNotPassIt(t *testing.T) {
	p, home := rigWith(t, func(*Config) {})
	lc := p.Lifecycle(cfgOf(p), LifecycleConfig{Env: &supervisor.Env{DB: p.db, Home: home, Log: func(string, ...any) {}}})
	c := controls{p: p, lc: lc}
	db.AddPending(p.db, "bob", "job", 1)
	if lc.Gate.Held("bob") != "" || lc.Env.Paused() != "" {
		t.Fatal("held with no pause")
	}
	if _, err := pause.Set(p.db, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	if !pause.IsPaused(lc.Gate.Held("bob")) || !pause.IsPaused(lc.Env.Paused()) {
		t.Fatalf("the pause does not hold: %q %q", lc.Gate.Held("bob"), lc.Env.Paused())
	}
	if ok, said, _ := c.ForceStart("bob"); ok || !strings.Contains(said, "does not pass it") {
		t.Fatalf("a force start past the pause: %v %q", ok, said)
	}
	var n int
	p.db.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'load_limit_passed'").Scan(&n)
	if n != 0 {
		t.Fatal("a refused force start recorded a pass")
	}
	pause.Clear(p.db)
	if lc.Gate.Held("bob") != "" {
		t.Fatal("held after the pause ended")
	}
}
