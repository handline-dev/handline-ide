package reads

import (
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// outlookHours is the span of each range: how far back and ahead the headroom curve goes, and how many days the cost covers.
var outlookHours = map[string]int{"12h": 12, "24h": 24, "3d": 72, "7d": 168, "14d": 336, "30d": 720}

var outlookRanges = "12h, 24h, 3d, 7d, 14d, 30d"

// round is Python's round(v, places): the nearest decimal to the exact binary value.
func round(v float64, places int) float64 {
	f, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'f', places, 64), 64)
	return f
}

func points(series []usage.OutlookPoint) []any {
	out := make([]any, 0, len(series))
	for _, p := range series {
		var left any
		if p.Left != nil {
			left = round(*p.Left, 2)
		}
		out = append(out, map[string]any{"at": ts(p.At), "left_pct": left})
	}
	return out
}

// runsOut is rpc_outlook._runs_out: seconds until the window that runs out first does, as the runway's words pick it; nil when there is
// no reading, no burn to go by, or the window resets first.
func runsOut(st *usage.AccountStatus, now int64, ld *usage.Load) any {
	in, _, ok := wallOf(st, now, ld)
	if !ok {
		return nil
	}
	return in
}

// wallOf is when the account reaches a limit, in seconds from now (0: it is at one), and when that limit's window resets; ok false when
// there is no reading, no burn to go by, or the window resets first.
func wallOf(st *usage.AccountStatus, now int64, ld *usage.Load) (in, reset int64, ok bool) {
	var burn *float64
	if ld != nil {
		burn = ld.Burn
	}
	r := usage.RunwayOf(st, burn, float64(now), ld)
	switch {
	case r == nil:
		return 0, 0, false
	case r.LeftShort == 0:
		return 0, st.Short.Reset, true
	case r.LeftLong != nil && *r.LeftLong == 0:
		return 0, st.Long.Reset, true
	}
	var five *float64
	if r.Burning && !r.PastReset {
		five = r.EtaS
	}
	if r.LongS != nil && (five == nil || *r.LongS < *five) {
		return int64(*r.LongS), st.Long.Reset, true
	}
	if five == nil {
		return 0, 0, false
	}
	return int64(*five), st.Short.Reset, true
}

// statsOutlook is stats.outlook: the week left of the accounts together (recorded, then guessed), each account's runway and what the
// agents cost per day.
func (e *Env) statsOutlook(_ *rpc.Conn, params map[string]any) (any, error) {
	name, _ := params["range"].(string)
	hours, ok := outlookHours[name]
	if !ok {
		return nil, badParam("range is one of "+outlookRanges, "range")
	}
	pick, err := optStr(params, "agent")
	if err != nil {
		return nil, err
	}
	return e.outlookAt(name, hours, time.Now(), pick)
}

// agentUseOf is the spec's agent_use row.
func agentUseOf(u usage.AgentUse) map[string]any {
	var est any
	if u.EstPct != nil {
		est = round(*u.EstPct, 2)
	}
	fracs := map[string]any{}
	for a, f := range u.Fracs {
		fracs[a] = round(f, 4)
	}
	return map[string]any{"agent": u.Agent, "node": u.Node, "tokens": u.Tokens, "cost": round(u.Cost, 4), "share": round(u.Share, 4), "est_pct": est, "fracs": fracs}
}

// overlayOf is the spec's overlay, at two places like left_pct.
func overlayOf(rows usage.Samples, uses []usage.AgentUse, pick string, past []usage.OutlookPoint, w usage.WeekWeights) []any {
	out := []any{}
	for _, v := range usage.Overlay(rows, uses, pick, past, w) {
		out = append(out, round(v, 2))
	}
	return out
}

// accountEnds is when each account the rows name ends (its subscription), and from when each that cannot be used now (its login gone or
// expired) could not.
func (e *Env) accountEnds(cfg usage.Config, rows usage.Samples, clock time.Time) (ends, outSince map[string]float64) {
	now := float64(clock.Unix())
	ends, outSince = map[string]float64{}, map[string]float64{}
	for _, a := range usage.SampleAccounts(rows) {
		if t, ok := cfg.EndOf(a); ok {
			ends[a] = float64(t.UnixNano()) / 1e9
		}
		if since, gone := (team.Accounts{Home: e.Home, Login: e.loginState}).GoneSince(a, now); gone {
			outSince[a] = since
		}
	}
	return ends, outSince
}

