package reads

import (
	"maps"

	"github.com/handline/handline/go/scheduler/runload"
)

// RunLoadTold is what of DaemonStatus.run_load the status topic is told a change of: all of it while the machine is slow, and
// without the count of heavy runs while it is calm, when that count moves with every run and no screen shows it.
func (e *Env) RunLoadTold() map[string]any { return runLoadTold(e.runLoad()) }

func runLoadTold(rl map[string]any) map[string]any {
	if rl == nil || rl["slow"] == true {
		return rl
	}
	out := maps.Clone(rl)
	delete(out, "heavy")
	return out
}

// runLoad is DaemonStatus.run_load: whether the machine is slow under the agents' runs and what is done about it; nil on a daemon
// that does not watch them.
func (e *Env) runLoad() map[string]any {
	if e.RunLoad == nil {
		return nil
	}
	v := e.RunLoad()
	if !v.Looked {
		return nil
	}
	out := runLoadBase(v)
	heavy := 0
	for _, r := range v.Runs {
		if r.Heavy {
			heavy++
		}
	}
	out["heavy"], out["waiting"] = heavy, len(v.Waiting)
	return out
}

// runLoadBase is what RunLoad and run.load both say: the policy, whether the machine is slow and since when, what says so, the cap.
func runLoadBase(v runload.View) map[string]any {
	var since, limit any
	if v.State.Slow {
		since = ts(v.State.Since)
	}
	if v.Cap > 0 {
		limit = v.Cap
	}
	causes := []string{}
	if v.State.Slow || v.Cap > 0 {
		causes = append(causes, v.State.Causes...)
	}
	return map[string]any{"policy": string(v.Config.Policy), "slow": v.State.Slow, "since": since, "causes": causes, "cap": limit}
}

// slowText is DaemonHealth's line while the machine is slow, by its first cause: it names no figure, so it holds still.
func slowText(causes []string) string {
	what := "heavy runs are taking longer than usual"
	if len(causes) > 0 {
		switch causes[0] {
		case runload.CauseTimeouts:
			what = "heavy runs are running out of time"
		case runload.CauseLoop:
			what = "Handline itself is answering slowly"
		}
	}
	return "The Mac is slow: " + what + "."
}
