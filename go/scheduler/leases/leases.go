// Package leases is how many test or build runs, simulators and browsers run at once: an agent asks for a lease in its turn (hl use),
// waits while every lease of the kind is held, and gives it back when its command exits, when it releases a held one, when its turn or
// session ends, or when the processes behind it are gone. A freed lease goes to the workspace that held the kind least in the last
// hour, first come first served inside it. With the kind's limit 0 (the default) every ask is granted at once: the leases are still
// listed, and nothing waits. The leases are stored, so a restart keeps what is held (docs/design/people-workspaces.md, Resource
// leases).
package leases

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/kit/peer"
	"github.com/handline/handline/go/scheduler/procload"
)

// The kinds of lease.
const (
	Heavy     = "heavy"     // a test or build run
	Simulator = "simulator" // iOS Simulator, Android emulator
	Browser   = "browser"   // Chrome or Chromium for shots, Playwright, CDP
	Port      = "port"      // a TCP port for a server the command starts, given to it as $HL_PORT
)

// Kinds are every kind, in the order the read lists them.
var Kinds = []string{Heavy, Simulator, Browser, Port}

// The config.yaml keys of each kind's limit: the most leases held at once, 0 for no limit.
const (
	HeavyMaxKey     = "heavy_max"
	SimulatorMaxKey = "simulator_max"
	BrowserMaxKey   = "browser_max"
)

// CeilingS is the longest an ask waits before it is refused.
const CeilingS = 30 * 60

// fairS is how far back a workspace's hold-seconds count toward who is granted next.
const fairS = 60 * 60

// ClaimS is how long a lease reserved for a task waits for its agent's hl use before it goes to the next ask (lease_claim).
const ClaimS = 10 * 60

// Noun is what a lease of kind is called in a sentence.
func Noun(kind string) string {
	if kind == Heavy {
		return "test or build run"
	}
	return kind
}

// MaxKey is kind's limit key.
func MaxKey(kind string) string { return kind + "_max" }

// Known is whether kind is a kind of lease.
func Known(kind string) bool {
	return kind == Heavy || kind == Simulator || kind == Browser || kind == Port
}

// Max is kind's limit in cfg: 0 (no limit) when it is absent or not a whole number above 0.
func Max(cfg usage.Config, kind string) int {
	switch n := cfg[MaxKey(kind)].(type) {
	case int:
		return max(n, 0)
	case int64:
		return int(max(n, 0))
	}
	return 0
}

// procOf is what a hold's processes look like, shown on the read; a hold is not freed by them.
var procOf = map[string]*regexp.Regexp{
	Heavy:     regexp.MustCompile(`(?i)go test|\.test( |$)|cargo|xcodebuild|swift-(build|test)|vitest|jest|pytest|gradle|mvn|bazel`),
	Simulator: regexp.MustCompile(`(?i)simulator|launchd_sim|qemu-system|/emulator`),
	Browser:   regexp.MustCompile(`(?i)chrom|playwright|headless_shell`),
}

// Lease is one held lease.
type Lease struct {
	ID        int64
	Kind      string
	Agent     string
	Workspace string // the agent's workspace key when it was granted; "" when it has none
	Since     int64  // epoch seconds it was granted
	Hold      bool   // taken with --hold: no command stands behind it
	PID       int    // the hl use process the command runs under; 0 for a hold or a reservation
	Pids      []int  // its processes at the last sample
	Port      int    // a port lease's number
	Task      int64  // a reservation's task
	Until     int64  // a reservation's claim deadline, epoch seconds; 0 once taken
	Extended  int    // the periods hl use --extend added past its max_hold
	Warned    bool   // told it nears its max_hold, this period

	sampled int64 // when Pids were last sampled
}

// Reserved is whether the lease waits for its task's agent to take it.
func (l *Lease) Reserved() bool { return l.Until != 0 }

// Ask is a task's ask waiting in line: its task is blocked by gate until a lease of kind is reserved for it.
type Ask struct {
	ID                     int64
	Kind, Agent, Workspace string
	Task, Gate             int64
	Since                  int64
}

