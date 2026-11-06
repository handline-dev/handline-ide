package reads

import (
	"time"

	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/procload"
)

// loadMinutesMax is how far back load.get's series may reach: the week the minutes are kept.
const loadMinutesMax = 7 * 24 * 60

// loadGet is load.get: what every process on the machine uses and whose it is, per agent and per process, and with minutes the per-minute
// series of each agent for that window. null before the first sample, or on a daemon that does not sample.
func (e *Env) loadGet(_ *rpc.Conn, params map[string]any) (any, error) {
	minutes, err := optInt(params, "minutes")
	if err != nil {
		return nil, err
	}
	if minutes != nil && (*minutes < 0 || *minutes > loadMinutesMax) {
		return nil, badParam("minutes is a whole number from 0 to 10080", "minutes")
	}
	agent, err := optStr(params, "agent")
	if err != nil {
		return nil, err
	}
	if e.Load == nil {
		return nil, nil
	}
	v, ok := e.Load()
	if !ok {
		return nil, nil
	}
	out := loadView(v, agent)
	if minutes != nil && *minutes > 0 && e.LoadSeries != nil {
		series, err := e.LoadSeries(time.Now().Unix() - *minutes*60)
		if err != nil {
			return nil, err
		}
		pts := map[string]any{}
		for name, ps := range series {
			if agent != nil && name != *agent {
				continue
			}
			rows := make([]any, 0, len(ps))
			for _, p := range ps {
				rows = append(rows, map[string]any{"minute": ts(p.Minute), "cpu_avg_pct": round(p.CPUAvg, 1), "cpu_max_pct": round(p.CPUMax, 1),
					"rss_avg_bytes": p.RSSAvg, "rss_max_bytes": p.RSSMax, "processes_max": p.ProcsMax})
			}
			pts[name] = rows
		}
		out["series"] = pts
	}
	return out, nil
}

func procRow(p procload.Proc) map[string]any {
	var agent, via any
	if p.Agent != "" {
		agent, via = p.Agent, p.Via
	}
	return map[string]any{"pid": p.PID, "ppid": p.PPID, "agent": agent, "found_by": via, "command": p.Cmd, "started": ts(p.Start),
		"cpu_pct": round(p.CPU, 1), "rss_bytes": p.RSS, "running": !p.Gone, "sampled": ts(p.At)}
}

func useRow(u procload.Use) map[string]any {
	return map[string]any{"cpu_pct": round(u.CPU, 1), "rss_bytes": u.RSS, "processes": u.Processes}
}

func loadView(v procload.View, only *string) map[string]any {
	m := v.Machine
	machine := map[string]any{"cores": m.Cores, "load_1m": m.Load1, "load_per_core": 0.0, "ram_total_bytes": m.RAMTotal, "ram_used_bytes": m.RAMUsed,
		"ram_free_bytes": m.RAMFree, "disk_free_bytes": m.DiskFree, "disk_total_bytes": m.DiskTotal}
	if m.Cores > 0 {
		machine["load_per_core"] = round(m.Load1/float64(m.Cores), 2)
	}
	agents := []any{}
	for _, a := range v.Agents {
		if only != nil && a.Agent != *only {
			continue
		}
		row := useRow(a.Use)
		top := []any{}
		for _, p := range a.Top {
			top = append(top, procRow(p))
		}
		row["agent"], row["found_by_env"], row["found_by_tree"], row["top"] = a.Agent, a.Env, a.Tree, top
		agents = append(agents, row)
	}
	procs := []any{}
	for _, p := range v.Processes {
		if only != nil && p.Agent != *only {
			continue
		}
		procs = append(procs, procRow(p))
	}
	unowned := useRow(v.Unowned.Use)
	top := []any{}
	for _, p := range v.Unowned.Top {
		top = append(top, procRow(p))
	}
	unowned["top"] = top
	return map[string]any{"at": ts(v.At), "every_s": int(procload.Every / time.Second), "machine": machine, "total": useRow(v.Total), "agents": agents,
		"unowned": unowned, "processes": procs, "sampling": map[string]any{"samples": v.Samples, "mean_ms": round(v.SampleMS, 1)}}
}
