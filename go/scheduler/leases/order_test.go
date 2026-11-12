package leases

import (
	"errors"
	"strings"
	"testing"
)

func outOfOrder(t *testing.T, err error) *OutOfOrder {
	t.Helper()
	var o *OutOfOrder
	if !errors.As(err, &o) {
		t.Fatalf("not refused for order: %v", err)
	}
	return o
}

func TestACommandUnderALaterKindIsRefusedNotBlockedForAnEarlierOne(t *testing.T) {
	m, b := cHasAHasUnder7(t, Heavy, Browser)                           // c has the one heavy slot
	o := outOfOrder(t, second(m.Take(nil, Heavy, "a", false, 7, long))) // a nested hl use under the browser's
	if !strings.Contains(o.Why, "hl use heavy browser") || len(b.waits) != 0 {
		t.Fatalf("%q waits %v", o.Why, b.waits)
	}
	if q := pool(m, Heavy).Queue; len(q) != 0 {
		t.Fatalf("queued: %+v", q)
	}
}

func TestATaskThatWaitsForAKindGivesUpItsReservationsOfLaterOnes(t *testing.T) {
	now := int64(1000)
	m, _ := boardManager(&now)
	sim, err := m.Take(nil, Simulator, "c", false, 5, long)
	if err != nil {
		t.Fatal(err)
	}
	br, err := m.Take(nil, Browser, "c", false, 5, long)
	if err != nil {
		t.Fatal(err)
	}
	blockedOn(t, second(m.Take(nil, Browser, "a", false, 7, long)))
	m.Release(br.ID, "c") // the browser is reserved for a's T10
	if h := pool(m, Browser).Held; len(h) != 1 || h[0].Task != 10 {
		t.Fatalf("reserved: %+v", h)
	}
	blockedOn(t, second(m.Take(nil, Browser, "b", false, 8, long)))
	// a's hl use simulator browser comes back; the simulator is held, so T10 waits for it and lets the browser go to b's T20.
	blockedOn(t, second(m.Take(nil, Simulator, "a", false, 9, long)))
	if h := pool(m, Browser).Held; len(h) != 1 || h[0].Task != 20 {
		t.Fatalf("browser: %+v", h)
	}
	m.Release(sim.ID, "c")
	if h := pool(m, Simulator).Held; len(h) != 1 || h[0].Task != 10 {
		t.Fatalf("simulator: %+v", h)
	}
}

func TestAnAskThatCannotCloseACycleWaits(t *testing.T) {
	for name, c := range map[string]struct {
		busy, mine string
		pid        int
	}{
		"another command of a, under a later kind": {Heavy, Browser, 8}, // a's dev server holds the browser
		"the same command, under an earlier kind":  {Browser, Heavy, 7},
	} {
		m, b := cHasAHasUnder7(t, c.busy, c.mine)
		if blockedOn(t, second(m.Take(nil, c.busy, "a", false, c.pid, long))).Task != 10 || len(b.waits) != 1 {
			t.Errorf("%s: waits %v", name, b.waits)
		}
	}
}

func TestAnAgentWithAHoldIsRefusedRatherThanBlocked(t *testing.T) {
	now := int64(1000)
	m, b := boardManager(&now)
	if _, err := m.Take(nil, Browser, "c", false, 5, long); err != nil {
		t.Fatal(err)
	}
	h, err := m.Take(nil, Heavy, "a", true, 0, long)
	if err != nil {
		t.Fatal(err)
	}
	o := outOfOrder(t, second(m.Take(nil, Browser, "a", false, 7, long)))
	if !strings.Contains(o.Why, "--release") || len(b.waits) != 0 {
		t.Fatalf("%q waits %v", o.Why, b.waits)
	}
	// A free lease is still granted beside the hold: only waiting is refused.
	m.Release(h.ID, "a")
	if _, err := m.Take(nil, Simulator, "a", true, 0, long); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Take(nil, Heavy, "a", false, 7, long); err != nil {
		t.Fatalf("a free heavy beside a hold: %v", err)
	}
}

func second(_ *Lease, err error) error { return err }

// cHasAHasUnder7 is boardManager with c holding the one busy lease and a the one of kind mine for its command under pid 7.
func cHasAHasUnder7(t *testing.T, busy, mine string) (*Manager, *board) {
	t.Helper()
	now := int64(1000)
	m, b := boardManager(&now)
	if _, err := m.Take(nil, busy, "c", false, 5, long); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Take(nil, mine, "a", false, 7, long); err != nil {
		t.Fatal(err)
	}
	return m, b
}
