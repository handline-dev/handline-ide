package reads

import (
	"reflect"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/scheduler/loadgate"
	"github.com/handline/handline/go/scheduler/runload"
	"github.com/handline/handline/go/scheduler/sysres"
)

func TestHealthIsHeldByTheFirstCauseThenNearASetLimitThenOK(t *testing.T) {
	limits := loadgate.Limits{LoadPct: 150, RAMFreePct: 10}
	machine := func(load float64, freePct int64) *sysres.Machine {
		return &sysres.Machine{Cores: 4, Load1: load, RAMTotal: 100, RAMFree: freePct}
	}
	calm := machine(1, 50)
	cases := []struct {
		name        string
		in          healthIn
		level, text string
	}{
		{"nothing", healthIn{limits: limits, m: calm}, "ok", ""},
		{"no measure yet", healthIn{limits: limits}, "ok", ""},
		{"paused first", healthIn{paused: "P", startHold: "L", needs: []string{"D"}, waiting: []string{"full"}, m: calm}, "held", "P"},
		{"then the load", healthIn{startHold: "L", needs: []string{"D"}, waiting: []string{"full"}}, "held", "L"},
		{"then a need", healthIn{needs: []string{"D", "E"}, waiting: []string{"full"}}, "held", "D"},
		{"one agent waits", healthIn{waiting: []string{"wall"}}, "held", "1 agent waits for an account: a usage limit"},
		{"two wait", healthIn{waiting: []string{"wall", "full"}}, "held", "2 agents wait for an account: a usage limit"},
		{"three wait", healthIn{waiting: []string{"full", "wall", "full"}}, "held", "3 agents wait for an account: no room to start"},
		{"load just near", healthIn{limits: limits, m: machine(5.6, 50)}, "near", "The computer's load is near its limit of 150% per core."},
		{"load not near", healthIn{limits: limits, m: machine(5.56, 50)}, "ok", ""},
		{"load limit off", healthIn{limits: loadgate.Limits{RAMFreePct: 10}, m: machine(40, 50)}, "ok", ""},
		{"memory just near", healthIn{limits: limits, m: machine(1, 19)}, "near", "Free memory is near its limit of 10%."},
		{"memory not near", healthIn{limits: limits, m: machine(1, 20)}, "ok", ""},
		{"memory limit off", healthIn{limits: loadgate.Limits{LoadPct: 150}, m: machine(1, 1)}, "ok", ""},
		{"held beats near", healthIn{waiting: []string{"full"}, limits: limits, m: machine(40, 1)}, "held", "1 agent waits for an account: no room to start"},
		{"a slow machine is near", healthIn{slow: "S", limits: limits, m: calm}, "near", "S"},
		{"slow before a limit that is near", healthIn{slow: "S", limits: limits, m: machine(40, 1)}, "near", "S"},
		{"held beats slow", healthIn{slow: "S", paused: "P"}, "held", "P"},
	}
	for _, c := range cases {
		if level, text := healthOf(c.in); level != c.level || text != c.text {
			t.Errorf("%s: %s %q, want %s %q", c.name, level, text, c.level, c.text)
		}
	}
}

func TestHealthSinceIsTheStartUntilTheLevelChanges(t *testing.T) {
	var none *HealthClock
	if got := none.at("held", 50, 10); got != 10 {
		t.Fatalf("no clock: %d", got)
	}
	h := &HealthClock{}
	if got := h.at("ok", 50, 10); got != 10 {
		t.Fatalf("first read: %d", got)
	}
	if got := h.at("ok", 60, 10); got != 10 {
		t.Fatalf("same level: %d", got)
	}
	if got := h.at("held", 70, 10); got != 70 {
		t.Fatalf("a new level: %d", got)
	}
	if got := h.at("held", 80, 10); got != 70 {
		t.Fatalf("it holds: %d", got)
	}
}

// A need is said in a line that does not move with its figure: the disk's free space is not in it.
func TestANeedIsSaidInALineThatStaysWhileItDoes(t *testing.T) {
	if got := needLine(map[string]any{"kind": "disk", "text": "4123 MB free"}); got != "The disk is too full for builds." {
		t.Fatal(got)
	}
	if got := needLine(map[string]any{"kind": "other", "text": "do this"}); got != "do this" {
		t.Fatal(got)
	}
}

