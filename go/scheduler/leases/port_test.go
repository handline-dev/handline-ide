package leases

import (
	"net"
	"testing"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/scheduler/procload"
)

func TestAPortLeaseTakesTheLowestPortOfTheRangeNoLeaseHoldsAndThatBinds(t *testing.T) {
	taken := map[int]bool{4001: true} // another process listens on 4001
	m := &Manager{Now: func() int64 { return 1000 }, Ports: func() (int, int, bool) { return 4000, 4002, true },
		PortFree: func(p int) bool { return !taken[p] }}
	a, err := m.Take(nil, Port, "a", true, 0, long)
	if err != nil || a.Port != 4000 {
		t.Fatalf("a: %+v %v", a, err)
	}
	b, err := m.Take(nil, Port, "b", true, 0, long)
	if err != nil || b.Port != 4002 {
		t.Fatalf("b: %+v %v", b, err)
	}
	if p := pool(m, Port); p.Max != 3 || len(p.Held) != 2 {
		t.Fatalf("pool %+v", p)
	}
	// Every port of the range is held or taken: the next ask waits, and gets 4000 once a gives it back.
	c := askKind(t, m, Port, "c")
	none(t, c)
	m.Release(a.ID, "a")
	if got := got(t, c); got.l == nil || got.l.Port != 4000 {
		t.Fatalf("c: %+v", got)
	}
}

func TestWithNoRangeAPortLeaseIsAFreePortTheSystemGivesAndNoneWaits(t *testing.T) {
	m := &Manager{Now: func() int64 { return 1000 }}
	a, err := m.Take(nil, Port, "a", false, 7, long)
	if err != nil || a.Port < 1 {
		t.Fatalf("a: %+v %v", a, err)
	}
	if p := pool(m, Port); p.Max != 0 {
		t.Fatalf("a range-less pool has no limit: %+v", p)
	}
}

func TestPortRangeReadsLoHi(t *testing.T) {
	for s, want := range map[string][2]int{"4000-4999": {4000, 4999}, "1024-1024": {1024, 1024}, "65535-65535": {65535, 65535}} {
		if lo, hi, ok := PortRange(usage.Config{PortRangeKey: s}); !ok || lo != want[0] || hi != want[1] {
			t.Errorf("%q: %d %d %v", s, lo, hi, ok)
		}
	}
	for _, s := range []string{"", "4000", "5000-4000", "0-10", "80-90", "1023-2000", "1024-65536", "99999-99999", "a-b", " 4000-4999", "4000 - 4999", "+4000-4999"} {
		if _, _, ok := ParsePortRange(s); ok {
			t.Errorf("%q read as a range", s)
		}
	}
}

func TestHeldPortLeasesMatchNoProcessAndTheCheckGoesOn(t *testing.T) {
	m := &Manager{Now: func() int64 { return 1000 }}
	a, err := m.Take(nil, Port, "a", true, 0, long)
	if err != nil {
		t.Fatal(err)
	}
	m.Check([]procload.Proc{{PID: 9, Agent: "a", Cmd: "node server.js"}})
	if p := pool(m, Port); len(p.Held) != 1 || p.Held[0].Port != a.Port {
		t.Fatalf("%+v", p)
	}
}

func TestAPortListenedOnTheWildcardIsNotFree(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()
	if (&Manager{}).portFree(ln.Addr().(*net.TCPAddr).Port) {
		t.Fatal("a port a server listens on read as free")
	}
}
