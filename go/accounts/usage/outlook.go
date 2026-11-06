package usage

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/handline/handline/go/data/db"
)

// The weekly headroom of all accounts together, the last stretch and the next (handline/usage/outlook.py's weekly_outlook), and what the
// agents cost per day (stats.py's daily_cost). Past points are what usage_samples recorded; the future is a guess, kept apart. Both are
// pure given the rows, the rows being read by SamplesSince and TokenRows.

const (
	// OutlookPoints over the whole range, past and future.
	OutlookPoints = 96
	// OutlookMaxAgeS: a sample older than this no longer says what the account held.
	OutlookMaxAgeS = 2 * 3600
	// OutlookMinSpanS: less than this is not a pace.
	OutlookMinSpanS = 3600
)

// OutlookPoint is the headroom left at a moment, in percent; Left nil where no account had a reading.
type OutlookPoint struct {
	At   int64
	Left *float64
}

// Outlook is weekly_outlook's result: measured points and guessed ones, apart.
type Outlook struct {
	Past, Future []OutlookPoint
	Accounts     int
}

// weekPoint is the longest window one sample holds: when, the percent used, and when it resets (nil unknown).
type weekPoint struct {
	TS    int64
	Used  float64
	Reset *int64
	Out   bool // the account could not be used then (loggedOut)
}

func weekOf(r sampleRow) (weekPoint, bool) {
	var got map[string]any
	for _, e := range entries(r) { // shortest first, so the last that passes is the longest
		if _, has := e["used_percent"]; !has || e["used_percent"] == nil {
			continue
		}
		if v, has := e["readable"]; has && !truthy(v) {
			continue
		}
		if m, ok := isInt(e["minutes"]); !ok || m <= 300 || truthy(e["lost"]) {
			continue
		}
		got = e
	}
	if got == nil {
		return weekPoint{}, false
	}
	used, ok := num(got["used_percent"])
	ts, tsOK := num(r["ts"])
	if !ok || !tsOK {
		return weekPoint{}, false
	}
	return weekPoint{TS: int64(ts), Used: used, Reset: asInt(got["resets_at"]), Out: truthy(r["logged_out"])}, true
}

func sameResetPtr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// at is the latest sample at or before t, unless it is too old.
func at(series []weekPoint, t int64) (weekPoint, bool) {
	var best weekPoint
	found := false
	for _, s := range series {
		if s.TS > t {
			break
		}
		best, found = s, true
	}
	return best, found && t-best.TS <= OutlookMaxAgeS
}

// pace is the percent of the week used per second over the samples of the current week.
func pace(series []weekPoint) float64 {
	last := series[len(series)-1]
	var first *weekPoint
	for i := range series {
		if sameResetPtr(series[i].Reset, last.Reset) {
			first = &series[i]
			break
		}
	}
	span := last.TS - first.TS
	if span < OutlookMinSpanS {
		return 0
	}
	return max(0, (last.Used-first.Used)/float64(span))
}

// WeeklyOutlook is outlook.weekly_outlook. rows are usage_samples rows, oldest first. The value is the headroom left of the week, in
// percent, averaged over the accounts that have a reading. ends is when an account that ends stops: looking back it is not counted
// once it has ended; looking ahead it stays in the average with nothing left after its end, so the line steps down by its share.
// An account that could not be used (loggedOut; outSince is from when, for one that cannot now) counts as nothing left, then and ahead, so
// the line falls by its share while it is out and rises when it is back.
func WeeklyOutlook(rows []sampleRow, now int64, hours int, ends, outSince map[string]float64) Outlook {
	by := map[string][]weekPoint{}
	var order []string // first appearance, as a Python dict keeps it
	for _, r := range rows {
		if w, ok := weekOf(r); ok {
			a, _ := r["account"].(string)
			if _, seen := by[a]; !seen {
				order = append(order, a)
			}
			w.Out = loggedOut(r, outSince)
			by[a] = append(by[a], w)
		}
	}
	out := Outlook{Accounts: len(by)}
	if len(by) == 0 {
		return out
	}
	step := max(900, int64(hours)*3600*2/OutlookPoints)
	paces := map[string]float64{}
	for a, s := range by {
		paces[a] = pace(s)
	}
	endsAt := func(a string) (float64, bool) { v, ok := ends[a]; return v, ok }
	ahead := 0
	isAhead := map[string]bool{}
	for _, a := range order {
		e, ended := endsAt(a)
		if now-by[a][len(by[a])-1].TS <= OutlookMaxAgeS && !(ended && float64(now) >= e) {
			isAhead[a] = true
			ahead++
		}
	}
	for t := now - int64(hours)*3600; t <= now+int64(hours)*3600; t += step {
		var left []float64
		for _, a := range order {
			series := by[a]
			if e, ended := endsAt(a); ended && float64(t) >= e {
				if t > now && isAhead[a] {
					left = append(left, 0) // a capacity that is gone, not an account that is
				}
				continue
			}
			if t <= now {
				if s, ok := at(series, t); ok && s.Out {
					left = append(left, 0)
				} else if ok {
					left = append(left, 100-s.Used)
				}
				continue
			}
			last := series[len(series)-1]
			if now-last.TS > OutlookMaxAgeS {
				continue
			}
			if _, gone := outSince[a]; gone {
				left = append(left, 0)
				continue
			}
			used := last.Used
			if last.Reset != nil && *last.Reset != 0 && t >= *last.Reset {
				used = paces[a] * float64((t-*last.Reset)%(7*86400)) // and again each week after
			} else {
				used += paces[a] * float64(t-now)
			}
			left = append(left, 100-min(100, used))
		}
		p := OutlookPoint{At: t}
		n := len(left)
		if t > now {
			n = ahead
		}
		if len(left) > 0 && n > 0 {
			var sum float64
			for _, v := range left {
				sum += v
			}
			avg := sum / float64(n)
			p.Left = &avg
		}
		if t <= now {
			out.Past = append(out.Past, p)
		} else {
			out.Future = append(out.Future, p)
		}
	}
	return out
}

