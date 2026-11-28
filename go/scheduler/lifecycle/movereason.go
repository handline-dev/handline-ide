package lifecycle

import (
	"fmt"

	"github.com/handline/handline/go/data/states"
)

// moveReason is the sentence a move keeps for a person (moved_account's reason): why it left from, with the numbers read off from now.
func (m *Manager) moveReason(trigger, from string) string {
	st, name := m.Launch.AccountUsage(from), m.Launch.AccountLabel(from)
	short := &WakeWait{Account: from, Label: name, Window: st.ShortLabel, Reset: st.ShortReset}
	long := &WakeWait{Account: from, Label: name, Window: st.LongLabel, Reset: st.LongReset, IsLong: true}
	switch trigger {
	case "usage":
		if st.ShortTrusted != nil {
			short.Pct = *st.ShortTrusted
			return waitReason(short)
		}
	case "usage_7d":
		if st.LongTrusted != nil {
			long.Pct = *st.LongTrusted
			return waitReason(long)
		}
	case states.EarlyUsageTrigger:
		w := short
		switch {
		case st.LongTrusted != nil && *st.LongTrusted >= states.MoveBeforeWeekPct:
			w = long
			w.Pct = *st.LongTrusted
		case st.ShortTrusted != nil:
			w.Pct = *st.ShortTrusted
		default:
			return states.MovedWhy(trigger, name)
		}
		return fmt.Sprintf("%s's %s was at %d%%, so it moved before running out%s.", name, windowWords(w.Window), w.Pct, resets(w, " (resets %s)"))
	}
	return states.MovedWhy(trigger, name)
}

// name is the account as a person sees it.
func (m *Manager) name(acct string) string { return m.Launch.AccountLabel(acct) }

// waitReason is a launch's move off the account w kept it from, said for a person.
func waitReason(w *WakeWait) string {
	if w.Paused {
		return w.Name() + " was paused."
	}
	return fmt.Sprintf("%s's %s was at %d%%%s.", w.Name(), windowWords(w.Window), w.Pct, resets(w, " (resets %s)"))
}

// resets is f with w's reset clock in it, "" when the reset is unknown.
func resets(w *WakeWait, f string) string {
	if w.Reset == 0 {
		return ""
	}
	return fmt.Sprintf(f, WaitClock(w, ""))
}

// windowWords is a window's label ("5h", "7d") as a person says it.
func windowWords(label string) string {
	switch label {
	case "7d":
		return "weekly limit"
	case "?", "":
		return "usage window"
	}
	if n := len(label); n > 1 && label[n-1] == 'h' {
		return label[:n-1] + "-hour window"
	}
	return label + " window"
}

// repickReason is a cold wake's move to to (WorthRepick): its prompt cache had expired and to's week resets sooner with more of it unused.
func (m *Manager) repickReason(to string) string {
	st := m.Launch.AccountUsage(to)
	said := ""
	if st.LongTrusted != nil {
		said = fmt.Sprintf(" (%d%% used%s)", *st.LongTrusted, resets(&WakeWait{Reset: st.LongReset, IsLong: true}, ", resets %s"))
	}
	return "Its prompt cache had expired, so it moved to " + m.Launch.AccountLabel(to) + ", whose week resets sooner" + said + "."
}
