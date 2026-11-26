package runload

import (
	"github.com/handline/handline/go/accounts/usage"
)

// Policy is what happens to heavy runs, as the person picked it.
type Policy string

const (
	// Observe only says when the machine is slow: nothing waits and nothing changes. The default.
	Observe Policy = "observe"
	// Priority lowers the priority of the heavy runs' processes while the machine is slow, so everything else stays quick.
	Priority Policy = "priority"
	// Turns has new heavy runs take turns while the machine is slow, under a cap that moves with it, and lowers priority too.
	Turns Policy = "turns"
	// Fixed has heavy runs take turns always, at most a set number at once, slow or not.
	Fixed Policy = "fixed"
)

// The config.yaml keys.
const (
	PolicyKey = "heavy_runs_policy"
	AtOnceKey = "heavy_runs_at_once"
)

// Policies are the values PolicyKey takes.
var Policies = []string{string(Observe), string(Priority), string(Turns), string(Fixed)}

// Config is the policy and, for Fixed, how many heavy runs go at once (0: half the cores).
type Config struct {
	Policy Policy
	AtOnce int
}

// ConfigOf reads the policy from config; anything it does not know is Observe.
func ConfigOf(cfg usage.Config) Config {
	c := Config{Policy: Observe}
	switch p, _ := cfg[PolicyKey].(string); Policy(p) {
	case Priority, Turns, Fixed:
		c.Policy = Policy(p)
	}
	switch n := cfg[AtOnceKey].(type) {
	case int:
		c.AtOnce = max(0, n)
	case int64:
		c.AtOnce = int(max(0, n))
	}
	return c
}

// What says the machine is slow (State.Causes).
const (
	CauseRuns     = "runs"     // heavy runs take much longer than they usually do
	CauseTimeouts = "timeouts" // heavy runs ended at their own time limit
	CauseLoop     = "loop"     // the daemon's own loop takes long
)

const (
	// Every is how often the machine is looked at, in seconds.
	Every = 15
	// enterAfter is how many looks in a row must show a cause before the machine is called slow: half a minute.
	enterAfter = 2
	// CalmFor is how long no cause may show before a slow machine is called calm again, and how long each step of the cap's way
	// back up takes; in seconds.
	CalmFor = 3 * 60
	// shrinkEvery is how long a cause must go on showing before the cap is lowered again: about the time a run takes to end, as
	// lowering the cap frees nothing until one does. In seconds.
	shrinkEvery = 3 * 60
	// LateRuns is how many heavy runs must be late at once to be a cause: one late run is that run's own matter.
	LateRuns = 2
	// TimeoutWindow is how far back a run that hit its time limit counts, in seconds; Timeouts is how many of them are a cause.
	TimeoutWindow = 5 * 60
	Timeouts      = 2
	// LoopSlowMs is how long the daemon's own loop takes before that is a cause. A calm one takes about 2 s; it took 10 s in hours
	// when every run was on time and 56 s in the one where a quarter of them hit their limit.
	LoopSlowMs = 30_000
)

// Signals is what one look at the machine found.
type Signals struct {
	Late     int   // heavy runs going that are late
	TimedOut int   // heavy runs that ended at their own time limit in the last TimeoutWindow
	LoopMs   int64 // how long the daemon's loop last took, or has taken so far when that is longer
	Heavy    int   // heavy runs going
	Cores    int
}

func (s Signals) causes() []string {
	var out []string
	if s.Late >= LateRuns {
		out = append(out, CauseRuns)
	}
	if s.TimedOut >= Timeouts {
		out = append(out, CauseTimeouts)
	}
	if s.LoopMs >= LoopSlowMs {
		out = append(out, CauseLoop)
	}
	return out
}

