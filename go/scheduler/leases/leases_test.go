package leases

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/scheduler/procload"
)

const long = time.Minute

func manager(limit int) *Manager {
	return &Manager{Max: func(string) int { return limit }, Now: func() int64 { return 1000 }}
}

type asked struct {
	l   *Lease
	err error
}

// ask starts Take beside the test and waits until it is queued.
func ask(t *testing.T, m *Manager, done <-chan struct{}, agent string, ceiling time.Duration) <-chan asked {
	t.Helper()
	before := queued(m, Browser)
	out := make(chan asked, 1)
	go func() {
		l, err := m.Take(done, Browser, agent, false, 7, ceiling)
		out <- asked{l, err}
	}()
	for i := 0; queued(m, Browser) == before; i++ {
		if i > 2000 {
			t.Fatalf("%s never queued", agent)
		}
		time.Sleep(time.Millisecond)
	}
	return out
}

func queued(m *Manager, kind string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queues[kind])
}

func got(t *testing.T, c <-chan asked) asked {
	t.Helper()
	select {
	case a := <-c:
		return a
	case <-time.After(5 * time.Second):
		t.Fatal("no answer")
	}
	return asked{}
}

func none(t *testing.T, c <-chan asked) {
	t.Helper()
	select {
	case a := <-c:
		t.Fatalf("answered too soon: %+v", a)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestNoLimitGrantsEveryAsk(t *testing.T) {
	m := manager(0)
	for _, a := range []string{"a", "b", "c"} {
		if l, err := m.Take(nil, Browser, a, false, 7, long); err != nil || l == nil {
			t.Fatalf("%s: %v %v", a, l, err)
		}
	}
	if v := m.View()[1]; len(v.Held) != 3 || len(v.Queue) != 0 || v.Max != 0 {
		t.Fatalf("view: %+v", v)
	}
}

func TestQueueIsFirstComeFirstServed(t *testing.T) {
	m := manager(1)
	first, err := m.Take(nil, Browser, "a", false, 7, long)
	if err != nil {
		t.Fatal(err)
	}
	b := ask(t, m, nil, "b", long)
	c := ask(t, m, nil, "c", long)
	if v := m.View()[1]; len(v.Queue) != 2 || v.Queue[0].Says != "waiting for a browser, 1st in line, held by a" ||
		v.Queue[1].Says != "waiting for a browser, 2nd in line, held by a" {
		t.Fatalf("queue: %+v", v.Queue)
	}
	none(t, b)
	if !m.Release(first.ID, "a") {
		t.Fatal("a's release")
	}
	gb := got(t, b)
	if gb.l == nil || gb.l.Agent != "b" {
		t.Fatalf("b: %+v", gb)
	}
	none(t, c)
	m.Release(gb.l.ID, "b")
	if gc := got(t, c); gc.l == nil || gc.l.Agent != "c" {
		t.Fatalf("c: %+v", gc)
	}
	// A new ask does not pass one already waiting: with c holding the lease, d waits behind nobody but still waits.
	d := ask(t, m, nil, "d", long)
	none(t, d)
}

func TestCeilingRefusesNamingTheHolders(t *testing.T) {
	m := manager(1)
	if _, err := m.Take(nil, Browser, "a", false, 7, long); err != nil {
		t.Fatal(err)
	}
	a := got(t, ask(t, m, nil, "b", 30*time.Millisecond))
	r, ok := a.err.(*Refusal)
	if !ok || a.l != nil || !strings.Contains(r.Why, "no browser came free") || !strings.Contains(r.Why, "held by a since") {
		t.Fatalf("got %+v", a)
	}
	if queued(m, Browser) != 0 {
		t.Fatal("the refused ask is still queued")
	}
}

func TestHangingUpLeavesTheQueue(t *testing.T) {
	m := manager(1)
	first, _ := m.Take(nil, Browser, "a", false, 7, long)
	done := make(chan struct{})
	b := ask(t, m, done, "b", long)
	close(done)
	if a := got(t, b); a.l != nil || a.err != nil {
		t.Fatalf("got %+v", a)
	}
	if queued(m, Browser) != 0 {
		t.Fatal("still queued")
	}
	m.Release(first.ID, "a")
	if v := m.View()[1]; len(v.Held) != 0 {
		t.Fatalf("a lease went to nobody: %+v", v.Held)
	}
}

func TestReleaseIsTheHoldersOnly(t *testing.T) {
	m := manager(0)
	l, _ := m.Take(nil, Browser, "a", false, 7, long)
	if m.Release(l.ID, "b") {
		t.Fatal("b released a's lease")
	}
	if !m.Release(l.ID, "a") || m.Release(l.ID, "a") {
		t.Fatal("a's release")
	}
}

func TestTurnEndFreesHoldsAndSessionEndFreesAll(t *testing.T) {
	m := manager(0)
	m.Take(nil, Simulator, "a", true, 0, long)
	m.Take(nil, Browser, "a", false, 7, long)
	m.Take(nil, Browser, "b", true, 0, long)
	if n := m.ReleaseAgent("a", "", true, "the turn ended"); n != 1 {
		t.Fatalf("turn end freed %d", n)
	}
	if v := m.View(); len(v[0].Held) != 0 || len(v[1].Held) != 2 {
		t.Fatalf("after the turn: %+v", v)
	}
	if n := m.ReleaseAgent("a", "", false, "the session ended"); n != 1 {
		t.Fatalf("session end freed %d", n)
	}
	if v := m.View()[1]; len(v.Held) != 1 || v.Held[0].Agent != "b" {
		t.Fatalf("after the session: %+v", v.Held)
	}
}

func TestCheckFreesACommandsLeaseOnceItsProcessesAreGone(t *testing.T) {
	running := map[int]bool{7: true, 8: true}
	m := manager(1)
	m.Alive = func(pid int) bool { return running[pid] }
	l, _ := m.Take(nil, Browser, "a", false, 7, long)
	b := ask(t, m, nil, "b", long)
	procs := []procload.Proc{{PID: 7, PPID: 1, Agent: "a"}, {PID: 8, PPID: 7, Cmd: "chrome --headless"}}
	m.Check(procs)
	if v := m.View()[1]; len(v.Held) != 1 || !reflect.DeepEqual(v.Held[0].Pids, []int{7, 8}) {
		t.Fatalf("held: %+v", v.Held)
	}
	// hl use was killed; the browser it started still runs, and holds the lease.
	delete(running, 7)
	m.Check(nil)
	none(t, b)
	delete(running, 8)
	m.Check(nil)
	if a := got(t, b); a.l == nil || a.l.Agent != "b" || a.l.ID == l.ID {
		t.Fatalf("b: %+v", a)
	}
}

func TestAHoldOutlivesItsProcesses(t *testing.T) {
	m := manager(0)
	m.Alive = func(int) bool { return false }
	m.Take(nil, Browser, "a", true, 0, long)
	m.Check([]procload.Proc{{PID: 9, Agent: "a", Cmd: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}, {PID: 10, Agent: "b", Cmd: "chrome"}})
	if v := m.View()[1]; len(v.Held) != 1 || !reflect.DeepEqual(v.Held[0].Pids, []int{9}) {
		t.Fatalf("held: %+v", v.Held)
	}
	m.Check(nil) // between two commands
	if !m.Holds("a", Browser) || m.Holds("a", Simulator) || m.Holds("b", Browser) {
		t.Fatal("a hold ended with its processes")
	}
}

func TestARaisedLimitLetsTheQueueIn(t *testing.T) {
	limit := 1
	m := &Manager{Max: func(string) int { return limit }, Now: func() int64 { return 1000 }}
	m.Take(nil, Browser, "a", true, 0, long)
	b := ask(t, m, nil, "b", long)
	limit = 0
	m.Check(nil)
	if a := got(t, b); a.l == nil {
		t.Fatalf("b: %+v", a)
	}
}

func TestMaxReadsTheConfig(t *testing.T) {
	cfg := usage.Config{"browser_max": 2, "simulator_max": -1}
	if Max(cfg, Browser) != 2 || Max(cfg, Simulator) != 0 || Max(usage.Config{}, Browser) != 0 {
		t.Fatal(Max(cfg, Browser), Max(cfg, Simulator))
	}
}
