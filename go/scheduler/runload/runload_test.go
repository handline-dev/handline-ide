package runload

import (
	"reflect"
	"testing"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/session"
)

// A shape is heavy from its start once at least half of three or more runs were long; its usual time is the median of its recorded
// heavy runs.
func TestShapesAreHeavyWhenMostOfTheirRunsAreLong(t *testing.T) {
	q, now := storetest.Open(t), int64(1_700_000_000)
	day := now / (24 * 3600)
	for _, r := range []struct {
		shape      string
		day        int64
		runs, long int
	}{
		{"check.sh", day, 2, 2}, {"check.sh", day - 1, 2, 1}, // 3 of 4 long
		{"python3", day, 40, 3},   // long now and then: a script that happened to be slow
		{"new.sh", day, 2, 2},     // too few to say
		{"old.sh", day - 8, 9, 9}, // out of the week
		{"@build-test", day, 10, 5},
	} {
		if _, err := q.Exec("INSERT INTO shape_days(shape, day, runs, long) VALUES(?, ?, ?, ?)", r.shape, r.day, r.runs, r.long); err != nil {
			t.Fatal(err)
		}
	}
	for _, secs := range []int64{300, 400, 360, 9000} {
		if _, err := q.Exec("INSERT INTO heavy_runs(agent, ended, seconds, shape) VALUES('a1', ?, ?, 'check.sh')", now-100, secs); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := q.Exec("INSERT INTO heavy_runs(agent, ended, seconds, shape) VALUES('a1', ?, 70, '@build-test')", now-100); err != nil {
		t.Fatal(err)
	}
	s, err := ReadShapes(q, now)
	if err != nil {
		t.Fatal(err)
	}
	for shape, want := range map[string]bool{"check.sh": true, "python3": false, "new.sh": false, "old.sh": false, "@build-test": true, "never": false} {
		if s.Heavy(shape) != want {
			t.Errorf("Heavy(%s) = %v, want %v (%+v)", shape, !want, want, s[shape])
		}
	}
	if s.Usual("check.sh") != 380 || s.Usual("@build-test") != 70 || s.Usual("python3") != 0 {
		t.Errorf("usual: %+v", s)
	}
}

func heavyShapes() Shapes {
	return Shapes{"check.sh": {Runs: 10, Long: 9, UsualS: 300}, "git status": {Runs: 50, Long: 1}}
}

// A foreground call runs from its start note to its own limit; it is heavy by its shape from the start or by running a minute, and
// late past one and a half times its shape's usual time.
func TestActiveRunsAreTheOpenCallsWithinTheirLimit(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	start := func(id, shape string, ago, limit time.Duration) session.OpenStart {
		return session.OpenStart{Agent: "a1", ID: id, At: now.Add(-ago), Limit: limit, Shape: shape}
	}
	got := Active([]session.OpenStart{
		start("fresh", "check.sh", 10*time.Second, 10*time.Minute),
		start("late", "check.sh", 451*time.Second, 10*time.Minute),
		start("ontime", "check.sh", 450*time.Second, 10*time.Minute),
		start("quick", "git status", 5*time.Second, 0),
		start("long", "git status", 61*time.Second, 0),
		start("stale", "git status", DefaultLimit+limitGrace+time.Second, 0),
		start("stale-own-limit", "check.sh", 10*time.Minute+limitGrace+time.Second, 10*time.Minute),
		start("wait", "", 90*time.Second, 0),
		start("ended", "check.sh", 30*time.Second, 10*time.Minute),
	}, nil, heavyShapes(), map[string]bool{"ended": true}, now)
	type row struct{ heavy, late bool }
	want := map[string]row{"fresh": {true, false}, "late": {true, true}, "ontime": {true, false}, "quick": {false, false}, "long": {true, false}}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i, r := range got {
		if w, ok := want[r.ID]; !ok || w != (row{r.Heavy, r.Late}) {
			t.Errorf("%s: heavy %v late %v, want %+v", r.ID, r.Heavy, r.Late, w)
		}
		if i > 0 && got[i-1].Started > r.Started {
			t.Errorf("not oldest first: %+v", got)
		}
	}
}

// A background shell counts only when its shape is heavy, and only for a few times its usual run: a server left running is not a
// run forever.
func TestABackgroundShellCountsWhileItsShapeIsHeavyAndItIsNotTooOld(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	bg := func(job, shape string, ago int64) Background {
		return Background{Agent: "a1", Job: job, Shape: shape, Started: now.Unix() - ago}
	}
	got := Active(nil, []Background{bg("check", "check.sh", 500), bg("server", "git status", 500), bg("forgotten", "check.sh", 4*1800+1), bg("gone", "check.sh", 5)},
		heavyShapes(), map[string]bool{"gone": true}, now)
	if len(got) != 1 || got[0].ID != "check" || !got[0].Background || !got[0].Heavy || !got[0].Late || got[0].UsualS != 300 {
		t.Errorf("%+v", got)
	}
	// A shape whose runs are long counts for four of its usual runs, past the half hour.
	long := Shapes{"e2e.sh": {Runs: 5, Long: 5, UsualS: 900}}
	got = Active(nil, []Background{bg("going", "e2e.sh", 4*900), bg("left", "e2e.sh", 4*900+1)}, long, nil, now)
	if len(got) != 1 || got[0].ID != "going" {
		t.Errorf("a long shape: %+v", got)
	}
}

func proc(pid, ppid int, agent, cmd string, start int64, cpu float64, rss int64) procload.Proc {
	return procload.Proc{PID: pid, PPID: ppid, Agent: agent, Cmd: cmd, Start: start, CPU: cpu, RSS: rss}
}

const toolCmd = "/bin/zsh -c source /h/.claude/shell-snapshots/snapshot-zsh-1-x.sh 2>/dev/null || true"

// A run's processes are the tool shell of its agent that started nearest after it, and everything under that shell; a run whose
// shell was found and is gone has ended, and one never found is left running.
func TestMatchFindsEachRunsShellAndTellsWhenItIsGone(t *testing.T) {
	runs := func() []Run {
		return []Run{{Agent: "a1", ID: "first", Started: 1000, Heavy: true}, {Agent: "a1", ID: "second", Started: 1030, Heavy: false},
			{Agent: "a2", ID: "other", Started: 1000, Heavy: true}, {Agent: "a3", ID: "unfound", Started: 1000, Heavy: true}}
	}
	procs := []procload.Proc{
		proc(10, 1, "a1", toolCmd, 1001, 5, 100), proc(11, 10, "a1", "bash tools/check.sh", 1001, 50, 200), proc(12, 11, "a1", "go test ./...", 1002, 300, 4000),
		proc(20, 1, "a1", toolCmd, 1031, 1, 10),
		proc(21, 10, "a1", toolCmd, 1001, 1, 1), // the copy the first run's shell forked of itself: counted under it, never a shell
		proc(30, 1, "a2", toolCmd, 1002, 2, 20),
		proc(40, 1, "a2", "/bin/zsh -c ls", 1000, 9, 9),      // not a tool shell
		proc(50, 1, "a3", toolCmd, 1000+matchWithin+1, 1, 1), // started too long after
		proc(60, 1, "a3", toolCmd, 900, 1, 1),                // started before the run
	}
	found := Found{}
	rs := runs()
	ended, heavy := Match(rs, procs, found)
	if len(ended) != 0 {
		t.Fatalf("ended: %v", ended)
	}
	if r := rs[0]; !r.Found || r.Procs != 4 || r.CPU != 356 || r.RSS != 4301 {
		t.Errorf("first: %+v", r)
	}
	if r := rs[1]; !r.Found || r.Procs != 1 || r.RSS != 10 {
		t.Errorf("second: %+v", r)
	}
	if r := rs[2]; !r.Found || r.RSS != 20 {
		t.Errorf("other: %+v", r)
	}
	if rs[3].Found {
		t.Errorf("unfound: %+v", rs[3])
	}
	if found["first"] != (procKey{10, 1001}) || found["second"] != (procKey{20, 1031}) {
		t.Errorf("found: %v", found)
	}
	want := map[procKey]bool{{10, 1001}: true, {11, 1001}: true, {12, 1002}: true, {21, 1001}: true, {30, 1002}: true}
	if len(heavy) != len(want) {
		t.Fatalf("heavy: %v", heavy)
	}
	for _, k := range heavy {
		if !want[k] {
			t.Errorf("heavy has %v: the processes of a run that is not heavy are left alone", k)
		}
	}
	// The first run's shell ends with no hook telling; the unfound one stays.
	var left []procload.Proc
	for _, p := range procs {
		if p.PID != 10 {
			left = append(left, p)
		}
	}
	gone := proc(10, 1, "a1", toolCmd, 1001, 0, 0)
	gone.Gone = true
	rs = runs()
	ended, _ = Match(rs, append(left, gone), found)
	if !reflect.DeepEqual(ended, map[string]bool{"first": true}) {
		t.Errorf("ended: %v", ended)
	}
	if _, kept := found["first"]; kept {
		t.Error("an ended run's shell is still kept")
	}
	// A pid used again by a later process is not the run's shell.
	rs = []Run{{Agent: "a2", ID: "other", Started: 1000, Heavy: true}}
	ended, _ = Match(rs, []procload.Proc{proc(30, 1, "a2", toolCmd, 5000, 2, 20)}, found)
	if !ended["other"] {
		t.Errorf("a new process under the old pid kept the run alive: %v", ended)
	}
}

// Two calls of one agent too close together to tell apart are neither paired, so neither can be dropped when the other's shell
// ends; a start note that never got a shell does not take the next call's.
func TestMatchPairsNothingItCannotTellApart(t *testing.T) {
	runs := []Run{{Agent: "a1", ID: "x", Started: 1000, Heavy: true}, {Agent: "a1", ID: "y", Started: 1000, Heavy: true}}
	procs := []procload.Proc{proc(10, 1, "a1", toolCmd, 1000, 1, 1), proc(11, 1, "a1", toolCmd, 1000, 1, 1)}
	found := Found{}
	if ended, heavy := Match(runs, procs, found); len(found) != 0 || len(ended) != 0 || len(heavy) != 0 || runs[0].Found || runs[1].Found {
		t.Fatalf("two calls in the same second: found %v ended %v heavy %v", found, ended, heavy)
	}
	if ended, _ := Match(runs, procs[1:], found); len(ended) != 0 {
		t.Fatalf("one shell ending dropped a run it was never paired with: %v", ended)
	}
	// One shell, two runs it could be: the stale note of a call that never started, and the call after it.
	runs = []Run{{Agent: "a1", ID: "stale", Started: 1000, Heavy: true}, {Agent: "a1", ID: "next", Started: 1010}}
	if Match(runs, []procload.Proc{proc(20, 1, "a1", toolCmd, 1010, 1, 1)}, found); len(found) != 0 {
		t.Fatalf("a shell two runs could own was given to one: %v", found)
	}
	// A shell already a run's is not a candidate for the run that comes later, which finds its own.
	runs = []Run{{Agent: "a1", ID: "long", Started: 1000, Heavy: true}}
	Match(runs, []procload.Proc{proc(30, 1, "a1", toolCmd, 1000, 1, 1)}, found)
	runs = append(runs, Run{Agent: "a1", ID: "later", Started: 1015})
	Match(runs, []procload.Proc{proc(30, 1, "a1", toolCmd, 1000, 1, 1), proc(31, 1, "a1", toolCmd, 1016, 1, 1)}, found)
	if found["long"] != (procKey{30, 1000}) || found["later"] != (procKey{31, 1016}) {
		t.Fatalf("a taken shell was counted against the later run: %v", found)
	}
}

func step(s State, sig Signals, from, n int64) (State, int64) {
	for i := int64(0); i < n; i++ {
		from += Every
		s = Step(s, sig, from)
	}
	return s, from
}

// The machine turns slow only when a cause shows twice in a row, and calm only after three minutes without one.
func TestTheMachineTurnsSlowAfterTwoLooksAndCalmAfterThreeQuietMinutes(t *testing.T) {
	late, calm := Signals{Late: 2, Heavy: 9, Cores: 12}, Signals{Heavy: 4, Cores: 12}
	s := Step(State{}, late, 100)
	if s.Slow {
		t.Fatal("slow on one look")
	}
	if s = Step(s, calm, 115); s.Slow || s.Signs != 0 || s.Causes != nil {
		t.Fatalf("a look with no cause between: %+v", s)
	}
	s = Step(Step(s, late, 130), late, 145)
	if !s.Slow || s.Since != 145 || s.Cap != 8 || s.Peak != 9 || !reflect.DeepEqual(s.Causes, []string{CauseRuns}) {
		t.Fatalf("after two looks: %+v", s)
	}
	at := int64(145)
	s, at = step(s, calm, at, CalmFor/Every-1)
	if !s.Slow || !reflect.DeepEqual(s.Causes, []string{CauseRuns}) {
		t.Fatalf("calm too soon, or its cause forgotten: %+v", s)
	}
	if s = Step(s, late, at+Every); !s.Slow || s.CalmSince != 0 {
		t.Fatalf("a cause again restarts the quiet time: %+v", s)
	}
	at += Every
	s, at = step(s, calm, at, CalmFor/Every)
	if !s.Slow {
		t.Fatalf("the quiet time did not start over: %+v", s)
	}
	if s, _ = step(s, calm, at, 1); s.Slow || s.Since != 0 {
		t.Fatalf("still slow after three quiet minutes: %+v", s)
	}
}

// Each of the three causes is one by itself, and each has its threshold.
func TestEachCauseHasItsThreshold(t *testing.T) {
	for _, c := range []struct {
		sig  Signals
		want []string
	}{
		{Signals{Late: 1, TimedOut: 1, LoopMs: LoopSlowMs - 1}, nil},
		{Signals{Late: 2}, []string{CauseRuns}},
		{Signals{TimedOut: 2}, []string{CauseTimeouts}},
		{Signals{LoopMs: LoopSlowMs}, []string{CauseLoop}},
		{Signals{Late: 3, TimedOut: 4, LoopMs: 60_000}, []string{CauseRuns, CauseTimeouts, CauseLoop}},
	} {
		if got := c.sig.causes(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v: %v, want %v", c.sig, got, c.want)
		}
	}
}

// The cap starts one under the runs going, shrinks by a quarter each three minutes the cause stays, never under a quarter of the
// cores, and comes back one at a time once calm until it passes the most runs seen.
func TestTheCapShrinksWhileSlowAndGrowsBackWhenCalm(t *testing.T) {
	late, calm := Signals{Late: 2, Heavy: 9, Cores: 12}, Signals{Heavy: 3, Cores: 12}
	s, at := step(State{}, late, 0, 2)
	if s.Cap != 8 {
		t.Fatalf("at the start: %+v", s)
	}
	s, at = step(s, late, at, shrinkEvery/Every-1)
	if s.Cap != 8 {
		t.Fatalf("shrunk before three minutes: %+v", s)
	}
	if s, at = step(s, late, at, 1); s.Cap != 6 {
		t.Fatalf("after three minutes: %+v", s)
	}
	s, at = step(s, late, at, 3*shrinkEvery/Every)
	if s.Cap != 3 {
		t.Fatalf("the floor is a quarter of the cores: %+v", s)
	}
	s, at = step(s, calm, at, CalmFor/Every+1)
	if s.Slow || s.Cap != 4 {
		t.Fatalf("calm: the cap stays and goes up one: %+v", s)
	}
	s, at = step(s, calm, at, CalmFor/Every)
	if s.Cap != 5 {
		t.Fatalf("one more each calm three minutes: %+v", s)
	}
	// Slow again on the way back up: a quarter off what it had reached.
	again, at2 := step(s, late, at, 2)
	if !again.Slow || again.Cap != 3 || again.Peak != 9 {
		t.Fatalf("slow again: %+v at %d", again, at2)
	}
	for i := 0; i < 5; i++ {
		s, at = step(s, calm, at, CalmFor/Every)
	}
	if !reflect.DeepEqual(s, State{}) {
		t.Fatalf("past the most runs seen the cap goes, and nothing is left: %+v", s)
	}
	// The most runs seen is the most while slow, not only at its start: more runs joining move where the cap goes away.
	grown, at3 := step(State{}, Signals{Late: 2, Heavy: 4, Cores: 12}, 0, 2)
	grown, at3 = step(grown, Signals{Late: 2, Heavy: 7, Cores: 12}, at3, 1)
	if grown.Peak != 7 || grown.Cap != 3 {
		t.Fatalf("runs joined while slow: %+v", grown)
	}
	// On the way back up, a cause that shows once starts the calm three minutes over without turning it slow.
	back, at3 := step(grown, calm, at3, CalmFor/Every+1)
	if back.Slow || back.Cap != 4 {
		t.Fatalf("calm again: %+v", back)
	}
	back, at3 = step(back, calm, at3, CalmFor/Every-1)
	back, at3 = step(back, late, at3, 1)
	if back.Slow || back.Cap != 4 || back.CalmSince != 0 {
		t.Fatalf("one look with a cause on the way up: %+v", back)
	}
	back, at3 = step(back, calm, at3, CalmFor/Every)
	if back.Cap != 4 {
		t.Fatalf("the cap rose before three calm minutes had passed again: %+v", back)
	}
	if back, _ = step(back, calm, at3, 1); back.Cap != 5 {
		t.Fatalf("three calm minutes after the cause: %+v", back)
	}
	// With few runs going the cap starts at the floor, which holds nothing back.
	if s, _ := step(State{}, Signals{LoopMs: 60_000, Heavy: 1, Cores: 12}, 0, 2); s.Cap != 3 {
		t.Errorf("few runs: %+v", s)
	}
	if s, _ := step(State{}, Signals{Late: 2, Heavy: 2, Cores: 4}, 0, 2); s.Cap != 2 {
		t.Errorf("a small machine: %+v", s)
	}
}

// Only Turns uses the cap that follows the machine, only Fixed a set one, and only Priority and Turns lower priority, while slow.
func TestThePolicyDecidesTheCapAndThePriority(t *testing.T) {
	slow, calm := State{Slow: true, Cap: 5}, State{Cap: 5}
	for _, c := range []struct {
		cfg    Config
		limit  int
		lowers bool
	}{
		{Config{Policy: Observe}, 0, false},
		{Config{Policy: Priority}, 0, true},
		{Config{Policy: Turns}, 5, true},
		{Config{Policy: Fixed}, 6, false},
		{Config{Policy: Fixed, AtOnce: 4}, 4, false},
	} {
		if got := slow.CapFor(c.cfg, 12); got != c.limit {
			t.Errorf("%+v: cap %d, want %d", c.cfg, got, c.limit)
		}
		if got := slow.LowersPriority(c.cfg); got != c.lowers {
			t.Errorf("%+v: lowers %v, want %v", c.cfg, got, c.lowers)
		}
		if calm.LowersPriority(c.cfg) {
			t.Errorf("%+v: lowers priority on a calm machine", c.cfg)
		}
	}
	if got := (State{}).CapFor(Config{Policy: Fixed}, 2); got != 2 {
		t.Errorf("fixed on two cores: %d", got)
	}
}

func TestConfigOfReadsThePolicyAndFallsBackToObserve(t *testing.T) {
	for _, c := range []struct {
		cfg  usage.Config
		want Config
	}{
		{usage.Config{}, Config{Policy: Observe}},
		{usage.Config{PolicyKey: "turns"}, Config{Policy: Turns}},
		{usage.Config{PolicyKey: "fixed", AtOnceKey: 4}, Config{Policy: Fixed, AtOnce: 4}},
		{usage.Config{PolicyKey: "fixed", AtOnceKey: int64(3)}, Config{Policy: Fixed, AtOnce: 3}},
		{usage.Config{PolicyKey: "kill", AtOnceKey: -2}, Config{Policy: Observe}},
		{usage.Config{PolicyKey: 3}, Config{Policy: Observe}},
	} {
		if got := ConfigOf(c.cfg); got != c.want {
			t.Errorf("%v: %+v, want %+v", c.cfg, got, c.want)
		}
	}
}
