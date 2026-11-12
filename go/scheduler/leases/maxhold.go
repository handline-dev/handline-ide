package leases

import (
	"fmt"
	"slices"
	"syscall"

	"github.com/handline/handline/go/accounts/usage"
)

// MaxHoldKey is the config.yaml key of kind's max_hold in minutes, 0 for none.
func MaxHoldKey(kind string) string { return kind + "_max_hold_min" }

// maxHoldMin is each kind's max_hold when config.yaml sets none.
var maxHoldMin = map[string]int{Heavy: 30, Simulator: 60, Browser: 30, Port: 0}

// MaxExtends is how many times hl use --extend may add a period to one lease.
const MaxExtends = 2

// killAfterS is how long a revoked command has after TERM before its processes are killed.
const killAfterS = 10

// MaxHold is kind's max_hold in cfg, in seconds: its key when a whole number of minutes from 0, else the kind's default.
func MaxHold(cfg usage.Config, kind string) int64 {
	switch n := cfg[MaxHoldKey(kind)].(type) {
	case int:
		if n >= 0 {
			return int64(n) * 60
		}
	case int64:
		if n >= 0 {
			return n * 60
		}
	}
	return int64(maxHoldMin[kind]) * 60
}

// periods is each kind's max_hold in seconds, 0 for none, read once. It counts only while the kind has a limit: with none nobody waits
// for one, so a lease is never warned or taken back.
func (m *Manager) periods(limits map[string]int) map[string]int64 {
	out := map[string]int64{}
	if m.MaxHold == nil {
		return out
	}
	for _, kind := range Kinds {
		if limits[kind] > 0 {
			out[kind] = m.MaxHold(kind)
		}
	}
	return out
}

// deadline is when l reaches its max_hold of period seconds, its extensions counted.
func (l *Lease) deadline(period int64) int64 { return l.Since + period*int64(1+l.Extended) }

// kill is a revoked command's processes, killed at at unless they ended; each is killed only if it began by seen, when they were sampled.
type kill struct {
	pids     []int
	seen, at int64
}

// overTime warns each lease at 80 % of its period, and for each ask waiting takes back one lease of its kind past its max_hold, oldest
// first. The caller holds mu.
func (m *Manager) overTime(periods map[string]int64) {
	now := m.now()
	for _, l := range m.held {
		p := periods[l.Kind]
		if p == 0 || l.Reserved() || l.Warned {
			continue
		}
		if end := l.deadline(p); now < end && now >= end-p/5 {
			l.Warned = true
			m.tell(l.Agent, fmt.Sprintf("Your %s (lease %d) reaches its max_hold in %d min. Once it has and someone asks for one, it is "+
				"taken back%s. hl use --extend adds %d min (%d times at most, not while anyone waits).", Noun(l.Kind), l.ID,
				(end-now+59)/60, m.howTaken(l), p/60, MaxExtends))
		}
	}
	for _, kind := range Kinds {
		// One per waiter: a port or a slot a revoked command still holds frees only once it exits, so the line may not shrink at once.
		for range len(m.queues[kind]) {
			if !m.revokeOne(kind, "", periods[kind]) {
				break
			}
		}
	}
}

// revokeOne takes back the lease of kind longest past its max_hold of period seconds, never asker's own (its ask is refused instead);
// false when none is past it. The caller holds mu.
func (m *Manager) revokeOne(kind, asker string, period int64) bool {
	if period == 0 {
		return false
	}
	var oldest *Lease
	for _, l := range m.held {
		if l.Kind == kind && !l.Reserved() && l.Agent != asker && m.now() >= l.deadline(period) && (oldest == nil || l.deadline(period) < oldest.deadline(period)) {
			oldest = l
		}
	}
	if oldest == nil {
		return false
	}
	m.revoke(oldest, period)
	return true
}

