package leases

import (
	"strings"
	"testing"
)

// sharedManager is boardManager whose limits are the shared runs' only: This Mac's x, and w1's and w2's own y, one at a time each.
func sharedManager(t *testing.T) (*Manager, *board) {
	t.Helper()
	now := int64(1000)
	m, b := boardManager(&now)
	m.Max = nil
	m.Shared = func() []Shared { return []Shared{{"mac:x", 1}, {"w1:y", 1}, {"w2:y", 1}} }
	return m, b
}

// A shared run's turns are counted by its kind: This Mac's across every workspace, a workspace's among its own agents only, which no
// other workspace's run of the same name holds back; heavy, simulator and browser limit nothing without Max.
func TestASharedRunWaitsOnlyForItsOwnTurns(t *testing.T) {
	m, _ := sharedManager(t)
	for _, k := range []struct{ kind, agent string }{{"w1:y", "a"}, {"w2:y", "b"}, {"mac:x", "a"}, {Browser, "b"}, {Browser, "c"}, {Heavy, "c"}} {
		if l, err := m.Take(nil, k.kind, k.agent, false, 5, long); err != nil || l == nil {
			t.Fatalf("%s for %s: %v %v", k.kind, k.agent, l, err)
		}
	}
	bl := blockedOn(t, second(m.Take(nil, "mac:x", "b", false, 8, long)))
	if !strings.Contains(bl.Why, "No turn at x is free") {
		t.Fatalf("why: %q", bl.Why)
	}
	blockedOn(t, second(m.Take(nil, "w1:y", "c", false, 9, long)))
	p := pool(m, "mac:x")
	if p.Max != 1 || len(p.Held) != 1 || len(p.Queue) != 1 || p.Queue[0].Agent != "b" || !strings.Contains(p.Queue[0].Says, "waiting for a turn at x, 1st") {
		t.Fatalf("mac:x %+v", p)
	}
	if got := len(m.View()); got != 7 {
		t.Fatalf("pools %d", got)
	}
}

// Shared runs come after the fixed kinds and among themselves by name, so a command under one may not wait for an earlier one.
func TestACommandUnderASharedRunIsRefusedForAnEarlierOne(t *testing.T) {
	m, b := sharedManager(t)
	m.Shared = func() []Shared { return []Shared{{"mac:x", 1}, {"mac:a", 1}} }
	if _, err := m.Take(nil, "mac:a", "c", false, 5, long); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Take(nil, "mac:x", "a", false, 7, long); err != nil {
		t.Fatal(err)
	}
	o := outOfOrder(t, second(m.Take(nil, "mac:a", "a", false, 7, long)))
	if !strings.Contains(o.Why, "hl use a x") || len(b.waits) != 0 {
		t.Fatalf("%q waits %v", o.Why, b.waits)
	}
	if !after("mac:x", "mac:a") || after("mac:a", "mac:x") || !after("w1:a", Port) || after(Port, "mac:a") || !after("w2:b", "mac:b") {
		t.Fatal("order")
	}
}

// A run removed while held keeps its lease until it ends, and an ask for it then limits nothing.
func TestARemovedRunLimitsNothing(t *testing.T) {
	m, _ := sharedManager(t)
	if _, err := m.Take(nil, "mac:x", "a", false, 5, long); err != nil {
		t.Fatal(err)
	}
	m.Shared = func() []Shared { return nil }
	if l, err := m.Take(nil, "mac:x", "b", false, 8, long); err != nil || l == nil {
		t.Fatalf("after the remove: %v %v", l, err)
	}
	if p := pool(m, "mac:x"); p.Max != 0 || len(p.Held) != 2 {
		t.Fatalf("pool %+v", p)
	}
}
