package usage

import "sort"

// The headroom of each account over a span, its 5h and weekly windows apart (stats.outlook draws them): what usage_samples recorded up to now, and
// a guess after it. The guess keeps each window's pace and starts a window again from nothing at its reset, so the 5h line is a saw.

const (
	fiveS = 5 * 3600
	// headroomPaceS: the 5h pace is what was used over the last day, so one busy or idle window does not set it.
	headroomPaceS = 86400
)

// HeadroomPoint is what an account had left at a moment, in percent; nil where it had no reading. Line is what its pace line leaves of
// the week (Paced only).
type HeadroomPoint struct {
	At               int64
	Five, Week, Line *float64
}

// Left is the headroom that binds: the smaller of the two windows, the week alone for an account with no 5-hour window.
func (p HeadroomPoint) Left() *float64 {
	if p.Week == nil {
		return nil
	}
	if p.Five == nil {
		return p.Week
	}
	v := min(*p.Five, *p.Week)
	return &v
}

// HeadroomReset is a window's reset: Window is "5h" or "7d".
type HeadroomReset struct {
	At     int64
	Window string
	// Unused is, for a weekly reset the samples recorded, the share of that week left at the last sample before it (HasUnused false for a
	// 5h reset, a guessed one, or a week with no sample): what the week left on the table.
	Unused    float64
	HasUnused bool
}

// AccountHeadroom is one account's points, past then future, and its resets in the span, in time order. Five is nil throughout for an
// account whose readings carry no 5-hour figure (HasFive false: a verified Ultra has no 5-hour limit).
type AccountHeadroom struct {
	Account      string
	Past, Future []HeadroomPoint
	Resets       []HeadroomReset
	HasFive      bool
	// Forecast is the guess at the week's next reset, nil when there is none to make: a reading older than OutlookMaxAgeS, an account
	// out or ending before it, or no reset known.
	Forecast *WeekForecast
	// FiveRunsOut is when the 5-hour window runs out at its pace, nil when it lasts to its reset, is not in use, or there is none.
	FiveRunsOut *int64
	// FiveTrace and WeekTrace are its readings from the span's start to now as chart lines (Trace); weekTrace is WeekTrace unthinned.
	FiveTrace, WeekTrace []TracePoint
	weekTrace            []TracePoint
}

// WeekForecast is what a week is guessed to leave unused at its reset At, in percent.
type WeekForecast struct {
	At     int64
	Unused float64
}

// HeadroomSpan is every account's, and their average at each point (Left nil where no account had a value).
type HeadroomSpan struct {
	Accounts     []AccountHeadroom
	Past, Future []OutlookPoint
}

// windowPoint is one window of a sample: when, the percent used, and when it resets (0 unknown).
type windowPoint struct {
	TS, Reset int64
	Used      float64
	Out       bool // the account could not be used then (loggedOut)
}

// windowsOf is a sample's 5h window (the shortest of 5 hours or less) and its weekly one (the longest above), as weekOf reads them.
func windowsOf(r sampleRow) (five, week windowPoint, hasFive, hasWeek bool) {
	ts, ok := num(r["ts"])
	if !ok {
		return
	}
	for _, e := range entries(r) {
		if e["used_percent"] == nil || truthy(e["lost"]) || staleWindow(r, e) {
			continue
		}
		if v, has := e["readable"]; has && !truthy(v) {
			continue
		}
		used, ok := num(e["used_percent"])
		m, isM := isInt(e["minutes"])
		if !ok || !isM {
			continue
		}
		var reset int64
		if p := asInt(e["resets_at"]); p != nil {
			reset = (*p + 30) / 60 * 60 // the same reset comes back a second apart
		}
		w := windowPoint{TS: int64(ts), Reset: reset, Used: used, Out: truthy(r["logged_out"])}
		switch {
		case m <= 300 && !hasFive:
			five, hasFive = w, true
		case m > 300:
			week, hasWeek = w, true
		}
	}
	return
}

// windowAt is the latest point at or before t, unless it is too old to say what the account held.
func windowAt(series []windowPoint, t int64) (windowPoint, bool) {
	i := sort.Search(len(series), func(i int) bool { return series[i].TS > t })
	if i == 0 || t-series[i-1].TS > OutlookMaxAgeS {
		return windowPoint{}, false
	}
	return series[i-1], true
}

// usedPerS is the percent used per second from since to the last point: what each step added within one window, and what a new window
// shows at its first point; 0 over less than OutlookMinSpanS.
func usedPerS(series []windowPoint, since int64) float64 {
	i := sort.Search(len(series), func(i int) bool { return series[i].TS >= since })
	if i >= len(series) || series[len(series)-1].TS-series[i].TS < OutlookMinSpanS {
		return 0
	}
	var used float64
	for j := i + 1; j < len(series); j++ {
		if series[j].Reset == series[j-1].Reset {
			used += max(0, series[j].Used-series[j-1].Used)
		} else {
			used += series[j].Used
		}
	}
	return used / float64(series[len(series)-1].TS-series[i].TS)
}