// firstDay is where cost_per_day starts: the end of the day before the range's first, a range under a day counting as one.
func firstDay(clock time.Time, hours int) int64 {
	days := max(1, hours/24)
	return time.Date(clock.Year(), clock.Month(), clock.Day()-(days-1), 0, 0, 0, 0, time.Local).Unix() - 1
}

// perDayOf is the spec's cost_per_day, oldest day first.
func perDayOf(cost map[string]map[string]float64) []any {
	days := make([]string, 0, len(cost))
	for d := range cost {
		days = append(days, d)
	}
	sort.Strings(days)
	out := []any{}
	for _, d := range days {
		out = append(out, map[string]any{"day": d, "agents": rounded(cost[d], 4)})
	}
	return out
}

// planPerDayOf is the spec's plan_cost_per_day: each day's plan money per agent and its extra usage per currency, oldest day first; a day
// with neither is absent.
func planPerDayOf(plan, extra map[string]map[string]float64) []any {
	days := make([]string, 0, len(plan)+len(extra))
	for d := range plan {
		days = append(days, d)
	}
	for d := range extra {
		if _, ok := plan[d]; !ok {
			days = append(days, d)
		}
	}
	sort.Strings(days)
	out := []any{}
	for _, d := range days {
		out = append(out, map[string]any{"day": d, "agents": rounded(plan[d], 4), "extra": rounded(extra[d], 2)})
	}
	return out
}

func rounded(m map[string]float64, places int) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = round(v, places)
	}
	return out
}

// samplesSince is usage.SamplesSince, from the daemon's held rows when it has them.
func (e *Env) samplesSince(since int64) (usage.Samples, error) {
	if e.samples == nil {
		return usage.SamplesSince(e.DB, since)
	}
	return e.samples.Since(e.DB, since)
}

func (e *Env) outlookAt(name string, hours int, clock time.Time, pick *string) (any, error) {
	now := clock.Unix()
	ue := &usage.Env{Home: e.Home, Now: func() float64 { return float64(now) }}
	cfg := ue.CurrentConfig()
	span := int64(hours) * 3600
	all, err := e.samplesSince(now - max(span, 86400) - usage.OutlookMaxAgeS) // by_account looks back at least a day
	if err != nil {
		return nil, err
	}
	listed := paths.ListAccounts(e.Home)
	all = usage.SamplesOf(all, listed) // the lines read the accounts the title and the rows show
	rows := usage.SamplesFrom(all, now-span-usage.OutlookMaxAgeS)
	ends, outSince := e.accountEnds(cfg, rows, clock)
	loads, err := ue.ReadAll(e.DB, "")
	if err != nil {
		return nil, err
	}
	runway, budgets := []any{}, []usage.AccountBudget{}
	weekUSD := map[string]float64{}
	for _, a := range listed {
		st := ue.Read(a)
		if !st.Unmetered {
			sub, tier := team.Accounts{Home: e.Home}.RecordedPlan(a)
			b := usage.BudgetOf(st, usage.PlanName(sub, tier), cfg.FeeOf(a, sub, tier), now)
			b.Account = a
			budgets, weekUSD[a] = append(budgets, b), b.WeekUSD
		}
		var runs any
		if !st.Unmetered {
			runs = runsOut(st, now, loads[a])
		}
		var resets any
		if st.Long.Reset != 0 {
			resets = ts(st.Long.Reset)
		}
		runway = append(runway, map[string]any{"account": a, "runs_out_in_seconds": runs, "week_resets_at": resets})
	}
	weights := usage.WeightsOf(budgets) // the line weighs each account as the budget's total does
	out := usage.WeeklyOutlook(rows, now, hours, ends, outSince, weights)
	byAccount, future := outlookByAccount(all, now, hours, ends, outSince, weekUSD, weights)
	self := joined.NodeIDOf(e.Home)
	c, err := usage.CallCosts(e.DB, self, firstDay(clock, hours), now-span, now, usage.PricesFromConfig(cfg), all, weekUSD)
	if err != nil {
		return nil, err
	}
	// A rise with no priced call is on no machine; this daemon's own unattributed row, when there is one, holds it as before.
	none := usage.UseKey{Agent: usage.Unattributed}
	ownRow := func(u usage.AgentUse) bool { return u.Agent == none.Agent && u.Node == self }
	if v, ok := c.Plan.Span[none]; ok && self != "" && slices.ContainsFunc(c.Uses, ownRow) {
		c.Plan.Span[usage.UseKey{Agent: none.Agent, Node: self}] += v
		delete(c.Plan.Span, none)
	}
	agentUse := []any{}
	for _, u := range c.Uses {
		row := agentUseOf(u)
		k := usage.UseKey{Agent: u.Agent, Node: u.Node}
		row["plan_cost"] = round(c.Plan.Span[k], 4)
		agentUse = append(agentUse, row)
		delete(c.Plan.Span, k)
	}
	if v, ok := c.Plan.Span[none]; ok { // a rise with no priced call: plan money no agent row holds
		agentUse = append(agentUse, map[string]any{"agent": usage.Unattributed, "node": nil, "tokens": 0, "cost": 0.0, "share": 0.0, "est_pct": nil,
			"fracs": map[string]any{}, "plan_cost": round(v, 4)})
	}
	res := map[string]any{"range": name, "now": ts(now), "accounts": out.Accounts, "past": points(out.Past), "future": points(future),
		"runway": runway, "by_account": byAccount, "cost_per_day": perDayOf(c.Daily), "agent_use": agentUse,
		"budget": budgetOf(budgets), "plan_cost_per_day": planPerDayOf(c.Plan.Daily, usage.ExtraDaily(all, firstDay(clock, hours), now))}
	if pick != nil {
		res["overlay"] = overlayOf(rows, c.Uses, *pick, out.Past, weights)
	}
	return res, nil
}

