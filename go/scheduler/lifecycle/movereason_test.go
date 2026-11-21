package lifecycle

import (
	"testing"
	"time"

	"github.com/handline/handline/go/data/states"
)

// usageLaunch reads each account's usage from st.
type usageLaunch struct {
	fakeLaunch
	st map[string]AccountStatus
}

func (l usageLaunch) AccountUsage(acct string) AccountStatus { return l.st[acct] }

// Each trigger's move keeps one sentence with the numbers that decided it; one with nothing to read says it from the trigger alone.
func TestEachMoveSaysWhyWithItsNumbers(t *testing.T) {
	t.Parallel()
	pct := func(n int) *int { return &n }
	eleven := time.Date(2026, 11, 21, 11, 0, 0, 0, time.Local).Unix()
	week := time.Date(2026, 11, 24, 9, 30, 0, 0, time.Local).Unix()
	m := &Manager{Launch: usageLaunch{st: map[string]AccountStatus{
		"delta":  {ShortTrusted: pct(82), LongTrusted: pct(40), ShortLabel: "5h", LongLabel: "7d", ShortReset: eleven, LongReset: week},
		"walled": {ShortTrusted: pct(100), LongTrusted: pct(99), ShortLabel: "5h", LongLabel: "7d", ShortReset: eleven, LongReset: week},
		"spent":  {ShortTrusted: pct(30), LongTrusted: pct(98), ShortLabel: "5h", LongLabel: "7d", ShortReset: eleven, LongReset: week},
	}}}
	for _, c := range []struct{ trigger, from, want string }{
		{"forced", "delta", "You moved it."},
		{"usage", "walled", "walled's 5-hour window was at 100% (resets 11:00)."},
		{"usage_7d", "walled", "walled's weekly limit was at 99% (resets 11-24 09:30)."},
		{states.EarlyUsageTrigger, "delta", "delta's 5-hour window was at 82%, so it moved before running out (resets 11:00)."},
		{states.EarlyUsageTrigger, "spent", "spent's weekly limit was at 98%, so it moved before running out (resets 11-24 09:30)."},
		{LoginTrigger, "delta", "delta's login expired."},
		{PausedTrigger, "delta", "delta was paused."},
		{"usage", "unread", "unread's 5-hour limit was reached."},
	} {
		if got := m.moveReason(c.trigger, c.from); got != c.want {
			t.Errorf("%s off %s: %q, want %q", c.trigger, c.from, got, c.want)
		}
	}
	if got := m.repickReason("spent"); got != "Its prompt cache had expired, so it moved to spent, whose week resets sooner (98% used, resets 11-24 09:30)." {
		t.Errorf("repick: %q", got)
	}
}
