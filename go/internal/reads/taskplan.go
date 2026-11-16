package reads

import (
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/textsafe"
)

// planOf fills TaskDetail's plan_state, plan, plan_ask and core_ask.
func (e *Env) planOf(tid int64, task map[string]any) error {
	state, plan, open, err := db.PlanState(e.DB, tid)
	if err != nil {
		return err
	}
	task["plan_state"], task["plan"], task["plan_ask"], task["core_ask"] = state, nil, nil, nil
	if plan != nil {
		versions := make([]any, len(plan.Versions))
		for i, v := range plan.Versions {
			versions[i] = map[string]any{"text": textsafe.Text(v.Text), "by": v.By, "at": ts(v.At)}
		}
		cur := plan.Current()
		var ok any
		if plan.OK != nil {
			ok = map[string]any{"by": plan.OK.By, "at": ts(plan.OK.At)}
		}
		task["plan"] = map[string]any{"text": textsafe.Text(cur.Text), "by": cur.By, "at": ts(cur.At), "version": len(plan.Versions), "ok": ok,
			"versions": versions}
	}
	if open != nil {
		task["plan_ask"] = askJSON(open)
	}
	cores, err := db.TaskAsks(e.DB, db.CoreAskKind, tid)
	if err != nil {
		return err
	}
	for i := len(cores) - 1; i >= 0; i-- {
		if !cores[i].Dropped {
			task["core_ask"] = askJSON(cores[i])
			break
		}
	}
	return nil
}

func askJSON(a *db.TaskAsk) map[string]any {
	out := map[string]any{"at": ts(a.At), "by": a.By, "to": a.To, "sent_at": nil}
	if a.Sent != 0 {
		out["sent_at"] = ts(a.Sent)
	}
	if a.Kind == db.CoreAskKind {
		out["note"] = nil
		if a.Note != "" {
			out["note"] = textsafe.Text(a.Note)
		}
	}
	return out
}

// planCostShown is how many of the newest plans planCost averages.
const planCostShown = 20

// planCost is settings.get's plan_cost_usd: what the newest plans cost on average, each what its agent spent from the ask's delivery to the
// plan; nil before any plan answered a sent ask.
func (e *Env) planCost() (any, error) {
	writes, err := db.PlanWrites(e.DB, planCostShown)
	if err != nil || len(writes) == 0 {
		return nil, err
	}
	u, sum := e.usageEnv(), 0.0
	for _, w := range writes {
		usd, err := u.AgentUSD(e.DB, w.Agent, w.Sent, w.Answer)
		if err != nil {
			return nil, err
		}
		sum += usd
	}
	return sum / float64(len(writes)), nil
}
