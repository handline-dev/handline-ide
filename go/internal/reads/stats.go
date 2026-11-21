package reads

import (
	"slices"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

var statsWindows = map[string]int64{"24h": 24 * 3600, "7d": 7 * 24 * 3600}

func (e *Env) countEvents(kind string, since, until int64) (int64, error) {
	r, err := queryOne(e.DB, "SELECT COUNT(*) AS n FROM events WHERE kind = ? AND ts >= ? AND ts <= ?", kind, since, until)
	return r.i("n"), err
}

// statsUse is stats.get's use: a row per agent and account, the account's weekly rise split by list-price cost.
func (e *Env) statsUse(since, until int64) ([]any, error) {
	ue := &usage.Env{Home: e.Home, Now: func() float64 { return float64(until) }}
	rows, err := usage.SamplesSince(e.DB, since-usage.OutlookMaxAgeS)
	if err != nil {
		return nil, err
	}
	uses, err := usage.PairUsesOf(e.DB, since, until, usage.PricesFromConfig(ue.CurrentConfig()), usage.WeekConsumed(rows, since, until))
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, u := range uses {
		var week any
		if u.WeekPct != nil {
			week = round(*u.WeekPct, 2)
		}
		out = append(out, map[string]any{"agent": u.Agent, "account": u.Account, "tokens": u.Tokens, "cost": round(u.Cost, 4), "share": round(u.Share, 4), "week_pct": week})
	}
	return out, nil
}

// statsGet is stats.get: what the Stats screen draws for the last 24 hours or 7 days.
func (e *Env) statsGet(_ *rpc.Conn, params map[string]any) (any, error) {
	window, _ := params["window"].(string)
	span, ok := statsWindows[window]
	if !ok {
		return nil, badParam("window is 24h or 7d", "window")
	}
	until := time.Now().Unix()
	since := until - span
	done, err := queryOne(e.DB, "SELECT COUNT(*) AS n FROM tasks WHERE done_at >= ? AND done_at <= ?", since, until)
	if err != nil {
		return nil, err
	}
	per := []any{}
	for _, a := range paths.ListAccounts(e.Home) {
		st := usage.ReadStatus(e.Home, a)
		pct := func(w usage.Window) any {
			if w.Pct == nil {
				return nil
			}
			return float64(*w.Pct)
		}
		per = append(per, map[string]any{"account": a, "five_hour_used_pct": pct(st.Short), "weekly_used_pct": pct(st.Long)})
	}
	use, err := e.statsUse(since, until)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"window": window, "since": ts(since), "until": ts(until), "tasks_done": done.i("n"), "use": use, "per_account": per}
	for key, kind := range map[string]string{"moves": "moved_account", "handoffs": "task_handoff", "blocks": "block_task"} {
		if out[key], err = e.countEvents(kind, since, until); err != nil {
			return nil, err
		}
	}
	if out["your_wait_s"], err = e.personWait(since, until); err != nil {
		return nil, err
	}
	return out, nil
}

// personWait is the median seconds from a turn put on a person to that person giving it back, over turns given back in [since, until]
// (the upper one of an even count); nil when none was.
func (e *Env) personWait(since, until int64) (any, error) {
	rows, err := query(e.DB, "SELECT json_extract(p.payload, '$.agent') AS who, p.ts - (SELECT MAX(q.ts) FROM events q "+
		"WHERE q.kind = 'task_turn_push' AND q.ts <= p.ts AND json_valid(q.payload) "+
		"AND json_extract(q.payload, '$.task_id') = json_extract(p.payload, '$.task_id') "+
		"AND json_extract(q.payload, '$.to') = json_extract(p.payload, '$.agent')) AS wait "+
		"FROM events p WHERE p.kind = 'task_turn_pop' AND p.ts >= ? AND p.ts <= ? AND json_valid(p.payload)", since, until)
	if err != nil {
		return nil, err
	}
	waits := []int64{}
	for _, r := range rows {
		if actor.IsPerson(r.str("who")) && r.set("wait") {
			waits = append(waits, r.i("wait"))
		}
	}
	if len(waits) == 0 {
		return nil, nil
	}
	slices.Sort(waits)
	return waits[len(waits)/2], nil
}
