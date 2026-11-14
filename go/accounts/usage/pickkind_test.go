package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
)

func TestPickGivesEachRefusalItsKindAndACandidateNone(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	at := func(collected int64, short, week int, weekLost bool) map[string]any {
		return map[string]any{"collected_at": collected, "windows": []any{
			map[string]any{"minutes": 300, "used_percent": short, "resets_at": clock + 3600, "observed_at": collected},
			map[string]any{"minutes": 10080, "used_percent": week, "resets_at": clock + 90000, "observed_at": collected, "lost": weekLost}}}
	}
	fresh, old := clock-5, clock-100000
	for n, st := range map[string]map[string]any{
		"ended": at(fresh, 10, 10, false), "endedstale": at(old, 10, 10, false), "endedout": at(fresh, 10, 10, false),
		"five": at(fresh, 100, 10, false), "fivecapped": at(fresh, 100, 10, false), "week": at(fresh, 10, 100, false),
		"weekold": at(old, 10, 100, false), "lost": at(fresh, 10, 10, true), "busy": at(fresh, 10, 10, false),
		"ok": at(fresh, 10, 10, false), "stale": at(old, 10, 10, false), "removing": at(fresh, 10, 10, false),
	} {
		writeStatus(t, home, n, st)
	}
	win := func(minutes, pct int, observed int64, lost bool) map[string]any {
		return map[string]any{"minutes": minutes, "used_percent": pct, "resets_at": clock + int64(minutes)*12, "observed_at": observed, "lost": lost}
	}
	writeStatus(t, home, "carriedold", map[string]any{"collected_at": fresh, "windows": []any{win(300, 10, clock-1000, false), win(10080, 10, fresh, false)}})
	writeStatus(t, home, "shortlost", map[string]any{"collected_at": fresh, "windows": []any{win(300, 10, fresh, true), win(10080, 10, fresh, false)}})
	// Stale but inside its window, and too near the wall for the idle floor to start on.
	writeStatus(t, home, "idlenear", at(clock-1000, 80, 10, false))
	writeStatus(t, home, "unmetered", at(fresh, 10, 10, false))
	e.Metered = func(a string) bool { return a != "unmetered" }
	for _, n := range []string{"endednofile", "blank"} {
		os.MkdirAll(teamtest.AccountDir(t, home, n), 0o755)
	}
	os.WriteFile(filepath.Join(teamtest.AccountDir(t, home, "removing"), ".removing"), nil, 0o644)
	cfg := map[string]any{}
	for _, n := range []string{"ended", "endedstale", "endedout", "endednofile"} {
		cfg["month_cycle_"+n], cfg["sub_end_"+n] = true, "2020-01-01"
	}
	writeConfig(t, home, cfg)
	e.LoginState = func(a string) (bool, bool) { return a != "endedout", true }
	loads := map[string]*Load{"busy": {Account: "busy", Active: 9}, "fivecapped": {Account: "fivecapped", Active: 2}}
	want := map[string]string{"ended": KindEnded, "endedstale": KindEnded, "endedout": KindEnded, "endednofile": KindEnded,
		"five": KindFiveHourCap, "fivecapped": KindFiveHourCap, "week": KindWeekCap, "weekold": KindWeekCap, "lost": KindOther,
		"busy": "", "ok": "", "stale": KindOther, "removing": KindOther, "blank": "",
		"carriedold": KindOther, "shortlost": KindOther, "idlenear": KindOther, "unmetered": KindOther}
	var names []string
	for n := range want {
		names = append(names, n)
	}
	picked, reasons, err := e.Pick(openDB(t), PickOpts{Candidates: names, Load: loads, Bootstrap: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Reason{}
	for _, r := range reasons {
		got[r.Account] = r
	}
	if picked != "ok" || len(got) != len(want) {
		t.Fatalf("picked %q, reasons %v", picked, reasons)
	}
	for n, k := range want {
		if got[n].Kind != k {
			t.Errorf("%s: kind %q, want %q (%s)", n, got[n].Kind, k, got[n].Why)
		}
	}
	note := func(n string) bool { return strings.HasSuffix(got[n].Why, StaleEndNote) }
	if !note("ended") || note("endedstale") || note("endedout") || note("endednofile") {
		t.Errorf("only a logged-in ended account with a fresh reading says its end day may be stale: %v", reasons)
	}
}

func TestPickRefusesEveryAccountAsOtherWhenTheAgentsFileIsUnreadable(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	e.AgentConfig = func(string) (team.Config, error) { return nil, os.ErrNotExist }
	writeStatus(t, home, "a", map[string]any{"collected_at": clock - 5, "windows": []any{
		map[string]any{"minutes": 300, "used_percent": 10, "resets_at": clock + 3600, "observed_at": clock - 5}}})
	picked, reasons, err := e.Pick(openDB(t), PickOpts{Candidates: []string{"a"}, Agent: "x"})
	if err != nil || picked != "" || len(reasons) != 1 || reasons[0].Kind != KindOther {
		t.Fatalf("%q %v %v", picked, reasons, err)
	}
}

func TestAPausedAccountTakesNoAgentUntilItIsResumed(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	st := func(week int) map[string]any {
		return map[string]any{"collected_at": clock - 5, "windows": []any{
			map[string]any{"minutes": 300, "used_percent": 10, "resets_at": clock + 3600, "observed_at": clock - 5},
			map[string]any{"minutes": 10080, "used_percent": week, "resets_at": clock + 90000, "observed_at": clock - 5}}}
	}
	writeStatus(t, home, "rest", st(5)) // the most room: picked unless paused
	writeStatus(t, home, "busy", st(50))
	writeStatus(t, home, "both", st(20))
	writeStatus(t, home, "quiet", st(1)) // unmetered and paused: says paused
	pick := func() (string, map[string]string) {
		t.Helper()
		picked, reasons, err := e.Pick(openDB(t), PickOpts{Candidates: []string{"rest", "busy", "both", "quiet"}, Bootstrap: true})
		if err != nil {
			t.Fatal(err)
		}
		kinds := map[string]string{}
		for _, r := range reasons {
			kinds[r.Account] = r.Kind
		}
		return picked, kinds
	}
	e.Metered = func(a string) bool { return a != "quiet" }
	writeConfig(t, home, map[string]any{"paused_rest": true, "paused_both": true, "paused_quiet": true, "month_cycle_both": true, "sub_end_both": "2020-01-01"})
	picked, kinds := pick()
	if picked != "busy" || kinds["rest"] != KindPaused || kinds["both"] != KindEnded || kinds["quiet"] != KindPaused {
		t.Fatalf("paused: picked %q, kinds %v (an ended account says ended first)", picked, kinds)
	}
	writeConfig(t, home, map[string]any{})
	if picked, kinds = pick(); picked != "rest" || kinds["rest"] != "" {
		t.Errorf("resumed: picked %q, kinds %v", picked, kinds)
	}
}