// Store keeps the leases and the asks across a restart. A hold is stored as PID 0 with no Until.
type Store interface {
	// Load is every stored lease and ask and the highest id ever given, so an id is not given twice.
	Load() ([]Lease, []Ask, int64, error)
	Put(Lease) error
	AddAsk(Ask) error
	Remove(id int64) error
}

// Tasks is the task board the asks of agents with a current task wait on.
type Tasks interface {
	// Current is agent's current task, 0 for none.
	Current(agent string) int64
	// Open is whether task is open and agent's.
	Open(agent string, task int64) bool
	// Wait blocks task on a new gate for a lease of kind, releasing it when doing, and answers the gate.
	Wait(agent string, task int64, kind string) (gate int64, err error)
	// Granted marks gate done: a lease of kind is reserved for its task, which comes back to its agent.
	Granted(gate int64, kind string)
	// Dropped closes gate: its ask ended (why).
	Dropped(gate int64, why string)
	// CloseStray closes the lease gates over a minute old that are not in keep: their ask or their close was lost.
	CloseStray(keep map[int64]bool)
}

// waiter is an ask in line: a task's (task and gate set, got nil) or a call held open (got set).
type waiter struct {
	id              int64
	kind, agent, ws string
	since           int64
	hold            bool
	pid             int
	task, gate      int64
	got             chan *Lease // granted, buffered
}

// span is an ended lease's time, kept for an hour for the fair grant.
type span struct {
	kind, ws    string
	from, until int64
}

// Manager holds the leases and the queues.
type Manager struct {
	// Max is kind's limit now, 0 for none.
	Max func(kind string) int
	// Alive is whether a process runs; nil asks the system.
	Alive func(pid int) bool
	// Started is when a running process began, in epoch seconds; nil asks the system.
	Started func(pid int) (int64, bool)
	Now     func() int64
	Log     func(format string, args ...any)
	// Store keeps the leases; nil keeps them in memory only. Adopt reads it back at start.
	Store Store
	// WorkspaceOf is agent's workspace key, "" for none; nil puts every agent in one.
	WorkspaceOf func(agent string) string
	// WorkspaceMax is the kept rule lease_max.<kind> of every workspace, read once per pass; nil is no rule.
	WorkspaceMax func(kind string) WorkspaceLimits
	// Tasks blocks an agent's current task while its ask waits; nil holds every ask's call open instead.
	Tasks Tasks
	// Ports is the range port leases are picked from (PortRange); nil or not ok, any free port the system gives.
	Ports func() (lo, hi int, ok bool)
	// PortFree is whether a port binds now; nil tries it.
	PortFree func(port int) bool
	// MaxHold is kind's max_hold in seconds, 0 for none; nil is none for every kind.
	MaxHold func(kind string) int64
	// Tell says a line to an agent: its lease nears its max_hold, or was taken back. Nil tells nobody.
	Tell func(agent, text string)
	// Signal sends a signal to a process; nil is the system's kill.
	Signal func(pid int, sig syscall.Signal) error

	mu     sync.Mutex
	next   int64
	held   []*Lease
	ended  []span
	queues map[string][]*waiter
	asking map[int64]bool // the tasks whose gate is being opened
	after  []func()       // the task board's writes and the tellings, run once mu is let go
	kills  []kill         // revoked commands to kill once their time comes
}

// unlock lets mu go and runs what was left for after it.
func (m *Manager) unlock() {
	fs := m.after
	m.after = nil
	m.mu.Unlock()
	for _, f := range fs {
		f()
	}
}

func (m *Manager) workspaceOf(agent string) string {
	if m.WorkspaceOf == nil {
		return ""
	}
	return m.WorkspaceOf(agent)
}

