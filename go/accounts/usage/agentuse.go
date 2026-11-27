package usage

import (
	"sort"

	"github.com/handline/handline/go/data/db"
)

// AgentUse is stats.AgentUse: one agent's use on one machine over a span, all accounts.
type AgentUse struct {
	Agent  string
	Node   string // the machine that spent it, a node id
	Tokens int64
	Cost   float64 // USD at list price
	Share  float64 // of everyone's cost in the span
	// Fracs is its cost share of each account in the span; an account it spent nothing on is absent.
	Fracs map[string]float64
}

type agentTally struct {
	tokens int64 // inp, cc, cr and out: thinking is a part of out
	cost   float64
}

type callKey struct{ agent, node, acct string }

// UseKey is an agent's use on one machine.
type UseKey struct{ Agent, Node string }

// scanCalls calls each for every call in (lo, hi] with its agent, machine and account, tokens and list-price cost, oldest first so float
// sums come out the same on every run. A call kept with no machine is self's, the master's own.
func scanCalls(q db.Q, self string, lo, hi int64, table []priceEntry, each func(ts int64, k callKey, tokens int64, cost float64)) error {
	trees, err := worktrees(q)
	if err != nil {
		return err
	}
	rows, err := q.Query("SELECT ts, account, node, cwd, model, inp, cc, cr, out, thinking, cc1h FROM token_calls WHERE ts > ? AND ts <= ? ORDER BY ts, rowid",
		lo, hi)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ts int64
		var acct, node, cwd, model string
		var t Tokens
		if err := rows.Scan(&ts, &acct, &node, &cwd, &model, &t.Inp, &t.CC, &t.CR, &t.Out, &t.Thinking, &t.CC1h); err != nil {
			return err
		}
		if node == "" {
			node = self
		}
		each(ts, callKey{agentForCwd(cwd, trees), node, acct}, t.Inp+t.CC+t.CR+t.Out, Cost(model, t, table))
	}
	return rows.Err()
}

type tally map[callKey]*agentTally

func (per tally) add(k callKey, tokens int64, cost float64) {
	if per[k] == nil {
		per[k] = &agentTally{}
	}
	per[k].tokens += tokens
	per[k].cost += cost
}

// keys are per's keys in a fixed order, so sums over them come out the same on every run.
func (per tally) keys() []callKey {
	keys := make([]callKey, 0, len(per))
	for k := range per {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.agent != b.agent {
			return a.agent < b.agent
		}
		if a.node != b.node {
			return a.node < b.node
		}
		return a.acct < b.acct
	})
	return keys
}

// tallyCalls is the tokens and list-price cost of the calls in (lo, hi] per agent, machine and account, with its keys in order.
func tallyCalls(q db.Q, self string, lo, hi int64, table []priceEntry) (tally, []callKey, error) {
	per := tally{}
	if err := scanCalls(q, self, lo, hi, table, func(_ int64, k callKey, tokens int64, cost float64) { per.add(k, tokens, cost) }); err != nil {
		return nil, nil, err
	}
	return per, per.keys(), nil
}

// PairUse is one agent's use of one account on one machine in a span. Agent is "" for the calls of no agent's worktree.
type PairUse struct {
	Agent, Node, Account string
	Tokens               int64
	Cost                 float64 // USD at list price
	Share                float64 // of the account's cost in the span
	// WeekPct is the points of the account's week it used: Share times the account's rise (WeekConsumed), so an account's rows add up to
	// its rise. nil when the account has no reading in the span or its calls had no list price to split by.
	WeekPct *float64
}

// PairUsesOf is stats.get's use: a row per agent, machine and account with a call in (lo, hi], largest cost first, ties by agent, machine
// then account. A call kept with no machine is self's.
func PairUsesOf(q db.Q, self string, lo, hi int64, table []priceEntry, weekUsed map[string]float64) ([]PairUse, error) {
	per, keys, err := tallyCalls(q, self, lo, hi, table)
	if err != nil {
		return nil, err
	}
	acctCost := map[string]float64{}
	for _, k := range keys {
		acctCost[k.acct] += per[k].cost
	}
	out := make([]PairUse, 0, len(keys))
	for _, k := range keys {
		t := per[k]
		u := PairUse{Agent: k.agent, Node: k.node, Account: k.acct, Tokens: t.tokens, Cost: t.cost}
		if u.Agent == Unattributed {
			u.Agent = ""
		}
		if acctCost[k.acct] > 0 {
			u.Share = t.cost / acctCost[k.acct]
			if pts, ok := weekUsed[k.acct]; ok {
				w := u.Share * pts
				u.WeekPct = &w
			}
		}
		out = append(out, u)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Cost > out[j].Cost })
	return out, nil
}

