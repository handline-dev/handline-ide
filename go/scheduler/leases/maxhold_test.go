package leases

import (
	"fmt"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/handline/handline/go/accounts/usage"
)

// holdRig is boardManager with a 10-minute max_hold, every process begun at 0, and what was told and signalled kept.
type holdRig struct {
	m       *Manager
	b       *board
	now     int64
	told    []string // "a: text"
	signals []string // "TERM 7"
}

func newHoldRig() *holdRig {
	r := &holdRig{now: 1000}
	r.m, r.b = boardManager(&r.now)
	r.m.MaxHold = func(string) int64 { return 600 }
	r.m.Started = func(int) (int64, bool) { return 0, true }
	r.m.Tell = func(agent, text string) { r.told = append(r.told, agent+": "+text) }
	r.m.Signal = func(pid int, sig syscall.Signal) error {
		name := map[syscall.Signal]string{syscall.SIGTERM: "TERM", syscall.SIGKILL: "KILL"}[sig]
		r.signals = append(r.signals, fmt.Sprintf("%s %d", name, pid))
		return nil
	}
	return r
}

func (r *holdRig) at(t int64) { r.now = t; r.m.Check(nil) }

func TestALeaseIsWarnedOnceAt80PercentOfItsMaxHold(t *testing.T) {
	r := newHoldRig()
	l, err := r.m.Take(nil, Browser, "a", true, 0, long)
	if err != nil {
		t.Fatal(err)
	}
	r.at(1000 + 479)
	if len(r.told) != 0 {
		t.Fatalf("too soon: %q", r.told)
	}
	r.at(1000 + 480)
	r.at(1000 + 500)
	if len(r.told) != 1 || !strings.Contains(r.told[0], fmt.Sprintf("a: Your browser (lease %d) reaches its max_hold in 2 min", l.ID)) {
		t.Fatalf("%q", r.told)
	}
}

func TestPastItsMaxHoldALeaseOnlyGoesWhenSomeoneAsks(t *testing.T) {
	r := newHoldRig()
	if _, err := r.m.Take(nil, Browser, "a", false, 7, long); err != nil {
		t.Fatal(err)
	}
	r.m.Check(nil) // a's command has no process under it but 7
	r.at(1000 + 900)
	if h := pool(r.m, Browser).Held; len(h) != 1 || h[0].Agent != "a" {
		t.Fatalf("taken back with nobody asking: %+v", h)
	}
	b, err := r.m.Take(nil, Browser, "b", false, 8, long) // b's ask takes it back and is granted at once
	if err != nil || b == nil || b.Agent != "b" {
		t.Fatalf("%+v %v", b, err)
	}
	if !slices.Equal(r.signals, []string{"TERM 7"}) || !strings.Contains(r.told[len(r.told)-1], "a: Your browser (lease 1) was taken back") {
		t.Fatalf("signals %q told %q", r.signals, r.told)
	}
	r.at(1000 + 909)
	if len(r.signals) != 1 {
		t.Fatalf("killed too soon: %q", r.signals)
	}
	r.at(1000 + 910)
	if !slices.Equal(r.signals, []string{"TERM 7", "KILL 7"}) {
		t.Fatalf("signals %q", r.signals)
	}
	r.at(1000 + 920)
	if len(r.signals) != 2 {
		t.Fatalf("killed twice: %q", r.signals)
	}
}

func TestAWaitingTaskGetsAHoldPastItsMaxHoldAtTheNextCheck(t *testing.T) {
	r := newHoldRig()
	if _, err := r.m.Take(nil, Browser, "a", true, 0, long); err != nil {
		t.Fatal(err)
	}
	blockedOn(t, second(r.m.Take(nil, Browser, "b", false, 8, long)))
	r.at(1000 + 599)
	if h := pool(r.m, Browser).Held; len(h) != 1 || h[0].Agent != "a" {
		t.Fatalf("too soon: %+v", h)
	}
	r.at(1000 + 600)
	if h := pool(r.m, Browser).Held; len(h) != 1 || h[0].Task != 20 {
		t.Fatalf("not reserved for T20: %+v", h)
	}
	if len(r.signals) != 0 || !strings.Contains(r.told[len(r.told)-1], ": stop using it") {
		t.Fatalf("a hold has no process to signal: %q %q", r.signals, r.told)
	}
}

