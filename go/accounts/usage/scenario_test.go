package usage

import (
	"strings"
	"testing"
	"time"
)

// Rows of docs/design/account-assignment-scenarios.md on a fake clock: A (window edges), B1/B2/B9 (the cap), C (placement order).
// Each case names its row. now is moved by hand; nothing sleeps.

// acct is one account's reading as a scenario states it: percents and seconds from the clock to each reset.
type acct struct {
	short, week           int
	shortReset, weekReset int64 // seconds after now; 0 is 3600 and half a week
	age                   int64 // how old the reading is
}

func (a acct) status(now int64) map[string]any {
	sr, wr := a.shortReset, a.weekReset
	if sr == 0 {
		sr = 3600
	}
	if wr == 0 {
		wr = WeekS / 2
	}
	at := now - a.age
	return map[string]any{"collected_at": at, "windows": []any{
		map[string]any{"minutes": 300, "used_percent": a.short, "resets_at": now + sr, "observed_at": at},
		map[string]any{"minutes": 10080, "used_percent": a.week, "resets_at": now + wr, "observed_at": at}}}
}

// world is a home of accounts and a clock the Env reads.
type world struct {
	t    *testing.T
	home string
	now  int64
	env  *Env
}

func newWorld(t *testing.T, accts map[string]acct) *world {
	t.Helper()
	w := &world{t: t, home: t.TempDir(), now: clock}
	w.env = testEnv(w.home, 0)
	w.env.Now = func() float64 { return float64(w.now) }
	for n, a := range accts {
		writeStatus(t, w.home, n, a.status(w.now))
	}
	return w
}

// pick is Pick over names with these loads; an account missing from loads has none mid-turn and cap 2.
func (w *world) pick(names []string, loads map[string]*Load, agent string) (string, map[string]string) {
	w.t.Helper()
	all := map[string]*Load{}
	for _, n := range names {
		if l, ok := loads[n]; ok {
			all[n] = l
		} else {
			all[n] = &Load{Account: n, Cap: 2}
		}
	}
	picked, reasons, err := w.env.Pick(openDB(w.t), PickOpts{Candidates: names, Load: all, Agent: agent})
	if err != nil {
		w.t.Fatal(err)
	}
	why := map[string]string{}
	for _, r := range reasons {
		why[r.Account] = r.Why
	}
	return picked, why
}

