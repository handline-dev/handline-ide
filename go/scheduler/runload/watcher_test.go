package runload

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/session"
	"github.com/handline/handline/go/scheduler/sysres"
)

type rig struct {
	t     *testing.T
	w     *Watcher
	q     *sql.DB
	home  string
	now   time.Time
	cfg   Config
	procs []procload.Proc
	low   []int
	slept time.Duration // what the next look finds the machine slept since the one before
}

// newRig is a watcher on a scratch home whose check.sh is a heavy shape that usually takes 300 s, on a 12-core machine.
func newRig(t *testing.T) *rig {
	r := &rig{t: t, q: storetest.Open(t), home: t.TempDir(), now: time.Unix(1_700_000_000, 0), cfg: Config{Policy: Observe}}
	day := r.now.Unix() / (24 * 3600)
	r.exec("INSERT INTO shape_days(shape, day, runs, long) VALUES('check.sh', ?, 10, 10)", day)
	for i := 0; i < 3; i++ {
		r.exec("INSERT INTO heavy_runs(agent, ended, seconds, shape) VALUES('a1', ?, 300, 'check.sh')", r.now.Unix()-1000)
	}
	r.w = r.watcher()
	return r
}

func (r *rig) watcher() *Watcher {
	return &Watcher{Home: r.home, DB: r.q, Config: func() Config { return r.cfg }, Now: func() time.Time { return r.now },
		Machine:  func() *sysres.Machine { return &sysres.Machine{Cores: 12, Load1: 24} },
		Procs:    func() ([]procload.Proc, bool) { return r.procs, true },
		ReadHost: func() (int, int64, bool) { return 2, r.now.Unix() * 1000, true },
		Lower:    func(pid int, _ int64) error { r.low = append(r.low, pid); return nil },
		Slept:    func(time.Time, time.Time) time.Duration { d := r.slept; r.slept = 0; return d }}
}

func (r *rig) exec(query string, args ...any) {
	r.t.Helper()
	if _, err := r.q.Exec(query, args...); err != nil {
		r.t.Fatal(err)
	}
}

func call(id, command string) map[string]any {
	return map[string]any{"tool_name": "Bash", "tool_use_id": id, "tool_input": map[string]any{"command": command, "timeout": 600000.0}}
}

// start begins a check.sh call of agent, as its PreToolUse hook does once it is let through.
func (r *rig) start(agent, id string) {
	session.NoteBashStart(r.home, agent, call(id, "tools/check.sh"), r.now)
}

// end is the call's PostToolUse.
func (r *rig) end(agent, id string) {
	session.EndedCall(r.home, agent, "PostToolUse", call(id, "tools/check.sh"), r.now)
}

func (r *rig) pass(d time.Duration) { r.now = r.now.Add(d) }

// look passes d and looks, as the watcher's loop does.
func (r *rig) look(d time.Duration) {
	r.pass(d)
	r.w.Look()
}

// Two runs late for half a minute make the machine slow; it is kept across a restart, and nothing is held or lowered under Observe.
func TestTheWatcherCallsTheMachineSlowWhenRunsAreLateAndKeepsItAcrossARestart(t *testing.T) {
	r := newRig(t)
	r.start("a1", "one")
	r.start("a2", "two")
	r.start("a3", "three")
	r.look(0)
	if v := r.w.View(); v.State.Slow || len(v.Runs) != 3 || v.Signals.Heavy != 3 || v.Signals.Late != 0 || v.Runs[0].UsualS != 300 {
		t.Fatalf("at the start: %+v", v)
	}
	r.look(451 * time.Second)
	if v := r.w.View(); v.State.Slow || v.Signals.Late != 3 {
		t.Fatalf("late on one look: %+v", v)
	}
	r.look(Every * time.Second)
	v := r.w.View()
	if !v.State.Slow || v.State.Cap != 3 || v.Cap != 0 || !reflect.DeepEqual(v.State.Causes, []string{CauseRuns}) {
		t.Fatalf("after two looks: %+v", v)
	}
	if v.Host.Pressure != "warn" || v.Host.LoadPerCore != 2 || v.Host.CompressedPerS != 1000 {
		t.Errorf("host: %+v", v.Host)
	}
	if a := r.w.Admit("a4", "four", "check.sh"); !a.Go {
		t.Errorf("held under observe: %+v", a)
	}
	if len(r.low) != 0 {
		t.Errorf("lowered under observe: %v", r.low)
	}
	kept, _, _ := db.Meta(r.q, metaKey)
	if kept == "" {
		t.Fatal("the state was not kept")
	}
	again := r.watcher()
	if v := again.View(); !v.State.Slow || v.State.Cap != 3 || v.Looked {
		t.Errorf("after a restart: %+v", v)
	}
}

