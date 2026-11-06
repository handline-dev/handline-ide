package usage

import (
	"strings"
	"testing"
	"time"
)

func TestSubagentRefusalHoldsTheShortestWindowAndTheWeekToTheirResets(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	accountAt(t, home, 50, 60, 10, 86400) // 50 points left in the hour before the 5h reset
	st, now := e.Read("a"), float64(clock)
	at := func(s float64) string { return time.Unix(int64(s), 0).Format("15:04") }
	// 0.5%/min now and 1%/min more: the 50 points last 2000s of the 3600 to the reset.
	short := Prediction{Short: 2000.0 / 3600, ShortKnown: true, Long: 1, Burn: 0.5 / 60, Extra: 1.0 / 60}
	want := "account a would hit its 5h limit at " + at(now+2000) + ", before its " + at(now+3600) + " reset, with one more subagent"
	if got := SubagentRefusal("a", st, short, now); !strings.HasPrefix(got, want) {
		t.Fatalf("share below 1 is refused with its times:\n got %q\nwant %q...", got, want)
	}
	if got := SubagentRefusal("a", st, Prediction{Short: 1.02, ShortKnown: true, Long: 3, LongKnown: true}, now); got != "" {
		t.Fatalf("both windows outlast their resets: %q", got)
	}
	if got := SubagentRefusal("a", st, Prediction{Short: 0.2, Long: 0.1}, now); got != "" {
		t.Fatalf("nothing known refuses nothing: %q", got)
	}
	if got := SubagentRefusal("a", st, Prediction{Short: 1, ShortKnown: true, Long: 0.06, LongKnown: true}, now); !strings.Contains(got, "would hit its weekly limit") {
		t.Fatalf("the week running out first: %q", got)
	}
}