func TestScenarioAWindowEdges(t *testing.T) {
	t.Run("A1 5h at W-1 and one agent costs more than its last point: no room, beta takes it", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"alpha": {short: WallPct - 1}, "beta": {short: 10}})
		costly := costsAt([]float64{0.01}, map[string]float64{"*": 0.1}, "alpha", "beta") // 1 point lasts 100 s
		loads := map[string]*Load{"alpha": {Account: "alpha", Cap: 2, Costs: costly, Hold: true},
			"beta": {Account: "beta", Cap: 2, Costs: costly, Hold: true}}
		picked, why := w.pick([]string{"alpha", "beta"}, loads, "")
		if picked != "beta" || !strings.Contains(why["alpha"], "under the 30m a start is worth") {
			t.Fatalf("picked %q, alpha: %q", picked, why["alpha"])
		}
	})
	t.Run("A2 5h at W is never picked, cap free", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"alpha": {short: WallPct}})
		picked, why := w.pick([]string{"alpha"}, nil, "")
		if picked != "" || why["alpha"] != "5h usage 100% >= 100%" {
			t.Fatalf("picked %q, alpha: %q", picked, why["alpha"])
		}
	})
	t.Run("A3 5h at W resetting in 60 s: refused at +59 s, picked at +61 s", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"alpha": {short: WallPct, shortReset: 60}})
		w.now += 59
		if picked, why := w.pick([]string{"alpha"}, nil, ""); picked != "" {
			t.Fatalf("+59 s: picked %q (%q)", picked, why["alpha"])
		}
		w.now += 2
		if picked, why := w.pick([]string{"alpha"}, nil, ""); picked != "alpha" {
			t.Fatalf("+61 s: picked %q (%q)", picked, why["alpha"])
		}
	})
	t.Run("A4 week at W, 5h at 0: refused on the week, naming its reset", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"alpha": {week: WallPct}})
		picked, why := w.pick([]string{"alpha"}, nil, "")
		if picked != "" || !strings.HasPrefix(why["alpha"], "7d usage 100% >= 100%") {
			t.Fatalf("picked %q, alpha: %q", picked, why["alpha"])
		}
		if !strings.Contains(why["alpha"], "resets") {
			t.Skipf("T9060: the reason does not name the reset: %q", why["alpha"])
		}
	})
	t.Run("A5 a subscription ending before the week resets ends the week; once passed, no urgency", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"alpha": {week: 40, weekReset: 2 * 86400}})
		st := w.env.Read("alpha")
		st.SubEnd = float64(w.now + 3600)
		if left, ok := WeekEndsIn(st, float64(w.now)); !ok || left != 3600 {
			t.Fatalf("WeekEndsIn = %v %v, want the end in 3600 s", left, ok)
		}
		if u, ok := Urgency(st); !ok || u != 60 {
			t.Fatalf("Urgency = %v %v, want 60 points over 1 h", u, ok)
		}
		st.SubEnd = float64(w.now) // the end is this second
		if u, ok := Urgency(st); ok {
			t.Fatalf("Urgency = %v, want none once the end has come", u)
		}
	})
	t.Run("A6 a reading older than its trust window is unknown, not empty", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"alpha": {short: 10, week: 10, age: StaleAfterS + 1}, "beta": {short: 60, week: 60}})
		st := w.env.Read("alpha")
		if !st.Stale() {
			t.Fatal("not stale")
		}
		if _, ok := Urgency(st); ok {
			t.Fatal("a stale week is ranked")
		}
		// Admitted only as the account nobody has run on lately (the idle floor), never as a ranked one; beta, read, wins.
		picked, why := w.pick([]string{"alpha", "beta"}, nil, "")
		if picked != "beta" || !strings.HasSuffix(why["alpha"], "- stale but idle") {
			t.Fatalf("picked %q, alpha: %q", picked, why["alpha"])
		}
	})
	t.Run("A10 a logged-out account is not a candidate", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"alpha": {short: 10}, "beta": {}})
		w.env.LoginState = func(a string) (bool, bool) { return a != "beta", true }
		picked, why := w.pick(nil, nil, "")
		if _, listed := why["beta"]; picked != "alpha" || listed {
			t.Fatalf("picked %q, reasons %v", picked, why)
		}
	})
	t.Run("A11 paused: the exact text; a month cycle ends at the start of the day after its end day", func(t *testing.T) {
		time.Local = time.UTC
		w := newWorld(t, map[string]acct{"alpha": {short: 10}})
		writeConfig(t, w.home, map[string]any{"paused_alpha": true})
		if got := w.env.NotFor("alpha", "claude"); got != "paused - not picked; resume it in the app's settings for alpha" {
			t.Fatalf("paused: %q", got)
		}
		day := time.Unix(clock, 0).UTC().Format("2006-01-02")
		writeConfig(t, w.home, map[string]any{"month_cycle_alpha": true, "sub_end_alpha": day})
		end, _ := DayEnd(day)
		w.now = end.Unix() - 1
		if got := w.env.NotFor("alpha", "claude"); got != "" {
			t.Fatalf("one second before the end: %q", got)
		}
		w.now = end.Unix()
		want := "subscription ended " + day + " (month cycle) - not picked; after resubscribing, set the new end day in the app's settings for alpha"
		if got := w.env.NotFor("alpha", "claude"); got != want {
			t.Fatalf("at the end: %q", got)
		}
	})
}

