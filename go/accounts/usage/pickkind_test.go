package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		"weekold": at(old, 10, 100, false), "lost": at(fresh, 10, 10, true), "capped": at(fresh, 10, 10, false),
		"ok": at(fresh, 10, 10, false), "stale": at(old, 10, 10, false), "removing": at(fresh, 10, 10, false),
	} {
		writeStatus(t, home, n, st)
	}
	for _, n := range []string{"endednofile", "blank"} {
		os.MkdirAll(filepath.Join(home, "accounts", n), 0o755)
	}
	os.WriteFile(filepath.Join(home, "accounts", "removing", ".removing"), nil, 0o644)
	cfg := map[string]any{}
	for _, n := range []string{"ended", "endedstale", "endedout", "endednofile"} {
		cfg["month_cycle_"+n], cfg["sub_end_"+n] = true, "2020-01-01"
	}
	writeConfig(t, home, cfg)
	e.LoginState = func(a string) (bool, bool) { return a != "endedout", true }
	loads := map[string]*Load{"capped": {Account: "capped", Active: 2, Cap: 2}, "fivecapped": {Account: "fivecapped", Active: 2, Cap: 2}}
	want := map[string]string{"ended": KindEnded, "endedstale": KindEnded, "endedout": KindEnded, "endednofile": KindEnded,
		"five": KindFiveHourCap, "fivecapped": KindFiveHourCap, "week": KindWeekCap, "weekold": KindWeekCap, "lost": KindOther,
		"capped": KindHeld, "ok": "", "stale": KindOther, "removing": KindOther, "blank": ""}
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