// State is whether the machine is slow and the cap that follows from it; it is kept across a restart.
type State struct {
	Slow  bool  `json:"slow,omitempty"`
	Since int64 `json:"since,omitempty"` // since when it has been slow, epoch seconds
	// Causes are what says so now, or said so last while it is slow or capped.
	Causes []string `json:"causes,omitempty"`
	// Signs is how many looks in a row showed a cause, while not slow. It is not kept across a restart: a daemon that was down has
	// no look before its first.
	Signs int `json:"-"`
	// CalmSince is since when no cause has shown, while slow or capped; 0 while one shows.
	CalmSince int64 `json:"calm_since,omitempty"`
	// Cap is the most heavy runs that go at once under Turns; 0 is no cap. It is set when the machine turns slow, lowered while it
	// stays slow, and raised one at a time once it is calm, until it passes Peak and goes.
	Cap   int   `json:"cap,omitempty"`
	CapAt int64 `json:"cap_at,omitempty"` // when the cap last moved
	Peak  int   `json:"peak,omitempty"`   // the most heavy runs seen at once since the cap was set
}

// kept is the state as it is kept across a restart: without the count of looks.
func (s State) kept() State {
	s.Signs = 0
	return s
}

// minCap is the least the cap goes to: a quarter of the cores, and never under 2.
func minCap(cores int) int { return max(2, cores/4) }

// Step is the state after one more look at now (epoch seconds). The machine turns slow when enterAfter looks in a row show a
// cause, and calm when none has shown for CalmFor: the two differ so it does not flicker. The cap starts one under the heavy runs
// going, shrinks by a quarter each shrinkEvery the cause stays, and grows by one each calm CalmFor.
func Step(prev State, sig Signals, now int64) State {
	c, s, floor := sig.causes(), prev, minCap(sig.Cores)
	if len(c) > 0 {
		s.Causes = c
	}
	if prev.Slow {
		s.Peak = max(s.Peak, sig.Heavy)
		if len(c) > 0 {
			s.CalmSince = 0
			if now-s.CapAt >= shrinkEvery {
				s.Cap, s.CapAt = max(floor, s.Cap*3/4), now
			}
			return s
		}
		if s.CalmSince == 0 {
			s.CalmSince = now
		}
		if now-s.CalmSince >= CalmFor {
			s.Slow, s.Since, s.Signs = false, 0, 0
			return s.raise(now)
		}
		return s
	}
	if len(c) == 0 {
		s.Signs = 0
	} else {
		s.Signs++
	}
	if s.Signs >= enterAfter {
		s.Slow, s.Since, s.Signs, s.CalmSince, s.CapAt = true, now, 0, 0, now
		if prev.Cap > 0 { // slow again on the way back up
			s.Cap, s.Peak = max(floor, prev.Cap*3/4), max(prev.Peak, sig.Heavy)
		} else {
			s.Cap, s.Peak = max(floor, sig.Heavy-1), sig.Heavy
		}
		return s
	}
	if prev.Cap == 0 {
		if len(c) == 0 {
			s.Causes = nil
		}
		return s
	}
	if len(c) > 0 {
		s.CalmSince = 0
		return s
	}
	if s.CalmSince == 0 {
		s.CalmSince = now
	}
	if now-max(s.CalmSince, s.CapAt) >= CalmFor {
		return s.raise(now)
	}
	return s
}

// raise lifts the cap by one at now, and takes it away once it passes the most runs seen.
func (s State) raise(now int64) State {
	s.Cap, s.CapAt, s.CalmSince = s.Cap+1, now, now
	if s.Cap > s.Peak {
		return State{}
	}
	return s
}

// CapFor is the most heavy runs that go at once under c on a machine of cores; 0 is no cap.
func (s State) CapFor(c Config, cores int) int {
	switch c.Policy {
	case Turns:
		return s.Cap
	case Fixed:
		if c.AtOnce > 0 {
			return c.AtOnce
		}
		return max(2, cores/2)
	}
	return 0
}

// LowersPriority is whether the heavy runs' processes are given a lower priority now under c.
func (s State) LowersPriority(c Config) bool {
	return s.Slow && (c.Policy == Priority || c.Policy == Turns)
}