// Health counts an agent whose status reason is full or wall, each alone, and no other reason; its text has no figure.
func TestHealthCountsAnAgentWaitingOnEachAccountReason(t *testing.T) {
	for reason, line := range map[string]string{"full": "no room to start", "wall": "a usage limit", "pace": "", "load": ""} {
		e := testEnv(t)
		teamtest.Seed(t, e.Home, "bob")
		run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES(?, ?, 1)`, teamtest.Key(t, e.Home, "bob"),
			`{"status":{"state":"stopped","reason":"`+reason+`","text":"until 04:00 (12%)"}}`)
		h, err := e.Health()
		if err != nil {
			t.Fatal(err)
		}
		want := Health{Level: "ok"}
		if line != "" {
			want = Health{Level: "held", Text: "1 agent waits for an account: " + line}
		}
		if h.Level != want.Level || h.Text != want.Text {
			t.Errorf("%s: %s %q, want %s %q", reason, h.Level, h.Text, want.Level, want.Text)
		}
	}
}

// The line a slow machine shows names its first cause in words and no figure, so it holds still while the figures move.
func TestSlowTextNamesTheFirstCause(t *testing.T) {
	for causes, want := range map[string]string{
		"":              "The Mac is slow: heavy runs are taking longer than usual.",
		"runs,loop":     "The Mac is slow: heavy runs are taking longer than usual.",
		"timeouts,loop": "The Mac is slow: heavy runs are running out of time.",
		"loop":          "The Mac is slow: Handline itself is answering slowly.",
	} {
		var c []string
		if causes != "" {
			c = strings.Split(causes, ",")
		}
		if got := slowText(c); got != want {
			t.Errorf("%v: %q", c, got)
		}
	}
}

// daemon.status carries run_load once the runs have been looked at, with the cap only under a policy that uses one, and health
// turns near while the machine is slow.
func TestStatusCarriesTheRunLoadAndHealthIsNearWhileSlow(t *testing.T) {
	view := runload.View{}
	e := &Env{RunLoad: func() runload.View { return view }}
	if got := e.runLoad(); got != nil {
		t.Fatalf("before the first look: %v", got)
	}
	view = runload.View{Looked: true, Config: runload.Config{Policy: runload.Turns}, Cap: 4,
		State:   runload.State{Slow: true, Since: 1_700_000_000, Causes: []string{runload.CauseRuns}, Cap: 4},
		Runs:    []runload.Run{{Heavy: true}, {Heavy: true}, {}},
		Waiting: []runload.Waiting{{Agent: "a1"}}}
	got := e.runLoad()
	want := map[string]any{"policy": "turns", "slow": true, "since": "2023-11-14T22:13:20Z", "causes": []string{"runs"}, "cap": 4, "heavy": 2, "waiting": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%v", got)
	}
	view = runload.View{Looked: true, Config: runload.Config{Policy: runload.Observe}, State: runload.State{Causes: []string{runload.CauseRuns}}}
	got = e.runLoad()
	if got["since"] != nil || got["cap"] != nil || len(got["causes"].([]string)) != 0 || got["slow"] != false {
		t.Errorf("calm, with a cause seen once: %v", got)
	}
	if (&Env{}).runLoad() != nil {
		t.Error("a daemon that does not watch the runs")
	}
	// A cap kept from a slow spell says nothing under a policy that uses none.
	view = runload.View{Looked: true, Config: runload.Config{Policy: runload.Observe}, State: runload.State{Cap: 4, Causes: []string{runload.CauseRuns}}}
	if got = e.runLoad(); len(got["causes"].([]string)) != 0 || got["cap"] != nil {
		t.Errorf("observe after a slow spell: %v", got)
	}
}

// The status topic is told a change of the run load, but not of the count of heavy runs alone while the machine is calm.
func TestTheCountOfHeavyRunsIsToldOnlyWhileSlow(t *testing.T) {
	view := runload.View{Looked: true, Config: runload.Config{Policy: runload.Observe}, Runs: []runload.Run{{Heavy: true}}}
	e := &Env{RunLoad: func() runload.View { return view }}
	calm := e.RunLoadTold()
	view.Runs = append(view.Runs, runload.Run{Heavy: true})
	if again := e.RunLoadTold(); !reflect.DeepEqual(calm, again) {
		t.Errorf("a run more on a calm machine moved what is told: %v then %v", calm, again)
	}
	if e.runLoad()["heavy"] != 2 {
		t.Error("the status itself lost the count")
	}
	view.State = runload.State{Slow: true, Since: 1_700_000_000}
	slow := e.RunLoadTold()
	view.Runs = view.Runs[:1]
	if again := e.RunLoadTold(); reflect.DeepEqual(slow, again) {
		t.Errorf("a run fewer on a slow machine is not told: %v", again)
	}
	if (&Env{}).RunLoadTold() != nil {
		t.Error("a daemon that does not watch the runs")
	}
}
