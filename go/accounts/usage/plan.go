package usage

import "sort"

// What the plans cost, as plan money: each account's monthly fee turned into a weekly budget, of which the week's percent used is spent.
// Token list prices never size it; they only split an account's plan money between the agents that used it.

// PlanFee is a plan's monthly fee in US dollars as Anthropic lists it: Pro 20, Max 100, Ultra (claude's max_20x tier) 200. 0 for a plan
// with no listed fee (team, enterprise) or none recorded.
func PlanFee(subscription, tier string) float64 {
	switch PlanName(subscription, tier) {
	case "Ultra":
		return 200
	case "Max":
		return 100
	case "Pro":
		return 20
	}
	return 0
}

// MaxFee is the largest monthly fee an account may set: more is no plan, and its sums would overflow.
const MaxFee = 100_000

// WeekBudget is the plan money of one week of a monthly fee: a year's twelve fees over its 52 weeks.
func WeekBudget(fee float64) float64 { return fee * 12 / 52 }

// FeeOf is the account's monthly fee: the one set on its row (plan_fee), else its plan's.
func (c Config) FeeOf(acct, subscription, tier string) float64 {
	if v, ok := num(c["plan_fee_"+acct]); ok {
		return v
	}
	return PlanFee(subscription, tier)
}

// AccountBudget is one account's week in plan money.
type AccountBudget struct {
	Account, Plan string
	FeeUSD        float64
	WeekUSD       float64
	// UsedPct is the week's percent used now; nil with no reading. A week whose reset has passed is 0.
	UsedPct *float64
	// FiveHour is whether its reading carries a 5-hour window: an Ultra account has none.
	FiveHour bool
	// Extra is its extra usage (extra.go), beside the plan money and never in it; nil when unknown.
	Extra *Extra
}

// Spent is the plan money of the week used, at most the week; nil with no reading.
func (b AccountBudget) Spent() *float64 {
	if b.UsedPct == nil {
		return nil
	}
	v := min(*b.UsedPct, 100) / 100 * b.WeekUSD
	return &v
}

// BudgetOf is the account's AccountBudget from its status at now, its plan and its fee.
func BudgetOf(st *AccountStatus, plan string, fee float64, now int64) AccountBudget {
	b := AccountBudget{Account: st.Account, Plan: plan, FeeUSD: fee, WeekUSD: WeekBudget(fee), FiveHour: st.Short.Pct != nil, Extra: st.CurrentExtra()}
	if st.Long.Pct != nil {
		used := float64(*st.Long.Pct)
		if st.Long.Reset != 0 && st.Long.Reset <= now {
			used = 0
		}
		b.UsedPct = &used
	}
	return b
}

// BudgetTotal is the accounts' weeks together, each weighted by its budget: over the accounts with a fee and a reading.
type BudgetTotal struct {
	WeekUSD, SpentUSD float64
	// LeftPct is the percent of WeekUSD not spent; nil when no account counts.
	LeftPct *float64
}

// TotalOf is the budget-weighted total of bs.
func TotalOf(bs []AccountBudget) BudgetTotal {
	var t BudgetTotal
	for _, b := range bs {
		if s := b.Spent(); s != nil && b.WeekUSD > 0 {
			t.WeekUSD += b.WeekUSD
			t.SpentUSD += *s
		}
	}
	if t.WeekUSD > 0 {
		left := (t.WeekUSD - t.SpentUSD) / t.WeekUSD * 100
		t.LeftPct = &left
	}
	return t
}

// PlanCosts are the plan money the agents used: per local day and agent, and per agent over the span. An account's plan money in a
// stretch is the points its week rose (WeekConsumed) times its budget; each agent takes its share of that account's list-price cost in
// the stretch. Money of an account that rose with no priced call goes to Unattributed.
type PlanCosts struct {
	Daily map[string]map[string]float64
	Span  map[string]float64
}

// dayCalls is the list-price cost of each day's calls by agent and account.
type dayCalls map[string]tally

func (d dayCalls) add(day string, k callKey, cost float64) {
	if d[day] == nil {
		d[day] = tally{}
	}
	d[day].add(k, 0, cost)
}

// planSplit adds each account's plan money rise x weekUSD/100 to out, split by per's cost shares.
func planSplit(out map[string]float64, per tally, rise map[string]float64, weekUSD map[string]float64) {
	keys := per.keys()
	acctCost := map[string]float64{}
	for _, k := range keys {
		acctCost[k.acct] += per[k].cost
	}
	accts := make([]string, 0, len(rise))
	for a := range rise {
		accts = append(accts, a)
	}
	sort.Strings(accts)
	for _, a := range accts {
		usd := rise[a] * weekUSD[a] / 100
		if usd == 0 {
			continue
		}
		if acctCost[a] == 0 {
			out[Unattributed] += usd
			continue
		}
		for _, k := range keys {
			if k.acct == a {
				out[k.agent] += usd * per[k].cost / acctCost[a]
			}
		}
	}
}

// rises are each account's week rises, per local day of (dayLo, hi] and over [spanLo, hi], from one pass over rows (oldest first): a rise
// counts where the sample that shows it is. Only a range's first rise needs a sample before it no older than OutlookMaxAgeS, as in
// WeekConsumed; a later gap (the machine asleep) still counts.
func rises(rows []sampleRow, dayLo, spanLo, hi int64) (byDay map[string]map[string]float64, span map[string]float64) {
	byDay, span = map[string]map[string]float64{}, map[string]float64{}
	prev := map[string]weekPoint{}
	from := min(dayLo+1, spanLo)
	for _, r := range rows {
		ts, _ := num(r["ts"])
		if int64(ts) > hi || from-int64(ts) > OutlookMaxAgeS {
			continue
		}
		w, ok := weekOf(r)
		if !ok {
			continue
		}
		a, _ := r["account"].(string)
		p, had := prev[a]
		prev[a] = w
		if !had {
			continue
		}
		up := rise(p, w)
		if w.TS > dayLo && (p.TS > dayLo || dayLo-p.TS <= OutlookMaxAgeS) {
			day := dayOf(w.TS)
			if byDay[day] == nil {
				byDay[day] = map[string]float64{}
			}
			byDay[day][a] += up
		}
		if w.TS >= spanLo && (p.TS >= spanLo || spanLo-p.TS <= OutlookMaxAgeS) {
			span[a] += up
		}
	}
	return byDay, span
}

// planCostsOf is PlanCosts from the calls' costs per day of (dayLo, hi] and over the span from spanLo, the samples, and each account's
// budget.
func planCostsOf(days dayCalls, span tally, rows []sampleRow, dayLo, spanLo, hi int64, weekUSD map[string]float64) PlanCosts {
	out := PlanCosts{Daily: map[string]map[string]float64{}, Span: map[string]float64{}}
	byDay, spanRise := rises(rows, dayLo, spanLo, hi)
	for day, rise := range byDay {
		m := map[string]float64{}
		planSplit(m, days[day], rise, weekUSD)
		if len(m) > 0 {
			out.Daily[day] = m
		}
	}
	planSplit(out.Span, span, spanRise, weekUSD)
	return out
}
