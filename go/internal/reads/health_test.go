package reads

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/scheduler/loadgate"
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
		{"paused first", healthIn{paused: "P", startHold: "L", needs: []string{"D"}, waiting: []string{"cap"}, m: calm}, "held", "P"},
		{"then the load", healthIn{startHold: "L", needs: []string{"D"}, waiting: []string{"cap"}}, "held", "L"},
		{"then a need", healthIn{needs: []string{"D", "E"}, waiting: []string{"cap"}}, "held", "D"},
		{"one agent waits", healthIn{waiting: []string{"wall"}}, "held", "1 agent waits for an account: a usage limit"},
		{"two wait", healthIn{waiting: []string{"pace", "cap"}}, "held", "2 agents wait for an account: the weekly pace"},
		{"three wait", healthIn{waiting: []string{"cap", "wall", "pace"}}, "held", "3 agents wait for an account: no free slot"},
		{"load just near", healthIn{limits: limits, m: machine(5.6, 50)}, "near", "The computer's load is near its limit of 150% per core."},
		{"load not near", healthIn{limits: limits, m: machine(5.56, 50)}, "ok", ""},
		{"load limit off", healthIn{limits: loadgate.Limits{RAMFreePct: 10}, m: machine(40, 50)}, "ok", ""},
		{"memory just near", healthIn{limits: limits, m: machine(1, 19)}, "near", "Free memory is near its limit of 10%."},
		{"memory not near", healthIn{limits: limits, m: machine(1, 20)}, "ok", ""},
		{"memory limit off", healthIn{limits: loadgate.Limits{LoadPct: 150}, m: machine(1, 1)}, "ok", ""},
		{"held beats near", healthIn{waiting: []string{"cap"}, limits: limits, m: machine(40, 1)}, "held", "1 agent waits for an account: no free slot"},
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

// Health counts an agent whose status reason is cap, wall or pace, each alone, and no other reason; its text has no figure.
func TestHealthCountsAnAgentWaitingOnEachAccountReason(t *testing.T) {
	for reason, line := range map[string]string{"cap": "no free slot", "wall": "a usage limit", "pace": "the weekly pace", "load": ""} {
		e := testEnv(t)
		os.MkdirAll(filepath.Join(e.Home, "agents", "bob"), 0o755)
		teamtest.Seed(t, e.Home, "bob")
		run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES('bob', ?, 1)`,
			`{"status":{"state":"stopped","reason":"`+reason+`","text":"ahead of pace by 12%"}}`)
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
