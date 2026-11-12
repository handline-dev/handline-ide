// Package leases is how many simulators and browsers run at once: an agent asks for a lease in its turn (hl use), waits while every
// lease of the kind is held, and gives it back when its command exits, when it releases a held one, when its turn or session ends, or
// when the processes behind it are gone. A freed lease goes to the workspace that held the kind least in the last hour, first come
// first served inside it. With the kind's limit 0 (the default) every ask is granted at once: the leases are still listed, and nothing
// waits. The leases are stored, so a restart keeps what is held (docs/design/people-workspaces.md, Resource leases).
package leases

import (
	"fmt"
	"regexp"
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
	Simulator = "simulator" // iOS Simulator, Android emulator
	Browser   = "browser"   // Chrome or Chromium for shots, Playwright, CDP
)

// Kinds are every kind, in the order the read lists them.
var Kinds = []string{Simulator, Browser}

// The config.yaml keys of each kind's limit: the most leases held at once, 0 for no limit.
const (
	SimulatorMaxKey = "simulator_max"
	BrowserMaxKey   = "browser_max"
)

// CeilingS is the longest an ask waits before it is refused.
const CeilingS = 30 * 60

// fairS is how far back a workspace's hold-seconds count toward who is granted next.
const fairS = 60 * 60

// MaxKey is kind's limit key.
func MaxKey(kind string) string { return kind + "_max" }

// Known is whether kind is a kind of lease.
func Known(kind string) bool { return kind == Simulator || kind == Browser }

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
	PID       int    // the hl use process the command runs under; 0 for a hold
	Pids      []int  // its processes at the last sample
}

// Store keeps the held leases across a restart. A hold is stored as PID 0, and comes back with Hold set.
type Store interface {
	// Load is every stored lease and the highest id ever given, so an id is not given twice.
	Load() ([]Lease, int64, error)
	Add(Lease) error
	Remove(id int64) error
}

type waiter struct {
	kind, agent, ws string
	since           int64
	hold            bool
	pid             int
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

	mu     sync.Mutex
	next   int64
	held   []*Lease
	ended  []span
	queues map[string][]*waiter
}

func (m *Manager) workspaceOf(agent string) string {
	if m.WorkspaceOf == nil {
		return ""
	}
	return m.WorkspaceOf(agent)
}

// Adopt takes back the stored leases after a restart: a hold stays until its holder's turn ends, and a command's lease stays while
// the hl use process it was granted to runs, and is dropped when that process is gone or its pid now names a later one.
func (m *Manager) Adopt() error {
	if m.Store == nil {
		return nil
	}
	stored, last, err := m.Store.Load()
	if err != nil {
		return fmt.Errorf("loading the stored leases: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next = max(m.next, last)
	for _, l := range stored {
		m.next = max(m.next, l.ID)
		if !l.Hold && !m.ranSince(l.PID, l.Since) {
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
		if m.Max != nil {
			out[kind] = m.Max(kind)
		}
	}
	return out
}

func (m *Manager) free(kind string, limits map[string]int) bool {
	if limits[kind] <= 0 {
		return true
	}
	n := 0
	for _, l := range m.held {
		if l.Kind == kind {
			n++
		}
	}
	return n < limits[kind]
}

func (m *Manager) grant(kind, agent, ws string, hold bool, pid int) *Lease {
	m.next++
	l := &Lease{ID: m.next, Kind: kind, Agent: agent, Workspace: ws, Since: m.now(), Hold: hold, PID: pid}
	m.held = append(m.held, l)
	if m.Store != nil {
		if err := m.Store.Add(*l); err != nil {
			m.logf("lease: storing lease %d: %v", l.ID, err)
		}
	}
	return l
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
		if len(m.queues[kind]) == 0 || !m.free(kind, limits) {
			continue
		}
		granted := map[*waiter]bool{}
		for _, w := range m.inLine(kind) {
			if !m.free(kind, limits) {
				break
			}
			granted[w] = true
			w.got <- m.grant(w.kind, w.agent, w.ws, w.hold, w.pid)
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

// Take is a lease of kind for agent, granted once one is free and no waiter is ahead in line: a hold, or one for the command running under
// pid. Past ceiling it is a *Refusal naming the holders; done closing (the asker hung up) ends the wait with no lease.
func (m *Manager) Take(done <-chan struct{}, kind, agent string, hold bool, pid int, ceiling time.Duration) (*Lease, error) {
	ws := m.workspaceOf(agent)
	m.mu.Lock()
	if m.queues == nil {
		m.queues = map[string][]*waiter{}
	}
	if len(m.queues[kind]) == 0 && m.free(kind, m.limits()) {
		l := m.grant(kind, agent, ws, hold, pid)
		m.mu.Unlock()
		return l, nil
	}
	w := &waiter{kind: kind, agent: agent, ws: ws, since: m.now(), hold: hold, pid: pid, got: make(chan *Lease, 1)}
	m.queues[kind] = append(m.queues[kind], w)
	m.mu.Unlock()
	timer := time.NewTimer(ceiling)
	defer timer.Stop()
	select {
	case l := <-w.got:
		return m.unlessGone(done, l), nil
	case <-done:
	case <-timer.C:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
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
	q := m.queues[kind]
	for i, x := range q {
		if x == w {
			m.queues[kind] = append(q[:i:i], q[i+1:]...)
			break
		}
	}
	select {
	case <-done:
		return nil, nil
	default:
	}
	return nil, &Refusal{fmt.Sprintf("Refused: no %s came free in %s; %s. Nothing ran.", kind, ceiling.Round(time.Minute), m.heldBy(kind))}
}

// unlessGone is l, or nil with l given back when its asker hung up as it was granted.
func (m *Manager) unlessGone(done <-chan struct{}, l *Lease) *Lease {
	select {
	case <-done:
		m.mu.Lock()
		defer m.mu.Unlock()
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
		if l.Kind == kind {
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
	defer m.mu.Unlock()
	for _, l := range m.held {
		if l.ID == id && l.Agent == agent {
			return m.drop(id, "released")
		}
	}
	return false
}

// ReleaseAgent ends agent's leases: of kind ("" for every kind), only its holds when holdsOnly. It is how many ended.
func (m *Manager) ReleaseAgent(agent, kind string, holdsOnly bool, why string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []int64
	for _, l := range m.held {
		if l.Agent == agent && (kind == "" || l.Kind == kind) && (l.Hold || !holdsOnly) {
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
// under it are gone, so a killed run holds nothing. It also lets in the waiters a raised limit admits.
func (m *Manager) Check(procs []procload.Proc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	children := map[int][]int{}
	for _, p := range procs {
		if !p.Gone {
			children[p.PPID] = append(children[p.PPID], p.PID)
		}
	}
	var gone []int64
	for _, l := range m.held {
		if l.Hold {
			l.Pids = nil
			for _, p := range procs {
				if !p.Gone && p.Agent == l.Agent && procOf[l.Kind].MatchString(p.Cmd) {
					l.Pids = append(l.Pids, p.PID)
				}
			}
			sort.Ints(l.Pids)
			continue
		}
		if m.alive(l.PID) {
			l.Pids = tree(l.PID, children)
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
	m.promote()
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
			p.Queue = append(p.Queue, Waiting{Agent: w.agent, Workspace: w.ws, Since: w.since, Position: i + 1, Says: says})
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
