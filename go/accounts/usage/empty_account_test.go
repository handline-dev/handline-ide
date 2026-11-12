package usage

import "testing"

// An account at 0% on both windows halfway through its week takes a waiting start: with the launch pace hold on and its costs measured,
// the only thing that keeps a start off it is the cap. A week nearly spent shows the week check is live.
func TestAnEmptyAccountIsKeptOffOnlyByItsCap(t *testing.T) {
	t.Parallel()
	usedAt := func(week int, age int64) map[string]any {
		return map[string]any{"collected_at": clock - age, "windows": []any{
			map[string]any{"minutes": 300, "used_percent": 0, "resets_at": clock + 3600, "observed_at": clock - age},
			map[string]any{"minutes": 10080, "used_percent": week, "resets_at": clock + WeekS/2, "observed_at": clock - age}}}
	}
	burn := 0.01 // percent of the 5h window per second already burning on it
	for _, c := range []struct {
		name   string
		status any
		stale  bool
		active int
		want   string
	}{
		{"fresh, nobody on it", usedAt(0, 5), false, 0, ""},
		{"fresh, one short of the cap", usedAt(0, 5), false, 1, ""},
		{"fresh, at the cap", usedAt(0, 5), false, 2, "at its cap: 2 of 2 agents mid-turn"},
		{"fresh, past the cap", usedAt(0, 5), false, 3, "at its cap: 3 of 2 agents mid-turn"},
		{"as old as a fresh reading may be", usedAt(0, StaleAfterS), false, 0, ""},
		{"one second stale", usedAt(0, StaleAfterS+1), true, 0, ""},
		{"stale at the cap", usedAt(0, StaleAfterS+1), true, 2, "at its cap: 2 of 2 agents mid-turn"},
		{"no reading", nil, true, 0, ""},
		{"no reading at the cap", nil, true, 2, "at its cap: 2 of 2 agents mid-turn"},
		{"control: a week nearly spent is refused", usedAt(99, 5), false, 0, "7d 99%, resets in 3d12h: one more agent lasts ~7m of it - under the 30m a start is worth"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			writeStatus(t, home, "a", c.status)
			st := testEnv(home, clock).Read("a")
			if st.Stale() != c.stale {
				t.Fatalf("Stale() = %v, want %v", st.Stale(), c.stale)
			}
			ld := &Load{Account: "a", Active: c.active, Cap: 2, Burn: &burn, Costs: costsAt([]float64{0.01}, map[string]float64{"*": 0.1}, "a"), Hold: true}
			if got := Unfit(st, ld, ""); got != c.want {
				t.Fatalf("Unfit = %q, want %q", got, c.want)
			}
		})
	}
}
