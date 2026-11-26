package usage

import (
	"math"
	"testing"
	"time"
)

func eq6(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestAPlanFeeIsTheListedOneUnlessTheAccountSetsItsOwn(t *testing.T) {
	for _, c := range []struct {
		sub, tier  string
		fee, scale float64
	}{
		{"pro", "", 20, 1}, {"max", "default_claude_max_5x", 100, 5}, {"max", "", 100, 5},
		{"max", "default_claude_max_20x", 200, 10}, {"team", "", 0, 1}, {"", "", 0, 1},
	} {
		if PlanFee(c.sub, c.tier) != c.fee || PlanScale(c.sub, c.tier) != c.scale {
			t.Errorf("%s %s: fee %v scale %v, want %v %v", c.sub, c.tier, PlanFee(c.sub, c.tier), PlanScale(c.sub, c.tier), c.fee, c.scale)
		}
	}
	if !eq6(WeekBudget(200), 46.153846) || !eq6(WeekBudget(20), 4.615385) {
		t.Errorf("a week of a fee: %v %v", WeekBudget(200), WeekBudget(20))
	}
	cfg := Config{"plan_fee_b": 150.0}
	if cfg.FeeOf("b", "pro", "") != 150 || cfg.FeeOf("a", "pro", "") != 20 {
		t.Error("an account's own fee wins over its plan's")
	}
}

func status(acct string, week, five *int, reset int64) *AccountStatus {
	return &AccountStatus{Account: acct, Short: AcctWindow{Minutes: 300, Pct: five}, Long: AcctWindow{Minutes: 10080, Pct: week, Reset: reset}}
}

// The four accounts as read on the live board: an Ultra at 4% of its week with no 5-hour window, and three Pro accounts at 100%, 100%
// and 29%. Weighted by budget the week is 79% left, where a plain average of the percents says 42%.
func TestTheWeekLeftIsWeightedByEachAccountsBudget(t *testing.T) {
	now := int64(1_800_000_000)
	ultra := BudgetOf(status("a1", pct(4), nil, now+3600), "Ultra", 200, now)
	bs := []AccountBudget{ultra,
		BudgetOf(status("a2", pct(100), pct(100), now+3600), "Pro", 20, now),
		BudgetOf(status("a3", pct(100), pct(0), now+3600), "Pro", 20, now),
		BudgetOf(status("a4", pct(29), pct(3), now+3600), "Pro", 20, now),
		BudgetOf(status("none", nil, nil, 0), "Pro", 20, now),
		BudgetOf(status("free", pct(50), nil, now+3600), "Team", 0, now),
	}
	if ultra.FiveHour || !bs[1].FiveHour || !eq6(*ultra.Spent(), 0.04*200*12/52) {
		t.Fatalf("the Ultra: five hour %v, spent %v", ultra.FiveHour, *ultra.Spent())
	}
	tot := TotalOf(bs)
	week := WeekBudget(200) + 3*WeekBudget(20)
	spent := WeekBudget(200)*0.04 + WeekBudget(20)*(1+1+0.29)
	if !eq6(tot.WeekUSD, week) || !eq6(tot.SpentUSD, spent) || tot.LeftPct == nil || math.Round(*tot.LeftPct) != 79 {
		t.Fatalf("total %+v, want week %v spent %v and 79%% left", tot, week, spent)
	}
	if bs[4].Spent() != nil {
		t.Error("an account with no reading spends nothing known")
	}
	if b := BudgetOf(status("old", pct(70), nil, now-1), "Pro", 20, now); b.UsedPct == nil || *b.UsedPct != 0 {
		t.Error("a week whose reset has passed starts from nothing")
	}
	if TotalOf(nil).LeftPct != nil {
		t.Error("no account, no percent")
	}
}

func weekRow(acct string, ts int64, used int) sampleRow {
	return sampleRow{"account": acct, "ts": float64(ts), "windows": `[{"minutes": 10080, "used_percent": ` + itoa(used) + `}]`}
}

// An account's plan money in a day is the points its week rose that day times its budget, split between the agents by their list-price
// cost on it; an account that rose with no priced call is nobody's. Token prices never size it: ten times the price is the same money.
func TestPlanMoneyComesFromTheWeeksRiseAndTokenPricesOnlySplitIt(t *testing.T) {
	day1 := time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local).Unix()
	day2 := day1 + 86400
	rows := []sampleRow{
		weekRow("u", day1+3600, 10), weekRow("u", day1+7200, 15), weekRow("p", day1+3600, 0),
		weekRow("u", day2-600, 15), weekRow("p", day2-600, 0), weekRow("u", day2+3600, 20), weekRow("p", day2+3600, 50),
	}
	weekUSD := map[string]float64{"u": WeekBudget(200), "p": WeekBudget(20)}
	for _, scale := range []float64{1, 10} {
		days := dayCalls{}
		days.add("2026-11-20", callKey{"x", "u"}, 3*scale)
		days.add("2026-11-20", callKey{"y", "u"}, 1*scale)
		days.add("2026-11-21", callKey{"x", "u"}, 2*scale)
		span := tally{}
		span.add(callKey{"x", "u"}, 0, 5*scale)
		span.add(callKey{"y", "u"}, 0, 1*scale)
		got := planCostsOf(days, span, rows, day1-1, day1-1, day2+7200, weekUSD)
		d1, d2 := got.Daily["2026-11-20"], got.Daily["2026-11-21"]
		u := WeekBudget(200) / 100
		if !eq6(d1["x"], 5*u*0.75) || !eq6(d1["y"], 5*u*0.25) || len(d1) != 2 {
			t.Errorf("x%v day one: %v", scale, d1)
		}
		if !eq6(d2["x"], 5*u) || !eq6(d2[Unattributed], 50*WeekBudget(20)/100) {
			t.Errorf("x%v day two: %v", scale, d2)
		}
		if !eq6(got.Span["x"], 10*u*5/6) || !eq6(got.Span["y"], 10*u/6) || !eq6(got.Span[Unattributed], 50*WeekBudget(20)/100) {
			t.Errorf("x%v span: %v", scale, got.Span)
		}
	}
}

// A rise counts on the day of the sample that shows it, a gap overnight (the machine asleep) included; only the range's first rise needs a
// sample at most OutlookMaxAgeS before the range.
func TestARiseCountsOnTheDayItIsSeenAcrossAGap(t *testing.T) {
	day1 := time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local).Unix()
	day2 := day1 + 86400
	rows := []sampleRow{weekRow("s", day1-5*3600, 0), weekRow("s", day1+21*3600, 10), weekRow("s", day2+8*3600, 30),
		weekRow("n", day1-3600, 5), weekRow("n", day1+3600, 7)}
	byDay, span := rises(rows, day1-1, day1+20*3600, day2+86399)
	if byDay["2026-11-20"]["s"] != 0 || byDay["2026-11-21"]["s"] != 20 || byDay["2026-11-20"]["n"] != 2 {
		t.Fatalf("per day %v", byDay)
	}
	if span["s"] != 20 || span["n"] != 0 {
		t.Fatalf("span %v: s's first rise starts from a sample five hours before the range, so only the overnight one counts", span)
	}
}
