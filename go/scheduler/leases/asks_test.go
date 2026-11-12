package leases

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// board is a task board: each agent's current task, the open tasks by agent, and what the manager asked of it.
type board struct {
	current map[string]int64
	open    map[int64]string // task: its agent
	gates   int64
	waits   []string // "T50 browser gate 1001"
	granted []int64
	dropped []int64
	fail    error
	kept    map[int64]bool // the gates CloseStray last kept
	during  func()         // runs inside Wait, once
}

func newBoard() *board {
	return &board{current: map[string]int64{}, open: map[int64]string{}, gates: 1000}
}

func (b *board) Current(agent string) int64         { return b.current[agent] }
func (b *board) Open(agent string, task int64) bool { return b.open[task] == agent }
func (b *board) Granted(gate int64, kind string)    { b.granted = append(b.granted, gate) }
func (b *board) CloseStray(keep map[int64]bool)     { b.kept = keep }
func (b *board) Dropped(gate int64, why string)     { b.dropped = append(b.dropped, gate) }
func (b *board) Wait(agent string, task int64, kind string) (int64, error) {
	if b.fail != nil {
		return 0, b.fail
	}
	if f := b.during; f != nil {
		b.during = nil
		f()
	}
	b.gates++
	b.waits = append(b.waits, "T"+itoa(task)+" "+kind+" gate "+itoa(b.gates))
	return b.gates, nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// boardManager has one browser, the clock at *now, every process running, and agents a (T10), b (T20) and c (T30), each with that task open.
func boardManager(now *int64) (*Manager, *board) {
	b := newBoard()
	for agent, task := range map[string]int64{"a": 10, "b": 20, "c": 30} {
		b.current[agent], b.open[task] = task, agent
	}
	return &Manager{Max: func(string) int { return 1 }, Now: func() int64 { return *now }, Alive: func(int) bool { return true }, Tasks: b}, b
}

func blockedOn(t *testing.T, err error) *Blocked {
	t.Helper()
	var bl *Blocked
	if !errors.As(err, &bl) {
		t.Fatalf("not blocked: %v", err)
	}
	return bl
}

func TestAnAgentsAskWaitsAsAGateOnItsTaskAndComesBackReserved(t *testing.T) {
	now := int64(1000)
	m, b := boardManager(&now)
	first, err := m.Take(nil, Browser, "a", false, 7, long)
	if err != nil || first == nil {
		t.Fatal(first, err)
	}
	l, err := m.Take(nil, Browser, "b", false, 8, long)
	bl := blockedOn(t, err)
	if l != nil || bl.Task != 20 || bl.Gate != 1001 || !strings.Contains(bl.Why, "T20 now waits for one (gate T1001)") ||
		!reflect.DeepEqual(b.waits, []string{"T20 browser gate 1001"}) {
		t.Fatalf("%+v %v", bl, b.waits)
	}
	// Asking again while in line opens no second gate.
	if _, err := m.Take(nil, Browser, "b", false, 9, long); blockedOn(t, err).Gate != 1001 || len(b.waits) != 1 {
		t.Fatalf("asked twice: %v", b.waits)
	}
	if q := pool(m, Browser).Queue; len(q) != 1 || q[0].Task != 20 {
		t.Fatalf("queue: %+v", q)
	}
	m.Release(first.ID, "a")
	held := pool(m, Browser).Held
	if len(held) != 1 || !held[0].Reserved() || held[0].Task != 20 || held[0].Until != 1000+ClaimS || !reflect.DeepEqual(b.granted, []int64{1001}) {
		t.Fatalf("reserved: %+v granted %v", held, b.granted)
	}
	// The reservation counts toward the limit: c's task waits behind it.
	if _, err := m.Take(nil, Browser, "c", false, 5, long); blockedOn(t, err).Task != 30 {
		t.Fatal("c went past a reservation")
	}
	// b runs hl use again on T20 and takes it at once.
	got, err := m.Take(nil, Browser, "b", false, 11, long)
	if err != nil || got == nil || got.ID != held[0].ID || got.Reserved() || got.PID != 11 {
		t.Fatalf("taken: %+v %v", got, err)
	}
}

func TestAReservationNotTakenGoesToTheNextAsk(t *testing.T) {
	now := int64(1000)
	m, b := boardManager(&now)
	first, _ := m.Take(nil, Browser, "a", false, 7, long)
	m.Take(nil, Browser, "b", false, 8, long)
	m.Take(nil, Browser, "c", false, 9, long)
	m.Release(first.ID, "a") // reserved for T20
	now += ClaimS - 1
	m.Check(nil)
	if h := pool(m, Browser).Held; len(h) != 1 || h[0].Task != 20 {
		t.Fatalf("lapsed early: %+v", h)
	}
	now++
	m.Check(nil)
	h, q := pool(m, Browser).Held, pool(m, Browser).Queue
	if len(h) != 1 || h[0].Task != 30 || len(q) != 0 {
		t.Fatalf("after the lapse: held %+v queue %+v", h, q)
	}
	// T20 is not taken from its agent: it asks again at its next hl use.
	if len(b.waits) != 2 || !reflect.DeepEqual(b.granted, []int64{1001, 1002}) {
		t.Fatalf("waits %v granted %v", b.waits, b.granted)
	}
	if _, err := m.Take(nil, Browser, "b", false, 8, long); blockedOn(t, err).Gate != 1003 {
		t.Fatalf("T20 asks again: %v", b.waits)
	}
}

func TestAReservationNobodyElseWantsLapsesToFree(t *testing.T) {
	now := int64(1000)
	m, b := boardManager(&now)
	first, _ := m.Take(nil, Browser, "a", false, 7, long)
	m.Take(nil, Browser, "b", false, 8, long)
	m.Release(first.ID, "a")
	now += ClaimS
	m.Check(nil)
	if p := pool(m, Browser); len(p.Held) != 0 || len(p.Queue) != 0 || len(b.waits) != 1 {
		t.Fatalf("held %+v queue %+v waits %v", p.Held, p.Queue, b.waits)
	}
}

func TestTwoAsksOfOneTaskAtOnceOpenOneGate(t *testing.T) {
	now := int64(1000)
	m, b := boardManager(&now)
	m.Take(nil, Browser, "a", false, 7, long)
	var second error
	b.during = func() { _, second = m.Take(nil, Browser, "b", false, 9, long) } // b's other shell asks while the first opens the gate
	_, err := m.Take(nil, Browser, "b", false, 8, long)
	if blockedOn(t, err).Gate != 1001 || blockedOn(t, second).Gate != 0 || len(b.waits) != 1 || len(pool(m, Browser).Queue) != 1 {
		t.Fatalf("waits %v queue %+v", b.waits, pool(m, Browser).Queue)
	}
}

func TestCheckKeepsTheGatesOfTheAsksInLine(t *testing.T) {
	now := int64(1000)
	m, b := boardManager(&now)
	m.Take(nil, Browser, "a", false, 7, long)
	m.Take(nil, Browser, "b", false, 8, long)
	m.Check(nil)
	if !reflect.DeepEqual(b.kept, map[int64]bool{1001: true}) {
		t.Fatalf("kept %v", b.kept)
	}
}

func TestAnEndedTaskDropsItsAskAndItsReservation(t *testing.T) {
	now := int64(1000)
	m, b := boardManager(&now)
	first, _ := m.Take(nil, Browser, "a", false, 7, long)
	m.Take(nil, Browser, "b", false, 8, long)
	m.Take(nil, Browser, "c", false, 9, long)
	delete(b.open, 30) // T30 is done
	m.Check(nil)
	if q := pool(m, Browser).Queue; len(q) != 1 || q[0].Task != 20 || !reflect.DeepEqual(b.dropped, []int64{1002}) {
		t.Fatalf("queue %+v dropped %v", q, b.dropped)
	}
	m.Release(first.ID, "a") // reserved for T20
	b.open[20] = "c"         // handed to c
	m.Check(nil)
	if h := pool(m, Browser).Held; len(h) != 0 {
		t.Fatalf("a moved task kept its reservation: %+v", h)
	}
}

func TestATurnOrSessionEndLeavesAReservation(t *testing.T) {
	now := int64(1000)
	m, _ := boardManager(&now)
	first, _ := m.Take(nil, Browser, "a", false, 7, long)
	m.Take(nil, Browser, "b", false, 8, long)
	m.Release(first.ID, "a")
	if n := m.ReleaseAgent("b", "", false, "the session ended"); n != 0 || len(pool(m, Browser).Held) != 1 {
		t.Fatalf("the session end took %d", n)
	}
}

func TestAnAskThatCannotWaitIsAnError(t *testing.T) {
	now := int64(1000)
	m, b := boardManager(&now)
	m.Take(nil, Browser, "a", false, 7, long)
	b.fail = errors.New("no such task")
	if _, err := m.Take(nil, Browser, "b", false, 8, long); err == nil || errors.As(err, new(*Blocked)) {
		t.Fatalf("got %v", err)
	}
	if q := pool(m, Browser).Queue; len(q) != 0 {
		t.Fatalf("queued: %+v", q)
	}
}

func TestARestartKeepsTheLineAndTheReservations(t *testing.T) {
	now := int64(1000)
	st := &memStore{rows: map[int64]Lease{}}
	m, b := boardManager(&now)
	m.Store = st
	first, _ := m.Take(nil, Browser, "a", false, 7, long)
	m.Take(nil, Browser, "b", false, 8, long)
	m.Take(nil, Browser, "c", false, 9, long)
	m.Release(first.ID, "a") // reserved for T20; T30 in line
	again := &Manager{Max: func(string) int { return 1 }, Now: func() int64 { return now }, Alive: m.Alive, Tasks: b, Store: st}
	if err := again.Adopt(); err != nil {
		t.Fatal(err)
	}
	h, q := pool(again, Browser).Held, pool(again, Browser).Queue
	if len(h) != 1 || h[0].Task != 20 || !h[0].Reserved() || len(q) != 1 || q[0].Task != 30 {
		t.Fatalf("after the restart: held %+v queue %+v", h, q)
	}
	got, err := again.Take(nil, Browser, "b", false, 12, long)
	if err != nil || got == nil || got.ID != h[0].ID {
		t.Fatalf("b's reservation after the restart: %+v %v", got, err)
	}
}
