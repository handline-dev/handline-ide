package usage

import (
	"reflect"
	"testing"
)

type tp struct {
	At   int64
	Left any // float64, or nil at a break
}

func flat(ps []TracePoint) []tp {
	out := make([]tp, len(ps))
	for i, p := range ps {
		out[i] = tp{At: p.At}
		if p.Left != nil {
			out[i].Left = *p.Left
		}
	}
	return out
}

func TestAResetBetweenTwoReadingsDropsStraightDownAtItsTime(t *testing.T) {
	series := []windowPoint{
		{TS: 1000, Reset: 5000, Used: 60},
		{TS: 4000, Reset: 5000, Used: 80},
		{TS: 7000, Reset: 23000, Used: 10}, // the window reset at 5000, between the readings
	}
	got := flat(traceOf(series, 0, 8000))
	want := []tp{{1000, 40.0}, {4000, 20.0}, {5000, 20.0}, {5000, 100.0}, {7000, 90.0}, {8000, 90.0}} // the last holds to the end
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trace = %v, want %v", got, want)
	}
}

func TestAFallWithNoKnownResetIsStraightAtTheReadingThatShowsIt(t *testing.T) {
	series := []windowPoint{{TS: 1000, Used: 70}, {TS: 2000, Used: 5}}
	got := flat(traceOf(series, 0, 3000))
	want := []tp{{1000, 30.0}, {2000, 30.0}, {2000, 95.0}, {3000, 95.0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trace = %v, want %v", got, want)
	}
}

func TestASpanWithNoReadingsIsABreakAfterTheLastReadingHeldAndTheTraceStartsAtTheSpan(t *testing.T) {
	const late = 1500 + OutlookMaxAgeS + 1
	series := []windowPoint{
		{TS: 900, Used: 10}, // before the span, recent enough to start it
		{TS: 1500, Used: 20},
		{TS: late, Used: 30},
	}
	got := flat(traceOf(series, 1000, 20000))
	want := []tp{{1000, 90.0}, {1500, 80.0}, {late - 1, 80.0}, {late - 1, nil}, {late, 70.0}, {late + OutlookMaxAgeS, 70.0},
		{late + OutlookMaxAgeS, nil}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trace = %v, want %v", got, want)
	}
}

func TestThinningKeepsABucketsLowestAndHighestInTimeOrder(t *testing.T) {
	var ps []TracePoint
	for i, v := range []float64{50, 52, 90, 51, 10, 49, 48} { // one bucket
		ps = append(ps, TracePoint{At: int64(i), Left: leftOf(v)})
	}
	ps = append(ps, TracePoint{At: 50}, TracePoint{At: 60, Left: leftOf(30)}, TracePoint{At: 61, Left: leftOf(30)},
		TracePoint{At: 62, Left: leftOf(30)})
	got := flat(thin(ps, 0, 99, 10))
	want := []tp{{0, 50.0}, {2, 90.0}, {4, 10.0}, {6, 48.0}, {50, nil}, {60, 30.0}, {62, 30.0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("thin = %v, want %v", got, want)
	}
}

func TestTheMeanTraceStepsByTheResettingAccountsWeight(t *testing.T) {
	ultra := AccountHeadroom{Account: "u", weekTrace: []TracePoint{{At: 0, Left: leftOf(20)}, {At: 100, Left: leftOf(20)},
		{At: 100, Left: leftOf(100)}, {At: 200, Left: leftOf(100)}}}
	pro := AccountHeadroom{Account: "p", weekTrace: []TracePoint{{At: 0, Left: leftOf(50)}, {At: 200, Left: leftOf(50)}}}
	w := WeekWeights{"u": 9, "p": 1}
	got := flat(MeanTrace([]AccountHeadroom{ultra, pro}, w, 0, 200))
	want := []tp{{0, 23.0}, {100, 23.0}, {100, 95.0}, {200, 95.0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mean = %v, want %v", got, want)
	}
}

func TestThinningKeepsBothEndsOfAResetInsideABucket(t *testing.T) {
	ps := []TracePoint{{At: 0, Left: leftOf(10)}, {At: 1, Left: leftOf(8)}, {At: 2, Left: leftOf(7)}, {At: 3, Left: leftOf(7)},
		{At: 3, Left: leftOf(100)}, {At: 4, Left: leftOf(99)}}
	got := flat(thin(ps, 0, 99, 10))
	want := []tp{{0, 10.0}, {2, 7.0}, {3, 7.0}, {3, 100.0}, {4, 99.0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("thin = %v, want %v", got, want)
	}
}

func TestAGapRightAfterTheSpanStartsBreaksInsideTheSpan(t *testing.T) {
	got := flat(traceOf([]windowPoint{{TS: 900, Used: 10}, {TS: 15400, Used: 20}}, 1000, 15400))
	want := []tp{{1000, 90.0}, {900 + OutlookMaxAgeS, 90.0}, {900 + OutlookMaxAgeS, nil}, {15400, 80.0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trace = %v, want %v", got, want)
	}
	if got := traceOf([]windowPoint{{TS: 900, Used: 10}}, 1000, 950); got != nil {
		t.Fatalf("a span that ends before it starts has no trace, got %v", got)
	}
}

func TestTheMeanTraceStepsWhereOneAccountsReadingsEnd(t *testing.T) {
	a := AccountHeadroom{Account: "a", weekTrace: []TracePoint{{At: 0, Left: leftOf(10)}, {At: 90, Left: leftOf(10)}, {At: 90}}}
	b := AccountHeadroom{Account: "b", weekTrace: []TracePoint{{At: 0, Left: leftOf(90)}, {At: 100, Left: leftOf(90)}}}
	got := flat(MeanTrace([]AccountHeadroom{a, b}, nil, 0, 100))
	want := []tp{{0, 50.0}, {90, 50.0}, {90, 90.0}, {100, 90.0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mean = %v, want %v", got, want)
	}
}
