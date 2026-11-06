package usage

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
)

func outlookRow(a string, ts int64, short, long int, reset int64) sampleRow {
	w := func(m, pct int, resets int64) map[string]any {
		return map[string]any{"minutes": m, "used_percent": pct, "resets_at": resets, "observed_at": ts, "carried": false, "lost": false}
	}
	b, _ := json.Marshal([]any{w(300, short, ts+1000), w(10080, long, reset)})
	return sampleRow{"account": a, "ts": json.Number(itoa(int(ts))), "windows": string(b)}
}

// The want points are what outlook.weekly_outlook says for the same rows (a: pace 2 points an hour; b: ends in six hours; c: stale).
func TestWeeklyOutlookMatchesThePython(t *testing.T) {
	now := int64(1_800_000_000)
	var rows []sampleRow
	for i := 0; i <= 12; i++ {
		ts := now - 12*3600 + int64(i)*3600
		rows = append(rows, outlookRow("a", ts, 10, 20+i*2, now+3*86400), outlookRow("b", ts, 10, 50+i, now+86400))
		if i == 7 {
			rows = append(rows, outlookRow("c", now-5*3600, 5, 70, now+9*86400))
		}
	}
	o := WeeklyOutlook(rows, now, 12, map[string]float64{"b": float64(now + 6*3600)}, nil)
	if o.Accounts != 3 || len(o.Past) != 49 || len(o.Future) != 48 {
		t.Fatalf("%d accounts, %d past, %d future", o.Accounts, len(o.Past), len(o.Future))
	}
	check := func(series []OutlookPoint, every int, want []float64, offsets []int64) {
		t.Helper()
		for i, w := range want {
			p := series[i*every]
			if p.At-now != offsets[i] || p.Left == nil || math.Abs(*p.Left-w) > 1e-5 {
				t.Errorf("point %d: at %d left %v, want %d %v", i*every, p.At-now, p.Left, offsets[i], w)
			}
		}
	}
	check(o.Past, 6, []float64{65, 63.5, 60.5, 59, 56, 46.333333, 44.333333, 50, 47},
		[]int64{-43200, -37800, -32400, -27000, -21600, -16200, -10800, -5400, 0})
	check(o.Future, 5, []float64{46.625, 44.75, 42.875, 41, 39.125, 21.5, 20.25, 19, 17.75, 16.5},
		[]int64{900, 5400, 9900, 14400, 18900, 23400, 27900, 32400, 36900, 41400})
}

func TestWeeklyOutlookWithNoReadingIsEmptyAndAWeekThatResetsStartsAgain(t *testing.T) {
	now := int64(1_800_000_000)
	if o := WeeklyOutlook(nil, now, 24, nil, nil); o.Accounts != 0 || len(o.Past) != 0 || len(o.Future) != 0 {
		t.Fatalf("%+v", o)
	}
	rows := []sampleRow{outlookRow("a", now-2*3600, 10, 20, now+3600), outlookRow("a", now, 10, 30, now+3600)}
	o := WeeklyOutlook(rows, now, 12, nil, nil)
	// pace 10 points over 2 h; an hour on the week resets and is used from nothing at that pace.
	var after *float64
	for _, p := range o.Future {
		if p.At == now+3600+900*2 { // 2 steps past the reset (step 900)
			after = p.Left
		}
	}
	if after == nil || math.Abs(*after-(100-10.0/7200*1800)) > 1e-9 {
		t.Fatalf("%v", after)
	}
}

func TestDayCostsAttributesCallsToTheWorktreeThatHoldsTheCwd(t *testing.T) {
	_, q, _ := burnEnv(t)
	for agent, tree := range map[string]string{"team.ui": "id1::/w/ui/", "team": "/w"} {
		db.SetAgentState(q, agent, "IDLE", 0, "")
		db.SetTerminal(q, agent, "", tree)
	}
	db.SetExtraWorktrees(q, "team", []string{"/x/extra/"})
	day := time.Date(2026, 10, 8, 12, 0, 0, 0, time.Local).Unix()
	ins := func(req string, ts int64, cwd string) {
		if _, err := q.Exec("INSERT INTO token_calls(account, session, req, ts, model, cwd, side, inp, cc, cr, out, thinking) "+
			"VALUES('a','s',?,?,'claude-sonnet-5',?,0,1000000,0,0,0,0)", req, ts, cwd); err != nil {
			t.Fatal(err)
		}
	}
	ins("1", day, "/w/ui/sub")
	ins("2", day, "/w/other")
	ins("3", day+86400, "/x/extra")
	ins("4", day, "/wx")
	ins("5", day-90*86400, "/w")
	got, err := DayCosts(q, day-86400, day+2*86400, nil)
	if err != nil {
		t.Fatal(err)
	}
	// a million sonnet-5 input tokens is $2.
	want := map[string]map[string]float64{"2026-10-08": {"team.ui": 2, "team": 2, Unattributed: 2}, "2026-10-09": {"team": 2}}
	if len(got) != 2 {
		t.Fatalf("%v", got)
	}
	for d, per := range want {
		for a, c := range per {
			if math.Abs(got[d][a]-c) > 1e-9 {
				t.Errorf("%s %s: %v, want %v", d, a, got[d][a], c)
			}
		}
	}
}

