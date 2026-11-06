package reads

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/internal/status"
	"github.com/handline/handline/go/scheduler/pause"

	"github.com/handline/handline/go/scheduler/loadgate"
	"github.com/handline/handline/go/scheduler/sysres"
)

// The DaemonHealth levels.
const (
	healthOK   = "ok"
	healthNear = "near"
	healthHeld = "held"
)

// nearPoints is how close, in percentage points, a set limit counts as near.
const nearPoints = 10

// HealthClock keeps since when DaemonHealth has had its level, for every server of one daemon; nil reads as the daemon's start.
type HealthClock struct {
	mu    sync.Mutex
	level string
	since int64
}

// at is since when level has held, now being the read's time: a new level starts now.
func (h *HealthClock) at(level string, now, started int64) int64 {
	if h == nil {
		return started
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.level == "" {
		h.level, h.since = level, started
	}
	if h.level != level {
		h.level, h.since = level, now
	}
	return h.since
}

// Health is DaemonHealth; Since is unix seconds.
type Health struct {
	Level, Text string
	Since       int64
}

// Health is DaemonStatus.health as it stands: the daemon's health check reads it alone, and daemon.status with the rest.
func (e *Env) Health() (Health, error) {
	stored, err := e.storedViews()
	if err != nil {
		return Health{}, err
	}
	in := healthIn{limits: loadgate.LimitsOf(usage.LoadConfig(e.Home))}
	for _, n := range e.listAgents() {
		var view struct {
			Status struct{ Reason string } `json:"status"`
		}
		if v, ok := stored[n]; ok && json.Unmarshal(v, &view) == nil {
			if _, waits := accountWait[view.Status.Reason]; waits {
				in.waiting = append(in.waiting, view.Status.Reason)
			}
		}
	}
	if p, _ := pause.Get(e.DB, time.Now()); p != nil {
		in.paused = pause.Text(*p)
	}
	if r := e.resources(); r != nil {
		in.startHold, _ = r["start_hold"].(string)
	}
	for _, n := range e.machineNeeds() {
		in.needs = append(in.needs, needLine(n.(map[string]any)))
	}
	if e.Machine != nil {
		in.m = e.Machine()
	}
	level, text := healthOf(in)
	return Health{level, text, e.HealthClock.at(level, time.Now().Unix(), e.Started.Unix())}, nil
}

// needLine is a needs_you item as health says it: a line that stays the same while the need does, so the topic is not told each time
// its figure moves.
func needLine(n map[string]any) string {
	if n["kind"] == "disk" {
		return "The disk is too full for builds."
	}
	t, _ := n["text"].(string)
	return t
}

// accountWait is what an agent waits on, by its status reason, said without the status text's figures (how far ahead of pace, until
// when), so the topic is not told each time they move.
var accountWait = map[string]string{
	status.Cap:  "no free slot",
	status.Wall: "a usage limit",
	status.Pace: "the weekly pace",
}

func (h Health) view() map[string]any {
	return map[string]any{"level": h.Level, "text": h.Text, "since": ts(h.Since)}
}

// healthIn is what DaemonHealth is made from: each held cause's text ("" for none), the waiting agents' status reasons in name order,
// and the machine against its limits (m nil before the first measure).
type healthIn struct {
	paused, startHold string
	needs, waiting    []string
	limits            loadgate.Limits
	m                 *sysres.Machine
}

// healthOf is DaemonHealth's level and text: held for the first cause that holds work back, else near for a set limit within
// nearPoints, else ok. A near text names the limit, not the figure, so it does not move with every measure.
func healthOf(in healthIn) (level, text string) {
	switch {
	case in.paused != "":
		return healthHeld, in.paused
	case in.startHold != "":
		return healthHeld, in.startHold
	case len(in.needs) > 0:
		return healthHeld, in.needs[0]
	case len(in.waiting) == 1:
		return healthHeld, "1 agent waits for an account: " + accountWait[in.waiting[0]]
	case len(in.waiting) > 1:
		return healthHeld, fmt.Sprintf("%d agents wait for an account: %s", len(in.waiting), accountWait[in.waiting[0]])
	}
	if m := in.m; m != nil {
		if l := in.limits.LoadPct; l > 0 && m.Cores > 0 {
			if pct := int(m.Load1 * 100 / float64(m.Cores)); pct >= l-nearPoints {
				return healthNear, fmt.Sprintf("The computer's load is near its limit of %d%% per core.", l)
			}
		}
		if l := in.limits.RAMFreePct; l > 0 && m.RAMTotal > 0 {
			if pct := int(m.RAMFree * 100 / m.RAMTotal); pct < l+nearPoints {
				return healthNear, fmt.Sprintf("Free memory is near its limit of %d%%.", l)
			}
		}
	}
	return healthOK, ""
}
