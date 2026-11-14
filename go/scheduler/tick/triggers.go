// Package tick is the daemon's tick: what one pass decides for each agent and the passes that follow (session/triggers.py,
// daemon/evaluate.py and the small policies it calls). Everything it touches that another package owns it reaches through the
// interfaces in ports.go; tests use fakes.
package tick

import (
	"fmt"
	"strconv"
	"strings"
)

// The numbers the triggers and the account pick are made from (session/triggers.py).
const (
	// ContextClampFraction holds an agent's context threshold (team.ContextTokens or its own context_tokens) to this share of a model's
	// window when the window is known.
	ContextClampFraction = 0.60
	// WallPct is where both windows are spent to their own ceiling.
	WallPct = 100
	// PollS is how often the daemon looks.
	PollS = 30
	// MinStretchS is the least stretch of an account's life worth beginning a session for.
	MinStretchS = 30 * 60
)

// Trigger is whether an agent should be handed off now, and why.
type Trigger struct {
	Fired  bool
	Kind   string
	Reason string
}

// Evaluate is whether this agent should hand off now. Usage answers only at the wall; an unreadable figure is not a wall. ctxLimit is a
// token count the caller has already resolved (the agent's own figure or the default, clamped for a known model).
func Evaluate(shortPct, ctxUsed, longPct *int, ctxLimit int) Trigger {
	if shortPct != nil && *shortPct >= WallPct {
		return Trigger{true, "usage", fmt.Sprintf("shortest window %d%% - at the wall", *shortPct)}
	}
	if longPct != nil && *longPct >= WallPct {
		return Trigger{true, "usage_7d", fmt.Sprintf("longest window %d%% - at the wall", *longPct)}
	}
	if ctxUsed != nil && *ctxUsed >= ctxLimit {
		return Trigger{true, "context", fmt.Sprintf("context %s >= %s", commas(*ctxUsed), commas(ctxLimit))}
	}
	short, ctx := "?", "?"
	if shortPct != nil {
		short = strconv.Itoa(*shortPct) + "%"
	}
	if ctxUsed != nil {
		ctx = commas(*ctxUsed)
	}
	return Trigger{Reason: fmt.Sprintf("shortest window %s below the wall, context %s < %s", short, ctx, commas(ctxLimit))}
}

func commas(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		s = "-" + s
	}
	return s
}

// NextReset is when a window rolls over, used as wake_at for a parked agent: shortly from now when the reset is unknown or past.
func NextReset(reset, now int64) int64 {
	if reset > now {
		return reset
	}
	return now + 300
}

// WakeAfter is when a parked agent should look again: the window its trigger fired on has to roll over, and a trigger on the longest
// outlasts every reset of the shortest.
func WakeAfter(kind string, shortReset, longReset, now int64) int64 {
	if kind == "usage_7d" {
		return NextReset(longReset, now)
	}
	return NextReset(shortReset, now)
}
