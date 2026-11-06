package usage

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/handline/handline/go/data/team"
)

func TestStretchesRiseAtLeastTheMinimumAndStartOverAtAResetOrAFall(t *testing.T) {
	r := func(at int64, pct int, reset int64) WindowReading {
		return WindowReading{At: at, Pct: pct, Reset: reset}
	}
	got := PointStretches([]WindowReading{
		r(100, 10, 9000), r(200, 12, 9001), r(200, 12, 9000), r(300, 15, 9000), // 10 -> 15: one stretch
		r(400, 18, 9000),                                     // 15 -> 18: not enough before the reset
		r(500, 1, 20000), r(600, 4, 20000), r(700, 7, 20000), // the window reset: 1 -> 7
		r(800, 2, 20000), r(900, 8, 20000), // a fall starts over: 2 -> 8
	}, 5)
	want := []PointStretch{{From: 100, To: 300, Rise: 5}, {From: 500, To: 700, Rise: 6}, {From: 800, To: 900, Rise: 6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestOnlyLiveReadingsEnterAStretch(t *testing.T) {
	row := func(at, pct int64, carried bool) sampleRow {
		b, _ := json.Marshal([]map[string]any{{"minutes": 300, "used_percent": pct, "resets_at": 9000, "observed_at": at, "carried": carried},
			{"minutes": 10080, "used_percent": 1, "resets_at": 90000, "observed_at": at, "lost": true}})
		return sampleRow{"account": "a", "windows": string(b)}
	}
	short, long := readingsOf([]sampleRow{row(100, 10, false), row(200, 30, true), row(300, 12, false)})
	if want := []WindowReading{{At: 100, Pct: 10, Reset: 9000}, {At: 300, Pct: 12, Reset: 9000}}; !reflect.DeepEqual(short["a"], want) {
		t.Fatalf("short %+v", short["a"])
	}
	if len(long["a"]) != 0 {
		t.Fatalf("a lost week reading entered: %+v", long["a"])
	}
}

func TestAnAccountTakesItsOwnFactorAfterFiveStretchesAndThePooledOneScaledByItsPlanBefore(t *testing.T) {
	short := map[string][]float64{
		"pro":  {1, 1, 1, 1, 1},          // five of its own: its median
		"max":  {20, 20, 20, 20, 20, 20}, // Max 20x: 20 per point is 1 in Pro units
		"new":  {9},                      // one of its own: still pooled
		"none": nil,
	}
	scale := map[string]float64{"pro": 1, "max": 20, "new": 5, "none": 1}
	got := FactorsOf(short, map[string][]float64{}, scale)
	check := func(acct string, f Factor, usd float64, source string, n int) {
		t.Helper()
		if math.Abs(f.USDPerPct-usd) > 1e-9 || f.Source != source || f.Samples != n {
			t.Errorf("%s: %+v, want %v %s %d", acct, f, usd, source, n)
		}
	}
	check("pro", got["pro"].Short, 1, FactorOwn, 5)
	check("max", got["max"].Short, 20, FactorOwn, 6)
	// The pool in Pro units: five 1s, six 1s and 9/5: median 1, times new's scale 5.
	check("new", got["new"].Short, 5, FactorPooled, 1)
	check("none", got["none"].Short, 1, FactorPooled, 0)
	check("pro week", got["pro"].Long, 0, FactorNone, 0)
}

func TestThePlanScaleComesFromTheTierTheVendorRecords(t *testing.T) {
	dir := t.TempDir()
	write := func(oauth map[string]any) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"numStartups": 3, "oauthAccount": oauth})
		if err := os.WriteFile(filepath.Join(dir, ".claude.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		oauth map[string]any
		sub   string
		tier  string
		scale float64
	}{
		// The shape every account here held on 11-04 (Pro).
		{map[string]any{"billingType": "stripe_subscription", "seatTier": nil, "organizationRateLimitTier": "default_claude_ai", "userRateLimitTier": nil},
			"pro", "default_claude_ai", 1},
		{map[string]any{"organizationRateLimitTier": "default_claude_max_20x", "userRateLimitTier": nil}, "max", "default_claude_max_20x", 20},
		{map[string]any{"organizationRateLimitTier": "default_claude_ai", "userRateLimitTier": "default_claude_max_5x"}, "max", "default_claude_max_5x", 5},
		{map[string]any{}, "max", "", 5},
	} {
		write(c.oauth)
		tier := team.RateLimitTier(dir)
		if tier != c.tier || PlanScale(c.sub, tier) != c.scale {
			t.Errorf("%v: tier %q scale %v, want %q %v", c.oauth, tier, PlanScale(c.sub, tier), c.tier, c.scale)
		}
	}
	if team.RateLimitTier(filepath.Join(dir, "missing")) != "" {
		t.Error("no config file is no tier")
	}
}

func TestRefreshFactorsPricesTheCallsOfEachStretch(t *testing.T) {
	home, q := t.TempDir(), openDB(t)
	e := testEnv(home, clock)
	for i, pct := range []int{10, 13, 16} {
		at := clock - 600 + int64(i)*300
		b, _ := json.Marshal([]map[string]any{{"minutes": 300, "used_percent": pct, "resets_at": clock + 3600, "observed_at": at}})
		if _, err := q.Exec("INSERT INTO usage_samples(account, ts, exists_, windows) VALUES('a', ?, 1, ?)", at, string(b)); err != nil {
			t.Fatal(err)
		}
	}
	tok := Tokens{Inp: 1000, Out: 200_000}
	for i, at := range []int64{clock - 500, clock - 100, clock + 100} { // the last is after the stretch
		if _, err := q.Exec("INSERT INTO token_calls(account, session, req, ts, model, cwd, side, inp, cc, cr, out, thinking) "+
			"VALUES('a', 's', ?, ?, 'claude-opus-5-5', '/w', 0, ?, 0, 0, ?, 0)", i, at, tok.Inp, tok.Out); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.RefreshFactors(q, clock, true); err != nil {
		t.Fatal(err)
	}
	got, err := Factors(q)
	if err != nil {
		t.Fatal(err)
	}
	want := 2 * Cost("claude-opus-5-5", tok, e.prices()) / 6
	if f := got["a"].Short; f.Source != FactorPooled || f.Samples != 1 || math.Abs(f.USDPerPct-want) > 1e-9 {
		t.Fatalf("%+v, want %v pooled from 1", f, want)
	}
}