// ahead is a window's guess at t after now, from its last point: used at pace until its reset, then from nothing again each period.
func ahead(last windowPoint, pace float64, now, t, period int64) float64 {
	reset := last.Reset
	if reset == 0 {
		reset = now + period // no window open: the next one opens with the next use
	}
	used := last.Used + pace*float64(t-now)
	if t >= reset {
		used = pace * float64((t-reset)%period)
	}
	return 100 - min(100, used)
}

// resetsAhead are a window's resets from after now to to: the one its last point names and, while it is used, one each period after.
func resetsAhead(last windowPoint, pace float64, now, to, period int64, window string) []HeadroomReset {
	var out []HeadroomReset
	for r := last.Reset; r != 0 && r <= to; r += period {
		if r > now {
			out = append(out, HeadroomReset{At: r, Window: window})
		}
		if pace == 0 {
			break
		}
	}
	return out
}

func leftOf(v float64) *float64 { return &v }

// HeadroomOver is the headroom from from to to every step seconds; a point after now is a guess. rows are usage_samples rows, oldest first,
// reaching back a day before now for the pace. ends is when an account that ends stops: looking back it has no reading once ended,
// looking ahead nothing left. An account whose last reading is stale has no guess, and the average ahead leaves it out. An account that
// could not be used (loggedOut; outSince is from when, for one that cannot now) has nothing left then, and ahead.
func HeadroomOver(rows []sampleRow, now, from, to, step int64, ends, outSince map[string]float64) HeadroomSpan {
	return headroomOver(rows, now, from, to, step, ends, outSince, false)
}

// HeadroomKeepStale is HeadroomOver as stats.outlook's by_account guesses: the week at its pace since it began (sinceWeekBegan), the 5-hour
// window at its use over the last day, and an account whose last reading is stale keeps it until its reset. rows reach back a day.
func HeadroomKeepStale(rows []sampleRow, now, from, to, step int64, ends, outSince map[string]float64) HeadroomSpan {
	return headroomOver(rows, now, from, to, step, ends, outSince, true)
}

func headroomOver(rows []sampleRow, now, from, to, step int64, ends, outSince map[string]float64, keepStale bool) HeadroomSpan {
	fives, weeks := map[string][]windowPoint{}, map[string][]windowPoint{}
	for _, r := range rows {
		a, _ := r["account"].(string)
		five, week, hasFive, hasWeek := windowsOf(r)
		if !hasWeek {
			continue
		}
		week.Out = loggedOut(r, outSince)
		weeks[a] = append(weeks[a], week)
		if hasFive {
			five.Out = week.Out
			fives[a] = append(fives[a], five)
		}
	}
	accounts := make([]string, 0, len(weeks))
	for a := range weeks {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)
	out := HeadroomSpan{}
	for _, a := range accounts {
		f, w := fives[a], weeks[a]
		end, ended := ends[a]
		_, outNow := outSince[a]
		h := AccountHeadroom{Account: a, HasFive: len(f) > 0}
		seen := map[HeadroomReset]bool{}
		lastUsed := map[int64]float64{} // a week's last recorded use, by its reset
		for _, p := range w {
			lastUsed[p.Reset] = p.Used
		}
		recorded := func(r HeadroomReset) {
			if r.At != 0 && r.At >= from && r.At <= min(now, to) && !seen[r] {
				seen[r] = true
				h.Resets = append(h.Resets, r)
			}
		}
		for _, p := range f {
			recorded(HeadroomReset{At: p.Reset, Window: "5h"})
		}
		for _, p := range w {
			unused, has := lastUsed[p.Reset]
			recorded(HeadroomReset{At: p.Reset, Window: "7d", Unused: max(0, 100-unused), HasUnused: has})
		}
		read := min(now, to) // the traces end at the last moment it could have a reading
		if ended {
			read = min(read, int64(end))
		}
		h.FiveTrace = trace(f, from, read)
		h.weekTrace = traceOf(w, from, read)
		h.WeekTrace = thin(h.weekTrace, from, read, TraceBuckets)
		lastW := w[len(w)-1]
		var lastF windowPoint
		if h.HasFive {
			lastF = f[len(f)-1]
		}
		fresh := now-lastW.TS <= OutlookMaxAgeS && !(ended && float64(now) >= end)
		paceF, paceW := usedPerS(f, now-headroomPaceS), pace(toWeekPoints(w))
		if keepStale {
			fresh = !(ended && float64(now) >= end)
			paceW = 0 // a reading hours old, or one at the wall, keeps its figure until its reset
			if now-lastW.TS <= OutlookMaxAgeS {
				paceW = sinceWeekBegan(w, now)
			}
		}
		for t := from; t <= to; t += step {
			p := HeadroomPoint{At: t}
			gone := ended && float64(t) >= end
			switch {
			case t <= now:
				if wp, ok := windowAt(w, t); ok && !gone {
					p.Week = leftOf(100 - wp.Used)
					if fp, ok := windowAt(f, t); ok {
						p.Five = leftOf(100 - fp.Used)
					}
					if wp.Out {
						p.Five, p.Week = h.five(0), leftOf(0)
					}
				}
				h.Past = append(h.Past, p)
				continue
			case !fresh:
			case gone, outNow:
				p.Five, p.Week = h.five(0), leftOf(0)
			default:
				p.Five, p.Week = h.five(ahead(lastF, paceF, now, t, fiveS)), leftOf(ahead(lastW, paceW, now, t, WeekS))
			}
			h.Future = append(h.Future, p)
		}
		if fresh && !outNow {
			until := to
			if ended {
				until = min(to, int64(end))
			}
			if h.HasFive {
				h.Resets = append(h.Resets, resetsAhead(lastF, paceF, now, until, fiveS, "5h")...)
			}
			h.Resets = append(h.Resets, resetsAhead(lastW, paceW, now, until, WeekS, "7d")...)
			if r := lastW.Reset; r != 0 && now-lastW.TS <= OutlookMaxAgeS {
				used := lastW.Used + paceW*float64(r-now)
				for ; r <= now; r += WeekS { // the reset passed since the last reading: the next one, a whole week at pace
					used = paceW * WeekS
				}
				if !(ended && end < float64(r)) {
					h.Forecast = &WeekForecast{At: r, Unused: 100 - min(100, used)}
				}
			}
			if h.HasFive && now-lastF.TS <= OutlookMaxAgeS {
				h.FiveRunsOut = runsOutAt(lastF, paceF, now)
			}
		}
		sort.SliceStable(h.Resets, func(i, j int) bool { return h.Resets[i].At < h.Resets[j].At })
		out.Accounts = append(out.Accounts, h)
	}
	for i, t := 0, from; t <= to; i, t = i+1, t+step {
		p := averaged(out.Accounts, i, t)
		if t <= now {
			out.Past = append(out.Past, p)
		} else {
			out.Future = append(out.Future, p)
		}
	}
	return out
}