// revoke ends l, past its max_hold of period seconds, and tells its agent. A command's hl use gets TERM, which it passes on to the
// command, and its processes at the last sample are killed killAfterS later; a hold has no process to stop. A process is signalled
// only if it began by the time it was seen as the lease's, not a later one given its pid. The caller holds mu.
func (m *Manager) revoke(l *Lease, period int64) {
	m.tell(l.Agent, fmt.Sprintf("Your %s (lease %d) was taken back: it passed its max_hold of %d min and another ask waits for one%s.",
		Noun(l.Kind), l.ID, period*int64(1+l.Extended)/60, m.howTaken(l)))
	if !l.Hold && l.PID != 0 && m.ranSince(l.PID, l.Since) {
		m.signal(l.PID, syscall.SIGTERM)
		seen := max(l.sampled, l.Since)
		pids := slices.Clone(l.Pids) // the sample's tree has l.PID once it ran
		if !slices.Contains(pids, l.PID) {
			pids = append(pids, l.PID)
		}
		m.kills = append(m.kills, kill{pids: pids, seen: seen, at: m.now() + killAfterS})
	}
	m.drop(l.ID, "past its max_hold, revoked for a waiting ask")
}

// howTaken is how a revoke reaches l's holder, as a clause.
func (m *Manager) howTaken(l *Lease) string {
	if l.Hold {
		return ": stop using it"
	}
	return fmt.Sprintf(" and the command stopped (TERM, then KILL after %d s)", killAfterS)
}

// killDue kills the processes of revoked commands whose time came and that still run. The caller holds mu.
func (m *Manager) killDue() {
	now := m.now()
	left := m.kills[:0]
	for _, k := range m.kills {
		if now < k.at {
			left = append(left, k)
			continue
		}
		for _, pid := range k.pids {
			if m.alive(pid) && m.ranSince(pid, k.seen) {
				m.signal(pid, syscall.SIGKILL)
			}
		}
	}
	m.kills = left
}

func (m *Manager) signal(pid int, sig syscall.Signal) {
	kill := syscall.Kill
	if m.Signal != nil {
		kill = m.Signal
	}
	if err := kill(pid, sig); err != nil && err != syscall.ESRCH {
		m.logf("lease: signalling %d: %v", pid, err)
	}
}

// tell says text to agent once mu is let go. The caller holds mu.
func (m *Manager) tell(agent, text string) {
	if m.Tell != nil {
		m.after = append(m.after, func() { m.Tell(agent, text) })
	}
}

// Extend adds one period to each of agent's leases of kind (every kind with a max_hold when ""), as hl use --extend: refused, changing
// nothing, when one has no max_hold, was extended MaxExtends times, would still be past it, or has an ask of its kind waiting. It answers
// the leases extended.
func (m *Manager) Extend(agent, kind string) ([]Extension, error) {
	m.mu.Lock()
	defer m.unlock()
	periods := m.periods(m.limits())
	var mine []*Lease
	for _, l := range m.held {
		if l.Agent != agent || l.Reserved() || (kind != "" && l.Kind != kind) {
			continue
		}
		p := periods[l.Kind]
		switch {
		case p == 0 && kind == "":
			continue
		case p == 0:
			return nil, &Refusal{fmt.Sprintf("Refused: your %s (lease %d) has no max_hold, so there is nothing to extend.", Noun(l.Kind), l.ID)}
		case l.Extended >= MaxExtends:
			return nil, &Refusal{fmt.Sprintf("Refused: your %s (lease %d) was extended %d times already, the most.", Noun(l.Kind), l.ID, MaxExtends)}
		case len(m.queues[l.Kind]) > 0:
			return nil, &Refusal{fmt.Sprintf("Refused: an ask waits for a %s, so lease %d is not extended; finish and give it back.", Noun(l.Kind), l.ID)}
		case l.deadline(p)+p <= m.now():
			return nil, &Refusal{fmt.Sprintf("Refused: your %s (lease %d) would still be past its max_hold; finish and give it back.", Noun(l.Kind), l.ID)}
		}
		mine = append(mine, l)
	}
	if len(mine) == 0 {
		return nil, &Refusal{"Refused: you hold no lease with a max_hold to extend."}
	}
	out := make([]Extension, len(mine))
	for i, l := range mine {
		l.Extended++
		l.Warned = false
		m.put(l)
		out[i] = Extension{ID: l.ID, Kind: l.Kind, Extended: l.Extended, MaxHoldAt: l.deadline(periods[l.Kind])}
	}
	return out, nil
}

// Extension is a lease Extend extended: how many times now, and when it reaches its max_hold.
type Extension struct {
	ID        int64
	Kind      string
	Extended  int
	MaxHoldAt int64
}