// agentUses is stats.agent_use: each agent's tokens and list-price cost on each machine as per tallied them, largest cost first, ties by
// name then machine.
func agentUses(per tally, keys []callKey) []AgentUse {
	acctCost := map[string]float64{}
	for _, k := range keys {
		acctCost[k.acct] += per[k].cost
	}
	by := map[UseKey]*AgentUse{}
	var total float64
	for _, k := range keys {
		t := per[k]
		uk := UseKey{k.agent, k.node}
		u := by[uk]
		if u == nil {
			u = &AgentUse{Agent: k.agent, Node: k.node, Fracs: map[string]float64{}}
			by[uk] = u
		}
		u.Tokens += t.tokens
		u.Cost += t.cost
		total += t.cost
		if acctCost[k.acct] == 0 {
			continue
		}
		u.Fracs[k.acct] = t.cost / acctCost[k.acct]
	}
	out := make([]AgentUse, 0, len(by))
	for _, u := range by {
		if total != 0 {
			u.Share = u.Cost / total
		}
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Cost != b.Cost {
			return a.Cost > b.Cost
		}
		return a.Agent < b.Agent || (a.Agent == b.Agent && a.Node < b.Node)
	})
	return out
}

// PickUse is the use of a pick of the agents table, an agent, on every machine together. ok is false for a name that used nothing.
func PickUse(uses []AgentUse, pick string) (AgentUse, bool) {
	sum, found := AgentUse{Agent: pick, Fracs: map[string]float64{}}, false
	for _, u := range uses {
		if u.Agent != pick {
			continue
		}
		found = true
		sum.Tokens += u.Tokens
		sum.Cost += u.Cost
		sum.Share += u.Share
		for a, f := range u.Fracs {
			sum.Fracs[a] += f
		}
	}
	return sum, found
}

// WeekConsumed is outlook.week_consumed: the points of each account's week used between lo and hi, the rises from one sample to the next
// and after a reset what has been used since. The last sample before lo (not older than OutlookMaxAgeS) is where the first rise starts from.
func WeekConsumed(rows []sampleRow, lo, hi int64) map[string]float64 {
	by := map[string][]weekPoint{}
	inside := map[string]bool{}
	for _, r := range rows {
		w, ok := weekOf(r)
		if !ok || w.TS > hi {
			continue
		}
		a, _ := r["account"].(string)
		if w.TS >= lo {
			by[a] = append(by[a], w)
			inside[a] = true
		} else if lo-w.TS <= OutlookMaxAgeS {
			by[a] = []weekPoint{w} // the closest one before lo (rows are oldest first)
		}
	}
	out := map[string]float64{}
	for a, series := range by {
		if !inside[a] {
			continue // nothing read inside the span: not an account it says anything of
		}
		var total float64
		for i := 1; i < len(series); i++ {
			total += rise(series[i-1], series[i])
		}
		out[a] = total
	}
	return out
}

func rise(prev, cur weekPoint) float64 {
	if cur.Used >= prev.Used {
		return cur.Used - prev.Used
	}
	return cur.Used
}

// ConsumedCurve is outlook.consumed_curve: for each account, the points of its week used from the first time to each of times.
func ConsumedCurve(rows []sampleRow, times []int64) map[string][]float64 {
	by := map[string][]weekPoint{}
	for _, r := range rows {
		if w, ok := weekOf(r); ok {
			a, _ := r["account"].(string)
			by[a] = append(by[a], w)
		}
	}
	out := map[string][]float64{}
	for a, series := range by {
		cum := make([]float64, 0, len(times))
		var run float64
		var prev *weekPoint
		i := 0
		for _, t := range times {
			for i < len(series) && series[i].TS <= t {
				cur := series[i]
				if prev != nil && cur.TS >= times[0] {
					run += rise(*prev, cur)
				}
				prev = &series[i]
				i++
			}
			cum = append(cum, run)
		}
		out[a] = cum
	}
	return out
}

// Samples are the usage_samples rows WeeklyOutlook and the use of each agent read.
type Samples = []sampleRow

// Overlay is agent_overlay: the points of the combined week the pick had used by each time of past, its cost share of each account times
// what that account had used so far, averaged over the accounts by w as past is. Empty when the pick used nothing or is no agent.
func Overlay(rows Samples, uses []AgentUse, pick string, past []OutlookPoint, w WeekWeights) []float64 {
	use, found := PickUse(uses, pick)
	if !found || len(use.Fracs) == 0 {
		return nil
	}
	times := make([]int64, len(past))
	for i, p := range past {
		times[i] = p.At
	}
	curves := ConsumedCurve(rows, times)
	accts := make([]string, 0, len(curves))
	for a := range curves {
		accts = append(accts, a)
	}
	sort.Strings(accts)
	var total float64
	for _, a := range accts {
		total += w.of(a)
	}
	out := make([]float64, len(times))
	if total <= 0 {
		return out
	}
	for k := range times {
		var sum float64
		for _, a := range accts {
			sum += w.of(a) * use.Fracs[a] * curves[a][k]
		}
		out[k] = sum / total
	}
	return out
}