func TestSamplesFromKeepsTheRowsAtAndAfterSince(t *testing.T) {
	rows := Samples{{"ts": json.Number("10")}, {"ts": json.Number("20")}, {"ts": json.Number("20")}, {"ts": json.Number("30")}}
	if got := SamplesFrom(rows, 20); len(got) != 3 || got[0]["ts"] != json.Number("20") {
		t.Fatalf("%v", got)
	}
	if got := SamplesFrom(rows, 31); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func TestARowsWindowsAreDecodedOnceWhenRead(t *testing.T) {
	q := openDB(t)
	if _, err := q.Exec(`INSERT INTO usage_samples(account, ts, exists_, collected_at, windows) VALUES('a', 5, 1, 5, '[{"minutes":10080,"used_percent":40}]')`); err != nil {
		t.Fatal(err)
	}
	rows, err := SamplesSince(q, 0)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if _, isList := rows[0]["windows"].([]any); !isList {
		t.Fatalf("windows kept as %T", rows[0]["windows"])
	}
	if es := entries(rows[0]); len(es) != 1 || es[0]["used_percent"] != json.Number("40") {
		t.Fatalf("%v", es)
	}
}

// An account that could not be used counts as nothing left: the line falls by its share while it is out (as its samples were recorded,
// or from outSince on) and rises when it is back; ahead, one out now stays at nothing left.
func TestWeeklyOutlookCountsALoggedOutAccountAsNothingLeft(t *testing.T) {
	now := int64(1_800_000_000)
	var rows []sampleRow
	for i := 0; i <= 12; i++ {
		ts := now - 12*3600 + int64(i)*3600
		b := outlookRow("b", ts, 10, 60, now+3*86400)
		if i >= 3 && i < 6 {
			b["logged_out"] = json.Number("1")
		}
		rows = append(rows, outlookRow("a", ts, 10, 20, now+3*86400), b)
	}
	left := func(o Outlook, at int64) float64 {
		t.Helper()
		for _, p := range append(o.Past, o.Future...) {
			if p.At == at && p.Left != nil {
				return *p.Left
			}
		}
		t.Fatalf("no point at %d", at-now)
		return 0
	}
	o := WeeklyOutlook(rows, now, 12, nil, nil)
	if l := left(o, now-10*3600); l != 60 {
		t.Errorf("both in: %v, want 60", l)
	}
	if l := left(o, now-8*3600); l != 40 {
		t.Errorf("b out: %v, want 40 (a's 80 and b's nothing)", l)
	}
	if l := left(o, now-4*3600); l != 60 {
		t.Errorf("b back: %v, want 60", l)
	}
	if l := left(o, now+3600); l != 60 {
		t.Errorf("both in ahead: %v, want 60", l)
	}
	o = WeeklyOutlook(rows, now, 12, nil, map[string]float64{"b": float64(now - 2*3600)})
	if l := left(o, now-3600); l != 40 {
		t.Errorf("b out since two hours: %v, want 40", l)
	}
	if l := left(o, now+3600); l != 40 {
		t.Errorf("b out now, ahead: %v, want 40", l)
	}
}

// CallCosts counts a call in cost_per_day only after dayLo and in agent_use only after useLo, whichever cutoff comes first.
func TestCallCostsKeepsEachCutoff(t *testing.T) {
	_, q, _ := burnEnv(t)
	day := time.Date(2026, 10, 8, 12, 0, 0, 0, time.Local).Unix()
	for i, ts := range []int64{day - 3*3600, day - 3600, day} {
		if _, err := q.Exec("INSERT INTO token_calls(account, session, req, ts, model, cwd, side, inp, cc, cr, out, thinking) "+
			"VALUES('a','s',?,?,'claude-sonnet-5','/nowhere',0,1000000,0,0,0,0)", itoa(i), ts); err != nil {
			t.Fatal(err)
		}
	}
	spent := func(lo, useLo int64) (float64, float64) {
		t.Helper()
		daily, uses, err := CallCosts(q, lo, useLo, day, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		var d, u float64
		for _, per := range daily {
			d += per[Unattributed]
		}
		for _, x := range uses {
			u += x.Cost
		}
		return d, u
	}
	// $2 a call. The day starts before the span: the span's calls are fewer.
	if d, u := spent(day-4*3600, day-2*3600); d != 6 || u != 4 {
		t.Errorf("day first: cost %v, use %v; want 6 and 4", d, u)
	}
	// The span starts before the day.
	if d, u := spent(day-2*3600, day-4*3600); d != 4 || u != 6 {
		t.Errorf("span first: cost %v, use %v; want 4 and 6", d, u)
	}
}
