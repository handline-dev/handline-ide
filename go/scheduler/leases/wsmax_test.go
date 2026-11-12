package leases

import (
	"slices"
	"strings"
	"testing"
)

// inWorkspaces puts a and b in workspace w1 and c in w2, and gives w1 the kept rule lease_max.browser *max (read at each pass).
func inWorkspaces(m *Manager, max *int) {
	m.WorkspaceOf = func(agent string) string { return map[string]string{"a": "w1", "b": "w1", "c": "w2"}[agent] }
	m.WorkspaceMax = func(kind string) WorkspaceLimits {
		if kind != Browser {
			return WorkspaceLimits{}
		}
		return WorkspaceLimits{Own: map[string]Limit{"w1": {*max, "u1"}}}
	}
}

// wsRig is boardManager with a pool of 2 browsers and inWorkspaces.
func wsRig(max *int) *Manager {
	now := int64(1000)
	m, _ := boardManager(&now)
	m.Max = func(string) int { return 2 }
	inWorkspaces(m, max)
	return m
}

func reservedFor(m *Manager, agent string) bool {
	return slices.ContainsFunc(pool(m, Browser).Held, func(l Lease) bool { return l.Agent == agent && l.Reserved() })
}

func TestAWorkspaceHoldsNoMoreOfAKindThanItsLeaseMax(t *testing.T) {
	max := 1
	m := wsRig(&max)
	if _, err := m.Take(nil, Browser, "a", false, 7, long); err != nil {
		t.Fatal(err)
	}
	_, err := m.Take(nil, Browser, "b", false, 8, long) // a browser is free, but w1 holds its 1
	if bl := blockedOn(t, err); !strings.Contains(bl.Why, "No browser for you yet: your workspace holds 1, its kept rule lease_max.browser 1 (set by u1): T20 now waits") {
		t.Fatalf("%q", bl.Why)
	}
	// Another workspace is not held back, and does not queue behind b: the free browser goes to c at once.
	if c, err := m.Take(nil, Browser, "c", false, 9, long); err != nil || c == nil || c.Agent != "c" {
		t.Fatalf("%+v %v", c, err)
	}
	// Once a gives its back, b's ask is next for w1.
	if !m.Release(1, "a") {
		t.Fatal("not released")
	}
	if !reservedFor(m, "b") {
		t.Fatalf("not reserved for b: %+v", pool(m, Browser).Held)
	}
}

func TestWhenThePoolIsFullTooTheAskIsToldBoth(t *testing.T) {
	max := 1
	m := wsRig(&max)
	for i, agent := range []string{"a", "c"} {
		if _, err := m.Take(nil, Browser, agent, false, 7+i, long); err != nil {
			t.Fatal(err)
		}
	}
	_, err := m.Take(nil, Browser, "b", false, 9, long)
	if why := blockedOn(t, err).Why; !strings.Contains(why, "lease_max.browser 1 (set by u1), and ") || !strings.Contains(why, "held by") {
		t.Fatalf("%q", why)
	}
}

func TestLeaseMaxOfNoneOrAboveThePoolChangesNothing(t *testing.T) {
	for _, max := range []int{0, 3} {
		m := wsRig(&max)
		for i, agent := range []string{"a", "b"} {
			if l, err := m.Take(nil, Browser, agent, false, 7+i, long); err != nil || l == nil {
				t.Fatalf("max %d, %s: %+v %v", max, agent, l, err)
			}
		}
		if _, err := m.Take(nil, Browser, "c", false, 9, long); !strings.Contains(blockedOn(t, err).Why, "No browser is free: T30") {
			t.Fatalf("max %d: %v", max, err)
		}
	}
}

func TestAnAskWaitingOnItsWorkspacesRuleIsGrantedAtTheNextCheckOnceTheRuleIsRaised(t *testing.T) {
	max := 1
	m := wsRig(&max)
	m.Take(nil, Browser, "a", false, 7, long)
	blockedOn(t, func() error { _, err := m.Take(nil, Browser, "b", false, 8, long); return err }())
	max = 2
	m.Check(nil)
	if !reservedFor(m, "b") {
		t.Fatalf("not reserved for b: %+v", pool(m, Browser).Held)
	}
}

// A lease past its max_hold is taken back for an ask the pool holds back, never for one only its own workspace's rule holds back; and
// such an ask does not stop another workspace extending.
func TestOnlyAnAskThePoolHoldsBackTakesALeaseBackOrStopsAnExtend(t *testing.T) {
	r := newHoldRig()
	max := 1
	r.m.Max = func(string) int { return 1 }
	inWorkspaces(r.m, &max)
	if _, err := r.m.Take(nil, Browser, "a", true, 0, long); err != nil {
		t.Fatal(err)
	}
	r.at(1000 + 900) // a's hold is past its max_hold
	blockedOn(t, func() error { _, err := r.m.Take(nil, Browser, "b", false, 8, long); return err }())
	r.at(1000 + 901)
	if h := pool(r.m, Browser).Held; len(h) != 1 || h[0].Agent != "a" || len(r.told) != 0 {
		t.Fatalf("a's hold was taken back for an ask its own workspace's rule holds: %+v told %q", h, r.told)
	}
	// c, in w2, is held back by the pool: a's hold goes, and the browser is reserved for b, first in line.
	blockedOn(t, func() error { _, err := r.m.Take(nil, Browser, "c", false, 9, long); return err }())
	if h := pool(r.m, Browser).Held; len(h) != 1 || h[0].Agent == "a" || !strings.Contains(strings.Join(r.told, "\n"), "a: Your browser (lease 1) was taken back") {
		t.Fatalf("held %+v told %q", h, r.told)
	}
}

func TestAnExtendIsNotRefusedForAnAskOnlyItsOwnWorkspacesRuleHolds(t *testing.T) {
	r := newHoldRig()
	max := 1
	r.m.Max = func(string) int { return 2 }
	inWorkspaces(r.m, &max)
	r.m.Take(nil, Browser, "a", true, 0, long)
	r.m.Take(nil, Browser, "c", true, 0, long)
	blockedOn(t, func() error { _, err := r.m.Take(nil, Browser, "b", false, 8, long); return err }())
	r.at(1000 + 500)
	if _, err := r.m.Extend("c", Browser); err != nil {
		t.Fatalf("c's extend: %v", err)
	}
	if _, err := r.m.Extend("a", Browser); err == nil || !strings.Contains(err.Error(), "an ask waits") {
		t.Fatalf("a's would serve b's ask: %v", err)
	}
}
