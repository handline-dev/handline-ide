package lifecycle

import (
	"testing"
	"time"

	"github.com/handline/handline/go/data/states"
)

// usageLaunch reads each account's usage from st and its name from names.
type usageLaunch struct {
	fakeLaunch
	st    map[string]AccountStatus
	names map[string]string
}

func (l usageLaunch) AccountUsage(acct string) AccountStatus { return l.st[acct] }

func (l usageLaunch) AccountLabel(acct string) string {
	if n, ok := l.names[acct]; ok {
		return n
	}
	return acct
}

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

// A move's sentence names the account by its name; one without a name, by its key.
func TestAMoveSaysTheAccountsName(t *testing.T) {
	t.Parallel()
	pct := func(n int) *int { return &n }
	eleven := time.Date(2026, 11, 21, 11, 0, 0, 0, time.Local).Unix()
	week := time.Date(2026, 11, 24, 9, 30, 0, 0, time.Local).Unix()
	st := AccountStatus{ShortTrusted: pct(100), LongTrusted: pct(98), ShortLabel: "5h", LongLabel: "7d", ShortReset: eleven, LongReset: week}
	m := &Manager{Launch: usageLaunch{st: map[string]AccountStatus{"gamma": st, "delta": st}, names: map[string]string{"gamma": "Work"}}}
	for _, c := range []struct{ trigger, from, want string }{
		{"usage", "gamma", "Work's 5-hour window was at 100% (resets 11:00)."},
		{states.EarlyUsageTrigger, "gamma", "Work's weekly limit was at 98%, so it moved before running out (resets 11-24 09:30)."},
		{LoginTrigger, "gamma", "Work's login expired."},
		{"usage", "delta", "delta's 5-hour window was at 100% (resets 11:00)."},
	} {
		if got := m.moveReason(c.trigger, c.from); got != c.want {
			t.Errorf("%s off %s: %q, want %q", c.trigger, c.from, got, c.want)
		}
	}
	if got := m.repickReason("gamma"); got != "Its prompt cache had expired, so it moved to Work, whose week resets sooner (98% used, resets 11-24 09:30)." {
		t.Errorf("repick: %q", got)
	}
	if got := (&Mover{M: m}).OfferWords("gamma"); got != "Work is the closest, and may reach its limit again soon." {
		t.Errorf("offer: %q", got)
	}
	if got := (&Mover{M: m}).OfferWords("delta"); got != "delta is the closest, and may reach its limit again soon." {
		t.Errorf("offer, no name: %q", got)
	}
	named := &WakeWait{Account: "gamma", Label: "Work", Pct: 100, Window: "5h", Reset: eleven}
	if got := waitReason(named); got != "Work's 5-hour window was at 100% (resets 11:00)." {
		t.Errorf("wait: %q", got)
	}
	if got := DescribeWakeWait(named); got != "waiting for Work's 5h window (100%, rolls over 11:00)" {
		t.Errorf("describe: %q", got)
	}
	if got := DescribeWakeWait(&WakeWait{Account: "gamma", Paused: true, Slot: true}); got != "waiting for gamma: it is paused; resume it in the app's settings" {
		t.Errorf("paused, no name: %q", got)
	}
}

// A wall or a pause names its account by its name, and keeps the key as the account.
func TestAWaitNamesItsAccount(t *testing.T) {
	t.Parallel()
	pct := 99
	l := &Launcher{Ports: LaunchPorts{Usage: fakeUsage{names: map[string]string{"gamma": "Work"}, by: map[string]AccountStatus{
		"gamma": {ShortTrusted: &pct, ShortLabel: "5h"}, "delta": {ShortTrusted: &pct, ShortLabel: "5h"}, "held": {Paused: true}}}}}
	if w := l.wall("gamma", false); w == nil || w.Account != "gamma" || w.Name() != "Work" {
		t.Errorf("named wall: %+v", w)
	}
	if w := l.wall("delta", false); w == nil || w.Name() != "delta" {
		t.Errorf("wall with no name: %+v", w)
	}
	if w := l.paused("held", false); w == nil || w.Name() != "held" {
		t.Errorf("paused: %+v", w)
	}
}