func TestExtendAddsAPeriodTwiceAtMostAndNotWhileAnyoneWaits(t *testing.T) {
	r := newHoldRig()
	if _, err := r.m.Take(nil, Browser, "a", true, 0, long); err != nil {
		t.Fatal(err)
	}
	r.at(1000 + 500) // warned
	for i := 1; i <= MaxExtends; i++ {
		ls, err := r.m.Extend("a", "")
		if err != nil || len(ls) != 1 || ls[0].Extended != i || ls[0].MaxHoldAt != 1000+600*int64(1+i) {
			t.Fatalf("extend %d: %+v %v", i, ls, err)
		}
	}
	if _, err := r.m.Extend("a", Browser); err == nil || !strings.Contains(err.Error(), "extended 2 times") {
		t.Fatalf("a third: %v", err)
	}
	r.at(1000 + 1700) // warned again in the new period
	if len(r.told) != 2 {
		t.Fatalf("told %q", r.told)
	}
	r2 := newHoldRig()
	if _, err := r2.m.Take(nil, Browser, "a", true, 0, long); err != nil {
		t.Fatal(err)
	}
	blockedOn(t, second(r2.m.Take(nil, Browser, "b", false, 8, long)))
	if _, err := r2.m.Extend("a", ""); err == nil || !strings.Contains(err.Error(), "an ask waits") {
		t.Fatalf("while b waits: %v", err)
	}
	if _, err := r2.m.Extend("c", ""); err == nil || !strings.Contains(err.Error(), "no lease with a max_hold") {
		t.Fatalf("c holds none: %v", err)
	}
}

func TestWithNoLimitALeaseHasNoMaxHold(t *testing.T) {
	r := newHoldRig()
	r.m.Max = func(string) int { return 0 }
	if _, err := r.m.Take(nil, Heavy, "a", true, 0, long); err != nil {
		t.Fatal(err)
	}
	r.at(1000 + 10000)
	if len(r.told) != 0 {
		t.Fatalf("%q", r.told)
	}
	if _, err := r.m.Extend("a", ""); err == nil || !strings.Contains(err.Error(), "no lease with a max_hold") {
		t.Fatalf("%v", err)
	}
}

func TestMaxHoldReadsMinutesOrTheKindsDefault(t *testing.T) {
	cfg := usage.Config{MaxHoldKey(Simulator): 5, MaxHoldKey(Browser): 0, MaxHoldKey(Heavy): -3}
	for kind, want := range map[string]int64{Simulator: 300, Browser: 0, Heavy: 1800, Port: 0} {
		if got := MaxHold(cfg, kind); got != want {
			t.Errorf("%s: %d", kind, got)
		}
	}
	if MaxHold(nil, Simulator) != 3600 {
		t.Error("simulator default")
	}
}

func TestExtendWithNoKindPassesOverTheLeasesWithNoMaxHold(t *testing.T) {
	r := newHoldRig()
	r.m.MaxHold = func(kind string) int64 { return map[string]int64{Browser: 600}[kind] }
	if _, err := r.m.Take(nil, Heavy, "a", false, 7, long); err != nil {
		t.Fatal(err)
	}
	if _, err := r.m.Take(nil, Browser, "a", false, 7, long); err != nil {
		t.Fatal(err)
	}
	if ls, err := r.m.Extend("a", ""); err != nil || len(ls) != 1 || ls[0].Kind != Browser {
		t.Fatalf("%+v %v", ls, err)
	}
	if _, err := r.m.Extend("a", Heavy); err == nil || !strings.Contains(err.Error(), "no max_hold") {
		t.Fatalf("named: %v", err)
	}
}

func TestExtendIsRefusedWhenTheLeaseWouldStillBePastItsMaxHold(t *testing.T) {
	r := newHoldRig()
	if _, err := r.m.Take(nil, Browser, "a", true, 0, long); err != nil {
		t.Fatal(err)
	}
	r.now = 1000 + 1200
	if _, err := r.m.Extend("a", ""); err == nil || !strings.Contains(err.Error(), "would still be past") {
		t.Fatalf("%v", err)
	}
}

func TestAProcessThatBeganAfterItWasSeenIsNotSignalled(t *testing.T) {
	r := newHoldRig()
	if _, err := r.m.Take(nil, Browser, "a", false, 7, long); err != nil {
		t.Fatal(err)
	}
	r.m.Started = func(int) (int64, bool) { return 1500, true } // 7 ended, and its pid went to a later process
	r.now = 1000 + 900
	if _, err := r.m.Take(nil, Browser, "b", false, 8, long); err != nil {
		t.Fatal(err)
	}
	r.at(1000 + 920)
	if len(r.signals) != 0 {
		t.Fatalf("signalled a stranger: %q", r.signals)
	}
}

func TestOneLeaseIsTakenBackPerAskWaiting(t *testing.T) {
	r := newHoldRig()
	r.m.Max = func(string) int { return 2 }
	for _, a := range []string{"a", "c"} {
		if _, err := r.m.Take(nil, Browser, a, true, 0, long); err != nil {
			t.Fatal(err)
		}
	}
	r.m.Max = func(string) int { return 1 } // lowered: one revoke frees no slot
	blockedOn(t, second(r.m.Take(nil, Browser, "b", false, 8, long)))
	r.at(1000 + 600)
	if h := pool(r.m, Browser).Held; len(h) != 1 || h[0].Reserved() { // one of a and c is left, b still waits
		t.Fatalf("held %+v", h)
	}
}
