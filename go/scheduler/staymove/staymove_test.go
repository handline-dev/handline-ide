package staymove

import (
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }

const (
	window5h = 5 * 3600.0
	hour     = 3600.0
	minute   = 60.0
)

func TestTheWallChoiceInScenarios(t *testing.T) {
	for _, c := range []struct {
		name string
		s    Situation
		move bool
		why  string
	}{
		// alpha at 100%, reset in 33m, a cold start measured at 0, the session closed since 23:49: the room test then finds delta.
		{"a full account, a free start, a closed session", Situation{WorkWaiting: true, ResetInS: f(33 * minute), ColdStartPct: f(0),
			WindowS: window5h, TurnEndedS: f(157 * minute), CacheLifeS: hour}, true, "prompt cache is cold"},
		{"a free start and a warm cache", Situation{WorkWaiting: true, ResetInS: f(33 * minute), ColdStartPct: f(0), WindowS: window5h,
			TurnEndedS: f(minute), CacheLifeS: hour}, true, "resets too late"},
		{"never measured, a warm cache: the default T waits", Situation{WorkWaiting: true, ResetInS: f(33 * minute), WindowS: window5h,
			TurnEndedS: f(minute), CacheLifeS: hour}, false, "within 39m"},
		{"a dear start and a warm cache", Situation{WorkWaiting: true, ResetInS: f(33 * minute), ColdStartPct: f(13), WindowS: window5h,
			TurnEndedS: f(minute), CacheLifeS: hour}, false, "cheaper to wait"},
		{"a start dearer than the cache lives is capped at its life", Situation{WorkWaiting: true, ResetInS: f(50 * minute),
			ColdStartPct: f(40), WindowS: window5h, CacheLifeS: hour}, false, "within 60m"},
		{"a cache that lives exactly to the reset is still warm", Situation{WorkWaiting: true, ResetInS: f(50 * minute),
			ColdStartPct: f(40), WindowS: window5h, TurnEndedS: f(10 * minute), CacheLifeS: hour}, false, "within 60m"},
		{"the cache's age decides past the cap", Situation{WorkWaiting: true, ResetInS: f(50 * minute), ColdStartPct: f(40),
			WindowS: window5h, TurnEndedS: f(11 * minute), CacheLifeS: hour}, true, "prompt cache is cold"},
		{"nothing waits", Situation{ResetInS: f(5 * hour), ColdStartPct: f(0), WindowS: window5h, TurnEndedS: f(3 * hour),
			CacheLifeS: hour}, false, "no work is waiting"},
		{"an unknown reset", Situation{WorkWaiting: true, WindowS: window5h, TurnEndedS: f(minute), CacheLifeS: hour}, true,
			"reset is unknown"},
	} {
		move, why := Decide(c.s)
		if move != c.move || !strings.Contains(why, c.why) {
			t.Errorf("%s: move %v %q", c.name, move, why)
		}
	}
}

func TestOnlyANeverMeasuredStartTakesTheDefault(t *testing.T) {
	for _, c := range []struct {
		pct     *float64
		windowS float64
		want    float64
	}{{nil, window5h, DefaultStayWithinS}, {f(0), window5h, 0}, {f(1), window5h, 180}, {f(5), 0, DefaultStayWithinS}} {
		if got := StayWithin(c.pct, c.windowS); got != c.want {
			t.Errorf("StayWithin(%v, %v) = %v, want %v", c.pct, c.windowS, got, c.want)
		}
	}
}

// Rows A7-A9 of docs/design/account-assignment-scenarios.md: the week wall with work waiting, the cache's edge to the second, and T.
func TestScenarioAWallRows(t *testing.T) {
	warm := func(reset, ended float64, cold *float64) Situation {
		return Situation{WorkWaiting: true, ResetInS: f(reset), ColdStartPct: cold, WindowS: window5h, TurnEndedS: f(ended), CacheLifeS: hour}
	}
	for _, c := range []struct {
		row  string
		s    Situation
		move bool
	}{
		{"A7 reset in 10m, warm cache: waits", warm(10*minute, minute, nil), false},
		{"A7 reset in 10m, cold cache: moves", warm(10*minute, hour, nil), true},
		{"A8 turn ended + reset equal to the cache's life: stays", warm(30*minute, 30*minute, nil), false},
		{"A8 one second more: moves", warm(30*minute, 30*minute+1, nil), true},
		{"A9 never measured: T is 39m, so a 38m reset waits", warm(38*minute, 0, nil), false},
		{"A9 measured 0: T is 0, so it moves", warm(38*minute, 0, f(0)), true},
	} {
		if move, why := Decide(c.s); move != c.move {
			t.Errorf("%s: move %v (%s)", c.row, move, why)
		}
	}
}