// five is v for an account with a 5-hour window, nil for one with none.
func (h AccountHeadroom) five(v float64) *float64 {
	if !h.HasFive {
		return nil
	}
	return leftOf(v)
}

// sinceWeekBegan is the week's pace since it began: what its last point has used over the time since its reset a week before. A reset not
// known, or a week begun less than OutlookMinSpanS before, goes at its use over the last day instead. A burst day moves it by its share of
// the week so far, where the last day's pace takes it whole.
func sinceWeekBegan(w []windowPoint, now int64) float64 {
	last := w[len(w)-1]
	if span := last.TS - (last.Reset - WeekS); last.Reset != 0 && span >= OutlookMinSpanS {
		return last.Used / float64(span)
	}
	return usedPerS(w, now-headroomPaceS)
}

// runsOutAt is when a 5-hour window at last runs out going at pace since it: now when it already has, nil when it lasts to its reset or
// is not in use.
func runsOutAt(last windowPoint, pace float64, now int64) *int64 {
	if last.Reset <= now {
		return nil
	}
	used := last.Used + pace*float64(now-last.TS)
	if used >= 100 {
		return &now
	}
	if pace <= 0 {
		return nil
	}
	t := now + int64((100-used)/pace)
	if t >= last.Reset {
		return nil
	}
	return &t
}

// toWeekPoints is the weekly points as pace reads them, so the weekly guess goes as stats.outlook's does.
func toWeekPoints(series []windowPoint) []weekPoint {
	out := make([]weekPoint, len(series))
	for i, s := range series {
		reset := s.Reset
		out[i] = weekPoint{TS: s.TS, Used: s.Used, Reset: &reset, Out: s.Out}
	}
	return out
}

// averaged is the mean of the accounts' binding headroom at the i-th point, t, over those that have one.
func averaged(accounts []AccountHeadroom, i int, t int64) OutlookPoint {
	out := OutlookPoint{At: t}
	var sum float64
	n := 0
	for _, h := range accounts {
		p := HeadroomPoint{}
		if i < len(h.Past) {
			p = h.Past[i]
		} else {
			p = h.Future[i-len(h.Past)]
		}
		if l := p.Left(); l != nil {
			sum += *l
			n++
		}
	}
	if n > 0 {
		out.Left = leftOf(sum / float64(n))
	}
	return out
}
