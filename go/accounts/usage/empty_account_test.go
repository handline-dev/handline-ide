package usage

import "testing"

// Nothing but a wall keeps a start off an account: not how many agents are on it, nor a stale or missing reading, nor a week nearly spent.
func TestOnlyAWallKeepsAStartOffAnAccount(t *testing.T) {
	t.Parallel()
	usedAt := func(five, week int, age int64) map[string]any {
		return map[string]any{"collected_at": clock - age, "windows": []any{
			map[string]any{"minutes": 300, "used_percent": five, "resets_at": clock + 3600, "observed_at": clock - age},
			map[string]any{"minutes": 10080, "used_percent": week, "resets_at": clock + WeekS/2, "observed_at": clock - age}}}
	}
	for _, c := range []struct {
		name   string
		status any
		stale  bool
		want   string
	}{
		{"fresh and empty", usedAt(0, 0, 5), false, ""},
		{"one second stale", usedAt(0, 0, StaleAfterS+1), true, ""},
		{"no reading", nil, true, ""},
		{"a week nearly spent", usedAt(0, 99, 5), false, ""},
		{"the 5-hour wall", usedAt(100, 0, 5), false, "5h usage at the wall (100%)"},
		{"the weekly wall", usedAt(0, 100, 5), false, "7d usage 100% at the wall"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			writeStatus(t, home, "a", c.status)
			st := testEnv(home, clock).Read("a")
			if st.Stale() != c.stale {
				t.Fatalf("Stale() = %v, want %v", st.Stale(), c.stale)
			}
			if got := Unfit(st); got != c.want {
				t.Fatalf("Unfit = %q, want %q", got, c.want)
			}
		})
	}
}