// Adopt takes back the stored leases and asks after a restart: a hold stays until its holder's turn ends, a reservation until it is
// taken or lapses, an ask in line, and a command's lease while the hl use process it was granted to runs; that lease is dropped when
// the process is gone or its pid now names a later one.
func (m *Manager) Adopt() error {
	if m.Store == nil {
		return nil
	}
	stored, asks, last, err := m.Store.Load()
	if err != nil {
		return fmt.Errorf("loading the stored leases: %w", err)
	}
	m.mu.Lock()
	defer m.unlock()
	m.next = max(m.next, last)
	for _, a := range asks {
		m.next = max(m.next, a.ID)
		if m.Tasks == nil { // no task board to close its gate
			m.forget(a.ID)
			continue
		}
		m.enqueue(&waiter{id: a.ID, kind: a.Kind, agent: a.Agent, ws: a.Workspace, since: a.Since, task: a.Task, gate: a.Gate})
	}
	for _, l := range stored {
		m.next = max(m.next, l.ID)
		if !l.Hold && !l.Reserved() && !m.ranSince(l.PID, l.Since) {
			m.forget(l.ID)
			m.logf("lease: %s's %s (lease %d) freed: its hl use process ended while the daemon was down", l.Agent, l.Kind, l.ID)
			continue
		}
		c := l
		m.held = append(m.held, &c)
		m.logf("lease: %s's %s (lease %d) kept across the restart", l.Agent, l.Kind, l.ID)
	}
	return nil
}

// forget removes lease id from the store, logging a failure: the leases in memory stay the truth.
func (m *Manager) forget(id int64) {
	if m.Store == nil {
		return
	}
	if err := m.Store.Remove(id); err != nil {
		m.logf("lease: removing lease %d from the store: %v", id, err)
	}
}

func (m *Manager) now() int64 {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now().Unix()
}