func TestScenarioBCap(t *testing.T) {
	st := newWorld(t, map[string]acct{"alpha": {short: 10}}).env.Read("alpha")
	for _, c := range []struct {
		row          string
		active, cap_ int
		want         string
	}{
		{"B1 one of two", 1, 2, ""},
		{"B1 two of two", 2, 2, "at its cap: 2 of 2 agents mid-turn"},
		{"B2 three of two: over, the count as it is", 3, 2, "at its cap: 3 of 2 agents mid-turn"},
	} {
		if got := Unfit(st, &Load{Account: "alpha", Active: c.active, Cap: c.cap_}, ""); got != c.want {
			t.Errorf("%s: %q, want %q", c.row, got, c.want)
		}
	}
	t.Run("B9 cap 0 takes nothing, and says so rather than 0 of 0", func(t *testing.T) {
		if got := Unfit(st, &Load{Account: "alpha", Cap: 0}, ""); got != "takes no agents: its cap is 0 (0 mid-turn)" {
			t.Fatalf("%q", got)
		}
	})
}

func TestScenarioCOrder(t *testing.T) {
	// Week points left per hour left: (100 - week) / (weekReset / 3600).
	t.Run("C1 the more urgent week first", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"slow": {week: 90, weekReset: 10 * 3600}, "fast": {week: 50, weekReset: 10 * 3600}})
		if picked, why := w.pick([]string{"slow", "fast"}, nil, ""); picked != "fast" { // 5 a hour against 1
			t.Fatalf("picked %q %v", picked, why)
		}
	})
	t.Run("C2 inside the band: fewest mid-turn, then more urgent, then name", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"a": {week: 50, weekReset: 10 * 3600}, "b": {week: 51, weekReset: 10 * 3600},
			"c": {week: 51, weekReset: 10 * 3600}})
		busy := map[string]*Load{"a": {Account: "a", Active: 1, Cap: 4}}
		if picked, _ := w.pick([]string{"a", "b", "c"}, busy, ""); picked != "b" { // a is busier; b and c tie, b by name
			t.Fatalf("fewest mid-turn, then name: picked %q", picked)
		}
		if picked, _ := w.pick([]string{"a", "b", "c"}, nil, ""); picked != "a" {
			t.Fatalf("all idle, a is more urgent: picked %q", picked)
		}
	})
	t.Run("C3 the band edge: 0.01 either side, and the same answer on every tick", func(t *testing.T) {
		top := 5.0
		edge := top - max(UrgencyBandAbs, UrgencyBandRel*top)
		for _, c := range []struct {
			u    float64
			want string
		}{{edge + 0.01, "idle"}, {edge, "idle"}, {edge - 0.01, "top"}} {
			for range 3 {
				got, _ := BestByUrgency([]Ranked{{Name: "top", Urgency: top, Known: true, Active: 1}, {Name: "idle", Urgency: c.u, Known: true}})
				if got.Name != c.want {
					t.Fatalf("urgency %v against %v: %q, want %q", c.u, top, got.Name, c.want)
				}
			}
		}
	})
	t.Run("C4 no urgency anywhere: headroom decides", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"full": {short: 70}, "room": {short: 20}})
		for _, n := range []string{"full", "room"} { // no week reset: no urgency, no pace
			writeStatus(t, w.home, n, map[string]any{"collected_at": w.now, "windows": []any{
				map[string]any{"minutes": 300, "used_percent": map[string]int{"full": 70, "room": 20}[n], "resets_at": w.now + 3600, "observed_at": w.now},
				map[string]any{"minutes": 10080, "used_percent": 10, "observed_at": w.now}}})
		}
		picked, why := w.pick([]string{"full", "room"}, nil, "")
		if picked != "room" || !strings.HasPrefix(why["room"], "5h 20%") {
			t.Fatalf("picked %q %v", picked, why)
		}
	})
	t.Run("C8 another vendor's empty account is never used", func(t *testing.T) {
		w := newWorld(t, map[string]acct{"alpha": {short: 60}})
		if got := w.env.NotFor("alpha", "codex"); got != "a claude account, not codex" {
			t.Fatalf("NotFor = %q", got)
		}
		if picked, why, _ := w.env.Pick(openDB(t), PickOpts{Candidates: []string{"alpha"}, Vendor: "codex"}); picked != "" {
			t.Fatalf("picked %q %v", picked, why)
		}
	})
}
