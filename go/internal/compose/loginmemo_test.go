package compose

import (
	"testing"

	"github.com/handline/handline/go/data/states"
)

// The login check behind a sample runs once per account per LoginCheckCooldownS, not on every sample.
func TestLoginMemoAsksOncePerCooldown(t *testing.T) {
	t.Parallel()
	asked := 0
	out := true
	m := &loginMemo{ask: func(string, int64) bool { asked++; return out }, at: map[string]loginAnswer{}}
	if !m.loggedOut("a", 1000) {
		t.Fatal("the first answer is the check's")
	}
	out = false
	if !m.loggedOut("a", 1000+states.LoginCheckCooldownS-1) || asked != 1 {
		t.Fatalf("within the cooldown the answer is kept: asked %d", asked)
	}
	if m.loggedOut("a", 1000+states.LoginCheckCooldownS) || asked != 2 {
		t.Fatalf("after it the check runs again: asked %d", asked)
	}
	if m.loggedOut("b", 1000) || asked != 3 {
		t.Fatalf("each account has its own: asked %d", asked)
	}
}