func (m *Manager) alive(pid int) bool {
	if m.Alive != nil {
		return m.Alive(pid)
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// ranSince is whether pid runs and began at or before at: the same process, not a later one given its pid.
func (m *Manager) ranSince(pid int, at int64) bool {
	if m.Started != nil {
		began, ok := m.Started(pid)
		return ok && began <= at
	}
	began, ok := peer.Started(pid)
	return ok && began.Unix() <= at
}

func (m *Manager) logf(format string, args ...any) {
	if m.Log != nil {
		m.Log(format, args...)
	}
}

// limits is each kind's limit, read once.
func (m *Manager) limits() map[string]int {
	out := map[string]int{}
	for _, kind := range Kinds {
		if m.Max != nil && kind != Port {
			out[kind] = m.Max(kind)
		}
	}
	if lo, hi, ok := m.portRange(); ok {
		out[Port] = hi - lo + 1
	}
	return out
}

// free is whether a lease of kind can be granted now, and for a port the port to grant.
func (m *Manager) free(kind string, limits map[string]int) (port int, ok bool) {
	if kind == Port { // a port another process took is not free either
		port = m.pickPort()
		return port, port != 0
	}
	if limits[kind] <= 0 {
		return 0, true
	}
	n := 0
	for _, l := range m.held {
		if l.Kind == kind {
			n++
		}
	}
	return 0, n < limits[kind]
}

// Limit is a kept rule lease_max.<kind>: the most leases of the kind a workspace holds at once (0 for none), and who set it.
type Limit struct {
	N     int
	SetBy string
}

// WorkspaceLimits is lease_max.<kind> by workspace key, the machine's for a workspace with no row of its own.
type WorkspaceLimits struct {
	Machine Limit
	Own     map[string]Limit
}

// of is ws's limit; none for an agent with no workspace.
func (w WorkspaceLimits) of(ws string) Limit {
	if ws == "" {
		return Limit{}
	}
	if l, ok := w.Own[ws]; ok {
		return l
	}
	return w.Machine
}

// wsLimits is kind's lease_max by workspace.
func (m *Manager) wsLimits(kind string) WorkspaceLimits {
	if m.WorkspaceMax == nil {
		return WorkspaceLimits{}
	}
	return m.WorkspaceMax(kind)
}

// waitingAhead is whether a waiter for kind is in line that a free lease would go to, one not held back by its workspace's lease_max.
// The caller holds mu.
func (m *Manager) waitingAhead(kind string, lim WorkspaceLimits) bool {
	return slices.ContainsFunc(m.queues[kind], func(w *waiter) bool { return m.wsFull(kind, w.ws, lim) == "" })
}

// wsFull is why ws may hold no more of kind, its lease_max rule as a clause; "" when it may. A reserved lease counts. The caller holds mu.
func (m *Manager) wsFull(kind, ws string, lim WorkspaceLimits) string {
	limit := lim.of(ws)
	if limit.N <= 0 {
		return ""
	}
	n := 0
	for _, l := range m.held {
		if l.Kind == kind && l.Workspace == ws {
			n++
		}
	}
	if n < limit.N {
		return ""
	}
	by := ""
	if limit.SetBy != "" {
		by = " (set by " + limit.SetBy + ")"
	}
	return fmt.Sprintf("your workspace holds %d, its kept rule lease_max.%s %d%s", n, kind, limit.N, by)
}

// grant holds l under a new id from now. The caller holds mu.
func (m *Manager) grant(l Lease) *Lease {
	m.next++
	l.ID, l.Since = m.next, m.now()
	m.held = append(m.held, &l)
	m.put(&l)
	return &l
}

// put stores l, logging a failure: the leases in memory stay the truth.
func (m *Manager) put(l *Lease) {
	if m.Store == nil {
		return
	}
	if err := m.Store.Put(*l); err != nil {
		m.logf("lease: storing lease %d: %v", l.ID, err)
	}
}

// enqueue puts w at the back of its kind's line. The caller holds mu.
func (m *Manager) enqueue(w *waiter) {
	if m.queues == nil {
		m.queues = map[string][]*waiter{}
	}
	m.queues[w.kind] = append(m.queues[w.kind], w)
}

// heldSeconds is each workspace's seconds holding kind in the last hour, ended leases and held ones. The caller holds mu.
func (m *Manager) heldSeconds(kind string, now int64) map[string]int64 {
	from := now - fairS
	out := map[string]int64{}
	for _, s := range m.ended {
		if s.kind == kind && s.until > from {
			out[s.ws] += s.until - max(s.from, from)
		}
	}
	for _, l := range m.held {
		if l.Kind == kind {
			out[l.Workspace] += now - max(l.Since, from)
		}
	}
	return out
}

// inLine is kind's waiters in the order they are granted: the workspace that held kind least in the last hour first, first come first
// served among equals. The caller holds mu.
func (m *Manager) inLine(kind string) []*waiter {
	q := append([]*waiter{}, m.queues[kind]...)
	if len(q) > 1 {
		used := m.heldSeconds(kind, m.now())
		sort.SliceStable(q, func(i, j int) bool { return used[q[i].ws] < used[q[j].ws] })
	}
	return q
}

// promote grants kind's waiters in line while a lease of it is free. The caller holds mu.
func (m *Manager) promote() {
	if len(m.queues) == 0 {
		return
	}
	limits := m.limits()
	for _, kind := range Kinds {
		if len(m.queues[kind]) == 0 {
			continue
		}
		granted, lim := map[*waiter]bool{}, m.wsLimits(kind)
		for _, w := range m.inLine(kind) {
			if m.wsFull(kind, w.ws, lim) != "" { // its workspace is at its own limit; the next in line may not be
				continue
			}
			port, ok := m.free(kind, limits)
			if !ok {
				break
			}
			granted[w] = true
			if w.got != nil {
				w.got <- m.grant(Lease{Kind: w.kind, Agent: w.agent, Workspace: w.ws, Hold: w.hold, PID: w.pid, Port: port})
				continue
			}
			l := m.grant(Lease{Kind: w.kind, Agent: w.agent, Workspace: w.ws, Task: w.task, Until: m.now() + ClaimS, Port: port})
			m.forget(w.id)
			m.logf("lease: %s (lease %d) reserved for %s's T%d", w.kind, l.ID, w.agent, w.task)
			gate, kind := w.gate, w.kind
			m.after = append(m.after, func() { m.Tasks.Granted(gate, kind) })
		}
		var left []*waiter
		for _, w := range m.queues[kind] {
			if !granted[w] {
				left = append(left, w)
			}
		}
		m.queues[kind] = left
	}
}

// Refusal is an ask that waited its ceiling and got nothing.
type Refusal struct{ Why string }

func (r *Refusal) Error() string { return r.Why }

// OutOfOrder is an ask that would wait while its asker holds a hold, or a lease of a kind after the asked one in Kinds: refused, so
// nothing waits.
type OutOfOrder struct{ Why string }

func (o *OutOfOrder) Error() string { return o.Why }

// holdAndWait is why agent's ask for kind, made by pid (0 for a hold), may not wait, "" when it may. A hold counts agent-wide; a
// command's lease only for the process that took it or one under it, as only a nested hl use can wait on its own lease. The caller
// holds mu.
func (m *Manager) holdAndWait(kind, agent string, pid int) string {
	for _, l := range m.held {
		if l.Reserved() || l.Agent != agent {
			continue
		}
		if l.Hold {
			return fmt.Sprintf("Refused: no %s is free, and you hold a %s (lease %d, --hold): hl use --release it, then ask again. "+
				"Nothing ran.", Noun(kind), Noun(l.Kind), l.ID)
		}
		mine := pid != 0 && (l.PID == pid || slices.Contains(l.Pids, pid))
		if mine && slices.Index(Kinds, l.Kind) > slices.Index(Kinds, kind) {
			return fmt.Sprintf("Refused: no %s is free, and this command runs under a %s (lease %d); ask for both in one hl use "+
				"(hl use %s %s -- ...). Nothing ran.", Noun(kind), Noun(l.Kind), l.ID, kind, l.Kind)
		}
	}
	return ""
}

// Blocked is an ask that waits in line as a blocker on its task: nothing ran, and the task comes back when a lease is reserved for it.
type Blocked struct {
	Task, Gate int64
	Why        string
}

func (b *Blocked) Error() string { return b.Why }

// Take is a lease of kind for agent: a hold, or one for the command running under pid. A lease reserved for the agent's current task
// is taken at once, as is a free one when no ask is ahead in line. Otherwise an agent with a current task gets *Blocked: its ask waits
// in line as a blocker on the task. One with none waits here: past ceiling it is a *Refusal naming the holders, and done closing (the
// asker hung up) ends the wait with no lease.
func (m *Manager) Take(done <-chan struct{}, kind, agent string, hold bool, pid int, ceiling time.Duration) (*Lease, error) {
	ws, task, lim := m.workspaceOf(agent), int64(0), m.wsLimits(kind)
	if m.Tasks != nil {
		task = m.Tasks.Current(agent)
	}
	m.mu.Lock()
	for _, l := range m.held {
		if task != 0 && l.Reserved() && l.Task == task && l.Kind == kind {
			l.Agent, l.Hold, l.PID, l.Until = agent, hold, pid, 0 // Since stays: the reserved time was held
			m.put(l)
			m.logf("lease: %s took the %s (lease %d) reserved for T%d", agent, kind, l.ID, task)
			c := *l
			m.unlock()
			return &c, nil
		}
	}
	limits := m.limits()
	full := m.wsFull(kind, ws, lim)
	if full != "" && kind != Port {
		if _, free := m.free(kind, limits); !free {
			full += fmt.Sprintf(", and %s", m.heldBy(kind))
		}
	}
	if full == "" && !m.waitingAhead(kind, lim) {
		port, ok := m.free(kind, limits)
		if !ok && m.revokeOne(kind, agent, m.periods(limits)[kind]) {
			port, ok = m.free(kind, limits)
		}
		if ok {
			l := m.grant(Lease{Kind: kind, Agent: agent, Workspace: ws, Hold: hold, PID: pid, Port: port})
			m.unlock()
			return l, nil
		}
	}
	if why := m.holdAndWait(kind, agent, pid); why != "" {
		m.unlock()
		return nil, &OutOfOrder{why}
	}
	if task != 0 {
		return nil, m.block(kind, agent, ws, task, full)
	}
	w := &waiter{kind: kind, agent: agent, ws: ws, since: m.now(), hold: hold, pid: pid, got: make(chan *Lease, 1)}
	m.enqueue(w)
	m.unlock()
	timer := time.NewTimer(ceiling)
	defer timer.Stop()
	select {
	case l := <-w.got:
		return m.unlessGone(done, l), nil
	case <-done:
	case <-timer.C:
	}
	m.mu.Lock()
	defer m.unlock()
	select {
	case l := <-w.got: // granted as the wait ended
		select {
		case <-done:
			m.drop(l.ID, "its asker hung up")
			return nil, nil
		default:
			return l, nil
		}
	default:
	}
	m.unqueue(w)
	select {
	case <-done:
		return nil, nil
	default:
	}
	why := m.heldBy(kind)
	if full = m.wsFull(kind, ws, lim); full != "" {
		why = full + ", and " + why
	}
	return nil, &Refusal{fmt.Sprintf("Refused: no %s came free in %s; %s. Nothing ran.", Noun(kind), ceiling.Round(time.Minute), why)}
}

// block puts task's ask for kind in line behind a new gate on the task, or answers the ask already there. Called holding mu; lets it go.
func (m *Manager) block(kind, agent, ws string, task int64, full string) error {
	for _, w := range m.queues[kind] {
		if w.task == task {
			m.unlock()
			return m.blocked(kind, task, w.gate, full)
		}
	}
	if m.asking[task] { // another hl use of this task is opening its gate
		m.unlock()
		return m.blocked(kind, task, 0, full)
	}
	if m.asking == nil {
		m.asking = map[int64]bool{}
	}
	m.asking[task] = true
	m.unlock()
	gate, err := m.Tasks.Wait(agent, task, kind)
	m.mu.Lock()
	defer m.unlock()
	delete(m.asking, task)
	if err != nil {
		m.logf("lease: %s's T%d cannot wait for a %s: %v", agent, task, kind, err)
		return err
	}
	m.next++
	w := &waiter{id: m.next, kind: kind, agent: agent, ws: ws, since: m.now(), task: task, gate: gate}
	m.enqueue(w)
	if m.Store != nil {
		if err := m.Store.AddAsk(Ask{ID: w.id, Kind: kind, Agent: agent, Workspace: ws, Task: task, Gate: gate, Since: w.since}); err != nil {
			m.logf("lease: storing ask %d: %v", w.id, err)
		}
	}
	m.logf("lease: %s's T%d waits for a %s (gate T%d)", agent, task, kind, gate)
	var later []int64 // kept while the task waits, they would be hold-and-wait: they go to the next ask
	for _, l := range m.held {
		if l.Reserved() && l.Task == task && slices.Index(Kinds, l.Kind) > slices.Index(Kinds, kind) {
			later = append(later, l.ID)
		}
	}
	for _, id := range later {
		m.drop(id, fmt.Sprintf("T%d now waits for a %s, which comes first", task, Noun(kind)))
	}
	m.promote()
	return m.blocked(kind, task, gate, full)
}

// blocked is the ask's answer while it waits; full is its workspace's lease_max clause when that is what holds it back.
func (m *Manager) blocked(kind string, task, gate int64, full string) *Blocked {
	on := ""
	if gate != 0 {
		on = fmt.Sprintf(" (gate T%d)", gate)
	}
	none := fmt.Sprintf("No %s is free", Noun(kind))
	if full != "" {
		none = fmt.Sprintf("No %s for you yet: %s", Noun(kind), full)
	}
	return &Blocked{Task: task, Gate: gate, Why: fmt.Sprintf("%s: T%d now waits for one%s and comes back by itself once one "+
		"is reserved for it; then run this hl use again. Take your next task. Nothing ran.", none, task, on)}
}

// unqueue takes w out of its line. The caller holds mu.
func (m *Manager) unqueue(w *waiter) {
	q := m.queues[w.kind]
	for i, x := range q {
		if x == w {
			m.queues[w.kind] = append(q[:i:i], q[i+1:]...)
			return
		}
	}
}

// unlessGone is l, or nil with l given back when its asker hung up as it was granted.
func (m *Manager) unlessGone(done <-chan struct{}, l *Lease) *Lease {
	select {
	case <-done:
		m.mu.Lock()
		defer m.unlock()
		m.drop(l.ID, "its asker hung up")
		return nil
	default:
		return l
	}
}

// heldBy is who holds kind's leases, in words. The caller holds mu.
func (m *Manager) heldBy(kind string) string {
	var who []string
	for _, l := range m.held {
		switch {
		case l.Kind == kind && l.Reserved():
			who = append(who, fmt.Sprintf("reserved for %s's T%d", l.Agent, l.Task))
		case l.Kind == kind:
			who = append(who, fmt.Sprintf("%s since %s", l.Agent, time.Unix(l.Since, 0).Format("15:04")))
		}
	}
	if len(who) == 0 {
		return "none is held"
	}
	return "held by " + andList(who)
}

func andList(s []string) string {
	if len(s) < 2 {
		return strings.Join(s, "")
	}
	return strings.Join(s[:len(s)-1], ", ") + " and " + s[len(s)-1]
}

// drop ends lease id. The caller holds mu.
func (m *Manager) drop(id int64, why string) bool {
	for i, l := range m.held {
		if l.ID == id {
			m.held = append(m.held[:i:i], m.held[i+1:]...)
			m.forget(id)
			now := m.now()
			kept := m.ended[:0]
			for _, s := range m.ended {
				if s.until > now-fairS {
					kept = append(kept, s)
				}
			}
			m.ended = append(kept, span{l.Kind, l.Workspace, l.Since, now})
			m.logf("lease: %s's %s (lease %d) freed: %s", l.Agent, l.Kind, l.ID, why)
			m.promote()
			return true
		}
	}
	return false
}

// Release ends lease id when agent holds it; false when it does not.
func (m *Manager) Release(id int64, agent string) bool {
	m.mu.Lock()
	defer m.unlock()
	for _, l := range m.held {
		if l.ID == id && l.Agent == agent {
			return m.drop(id, "released")
		}
	}
	return false
}

// ReleaseAgent ends agent's leases: of kind ("" for every kind), only its holds when holdsOnly. It is how many ended. A reservation is
// its task's, so it stays.
func (m *Manager) ReleaseAgent(agent, kind string, holdsOnly bool, why string) int {
	m.mu.Lock()
	defer m.unlock()
	var ids []int64
	for _, l := range m.held {
		if l.Agent == agent && (kind == "" || l.Kind == kind) && (l.Hold || !holdsOnly) && !l.Reserved() {
			ids = append(ids, l.ID)
		}
	}
	for _, id := range ids {
		m.drop(id, why)
	}
	return len(ids)
}

// Holds is whether agent holds a lease of kind taken with --hold.
func (m *Manager) Holds(agent, kind string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range m.held {
		if l.Agent == agent && l.Kind == kind && l.Hold {
			return true
		}
	}
	return false
}

// Check notes each lease's processes from a sample and frees a command's lease once its hl use process and every process last seen
// under it are gone, so a killed run holds nothing. A reservation not taken in ClaimS goes to the next ask; its task asks again at its
// next hl use. An ask or a reservation whose task ended or left its agent is dropped, and a lease gate no ask names is closed. It also
// lets in the waiters a raised limit admits.
func (m *Manager) Check(procs []procload.Proc) {
	m.mu.Lock()
	defer m.unlock()
	m.dropEnded()
	children := map[int][]int{}
	for _, p := range procs {
		if !p.Gone {
			children[p.PPID] = append(children[p.PPID], p.PID)
		}
	}
	var gone []int64
	var lapsed []Lease
	for _, l := range m.held {
		if l.Reserved() {
			if l.Until <= m.now() {
				lapsed = append(lapsed, *l)
			}
			continue
		}
		if l.Hold {
			l.Pids = nil
			re := procOf[l.Kind] // nil for a port: no process looks like one
			for _, p := range procs {
				if !p.Gone && p.Agent == l.Agent && re != nil && re.MatchString(p.Cmd) {
					l.Pids = append(l.Pids, p.PID)
				}
			}
			sort.Ints(l.Pids)
			continue
		}
		if m.alive(l.PID) {
			l.Pids, l.sampled = tree(l.PID, children), m.now()
			continue
		}
		var left []int
		for _, pid := range l.Pids {
			if pid != l.PID && m.alive(pid) {
				left = append(left, pid)
			}
		}
		if l.Pids = left; len(left) == 0 {
			gone = append(gone, l.ID)
		}
	}
	for _, id := range gone {
		m.drop(id, "its processes ended")
	}
	for _, l := range lapsed {
		m.drop(l.ID, fmt.Sprintf("not taken for T%d in %d minutes", l.Task, ClaimS/60))
	}
	m.killDue()
	m.overTime(m.periods(m.limits()))
	m.promote()
	if m.Tasks != nil {
		keep := map[int64]bool{}
		for _, q := range m.queues {
			for _, w := range q {
				keep[w.gate] = true
			}
		}
		m.after = append(m.after, func() { m.Tasks.CloseStray(keep) })
	}
}

// dropEnded drops the asks and the reservations whose task ended or left its agent. The caller holds mu.
func (m *Manager) dropEnded() {
	if m.Tasks == nil {
		return
	}
	for _, kind := range Kinds {
		for _, w := range append([]*waiter{}, m.queues[kind]...) {
			if w.got == nil && !m.Tasks.Open(w.agent, w.task) {
				m.unqueue(w)
				m.forget(w.id)
				m.logf("lease: %s's ask for a %s dropped: T%d ended or moved", w.agent, kind, w.task)
				m.after = append(m.after, func() { m.Tasks.Dropped(w.gate, fmt.Sprintf("T%d no longer waits", w.task)) })
			}
		}
	}
	var ended []int64
	for _, l := range m.held {
		if l.Reserved() && !m.Tasks.Open(l.Agent, l.Task) {
			ended = append(ended, l.ID)
		}
	}
	for _, id := range ended {
		m.drop(id, "its task ended or moved")
	}
}

// tree is pid and its descendants, sorted.
func tree(pid int, children map[int][]int) []int {
	out := []int{pid}
	for i := 0; i < len(out) && i < 4096; i++ {
		out = append(out, children[out[i]]...)
	}
	sort.Ints(out)
	return out
}

// Waiting is one ask in a queue.
type Waiting struct {
	Agent     string
	Workspace string
	Task      int64 // the task the ask blocks; 0 for a call held open
	Since     int64
	Position  int    // 1 is next
	Says      string // "waiting for a browser, 2nd in line, held by a and b"
}

// Pool is one kind as the read serves it.
type Pool struct {
	Kind  string
	Max   int
	Held  []Lease
	Queue []Waiting
}

// View is every kind, in Kinds' order.
func (m *Manager) View() []Pool {
	m.mu.Lock()
	defer m.mu.Unlock()
	limits := m.limits()
	out := make([]Pool, 0, len(Kinds))
	for _, kind := range Kinds {
		p := Pool{Kind: kind, Max: limits[kind], Held: []Lease{}, Queue: []Waiting{}}
		var who []string
		for _, l := range m.held {
			if l.Kind == kind {
				c := *l
				c.Pids = append([]int{}, l.Pids...)
				p.Held = append(p.Held, c)
				who = append(who, l.Agent)
			}
		}
		for i, w := range m.inLine(kind) {
			says := fmt.Sprintf("waiting for a %s, %s in line", kind, ordinal(i+1))
			if len(who) > 0 {
				says += ", held by " + andList(who)
			}
			p.Queue = append(p.Queue, Waiting{Agent: w.agent, Workspace: w.ws, Task: w.task, Since: w.since, Position: i + 1, Says: says})
		}
		out = append(out, p)
	}
	return out
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
