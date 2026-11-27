package usage

import (
	"slices"
	"sort"
)

// A window's readings as a chart line: the points drawn joined by straight lines, so a reset is two points at one time and draws as a
// vertical drop, and a span with no reading is a break. The daemon decides the shape; a chart draws the points as given.

// TraceBuckets is how many time buckets a trace is thinned to: about a chart's width in pixels.
const TraceBuckets = 240

// TracePoint is what a window had left at At, in percent; Left nil breaks the line there.
type TracePoint = OutlookPoint

func levelOf(p windowPoint) float64 {
	if p.Out {
		return 0
	}
	return 100 - p.Used
}

// traceOf is series from from to to: the reading at or before from (within OutlookMaxAgeS) starts it at from, each reading after is a
// point, a reset between two readings is the level before and 100 at its time, and a rise of what is left with no known reset is straight
// up at the reading that shows it. A reading holds for OutlookMaxAgeS, as the grid's did: the last one to to, or until it is too old and
// a break follows; so does one followed by none for longer. Not thinned.
func traceOf(series []windowPoint, from, to int64) []TracePoint {
	if to < from {
		return nil
	}
	var out []TracePoint
	at := func(t int64, v float64) { out = append(out, TracePoint{At: t, Left: leftOf(v)}) }
	// hold draws prev's level on to until, at most OutlookMaxAgeS after it, then breaks where it ran out.
	hold := func(prev windowPoint, until int64) {
		end := min(until, prev.TS+OutlookMaxAgeS)
		if end > out[len(out)-1].At {
			at(end, levelOf(prev))
		}
		if end < until {
			out = append(out, TracePoint{At: end})
		}
	}
	var prev *windowPoint
	for i := range series {
		p := series[i]
		if p.TS > to {
			break
		}
		if p.TS < from {
			prev = nil
			if from-p.TS <= OutlookMaxAgeS {
				prev = &series[i]
			}
			continue
		}
		if len(out) == 0 && prev != nil {
			at(from, levelOf(*prev))
		}
		switch {
		case prev == nil:
		case p.TS-prev.TS > OutlookMaxAgeS:
			hold(*prev, p.TS)
		case prev.Reset != 0 && prev.Reset != p.Reset && prev.Reset <= p.TS && !p.Out:
			r := min(max(prev.Reset, prev.TS, from), p.TS)
			at(r, levelOf(*prev))
			at(r, 100)
		case levelOf(p) > levelOf(*prev):
			at(p.TS, levelOf(*prev))
		}
		at(p.TS, levelOf(p))
		prev = &series[i]
	}
	if prev != nil {
		if len(out) == 0 {
			at(from, levelOf(*prev))
		}
		hold(*prev, to)
	}
	return out
}

// thin keeps, of each run of points in one of buckets equal spans of from..to, the first, the lowest, the highest, the last and both ends
// of each vertical step, in time order, so no edge is lost at that width; a break is always kept, and the middle of three equal points in a
// row goes.
func thin(ps []TracePoint, from, to int64, buckets int) []TracePoint {
	span := max(1, to-from+1)
	bucket := func(t int64) int64 { return (t - from) * int64(buckets) / span }
	var kept []TracePoint
	for i := 0; i < len(ps); {
		if ps[i].Left == nil {
			kept = append(kept, ps[i])
			i++
			continue
		}
		j := i
		lo, hi := i, i
		for j < len(ps) && ps[j].Left != nil && bucket(ps[j].At) == bucket(ps[i].At) {
			if *ps[j].Left < *ps[lo].Left {
				lo = j
			}
			if *ps[j].Left > *ps[hi].Left {
				hi = j
			}
			j++
		}
		idx := []int{i, lo, hi, j - 1}
		for k := i + 1; k < j; k++ { // both ends of a vertical step
			if ps[k].At == ps[k-1].At {
				idx = append(idx, k-1, k)
			}
		}
		sort.Ints(idx)
		for k, x := range idx {
			if k == 0 || x != idx[k-1] {
				kept = append(kept, ps[x])
			}
		}
		i = j
	}
	out := kept[:0:0]
	for i, p := range kept {
		if i > 0 && i < len(kept)-1 && p.Left != nil && kept[i-1].Left != nil && kept[i+1].Left != nil &&
			*p.Left == *kept[i-1].Left && *p.Left == *kept[i+1].Left {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Trace is series from from to to as a chart line, thinned to TraceBuckets.
func trace(series []windowPoint, from, to int64) []TracePoint {
	return thin(traceOf(series, from, to), from, to, TraceBuckets)
}

// cursor reads one trace at rising times: k is the first point at or after the last time asked.
type cursor struct {
	ps []TracePoint
	k  int
}

// at is the trace's level just before t and just after it, by straight lines between its points; ok false on a side in a break, before
// its first point or after its last. Times asked must not fall.
func (c *cursor) at(t int64) (before, after float64, okB, okA bool) {
	ps := c.ps
	for c.k < len(ps) && ps[c.k].At < t {
		c.k++
	}
	k := c.k
	if k < len(ps) && ps[k].At == t {
		if k > 0 && ps[k-1].Left != nil && ps[k].Left != nil {
			before, okB = *ps[k].Left, true
		}
		e := k
		for e+1 < len(ps) && ps[e+1].At == t {
			e++
		}
		if e+1 < len(ps) && ps[e].Left != nil && ps[e+1].Left != nil {
			after, okA = *ps[e].Left, true
		}
		return
	}
	if k == 0 || k >= len(ps) || ps[k-1].Left == nil || ps[k].Left == nil {
		return
	}
	a, b := ps[k-1], ps[k]
	v := *a.Left + (*b.Left-*a.Left)*float64(t-a.At)/float64(b.At-a.At)
	return v, v, true, true
}

// MeanTrace is the accounts' week traces read together, weighed by w: at each time a trace has a point, the mean just before it and,
// where that differs, just after it, so one account's reset, or its first or last reading, is a vertical step; a break where no account
// counts. Thinned to TraceBuckets over from..to.
func MeanTrace(accounts []AccountHeadroom, w WeekWeights, from, to int64) []TracePoint {
	var times []int64
	cs := make([]cursor, len(accounts))
	for i, a := range accounts {
		cs[i].ps = a.weekTrace
		for _, p := range a.weekTrace {
			times = append(times, p.At)
		}
	}
	slices.Sort(times)
	times = slices.Compact(times)
	ls := make([]accountLeft, 0, len(accounts))
	lsA := make([]accountLeft, 0, len(accounts))
	var out []TracePoint
	for _, t := range times {
		ls, lsA = ls[:0], lsA[:0]
		for i := range cs {
			b, a, okB, okA := cs[i].at(t)
			if okB {
				ls = append(ls, accountLeft{accounts[i].Account, b})
			}
			if okA {
				lsA = append(lsA, accountLeft{accounts[i].Account, a})
			}
		}
		before, hasB := w.Mean(ls)
		after, hasA := w.Mean(lsA)
		if hasB {
			out = append(out, TracePoint{At: t, Left: leftOf(before)})
		}
		if hasA && (!hasB || after != before) {
			out = append(out, TracePoint{At: t, Left: leftOf(after)})
		}
		if !hasA && len(out) > 0 && out[len(out)-1].Left != nil {
			out = append(out, TracePoint{At: t})
		}
	}
	if n := len(out); n > 0 && out[n-1].Left == nil {
		out = out[:n-1]
	}
	return thin(out, from, to, TraceBuckets)
}
