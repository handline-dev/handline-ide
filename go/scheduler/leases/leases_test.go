package leases

import (
	"reflect"
	"sort"
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

// ask starts Take of a browser beside the test and waits until it is queued.
func ask(t *testing.T, m *Manager, done <-chan struct{}, agent string, ceiling time.Duration) <-chan asked {
	t.Helper()
	return askFor(t, m, done, Browser, agent, false, ceiling)
}

// askKind is ask for a hold of kind.
func askKind(t *testing.T, m *Manager, kind, agent string) <-chan asked {
	t.Helper()
	return askFor(t, m, nil, kind, agent, true, long)
}

func askFor(t *testing.T, m *Manager, done <-chan struct{}, kind, agent string, hold bool, ceiling time.Duration) <-chan asked {
	t.Helper()
	before := queued(m, kind)
	out := make(chan asked, 1)
	go func() {
		l, err := m.Take(done, kind, agent, hold, 7, ceiling)
		out <- asked{l, err}
	}()
	for i := 0; queued(m, kind) == before; i++ {
		if i > 2000 {
			t.Fatalf("%s never queued", agent)
		}
		time.Sleep(time.Millisecond)
	}
	return out
}

// pool is kind's pool on the read.
func pool(m *Manager, kind string) Pool {
	for _, p := range m.View() {
		if p.Kind == kind {
			return p
		}
	}
	return Pool{}
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
	if v := pool(m, Browser); len(v.Held) != 3 || len(v.Queue) != 0 || v.Max != 0 {
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
	if v := pool(m, Browser); len(v.Queue) != 2 || v.Queue[0].Says != "waiting for a browser, 1st in line, held by a" ||
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
	if v := pool(m, Browser); len(v.Held) != 0 {
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
	if len(pool(m, Simulator).Held) != 0 || len(pool(m, Browser).Held) != 2 {
		t.Fatalf("after the turn: %+v", m.View())
	}
	if n := m.ReleaseAgent("a", "", false, "the session ended"); n != 1 {
		t.Fatalf("session end freed %d", n)
	}
	if v := pool(m, Browser); len(v.Held) != 1 || v.Held[0].Agent != "b" {
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
	if v := pool(m, Browser); len(v.Held) != 1 || !reflect.DeepEqual(v.Held[0].Pids, []int{7, 8}) {
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
	if v := pool(m, Browser); len(v.Held) != 1 || !reflect.DeepEqual(v.Held[0].Pids, []int{9}) {
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

func TestAFreedLeaseGoesToTheWorkspaceThatHeldTheKindLeast(t *testing.T) {
	now := int64(1000)
	m := fairManager(1, &now)
	first, _ := m.Take(nil, Browser, "a1", false, 7, long)
	now += 600 // w1 held a browser for ten minutes
	m.Release(first.ID, "a1")
	held, _ := m.Take(nil, Browser, "c1", false, 7, long)
	a2 := ask(t, m, nil, "a2", long)
	b1 := ask(t, m, nil, "b1", long)
	if q := pool(m, Browser).Queue; len(q) != 2 || q[0].Agent != "b1" || q[0].Workspace != "w2" || q[1].Agent != "a2" {
		t.Fatalf("queue: %+v", q)
	}
	now += 60
	m.Release(held.ID, "c1")
	gb := got(t, b1)
	if gb.l == nil || gb.l.Agent != "b1" || gb.l.Workspace != "w2" {
		t.Fatalf("b1: %+v", gb)
	}
	none(t, a2)
	m.Release(gb.l.ID, "b1")
	if ga := got(t, a2); ga.l == nil || ga.l.Agent != "a2" {
		t.Fatalf("a2: %+v", ga)
	}
}

func TestHoldSecondsOlderThanAnHourDoNotCount(t *testing.T) {
	now := int64(1000)
	m := fairManager(1, &now)
	hold := func(kind, agent string, s int64) {
		l, _ := m.Take(nil, kind, agent, false, 7, long)
		now += s
		m.Release(l.ID, agent)
	}
	hold(Browser, "b1", 600)
	now += fairS // w2's ten minutes are now past the hour
	hold(Browser, "b1", 60)
	hold(Browser, "a1", 30)
	hold(Simulator, "a1", 600) // another kind does not count toward browsers
	held, _ := m.Take(nil, Browser, "c1", false, 7, long)
	b1 := ask(t, m, nil, "b1", long)
	a2 := ask(t, m, nil, "a2", long)
	m.Release(held.ID, "c1")
	if ga := got(t, a2); ga.l == nil || ga.l.Agent != "a2" {
		t.Fatalf("a2: %+v", ga)
	}
	none(t, b1)
}

// fairManager has a limit for every kind and the clock at *now.
func fairManager(limit int, now *int64) *Manager {
	ws := map[string]string{"a1": "w1", "a2": "w1", "b1": "w2", "c1": "w3"}
	return &Manager{Max: func(string) int { return limit }, Now: func() int64 { return *now }, WorkspaceOf: func(a string) string { return ws[a] }}
}

type memStore struct {
	rows    map[int64]Lease
	asks    map[int64]Ask
	last    int64
	removed []int64
}

func (s *memStore) Load() ([]Lease, []Ask, int64, error) {
	var out []Lease
	for _, l := range s.rows {
		out = append(out, l)
	}
	var asks []Ask
	for _, a := range s.asks {
		asks = append(asks, a)
	}
	sort.Slice(asks, func(i, j int) bool { return asks[i].ID < asks[j].ID })
	return out, asks, s.last, nil
}

func (s *memStore) Put(l Lease) error {
	s.rows[l.ID], s.last = l, max(s.last, l.ID)
	return nil
}

func (s *memStore) AddAsk(a Ask) error {
	if s.asks == nil {
		s.asks = map[int64]Ask{}
	}
	s.asks[a.ID], s.last = a, max(s.last, a.ID)
	return nil
}

func (s *memStore) Remove(id int64) error {
	delete(s.rows, id)
	delete(s.asks, id)
	s.removed = append(s.removed, id)
	return nil
}

func TestARestartKeepsHoldsAndLiveCommandsAndDropsTheRest(t *testing.T) {
	st := &memStore{last: 9, rows: map[int64]Lease{
		3: {ID: 3, Kind: Simulator, Agent: "a", Workspace: "w1", Since: 900, Hold: true},
		4: {ID: 4, Kind: Browser, Agent: "a", Workspace: "w1", Since: 950, PID: 7},
		5: {ID: 5, Kind: Browser, Agent: "b", Workspace: "w2", Since: 960, PID: 8},
		6: {ID: 6, Kind: Browser, Agent: "b", Workspace: "w2", Since: 970, PID: 9},
	}}
	m := manager(1)
	m.Store = st
	// pid 7 is lease 4's hl use; pid 8 is a later process that got lease 5's dead one's pid, and 9 is gone.
	m.Started = func(pid int) (int64, bool) { return map[int]int64{7: 940, 8: 990}[pid], pid != 9 }
	if err := m.Adopt(); err != nil {
		t.Fatal(err)
	}
	sim, br := pool(m, Simulator), pool(m, Browser)
	if len(sim.Held) != 1 || sim.Held[0].ID != 3 || !sim.Held[0].Hold || len(br.Held) != 1 || br.Held[0].ID != 4 || br.Held[0].Workspace != "w1" {
		t.Fatalf("adopted: %+v", m.View())
	}
	if sort.Slice(st.removed, func(i, j int) bool { return st.removed[i] < st.removed[j] }); !reflect.DeepEqual(st.removed, []int64{5, 6}) {
		t.Fatalf("removed: %v", st.removed)
	}
	// The adopted browser is the one under the limit: another ask waits, and gets a new id once hl use gives it back by its old one.
	b := ask(t, m, nil, "b", long)
	if !m.Release(4, "a") {
		t.Fatal("the adopted lease could not be given back by its id")
	}
	gb := got(t, b)
	if gb.l == nil || gb.l.ID != 10 {
		t.Fatalf("b: %+v", gb)
	}
	if _, ok := st.rows[gb.l.ID]; !ok || len(st.rows) != 2 {
		t.Fatalf("stored: %+v", st.rows)
	}
	m.ReleaseAgent("a", "", true, "the turn ended")
	if _, ok := st.rows[3]; ok {
		t.Fatal("the ended hold is still stored")
	}
}

func TestALeaseStillHeldCountsTowardItsWorkspace(t *testing.T) {
	now := int64(1000)
	m := fairManager(2, &now)
	m.Take(nil, Browser, "a1", false, 7, long)
	held, _ := m.Take(nil, Browser, "c1", false, 7, long)
	now += 600
	a2 := ask(t, m, nil, "a2", long)
	b1 := ask(t, m, nil, "b1", long)
	m.Release(held.ID, "c1")
	if gb := got(t, b1); gb.l == nil || gb.l.Agent != "b1" {
		t.Fatalf("b1: %+v", gb)
	}
	none(t, a2)
}

func TestTheLineForgetsALeaseThatEndedOverAnHourAgo(t *testing.T) {
	now := int64(1000)
	m := fairManager(1, &now)
	old, _ := m.Take(nil, Browser, "b1", false, 7, long)
	now += 600
	m.Release(old.ID, "b1")
	m.Take(nil, Browser, "c1", false, 7, long)
	ask(t, m, nil, "a2", long)
	ask(t, m, nil, "b1", long)
	now += fairS + 600 // no lease ends meanwhile, so nothing prunes w2's old ten minutes
	if q := pool(m, Browser).Queue; len(q) != 2 || q[0].Agent != "a2" || q[1].Agent != "b1" {
		t.Fatalf("queue: %+v", q)
	}
}
