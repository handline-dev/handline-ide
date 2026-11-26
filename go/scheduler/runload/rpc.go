package runload

import (
	"math"
	"time"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// Register serves run.admit, the hook's question before a Bash call, and run.load, the runs as a person reads them.
func Register(s *rpc.Server, w *Watcher) {
	s.Register("run.admit", rpc.Method{Read: true, Handler: w.admit})
	s.Register("run.load", rpc.Method{Read: true, Handler: func(*rpc.Conn, map[string]any) (any, error) {
		if l := w.Load(); l != nil {
			return l, nil
		}
		return nil, nil
	}})
}

func (w *Watcher) admit(c *rpc.Conn, p map[string]any) (any, error) {
	agent, _ := p["agent"].(string)
	id, _ := p["id"].(string)
	shape, ok := p["shape"].(string)
	if agent == "" {
		return nil, rpc.BadRequestError("agent is a name", "agent", rpc.InvalidParams)
	}
	if id == "" || len(id) > 200 {
		return nil, rpc.BadRequestError("id is the Bash call's tool_use_id", "id", rpc.InvalidParams)
	}
	if !ok || len(shape) > 200 {
		return nil, rpc.BadRequestError("shape is the command's shape", "shape", rpc.InvalidParams)
	}
	if err := c.RequireSelf(agent); err != nil {
		return nil, err
	}
	if w.DB != nil {
		if key, _ := paths.AgentKeyIn(w.DB, agent); key != "" {
			agent = key
		}
	}
	a := w.Admit(agent, id, shape)
	out := map[string]any{"go": a.Go, "waited_s": a.WaitedS}
	if !a.Go {
		out["ahead"], out["cap"] = a.Ahead, a.Cap
	}
	return out, nil
}

func stamp(secs int64) string { return time.Unix(secs, 0).UTC().Format(time.RFC3339) }

// Load is run.load's result; nil before the first look, as on a daemon that does not watch the runs.
func (w *Watcher) Load() map[string]any {
	v := w.View()
	if !v.Looked {
		return nil
	}
	var since, limit, pressure any
	if v.State.Slow {
		since = stamp(v.State.Since)
	}
	if v.Cap > 0 {
		limit = v.Cap
	}
	if v.Host.Pressure != "" {
		pressure = v.Host.Pressure
	}
	causes := []string{}
	if v.State.Slow || v.Cap > 0 {
		causes = append(causes, v.State.Causes...)
	}
	runs := make([]any, 0, len(v.Runs))
	for _, r := range v.Runs {
		var usual, cpu, rss, procs any
		if r.UsualS > 0 {
			usual = r.UsualS
		}
		if r.Found {
			cpu, rss, procs = math.Round(r.CPU*10)/10, r.RSS, r.Procs
		}
		runs = append(runs, map[string]any{"agent": r.Agent, "shape": r.Shape, "started": stamp(r.Started), "usual_s": usual,
			"background": r.Background, "heavy": r.Heavy, "late": r.Late, "cpu_pct": cpu, "rss_bytes": rss, "processes": procs})
	}
	waiting := make([]any, 0, len(v.Waiting))
	for _, a := range v.Waiting {
		waiting = append(waiting, map[string]any{"agent": a.Agent, "shape": a.Shape, "since": stamp(a.Since)})
	}
	return map[string]any{"policy": string(v.Config.Policy), "slow": v.State.Slow, "since": since, "causes": causes, "cap": limit,
		"late": v.Signals.Late, "timed_out": v.Signals.TimedOut, "loop_ms": v.Signals.LoopMs,
		"host": map[string]any{"load_per_core": math.Round(v.Host.LoadPerCore*100) / 100, "memory_pressure": pressure,
			"compressed_pages_per_s": math.Round(v.Host.CompressedPerS)},
		"runs": runs, "waiting": waiting}
}