// A run that hit its own time limit twice in five minutes is a cause by itself, and so is the daemon's own loop taking long, even
// one that has not ended yet.
func TestTimeoutsAndASlowLoopAreCauses(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 2; i++ {
		r.exec("INSERT INTO heavy_runs(agent, ended, seconds, shape, timed_out) VALUES('a1', ?, 600, 'check.sh', 1)", r.now.Unix()-10)
	}
	r.look(0)
	r.look(Every * time.Second)
	if v := r.w.View(); !v.State.Slow || !reflect.DeepEqual(v.State.Causes, []string{CauseTimeouts}) || v.Signals.TimedOut != 2 {
		t.Fatalf("timeouts: %+v", v)
	}
	r = newRig(t)
	r.w.LoopBegan()
	r.w.LoopEnded(2 * time.Second)
	r.look(0)
	r.w.LoopBegan() // and this one does not come back
	r.look(31 * time.Second)
	r.look(Every * time.Second)
	if v := r.w.View(); !v.State.Slow || !reflect.DeepEqual(v.State.Causes, []string{CauseLoop}) || v.Signals.LoopMs < LoopSlowMs {
		t.Fatalf("a loop still running: %+v", v)
	}
	r = newRig(t)
	r.w.LoopEnded(40 * time.Second)
	r.look(loopFresh + time.Second)
	if v := r.w.View(); v.Signals.LoopMs != 0 {
		t.Errorf("a loop that stopped reporting still counts: %+v", v.Signals)
	}
}

// Under Fixed, heavy runs past the count wait their turn in the order they asked, and go as the runs before them end.
func TestHeavyRunsTakeTurnsFirstComeFirstServed(t *testing.T) {
	r := newRig(t)
	r.cfg = Config{Policy: Fixed, AtOnce: 2}
	r.look(0)
	admit := func(agent, id string) Answer {
		a := r.w.Admit(agent, id, "check.sh")
		if a.Go {
			r.start(agent, id)
		}
		return a
	}
	if a := r.w.Admit("a9", "light", "git status"); !a.Go {
		t.Fatalf("a shape that is not heavy waited: %+v", a)
	}
	if a, b := admit("a1", "one"), admit("a2", "two"); !a.Go || !b.Go {
		t.Fatalf("the first two: %+v %+v", a, b)
	}
	if a := admit("a3", "three"); a.Go || a.Ahead != 0 || a.Cap != 2 {
		t.Fatalf("the third: %+v", a)
	}
	if a := admit("a4", "four"); a.Go || a.Ahead != 1 {
		t.Fatalf("the fourth: %+v", a)
	}
	if v := r.w.View(); len(v.Waiting) != 2 || v.Waiting[0].Agent != "a3" || v.Waiting[1].Agent != "a4" || v.Cap != 2 {
		t.Fatalf("waiting: %+v", v)
	}
	r.pass(AskEvery)
	if a, b := admit("a3", "three"), admit("a4", "four"); a.Go || b.Go { // both runs are seen going
		t.Fatalf("while two run: %+v %+v", a, b)
	}
	r.pass(AskEvery)
	r.end("a1", "one")
	if a := admit("a4", "four"); a.Go {
		t.Fatalf("the fourth went before the third: %+v", a)
	}
	if a := admit("a3", "three"); !a.Go || a.WaitedS != 6 {
		t.Fatalf("the third after one ended: %+v", a)
	}
	r.pass(AskEvery)
	if a := admit("a4", "four"); a.Go {
		t.Fatalf("the fourth while two still run: %+v", a)
	}
	r.pass(AskEvery)
	r.end("a2", "two")
	if a := admit("a4", "four"); !a.Go {
		t.Fatalf("the fourth after the second ended: %+v", a)
	}
	if v := r.w.View(); len(v.Waiting) != 0 {
		t.Errorf("still waiting: %+v", v.Waiting)
	}
}

