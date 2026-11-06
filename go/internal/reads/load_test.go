package reads

import (
	"testing"

	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/sysres"
)

func TestLoadGetServesAgentsProcessesAndTheUnownedApart(t *testing.T) {
	t.Parallel()
	view := procload.View{At: 1000, Machine: sysres.Machine{Cores: 4, Load1: 2},
		Total: procload.Use{CPU: 130, RSS: 3 << 20, Processes: 3},
		Agents: []procload.AgentUse{{Agent: "a", Use: procload.Use{CPU: 120, RSS: 2 << 20, Processes: 2}, Env: 1, Tree: 1,
			Top: []procload.Proc{{PID: 5, PPID: 1, Agent: "a", Via: procload.ViaEnv, Cmd: "node dev", CPU: 100, RSS: 1 << 20, At: 1000}}}},
		Unowned:   procload.AgentUse{Use: procload.Use{CPU: 10, RSS: 1 << 20, Processes: 1}, Top: []procload.Proc{{PID: 9, Cmd: "x", At: 1000}}},
		Processes: []procload.Proc{{PID: 5, Agent: "a", Via: procload.ViaEnv, Cmd: "node dev", At: 1000, Gone: true}, {PID: 9, Cmd: "x", At: 1000}},
	}
	e := &Env{Load: func() (procload.View, bool) { return view, true }}
	got, err := e.loadGet(nil, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	out := got.(map[string]any)
	agents := out["agents"].([]any)
	a := agents[0].(map[string]any)
	if len(agents) != 1 || a["agent"] != "a" || a["found_by_env"] != 1 || a["found_by_tree"] != 1 || a["cpu_pct"] != 120.0 {
		t.Fatalf("%v", agents)
	}
	procs := out["processes"].([]any)
	first, last := procs[0].(map[string]any), procs[1].(map[string]any)
	if first["running"] != false || first["found_by"] != "env" || last["agent"] != nil || last["found_by"] != nil {
		t.Fatalf("%v", procs)
	}
	if out["total"].(map[string]any)["processes"] != 3 || out["machine"].(map[string]any)["load_per_core"] != 0.5 {
		t.Fatalf("%v", out)
	}
	if _, has := out["series"]; has {
		t.Error("a series without minutes")
	}
	only, _ := e.loadGet(nil, map[string]any{"agent": "nobody"})
	if len(only.(map[string]any)["agents"].([]any)) != 0 || len(only.(map[string]any)["processes"].([]any)) != 0 {
		t.Errorf("agent filter: %v", only)
	}
}

func TestLoadGetIsNullBeforeTheFirstSampleAndRefusesABadWindow(t *testing.T) {
	t.Parallel()
	for _, e := range []*Env{{}, {Load: func() (procload.View, bool) { return procload.View{}, false }}} {
		if got, err := e.loadGet(nil, map[string]any{}); got != nil || err != nil {
			t.Errorf("%v %v", got, err)
		}
	}
	e := &Env{}
	for _, p := range []map[string]any{{"minutes": float64(-1)}, {"minutes": float64(10081)}, {"minutes": "5"}, {"agent": 5.0}} {
		if _, err := e.loadGet(nil, p); err == nil {
			t.Errorf("%v accepted", p)
		}
	}
}
