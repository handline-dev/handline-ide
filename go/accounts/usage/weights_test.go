package usage

import (
	"math"
	"testing"
)

// An Ultra and three Pros, as the Usage chart showed them: the week line at now is the budget's left, and each reset ahead lifts the
// line by its account's share of the budget, the Ultra's by far the most.
func TestTheWeekLineWeighsEachAccountByItsBudget(t *testing.T) {
	now := int64(1_800_000_000)
	used := map[string]int{"ultra": 86, "pro1": 15, "pro2": 22, "pro3": 38}
	resets := map[string]int64{"ultra": now + 2*3600, "pro1": now + 4*3600, "pro2": now + 6*3600, "pro3": now + 8*3600}
	order := []string{"ultra", "pro1", "pro2", "pro3"}
	var rows []sampleRow
	var bs []AccountBudget
	for _, a := range order {
		for _, ago := range []int64{1800, 0} { // flat, so no pace: a week that resets is all left
			rows = append(rows, outlookRow(a, now-ago, 0, used[a], resets[a]))
		}
		fee := 20.0
		if a == "ultra" {
			fee = 200
		}
		u := float64(used[a])
		bs = append(bs, AccountBudget{Account: a, WeekUSD: WeekBudget(fee), UsedPct: &u})
	}
	w := WeightsOf(bs)
	o := WeeklyOutlook(rows, now, 12, nil, nil, w)
	total := TotalOf(bs)
	if p := o.Past[len(o.Past)-1]; p.At != now || p.Left == nil || math.Abs(*p.Left-*total.LeftPct) > 1e-9 {
		t.Fatalf("the line at now is %v, the budget's left %v", p.Left, *total.LeftPct)
	}
	leftAt := func(t0 int64) float64 {
		var got float64
		for _, p := range o.Future {
			if p.At <= t0 {
				got = *p.Left
			}
		}
		return got
	}
	before := *total.LeftPct
	for _, a := range order {
		after := leftAt(resets[a])
		want := float64(used[a]) * WeekBudget(map[bool]float64{true: 200, false: 20}[a == "ultra"]) / total.WeekUSD
		if math.Abs(after-before-want) > 1e-9 {
			t.Errorf("%s's reset lifts the line %v, want its share %v", a, after-before, want)
		}
		before = after
	}
	if lift := leftAt(resets["ultra"]) - *total.LeftPct; lift < 60 {
		t.Errorf("the Ultra's reset lifts the line only %v", lift)
	}
}

// With no budget at all every account counts the same; with budgets, one with none counts for nothing.
func TestWeekWeightsWithNoBudgetCountAlike(t *testing.T) {
	left := []accountLeft{{"a", 20}, {"b", 60}}
	if m, ok := WeightsOf(nil).Mean(left); !ok || m != 40 {
		t.Fatalf("no budgets: %v %v", m, ok)
	}
	if m, ok := WeightsOf([]AccountBudget{{Account: "a", WeekUSD: 10}, {Account: "b"}}).Mean(left); !ok || m != 20 {
		t.Fatalf("b has no budget: %v %v", m, ok)
	}
	if _, ok := (WeekWeights{"a": 10}).Mean([]accountLeft{{"b", 60}}); ok {
		t.Fatal("no account that counts has a mean")
	}
	if _, ok := WeightsOf(nil).Mean(nil); ok {
		t.Fatal("nothing has no mean")
	}
}

// The pick's overlay is read against the week line, so it weighs the accounts as the line does: all of an Ultra's 40 points counts
// 46.15 of 50.77, a Pro's 20 points nothing for a pick that spent nothing there.
func TestTheOverlayWeighsTheAccountsAsTheLineDoes(t *testing.T) {
	now := int64(1_800_000_000)
	var rows []sampleRow
	for i, used := range []int{0, 40} {
		rows = append(rows, outlookRow("ultra", now-3600+int64(i)*3600, 0, used, now+86400), outlookRow("pro", now-3600+int64(i)*3600, 0, used/2, now+86400))
	}
	uses := []AgentUse{{Agent: "x", Fracs: map[string]float64{"ultra": 1}}}
	past := []OutlookPoint{{At: now - 3600}, {At: now}}
	w := WeekWeights{"ultra": WeekBudget(200), "pro": WeekBudget(20)}
	got := Overlay(rows, uses, "x", past, w)
	want := 40 * WeekBudget(200) / (WeekBudget(200) + WeekBudget(20))
	if len(got) != 2 || got[0] != 0 || math.Abs(got[1]-want) > 1e-9 {
		t.Fatalf("overlay %v, want [0 %v]", got, want)
	}
	if got := Overlay(rows, uses, "x", past, nil); math.Abs(got[1]-20) > 1e-9 {
		t.Fatalf("with no budgets the accounts count alike: %v", got)
	}
}
