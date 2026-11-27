package chatterm

import (
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
)

// The limit the stream named holds until its window resets.
func TestALimitHoldsUntilItsWindowResets(t *testing.T) {
	for _, c := range []struct {
		l    agent.Limit
		want bool
	}{
		{agent.Limit{Blocked: true, ResetsAt: 200}, true},
		{agent.Limit{Blocked: true, ResetsAt: 100}, false},
		{agent.Limit{Blocked: true}, true},
		{agent.Limit{ResetsAt: 200}, false},
	} {
		if got := atLimit(c.l, 100); got != c.want {
			t.Errorf("%+v: %v", c.l, got)
		}
	}
}
