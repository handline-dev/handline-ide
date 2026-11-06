package verdict

import "testing"

func TestOverdue(t *testing.T) {
	const now = 1_000_000
	for _, c := range []struct {
		name      string
		state     string
		holder    Holder
		waits     bool
		timeoutAt int64
		want      bool
	}{
		{"past its deadline", "doing", AgentHolds, false, now - 1, true},
		{"at its deadline", "doing", AgentHolds, false, now, true},
		{"before its deadline", "doing", AgentHolds, false, now + 1, false},
		{"no clock", "doing", AgentHolds, false, 0, false},
		{"queued", "queued", AgentHolds, false, now - 1, false},
		{"done", "done", AgentHolds, false, now - 1, false},
		{"waits on a blocker or child", "doing", AgentHolds, true, now - 1, false},
		{"the founder holds it", "doing", FounderHolds, false, now - 1, false},
		{"a gate holds it", "doing", GateHolds, false, now - 1, false},
	} {
		if got := Overdue(c.state, c.holder, c.waits, c.timeoutAt, now); got != c.want {
			t.Errorf("%s: Overdue = %v, want %v", c.name, got, c.want)
		}
	}
}
