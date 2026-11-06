package reads

import (
	"sort"
	"strconv"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/team"
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
func runsOut(e *usage.Env, st *usage.AccountStatus, acct string, now int64, ld *usage.Load) any {
	in, _, ok := wallOf(e, st, acct, now, ld)
	if !ok {
		return nil
	}
	return in
}

// wallOf is when the account reaches a limit, in seconds from now (0: it is at one), and when that limit's window resets; ok false when
// there is no reading, no burn to go by, or the window resets first.
func wallOf(e *usage.Env, st *usage.AccountStatus, acct string, now int64, ld *usage.Load) (in, reset int64, ok bool) {
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

// statsCost is stats.cost: stats.outlook's cost_per_day alone.
func (e *Env) statsCost(_ *rpc.Conn, params map[string]any) (any, error) {
	name, _ := params["range"].(string)
	hours, ok := outlookHours[name]
	if !ok {
		return nil, badParam("range is one of "+outlookRanges, "range")
	}
	return e.costAt(name, hours, time.Now())
}

func (e *Env) costAt(name string, hours int, clock time.Time) (any, error) {
	now := clock.Unix()
	ue := &usage.Env{Home: e.Home, Now: func() float64 { return float64(now) }}
	cost, err := usage.DayCosts(e.DB, firstDay(clock, hours), now, usage.PricesFromConfig(ue.CurrentConfig()))
	if err != nil {
		return nil, err
	}
	return map[string]any{"range": name, "cost_per_day": perDayOf(cost)}, nil
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
	return map[string]any{"agent": u.Agent, "tokens": u.Tokens, "cost": round(u.Cost, 4), "share": round(u.Share, 4), "est_pct": est, "fracs": fracs}
}

// overlayOf is the spec's overlay, at two places like left_pct.
func overlayOf(rows usage.Samples, uses []usage.AgentUse, pick string, past []usage.OutlookPoint) []any {
	out := []any{}
	for _, v := range usage.Overlay(rows, uses, pick, past) {
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
		agents := map[string]any{}
		for a, c := range cost[d] {
			agents[a] = round(c, 4)
		}
		out = append(out, map[string]any{"day": d, "agents": agents})
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
	rows := usage.SamplesFrom(all, now-span-usage.OutlookMaxAgeS)
	ends, outSince := e.accountEnds(cfg, rows, clock)
	out := usage.WeeklyOutlook(rows, now, hours, ends, outSince)
	byAccount, future, err := e.outlookByAccount(ue, cfg, all, now, hours, ends, outSince)
	if err != nil {
		return nil, err
	}
	cost, uses, err := usage.CallCosts(e.DB, firstDay(clock, hours), now-span, now, usage.PricesFromConfig(cfg), usage.WeekConsumed(rows, now-span, now))
	if err != nil {
		return nil, err
	}
	loads, err := ue.ReadAll(e.DB, "")
	if err != nil {
		return nil, err
	}
	runway := []any{}
	for _, a := range paths.ListAccounts(e.Home) {
		st := ue.Read(a)
		var runs any
		if !st.Unmetered {
			runs = runsOut(ue, st, a, now, loads[a])
		}
		var resets any
		if st.Long.Reset != 0 {
			resets = ts(st.Long.Reset)
		}
		runway = append(runway, map[string]any{"account": a, "runs_out_in_seconds": runs, "week_resets_at": resets})
	}
	agentUse := []any{}
	for _, u := range uses {
		agentUse = append(agentUse, agentUseOf(u))
	}
	res := map[string]any{"range": name, "now": ts(now), "accounts": out.Accounts, "past": points(out.Past), "future": points(future),
		"runway": runway, "by_account": byAccount, "cost_per_day": perDayOf(cost), "agent_use": agentUse}
	if pick != nil {
		res["overlay"] = overlayOf(rows, uses, *pick, out.Past)
	}
	return res, nil
}

// outlookByAccount is stats.outlook's by_account, and its future: the accounts' weekly guess averaged, each going as usage.Paced does.
// rows are the samples from at least a day back.
func (e *Env) outlookByAccount(ue *usage.Env, cfg usage.Config, rows usage.Samples, now int64, hours int, ends, outSince map[string]float64) ([]any, []usage.OutlookPoint, error) {
	span := int64(hours) * 3600
	costs, _ := ue.LoadCosts(e.DB) // unread costs leave the 5h guess unpaced
	ps := usage.PaceSettingsOf(cfg)
	paced := usage.Paced{On: ps.On, Target: float64(ps.Target), Ratio: func(a string) (float64, bool) {
		if costs == nil {
			return 0, false
		}
		return costs.LongPerShort(a)
	}}
	h := usage.HeadroomPaced(rows, now, now-span, now+span, max(900, span*2/usage.OutlookPoints), ends, outSince, paced)
	byAccount := []any{}
	var future []usage.OutlookPoint
	for _, a := range h.Accounts {
		resets := []any{}
		for _, r := range a.Resets {
			resets = append(resets, map[string]any{"at": ts(r.At), "window": r.Window})
		}
		byAccount = append(byAccount, map[string]any{"account": a.Account, "past": headroomWindows(a.Past, true), "future": headroomWindows(a.Future, true),
			"resets": resets})
		for i, p := range a.Future {
			if len(future) <= i {
				future = append(future, usage.OutlookPoint{At: p.At})
			}
		}
	}
	for i := range future {
		var sum float64
		n := 0
		for _, a := range h.Accounts {
			if w := a.Future[i].Week; w != nil {
				sum, n = sum+*w, n+1
			}
		}
		if n > 0 {
			avg := sum / float64(n)
			future[i].Left = &avg
		}
	}
	return byAccount, future, nil
}