// A turn given counts as a run at once, before its start note is read, so two asks at the same moment do not both take the last one.
func TestATurnGivenCountsBeforeItsRunIsSeen(t *testing.T) {
	r := newRig(t)
	r.cfg = Config{Policy: Fixed, AtOnce: 1}
	r.look(0)
	if a := r.w.Admit("a1", "one", "check.sh"); !a.Go {
		t.Fatalf("%+v", a)
	}
	if a := r.w.Admit("a2", "two", "check.sh"); a.Go {
		t.Fatalf("both took the one turn: %+v", a)
	}
	r.pass(grantFor + time.Second) // the first never started (its hook died): its turn is free again
	if a := r.w.Admit("a2", "two", "check.sh"); !a.Go {
		t.Fatalf("a turn never used was kept: %+v", a)
	}
}

// An ask not asked again gives up its place, and no run waits past MaxWait.
func TestAnAskGivesUpItsPlaceAndNothingWaitsPastMaxWait(t *testing.T) {
	r := newRig(t)
	r.cfg = Config{Policy: Fixed, AtOnce: 1}
	r.look(0)
	r.start("a1", "one")
	r.w.Admit("a1", "one", "check.sh")
	if a := r.w.Admit("a2", "two", "check.sh"); a.Go {
		t.Fatalf("%+v", a)
	}
	r.pass(askFor + time.Second)
	if v := r.w.View(); len(v.Waiting) != 0 {
		t.Fatalf("an ask whose hook went away still waits: %+v", v.Waiting)
	}
	if a := r.w.Admit("a3", "three", "check.sh"); a.Go || a.Ahead != 0 {
		t.Fatalf("the next ask is first in line: %+v", a)
	}
	for waited := time.Duration(0); waited < MaxWait-AskEvery; waited += AskEvery {
		r.pass(AskEvery)
		session.NoteBashStart(r.home, "a1", call("one", "tools/check.sh"), r.now) // the first run goes on and on
		if a := r.w.Admit("a3", "three", "check.sh"); a.Go {
			t.Fatalf("went after %s: %+v", waited, a)
		}
	}
	r.pass(AskEvery)
	if a := r.w.Admit("a3", "three", "check.sh"); !a.Go || a.WaitedS != int64(MaxWait.Seconds()) {
		t.Fatalf("still waiting at MaxWait: %+v", a)
	}
}

// Under Turns nothing waits until the machine is slow; then new heavy runs wait under the cap, and the runs' processes are lowered,
// each once.
func TestUnderTurnsRunsWaitOnlyWhileTheCapStandsAndTheirProcessesAreLowered(t *testing.T) {
	r := newRig(t)
	r.cfg = Config{Policy: Turns}
	for i := 1; i <= 4; i++ {
		id := fmt.Sprintf("run%d", i)
		r.start(fmt.Sprintf("a%d", i), id)
		r.procs = append(r.procs, proc(100*i, 1, fmt.Sprintf("a%d", i), toolCmd, r.now.Unix(), 10, 10), proc(100*i+1, 100*i, fmt.Sprintf("a%d", i), "go test", r.now.Unix(), 10, 10))
	}
	r.look(0)
	if a := r.w.Admit("a5", "five", "check.sh"); !a.Go {
		t.Fatalf("held on a calm machine: %+v", a)
	}
	if len(r.low) != 0 {
		t.Fatalf("lowered on a calm machine: %v", r.low)
	}
	r.look(451 * time.Second)
	r.look(Every * time.Second)
	v := r.w.View()
	if !v.State.Slow || v.Cap != 3 {
		t.Fatalf("slow with four runs going: %+v", v)
	}
	if a := r.w.Admit("a6", "six", "check.sh"); a.Go || a.Cap != 3 {
		t.Fatalf("a new run on a slow machine: %+v", a)
	}
	r.look(Every * time.Second)
	if len(r.low) != 8 {
		t.Fatalf("each process of the four runs is lowered: %v", r.low)
	}
	r.look(Every * time.Second)
	if len(r.low) != 8 {
		t.Errorf("lowered again: %v", r.low)
	}
	if v := r.w.View(); !v.Runs[0].Found || v.Runs[0].Procs != 2 || v.Runs[0].CPU != 20 {
		t.Errorf("a run's processes: %+v", v.Runs[0])
	}
	// A run whose shell is gone has ended, though no hook said so, and stays ended while its note stands.
	r.procs = r.procs[2:]
	for i := 0; i < 2; i++ {
		r.look(Every * time.Second)
		if v := r.w.View(); len(v.Runs) != 3 || v.Runs[0].ID != "run2" {
			t.Fatalf("look %d after its shell went: %+v", i+1, v.Runs)
		}
	}
}