// SamplesSince is the usage_samples rows from since on, oldest first, as WeeklyOutlook reads them.
func SamplesSince(q db.Q, since int64) ([]sampleRow, error) { return readSamples(q, since) }

// SamplesFrom is the end of rows, as SamplesSince read them, from since on: what SamplesSince(since) reads, without reading again.
func SamplesFrom(rows Samples, since int64) Samples {
	i := sort.Search(len(rows), func(i int) bool { ts, _ := num(rows[i]["ts"]); return ts >= float64(since) })
	return rows[i:]
}

type worktreeOf struct{ dir, agent string }

// Unattributed names the calls whose directory no agent's worktree holds.
const Unattributed = "(unattributed)"

// worktrees are (directory, agent), longest directory first. The row may hold Orca's id::path selector, of which only the path is the
// directory; an agent's further worktrees are its too.
func worktrees(q db.Q) ([]worktreeOf, error) {
	rows, err := q.Query("SELECT agent, worktree, extra_worktrees FROM agent_state WHERE worktree IS NOT NULL AND worktree != ''")
	if err != nil {
		return nil, err
	}
	type r struct{ agent, tree, extra string }
	var got []r
	for rows.Next() {
		var x r
		var extra *string
		if err := rows.Scan(&x.agent, &x.tree, &extra); err != nil {
			rows.Close()
			return nil, err
		}
		if extra != nil {
			x.extra = *extra
		}
		got = append(got, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	var dirs []worktreeOf
	for _, x := range got {
		tree := x.tree
		if i := strings.Index(tree, "::"); i >= 0 {
			tree = tree[i+2:]
		}
		dirs = append(dirs, worktreeOf{strings.TrimRight(tree, "/"), x.agent})
	}
	for _, x := range got {
		var extra []any
		if x.extra != "" && json.Unmarshal([]byte(x.extra), &extra) != nil {
			extra = nil
		}
		for _, v := range extra {
			if s, ok := v.(string); ok && s != "" {
				dirs = append(dirs, worktreeOf{strings.TrimRight(s, "/"), x.agent})
			}
		}
	}
	sort.SliceStable(dirs, func(i, j int) bool { return utf8.RuneCountInString(dirs[i].dir) > utf8.RuneCountInString(dirs[j].dir) })
	return dirs, nil
}

func agentForCwd(cwd string, trees []worktreeOf) string {
	cwd = strings.TrimRight(cwd, "/")
	for _, t := range trees {
		if t.dir != "" && (cwd == t.dir || strings.HasPrefix(cwd, t.dir+"/")) {
			return t.agent
		}
	}
	return Unattributed
}

// DayCosts is stats.daily_cost: the list-price cost of each agent per local calendar day over (lo, hi], {"YYYY-MM-DD": {agent: USD}}, a
// day with no call absent.
func DayCosts(q db.Q, lo, hi int64, table []priceEntry) (map[string]map[string]float64, error) {
	daily := map[string]map[string]float64{}
	err := scanCalls(q, lo, hi, table, func(ts int64, k callKey, _ int64, cost float64) { addDay(daily, ts, k.agent, cost) })
	return daily, err
}

// CallCosts is DayCosts over (dayLo, hi] and stats.agent_use over (useLo, hi] (agentUses), from one read of the calls.
func CallCosts(q db.Q, dayLo, useLo, hi int64, table []priceEntry, weekUsed map[string]float64) (map[string]map[string]float64, []AgentUse, error) {
	daily := map[string]map[string]float64{}
	per := tally{}
	err := scanCalls(q, min(dayLo, useLo), hi, table, func(ts int64, k callKey, tokens int64, cost float64) {
		if ts > dayLo {
			addDay(daily, ts, k.agent, cost)
		}
		if ts > useLo {
			per.add(k, tokens, cost)
		}
	})
	if err != nil {
		return nil, nil, err
	}
	return daily, agentUses(per, per.keys(), weekUsed), nil
}

func addDay(daily map[string]map[string]float64, ts int64, agent string, cost float64) {
	day := time.Unix(ts, 0).In(time.Local).Format("2006-01-02")
	if daily[day] == nil {
		daily[day] = map[string]float64{}
	}
	daily[day][agent] += cost
}

// loggedOut is whether the sample was taken while its account could not be used: as it was recorded, or from outSince[account] on.
func loggedOut(r sampleRow, outSince map[string]float64) bool {
	if truthy(r["logged_out"]) {
		return true
	}
	a, _ := r["account"].(string)
	since, ok := outSince[a]
	ts, _ := num(r["ts"])
	return ok && ts >= since
}

// SampleAccounts are the accounts the rows name, sorted.
func SampleAccounts(rows []sampleRow) []string {
	seen := map[string]bool{}
	for _, r := range rows {
		if a, ok := r["account"].(string); ok {
			seen[a] = true
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}
