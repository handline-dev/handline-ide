package usage

import (
	"os"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

// accountAt writes account "a" with its 5h window at shortUsed percent resetting in shortMin minutes and its week at longUsed resetting in
// longS seconds.
func accountAt(t *testing.T, home string, shortUsed int, shortMin int64, longUsed int, longS int64) {
	t.Helper()
	writeStatus(t, home, "a", map[string]any{"collected_at": clock - 5, "windows": []any{
		map[string]any{"minutes": 300, "used_percent": shortUsed, "resets_at": clock + shortMin*60, "observed_at": clock - 5},
		map[string]any{"minutes": 10080, "used_percent": longUsed, "resets_at": clock + longS, "observed_at": clock - 5}}})
}

// weekAt writes an account whose week is half over (it resets in half a week) with the given weekly figure.
func weekAt(t *testing.T, home, acct string, used int) {
	t.Helper()
	writeStatus(t, home, acct, map[string]any{"collected_at": clock - 5, "windows": []any{
		map[string]any{"minutes": 300, "used_percent": 10, "resets_at": clock + 3600, "observed_at": clock - 5},
		map[string]any{"minutes": 10080, "used_percent": used, "resets_at": clock + WeekS/2, "observed_at": clock - 5}}})
}

func TestTheBucketHoldsTheWeeksElapsedShareAndABurstLessWhatIsSpent(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	weekAt(t, home, "a", 50)
	h, ok := e.Headroom("a")
	if !ok || h < 0.0999 || h > 0.1001 {
		t.Fatalf("half the week and a 10%% burst, 50%% spent: %v %v", h, ok)
	}
	weekAt(t, home, "a", 60)
	if h, _ := e.Headroom("a"); h < -0.0001 || h > 0.0001 {
		t.Fatalf("spending the burst empties the bucket: %v", h)
	}
	writeStatus(t, home, "a", map[string]any{"collected_at": clock - 5, "windows": []any{}})
	if _, ok := e.Headroom("a"); ok {
		t.Fatal("a weekly figure nobody can read says nothing about pace")
	}
}

func TestWeekShareJudgesTheWeekAndFiveHourShareTheShortestWindow(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	accountAt(t, home, 50, 60, 90, 86400)
	st, now := e.Read("a"), float64(clock)
	if got, _ := WeekShare(st, 0, 0, false, now); got != 1 {
		t.Fatalf("no burn, no risk: %v", got)
	}
	// 50 points over an hour: 0.5/60 %/s lasts 100 minutes, past the reset: share 100/60.
	if got, ok := FiveHourShare(st, 0.5/60, 0, now); !ok || got < 1.66 || got > 1.67 {
		t.Fatalf("a burn that outlasts its reset: %v %v", got, ok)
	}
	// One more agent at 1%/min on top: 50 points last 50/1.5 = 33 of the 60 minutes.
	if got, _ := FiveHourShare(st, 0.5/60, 1.0/60, now); got < 0.55 || got > 0.56 {
		t.Fatalf("the newcomer takes it past the wall: %v", got)
	}
	if got, ok := FiveHourShare(st, 0, 0, now); !ok || got != 1 {
		t.Fatalf("no burn, no risk: %v %v", got, ok)
	}
	stale := *st
	stale.Short.Reset = 0
	if _, ok := FiveHourShare(&stale, 1, 1, now); ok {
		t.Fatal("no reset is no projection")
	}
	// The week: 10 points over a day, at a tenth of the shortest window's burn: 0.05%/min runs out in 200 minutes.
	if got, _ := WeekShare(st, 0.5/60, 0.1, true, now); got < 0.138 || got > 0.139 {
		t.Fatalf("the week cut off first: %v", got)
	}
}

func TestHeadroomIsTheTighterOfThe5hAndTheWeeklyBucket(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	// The week is half over with 20% spent (0.4 of room); the 5h window is a fifth over with 30% spent (0.0 of room).
	writeStatus(t, home, "a", map[string]any{"collected_at": clock - 5, "windows": []any{
		map[string]any{"minutes": 300, "used_percent": 30, "resets_at": clock + 4*3600, "observed_at": clock - 5},
		map[string]any{"minutes": 10080, "used_percent": 20, "resets_at": clock + WeekS/2, "observed_at": clock - 5}}})
	if h, ok := e.Headroom("a"); !ok || h < -0.0001 || h > 0.0001 {
		t.Fatalf("the 5h bucket is the tighter one: %v %v", h, ok)
	}
}

func TestThePaceStateIsWhereTheWeekStandsAgainstItsLine(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	os.MkdirAll(teamtest.AccountDir(t, home, "a"), 0o755)
	// Half the week gone: the line is at 49 (98 x 1/2), the burst 10 points over it.
	for _, c := range []struct {
		used  int
		state string
	}{{40, PaceBehind}, {49, PaceBehind}, {54, PaceAhead}, {59, PaceAhead}, {60, PaceOver}} {
		weekAt(t, home, "a", c.used)
		if p := e.PaceNow("a"); p == nil || p.State != c.state || p.Line != 49 || p.Used != c.used {
			t.Fatalf("used %d: pace %+v, want %s on a line of 49", c.used, p, c.state)
		}
	}
}

func TestAnAccountsRateEndsAtTheTarget(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	os.MkdirAll(teamtest.AccountDir(t, home, "a"), 0o755)
	weekAt(t, home, "a", 79)
	p := e.PaceNow("a")
	if p == nil || p.State != PaceOver {
		t.Fatalf("79%% half way through: %+v, want over", p)
	}
	if want := (98.0 - 79) / (WeekS / 2 / 3600); p.RateH < want-1e-9 || p.RateH > want+1e-9 {
		t.Errorf("rate %v, want %v points an hour", p.RateH, want)
	}
	weekAt(t, home, "a", 99)
	if p := e.PaceNow("a"); p.RateH != 0 {
		t.Errorf("past the target: rate %v, want 0", p.RateH)
	}
	// A reading days old still holds until its reset.
	writeStatus(t, home, "a", map[string]any{"collected_at": clock - 2*86400, "windows": []any{
		map[string]any{"minutes": 300, "used_percent": 10, "resets_at": clock - 86400, "observed_at": clock - 2*86400},
		map[string]any{"minutes": 10080, "used_percent": 79, "resets_at": clock + WeekS/2, "observed_at": clock - 2*86400}}})
	if p := e.PaceNow("a"); p == nil || p.Used != 79 {
		t.Errorf("a stale weekly reading: %+v, want it judged", p)
	}
	writeStatus(t, home, "a", map[string]any{"collected_at": clock - 5, "windows": []any{}})
	if p := e.PaceNow("a"); p != nil {
		t.Errorf("no weekly figure: %+v, want none", p)
	}
}

func TestPaceSettingsOutOfRangeFallBackToTheirDefaults(t *testing.T) {
	if ps := PaceSettingsOf(Config{}); ps != (PaceSettings{Target: 98, Burst: 10}) {
		t.Errorf("defaults: %+v", ps)
	}
	if ps := PaceSettingsOf(Config{"pace_target_pct": 120, "pace_burst_pct": 5}); ps != (PaceSettings{Target: 98, Burst: 5}) {
		t.Errorf("set: %+v", ps)
	}
}

func TestAWeekPastItsResetOrItsSubscriptionsEndIsPacedAsSuch(t *testing.T) {
	now, used := float64(clock), 60
	ps := PaceSettings{Target: 98, Burst: 10}
	st := &AccountStatus{AsOf: &now, Long: AcctWindow{Minutes: 10080, Pct: &used, Reset: int64(now) - 10}}
	if p := PaceOf(st, now, ps); p != nil {
		t.Errorf("a reading whose reset has passed: %+v, want none", p)
	}
	st.Long.Reset, st.SubEnd = int64(now)+86400, now-3600
	if p := PaceOf(st, now, ps); p == nil || p.Line != 98 || p.RateH != 0 {
		t.Errorf("past its subscription's end the line is at the target: %+v", p)
	}
}

// The 5-hour test prices the newcomer at its measured cost: 0.159 %/min, never the starting value.
func TestAStartFailsTheFiveHourTestOnlyWhenTheMeasuredCostWouldRunItOut(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	var rows []float64
	for range MinSamples {
		rows = append(rows, 0.159/60)
	}
	measured := costsAt(rows, nil, "a")
	for _, c := range []struct {
		name         string
		used, resets int // percent used, minutes to the reset
		fillPerMin   float64
		costs        *Costs
		unfit        bool
	}{
		{"an empty account early in its window", 0, 280, 0, measured, false},
		{"30% used with 93 minutes left and nothing running", 30, 93, 0, measured, false},
		{"70% used, 2 hours left, 0.2%/min already running", 70, 120, 0.2, measured, true},
		{"a fresh window that three agents are filling fast", 0, 280, 0.45, measured, true},
		{"only the starting value is known: it fits on a guess", 0, 280, 0.45, costsAt(nil, nil, "a"), false},
	} {
		accountAt(t, home, c.used, int64(c.resets), 5, WeekS/2)
		burn := c.fillPerMin / 60
		ld := &Load{Account: "a", Burn: &burn, Costs: c.costs}
		pct := c.used
		if why, _ := ShortFit(e.Read("a"), ld, &pct, ""); (why != "") != c.unfit {
			t.Errorf("%s: %q, unfit want %v", c.name, why, c.unfit)
		}
	}
}

func TestPredictReadsTheAgentsMidTurnAndTheNewcomersMeasuredCostAndHoldsNothingOnAGuess(t *testing.T) {
	q := openDB(t)
	home := t.TempDir()
	e := testEnv(home, clock)
	os.MkdirAll(teamtest.AccountDir(t, home, "a"), 0o755)
	accountAt(t, home, 0, 280, 5, WeekS/2)
	for _, x := range []string{"x", "y", "z"} {
		setAgent(t, q, x, "RUNNING", "a")
	}
	if p := e.Predict(q, "a", "", ""); p.ShortKnown || p.LongKnown {
		t.Fatalf("no pace, no factor: nothing is predicted, %+v", p)
	}
	seedCosts(t, q, 0.159, MinSamples, map[string]AccountFactors{"a": {Short: Factor{USDPerPct: 1, Source: FactorOwn}}})
	p := e.Predict(q, "a", "", "")
	// Three agents at 0.159 %/min and one more: 100 / (0.636 x 280) = 0.56. No week factor: the week is not judged.
	if share, ok := p.Share(); !ok || !p.ShortKnown || p.LongKnown || share < 0.55 || share > 0.57 {
		t.Fatalf("%+v", p)
	}
	putMeta(t, q, factorMeta, map[string]any{"at": 1, "accounts": map[string]AccountFactors{
		"a": {Short: Factor{USDPerPct: 1, Source: FactorOwn}, Long: Factor{USDPerPct: 100, Source: FactorOwn}}}})
	// The week: 95 points left for half a week at 0.636 %/min x 0.01 week points per 5-hour point.
	if p := e.Predict(q, "a", "", ""); !p.LongKnown || p.Long < 1 {
		t.Fatalf("the week outlasts its end: %+v", p)
	}
}