// Runs that were going while the machine slept have time on the clock that is not theirs: they are never late, so a wake does not
// call the machine slow. A run started after the wake is judged as any other.
func TestRunsGoingAcrossASleepAreNotLate(t *testing.T) {
	r := newRig(t)
	r.start("a1", "one")
	r.start("a2", "two")
	r.look(0)
	r.look(200 * time.Second)
	r.slept = 5 * time.Minute
	r.look(300 * time.Second)
	for i := 0; i < 3; i++ {
		if v := r.w.View(); v.State.Slow || v.Signals.Late != 0 || len(v.Runs) != 2 {
			t.Fatalf("look %d after the wake: %+v", i, v)
		}
		r.look(Every * time.Second)
	}
	// A short sleep (the lid closed and opened) sets nothing aside.
	r = newRig(t)
	r.start("a1", "one")
	r.start("a2", "two")
	r.look(0)
	r.slept = sleptMin - time.Second
	r.look(451 * time.Second)
	if v := r.w.View(); v.Signals.Late != 2 {
		t.Fatalf("after a short sleep: %+v", v.Signals)
	}
}

// The count of looks that showed a cause is not kept: a daemon that comes back sees its first look as the first.
func TestOneLookAfterARestartDoesNotTurnTheMachineSlow(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 2; i++ {
		r.exec("INSERT INTO heavy_runs(agent, ended, seconds, shape, timed_out) VALUES('a1', ?, 600, 'check.sh', 1)", r.now.Unix())
	}
	r.look(0) // the first look that shows the cause
	if v := r.w.View(); v.State.Slow || v.State.Signs != 1 {
		t.Fatalf("before the restart: %+v", v.State)
	}
	r.w = r.watcher()
	r.look(Every * time.Second)
	if v := r.w.View(); v.State.Slow {
		t.Fatalf("slow on the first look after a restart: %+v", v.State)
	}
	r.look(Every * time.Second)
	if v := r.w.View(); !v.State.Slow {
		t.Fatalf("not slow on the second: %+v", v.State)
	}
}

// A change of the policy lets every waiting run go at its next ask.
func TestAPolicyThatHoldsNothingLetsTheWaitingRunsGo(t *testing.T) {
	r := newRig(t)
	r.cfg = Config{Policy: Fixed, AtOnce: 1}
	r.look(0)
	r.start("a1", "one")
	r.w.Admit("a1", "one", "check.sh")
	if a := r.w.Admit("a2", "two", "check.sh"); a.Go {
		t.Fatalf("%+v", a)
	}
	r.cfg = Config{Policy: Observe}
	r.pass(AskEvery)
	if v := r.w.View(); v.Cap != 0 {
		t.Fatalf("the status still shows a cap: %+v", v)
	}
	if a := r.w.Admit("a2", "two", "check.sh"); !a.Go {
		t.Fatalf("still held under observe: %+v", a)
	}
	if v := r.w.View(); len(v.Waiting) != 0 {
		t.Errorf("still listed as waiting: %+v", v.Waiting)
	}
}

// run.load is null until the first look, then says the runs with what is not known as null.
func TestLoadIsNullBeforeTheFirstLook(t *testing.T) {
	r := newRig(t)
	if got := r.w.Load(); got != nil {
		t.Fatalf("%v", got)
	}
	r.start("a1", "one")
	r.look(0)
	got := r.w.Load()
	runs := got["runs"].([]any)
	row := runs[0].(map[string]any)
	if len(runs) != 1 || row["agent"] != "a1" || row["shape"] != "check.sh" || row["usual_s"] != int64(300) || row["cpu_pct"] != nil || row["heavy"] != true {
		t.Errorf("%+v", row)
	}
	if got["policy"] != "observe" || got["slow"] != false || got["since"] != nil || got["cap"] != nil || len(got["causes"].([]string)) != 0 {
		t.Errorf("%+v", got)
	}
}
