package reads

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
)

func TestOutlookStatesTheRunwayTheCostAndKeepsTheFutureApart(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	clock := time.Date(2026, 10, 8, 15, 0, 0, 0, time.Local)
	now := clock.Unix()
	for acct, pct := range map[string]int{"full": 100, "quiet": 20} {
		p := paths.StatusFile(e.Home, teamtest.Account(t, e.Home, acct))
		os.MkdirAll(filepath.Dir(p), 0o755)
		body := fmt.Sprintf(`{"collected_at": %d, "windows": [{"minutes": 300, "used_percent": %d, "resets_at": %d, "observed_at": %d, "carried": false, "lost": false},`+
			`{"minutes": 10080, "used_percent": 40, "resets_at": %d, "observed_at": %d, "carried": false, "lost": false}]}`,
			now, pct, now+3600, now, now+86400, now)
		os.WriteFile(p, []byte(body), 0o644)
		for i := 0; i < 3; i++ {
			ts := now - int64(3-i)*1800
			w := fmt.Sprintf(`[{"minutes":300,"used_percent":5,"resets_at":%d},{"minutes":10080,"used_percent":%d,"resets_at":%d}]`, ts+3600, 30+i*5, now+86400)
			if _, err := e.DB.Exec("INSERT INTO usage_samples(account, ts, exists_, windows) VALUES(?,?,1,?)", acct, ts, w); err != nil {
				t.Fatal(err)
			}
		}
	}
	e.DB.Exec(`INSERT INTO token_calls(account, session, req, ts, model, cwd, side, inp, cc, cr, out, thinking)
		VALUES('full','s','r',?, 'claude-sonnet-5', '/nowhere', 0, 1234567, 0, 0, 0, 0)`, now-60)

	got, err := e.outlookAt("24h", 24, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var out struct {
		Range, Now string
		Accounts   int
		Past       []struct {
			At       string
			Left_pct *float64
		}
		Future []struct {
			At       string
			Left_pct *float64
		}
		Runway []struct {
			Account        string
			Runs_out       *int64 `json:"runs_out_in_seconds"`
			Week_resets_at string
		}
		Cost []struct {
			Day    string
			Agents map[string]float64
		} `json:"cost_per_day"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Range != "24h" || out.Now != "2026-10-08T"+clock.UTC().Format("15:04:05")+"Z" || out.Accounts != 2 || len(out.Past) == 0 || len(out.Future) == 0 {
		t.Fatalf("%s", b)
	}
	if len(out.Runway) != 2 || out.Runway[0].Account != "full" || out.Runway[0].Runs_out == nil || *out.Runway[0].Runs_out != 0 ||
		out.Runway[1].Runs_out != nil || out.Runway[0].Week_resets_at != time.Unix(now+86400, 0).UTC().Format("2006-01-02T15:04:05Z") {
		t.Fatalf("an account at the wall runs out now, one with no burn does not: %s", b)
	}
	if len(out.Cost) != 1 || out.Cost[0].Day != "2026-10-08" || out.Cost[0].Agents["(unattributed)"] != 2.4691 {
		t.Fatalf("1.234567 M sonnet-5 input tokens at $2, rounded to four places: %s", b)
	}
}

// flatWeeks is stats.outlook 24h over a at 20 of its week used and b at 60, both flat for the last hour and a half.
func flatWeeks(t *testing.T, e *Env, clock time.Time) []byte {
	t.Helper()
	now := clock.Unix()
	for acct, used := range map[string]int{"a": 20, "b": 60} {
		for i := 0; i < 3; i++ {
			ts := now - int64(3-i)*1800
			w := fmt.Sprintf(`[{"minutes":300,"used_percent":5,"resets_at":%d},{"minutes":10080,"used_percent":%d,"resets_at":%d}]`, now+3600, used, now+3*86400)
			if _, err := e.DB.Exec("INSERT INTO usage_samples(account, ts, exists_, windows) VALUES(?,?,1,?)", acct, ts, w); err != nil {
				t.Fatal(err)
			}
		}
	}
	got, err := e.outlookAt("24h", 24, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	return b
}

// The future is the accounts' weekly guesses averaged: a at 20 used and b at 60, both flat, average 60 left.
func TestOutlookFutureIsTheAccountsWeekAveraged(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	clock := time.Date(2026, 10, 8, 15, 0, 0, 0, time.Local)
	e.LoginState = func(string) (bool, bool) { return true, true } // a logged-out account would count as nothing left
	b := flatWeeks(t, e, clock)
	var out struct {
		Future []struct {
			At       time.Time
			Left_pct *float64
		}
		By_account []struct {
			Future []struct {
				Week_left *float64
			}
		}
	}
	json.Unmarshal(b, &out)
	if len(out.Future) == 0 || len(out.By_account) != 2 {
		t.Fatalf("%s", b)
	}
	// Each week reset three days on, so it began four days back; its last reading, half an hour ago, sets its pace since then.
	now := clock.Unix()
	began := float64(now - 1800 - (now + 3*86400 - 7*86400))
	dt := out.Future[0].At.Unix() - now
	want := (100 - 20 - 20/began*float64(dt) + 100 - 60 - 60/began*float64(dt)) / 2
	if math.Abs(*out.Future[0].Left_pct-want) > 0.01 {
		t.Fatalf("the first guess is %v, want %v", *out.Future[0].Left_pct, want)
	}
	for i, p := range out.Future {
		wa, wb := out.By_account[0].Future[i].Week_left, out.By_account[1].Future[i].Week_left
		if p.Left_pct == nil || wa == nil || wb == nil || math.Abs(*p.Left_pct-(*wa+*wb)/2) > 0.01 || *p.Left_pct > 60 {
			t.Fatalf("point %d is not the accounts' week averaged: %s", i, b)
		}
	}
}

// An account that cannot be used now counts as nothing left ahead: a's guess and b out average to half of a's.
func TestOutlookCountsALoggedOutAccountAsNothingLeftAhead(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	clock := time.Date(2026, 10, 8, 15, 0, 0, 0, time.Local)
	e.LoginState = func(acct string) (bool, bool) { return acct == "a", true }
	b := flatWeeks(t, e, clock)
	var out struct {
		Future []struct {
			Left_pct *float64
		}
		By_account []struct {
			Future []struct {
				Week_left *float64
			}
		}
	}
	json.Unmarshal(b, &out)
	if len(out.Future) == 0 {
		t.Fatalf("%s", b)
	}
	for i, p := range out.Future {
		if wa := out.By_account[0].Future[i].Week_left; p.Left_pct == nil || wa == nil || math.Abs(*p.Left_pct-*wa/2) > 0.01 || *p.Left_pct > 40 {
			t.Fatalf("point %d is not a's 80 and b's nothing averaged: %s", i, b)
		}
	}
}

func TestOutlookRefusesARangeItDoesNotList(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if _, err := e.statsOutlook(nil, map[string]any{"range": "1y"}); err == nil {
		t.Fatal("bad_request expected")
	}
	if _, err := e.statsOutlook(nil, map[string]any{}); err == nil {
		t.Fatal("a missing range is refused")
	}
}

// outlookWithUse seeds two accounts whose week rose by 20 and 10 points, and two agents of the group grp who spent on them: grp.one $6 of
// a's $8 and $2 of b's $2, grp.two $2 of a's $8.
func outlookWithUse(t *testing.T) (*Env, time.Time) {
	t.Helper()
	e := testEnv(t)
	clock := time.Date(2026, 10, 8, 15, 0, 0, 0, time.Local)
	now := clock.Unix()
	for acct, rise := range map[string]int{"a": 20, "b": 10} {
		for i := 0; i < 3; i++ {
			ts := now - int64(3-i)*1800
			w := fmt.Sprintf(`[{"minutes":10080,"used_percent":%d,"resets_at":%d}]`, 10+rise*i/2, now+86400)
			run(t, e, "INSERT INTO usage_samples(account, ts, exists_, windows) VALUES(?,?,1,?)", acct, ts, w)
		}
	}
	run(t, e, "INSERT INTO agent_state(agent, state, updated, worktree) VALUES('grp.one', 'IDLE', 0, '/w/one'), ('grp.two', 'IDLE', 0, '/w/two')")
	call := func(acct, cwd string, tokens int) {
		run(t, e, `INSERT INTO token_calls(account, session, req, ts, model, cwd, side, inp, cc, cr, out, thinking)
			VALUES(?, 's', ?, ?, 'claude-sonnet-5', ?, 0, ?, 0, 0, 0, 0)`, acct, acct+cwd, now-60, cwd, tokens)
	}
	call("a", "/w/one", 3_000_000)
	call("a", "/w/two", 1_000_000)
	call("b", "/w/one", 1_000_000)
	return e, clock
}

// The plan money: a, a Pro account with its fee set to $26 ($6 a week), rose 20 points, b, recorded as Max ($23.08 a week), 10.
// grp.one takes 3/4 of a's and all of b's, grp.two 1/4 of a's; each day's is the same, all of it today.
func TestOutlookGivesThePlanMoneyNotTheListPrice(t *testing.T) {
	t.Parallel()
	e, clock := outlookWithUse(t)
	for acct, sub := range map[string]string{"a": `{"subscriptionType":"pro"}`, "b": `{"subscriptionType":"max"}`} {
		dir := paths.AccountDir(e.Home, teamtest.Account(t, e.Home, acct))
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "subscription.json"), []byte(sub), 0o644)
		five := map[string]string{"a": "null", "b": "4"}[acct] // a reads as an Ultra does: a 5-hour window with no figure
		os.WriteFile(filepath.Join(dir, "status.json"), []byte(fmt.Sprintf(`{"collected_at": %d, "windows": [{"minutes": 300, "used_percent": %s, "resets_at": null},`+
			`{"minutes": 10080, "used_percent": %d, "resets_at": %d}]}`, clock.Unix(), five, map[string]int{"a": 30, "b": 20}[acct], clock.Unix()+86400)), 0o644)
	}
	run(t, e, "UPDATE accounts SET plan_fee = 26 WHERE name = 'a'")
	e.LoginState = func(string) (bool, bool) { return true, true }
	got, err := e.outlookAt("24h", 24, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := got.(map[string]any)
	bud := res["budget"].(map[string]any)
	accts := bud["accounts"].([]any)
	want := "[map[account:a fee_usd:26 five_hour:false left_usd:4.2 plan:Pro spent_usd:1.8 used_pct:30 week_usd:6] " +
		"map[account:b fee_usd:100 five_hour:true left_usd:18.46 plan:Max spent_usd:4.62 used_pct:20 week_usd:23.08]]"
	if got := fmt.Sprint(accts); got != want {
		t.Fatalf("each account's week in plan money: %s", got)
	}
	if bud["week_usd"] != 29.08 || bud["spent_usd"] != 6.42 || bud["left_pct"] != 77.94 {
		t.Fatalf("weighted by budget: %v", bud)
	}
	day := res["plan_cost_per_day"].([]any)[0].(map[string]any)["agents"].(map[string]any)
	if day["grp.one"] != 3.2077 || day["grp.two"] != 0.3 {
		t.Fatalf("a's 20 points are $1.20, b's 10 are $2.3077: %v", day)
	}
	// a's readings carry no 5-hour window: it has no 5-hour figures, and its forecast's unused share is priced at its $6 week.
	ba := res["by_account"].([]any)[0].(map[string]any)
	f, _ := ba["forecast"].(map[string]any)
	if ba["account"] != "a" || ba["five_hour"] != false || ba["five_runs_out_at"] != nil || f == nil ||
		math.Abs(f["unused_usd"].(float64)-f["unused_pct"].(float64)*0.06) > 0.01 || ba["future"].([]any)[0].(map[string]any)["five_left"] != nil {
		t.Fatalf("a's forecast: %v", ba)
	}
	uses := res["agent_use"].([]any)
	if uses[0].(map[string]any)["plan_cost"] != 3.2077 || uses[1].(map[string]any)["plan_cost"] != 0.3 {
		t.Fatalf("%v", uses)
	}
}

func TestOutlookGivesWhatEachAgentUsedAndTheEstimateOfTheWeek(t *testing.T) {
	t.Parallel()
	e, clock := outlookWithUse(t)
	got, err := e.outlookAt("24h", 24, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := got.(map[string]any)
	if _, has := res["overlay"]; has {
		t.Fatal("no agent asked for, no overlay")
	}
	uses := res["agent_use"].([]any)
	one, two := uses[0].(map[string]any), uses[1].(map[string]any)
	fr := one["fracs"].(map[string]any)
	if len(uses) != 2 || one["agent"] != "grp.one" || two["agent"] != "grp.two" || one["tokens"] != int64(4_000_000) ||
		one["cost"] != 8.0 || one["share"] != 0.8 || two["share"] != 0.2 || fr["a"] != 0.75 || fr["b"] != 1.0 {
		t.Fatalf("%v", uses)
	}
	// grp.one: 3/4 of a's 20 points and all of b's 10, over two accounts; grp.two: 1/4 of a's.
	if one["est_pct"] != 12.5 || two["est_pct"] != 2.5 {
		t.Fatalf("%v %v", one["est_pct"], two["est_pct"])
	}
}

func TestOutlookOverlayIsThePicksCurveAgainstPast(t *testing.T) {
	t.Parallel()
	e, clock := outlookWithUse(t)
	for pick, nonEmpty := range map[string]bool{"grp.one": true, "grp.nobody": false, "nobody": false} {
		got, err := e.outlookAt("24h", 24, clock, &pick)
		if err != nil {
			t.Fatal(err)
		}
		res := got.(map[string]any)
		overlay := res["overlay"].([]any)
		if nonEmpty != (len(overlay) > 0) || (nonEmpty && len(overlay) != len(res["past"].([]any))) {
			t.Fatalf("%s: %d overlay entries for %d past", pick, len(overlay), len(res["past"].([]any)))
		}
		for i := 1; i < len(overlay); i++ {
			if overlay[i].(float64) < overlay[i-1].(float64) {
				t.Fatalf("%s: the points used only grow: %v", pick, overlay)
			}
		}
		if nonEmpty && overlay[len(overlay)-1].(float64) <= 0 {
			t.Fatalf("%s: %v", pick, overlay)
		}
	}
}

// The overlay by number: past is 49 half-hour points; each account's last three samples fall on its 46th to 48th. A rise counts only
// when its sample is inside past (a's 2 -> 4 before past is not counted):
//
//	a: 0 for the 45 points before 04:30, then 6 (4 -> 10), 16, 26, 26     b: 0 for 45, then 0, 5, 10, 10
//
// grp.one spent 3/4 of a's cost and all of b's, averaged over the two accounts: (3/4*a + b) / 2 = 2.25, 8.5, 14.75, 14.75
func TestOutlookOverlayValuesByNumber(t *testing.T) {
	t.Parallel()
	e, clock := outlookWithUse(t)
	now := clock.Unix()
	for _, s := range []struct {
		ago  int64
		used int
	}{{25*3600 + 1800, 2}, {25 * 3600, 4}} {
		w := fmt.Sprintf(`[{"minutes":10080,"used_percent":%d,"resets_at":%d}]`, s.used, now+86400)
		run(t, e, "INSERT INTO usage_samples(account, ts, exists_, windows) VALUES('a',?,1,?)", now-s.ago, w)
	}
	for pick, tail := range map[string][4]float64{"grp.one": {2.25, 8.5, 14.75, 14.75}} {
		got, err := e.outlookAt("24h", 24, clock, &pick)
		if err != nil {
			t.Fatal(err)
		}
		res := got.(map[string]any)
		past, overlay := res["past"].([]any), res["overlay"].([]any)
		if len(past) != 49 || past[0].(map[string]any)["at"] != ts(now-24*3600) {
			t.Fatalf("the grid this test counts on: %d points from %v", len(past), past[0])
		}
		want := make([]any, 45, 49)
		for i := range want {
			want[i] = 0.0
		}
		for _, v := range tail {
			want = append(want, v)
		}
		if fmt.Sprint(overlay) != fmt.Sprint(want) {
			t.Fatalf("%s:\n got %v\nwant %v", pick, overlay, want)
		}
	}
}

// stats.get's use is a row per agent and account: a's 20 points split 3:1 by cost, b's 10 all grp.one's; calls of no agent's worktree are
// agent "", and an account with no reading says null, not 0.
func TestStatsUseSplitsEachAccountsRiseByCostPerAgentAndAccount(t *testing.T) {
	t.Parallel()
	e, clock := outlookWithUse(t)
	now := clock.Unix()
	call := func(acct, cwd string, tokens int) {
		run(t, e, `INSERT INTO token_calls(account, session, req, ts, model, cwd, side, inp, cc, cr, out, thinking)
			VALUES(?, 's', ?, ?, 'claude-sonnet-5', ?, 0, ?, 0, 0, 0, 0)`, acct, acct+cwd+"x", now-60, cwd, tokens)
	}
	call("b", "/nowhere", 1_000_000) // b: grp.one $2 and no agent $2 of $4
	call("c", "/w/two", 1_000_000)   // c has calls and no reading
	got, err := e.statsUse(now-86400, now)
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ agent, acct string }
	rows := map[key]map[string]any{}
	for _, r := range got {
		m := r.(map[string]any)
		rows[key{m["agent"].(string), m["account"].(string)}] = m
	}
	week := func(agent, acct string) any { return rows[key{agent, acct}]["week_pct"] }
	if len(rows) != 5 || week("grp.one", "a") != 15.0 || week("grp.two", "a") != 5.0 || week("grp.one", "b") != 5.0 || week("", "b") != 5.0 || week("grp.two", "c") != nil {
		t.Fatalf("%v", got)
	}
	one := rows[key{"grp.one", "a"}]
	if one["share"] != 0.75 || one["tokens"] != int64(3_000_000) || one["cost"] != 6.0 {
		t.Fatalf("%v", one)
	}
	if got[0].(map[string]any)["cost"].(float64) < got[len(got)-1].(map[string]any)["cost"].(float64) {
		t.Fatalf("largest cost first: %v", got)
	}
}

// by_account looks back a day even on the 12h range: an account last sampled 18 hours ago is still one of its accounts.
func TestOutlookByAccountOfTwelveHoursReadsTheDayBefore(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	clock := time.Date(2026, 10, 8, 15, 0, 0, 0, time.Local)
	now := clock.Unix()
	e.LoginState = func(string) (bool, bool) { return true, true }
	ts := now - 18*3600
	w := fmt.Sprintf(`[{"minutes":300,"used_percent":5,"resets_at":%d},{"minutes":10080,"used_percent":30,"resets_at":%d}]`, ts+3600, now+3*86400)
	if _, err := e.DB.Exec("INSERT INTO usage_samples(account, ts, exists_, windows) VALUES('a',?,1,?)", ts, w); err != nil {
		t.Fatal(err)
	}
	got, err := e.outlookAt("12h", 12, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var out struct{ By_account []struct{ Account string } }
	json.Unmarshal(b, &out)
	if len(out.By_account) != 1 || out.By_account[0].Account != "a" {
		t.Fatalf("%s", b)
	}
}