// budgetOf is stats.outlook's budget: each account's week in plan money, and the accounts together weighted by their budgets.
func budgetOf(bs []usage.AccountBudget) map[string]any {
	accts := []any{}
	for _, b := range bs {
		var used, spent, left any
		if s := b.Spent(); s != nil {
			used, spent, left = round(*b.UsedPct, 2), round(*s, 2), round(max(0, b.WeekUSD-*s), 2)
		}
		var plan any
		if b.Plan != "" {
			plan = b.Plan
		}
		accts = append(accts, map[string]any{"account": b.Account, "plan": plan, "fee_usd": round(b.FeeUSD, 2), "week_usd": round(b.WeekUSD, 2),
			"used_pct": used, "spent_usd": spent, "left_usd": left, "five_hour": b.FiveHour, "extra": b.Extra.Map()})
	}
	t := usage.TotalOf(bs)
	var leftPct any
	if t.LeftPct != nil {
		leftPct = round(*t.LeftPct, 2)
	}
	return map[string]any{"accounts": accts, "week_usd": round(t.WeekUSD, 2), "spent_usd": round(t.SpentUSD, 2),
		"left_usd": round(t.WeekUSD-t.SpentUSD, 2), "left_pct": leftPct, "extra_spent": rounded(usage.ExtraSpent(bs), 2)}
}

// outlookByAccount is stats.outlook's by_account, and its future: the accounts' weekly guess averaged by w, each going as
// usage.HeadroomKeepStale does. rows are the samples from at least a day back; weekUSD is each account's weekly plan budget.
func outlookByAccount(rows usage.Samples, now int64, hours int, ends, outSince, weekUSD map[string]float64, w usage.WeekWeights) ([]any, []usage.OutlookPoint) {
	span := int64(hours) * 3600
	h := usage.HeadroomKeepStale(rows, now, now-span, now+span, max(900, span*2/usage.OutlookPoints), ends, outSince)
	byAccount := []any{}
	var future []usage.OutlookPoint
	for _, a := range h.Accounts {
		resets := []any{}
		for _, r := range a.Resets {
			resets = append(resets, map[string]any{"at": ts(r.At), "window": r.Window})
		}
		var forecast, runsOut any
		if f := a.Forecast; f != nil {
			var usd any
			if week, ok := weekUSD[a.Account]; ok {
				usd = round(f.Unused/100*week, 2)
			}
			forecast = map[string]any{"at": ts(f.At), "unused_pct": round(f.Unused, 2), "unused_usd": usd}
		}
		if a.FiveRunsOut != nil {
			runsOut = ts(*a.FiveRunsOut)
		}
		byAccount = append(byAccount, map[string]any{"account": a.Account, "past": headroomWindows(a.Past, true), "future": headroomWindows(a.Future, true),
			"resets": resets, "five_hour": a.HasFive, "forecast": forecast, "five_runs_out_at": runsOut})
		for i, p := range a.Future {
			if len(future) <= i {
				future = append(future, usage.OutlookPoint{At: p.At})
			}
		}
	}
	names := make([]string, len(h.Accounts))
	for i, a := range h.Accounts {
		names[i] = a.Account
	}
	for i := range future {
		weeks := make([]*float64, len(h.Accounts))
		for j, a := range h.Accounts {
			weeks[j] = a.Future[i].Week
		}
		if avg, ok := w.MeanOf(names, weeks); ok {
			future[i].Left = &avg
		}
	}
	return byAccount, future
}
