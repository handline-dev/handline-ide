// Package states is the states an agent is in, what wakes each, and the timing constants the daemon runs on (session/states.py, daemon/states.py).
package states

import (
	"time"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/rowstate"
)

const (
	Idle      = rowstate.Idle
	Queued    = rowstate.Queued
	Running   = rowstate.Running
	Done      = rowstate.Done
	Handoff   = rowstate.Handoff
	Degraded  = rowstate.Degraded
	Parked    = rowstate.Parked
	Throttled = rowstate.Throttled
	Stalled   = rowstate.Stalled
)

// Evaluated is what the tick looks at (8.4). Anything but STALLED has a way back.
var Evaluated = map[string]bool{Running: true, Handoff: true, Parked: true, Queued: true, Throttled: true, Idle: true, Degraded: true}

// Waker says what wakes each state the daemon can write; a state without a way back would sit idle for ever.
var Waker = map[string]string{
	Running:   "the agent's own Stop hook ends its turn - the daemon never touches a RUNNING row",
	Idle:      "would_wake, the moment a request or an answer is waiting for it",
	Queued:    "would_wake dispatches it at once - it already has work",
	Handoff:   "the handoff's own step commits, or expire_handoffs() times it out into DEGRADED",
	Degraded:  "the daemon retries it itself after DegradedRetryS",
	Parked:    "wake_at - the usage window, the async-reply fallback, or the short move recheck",
	Throttled: "wake_at - the ask window rolls over (team.ASK_WINDOW_S)",
	Stalled:   "a person: w, behind a confirmation (unstall, folded into wake)",
}

const (
	// MovingMark is the word a moving row's detail and view show; ParkReason, not the word, says a row is moving.
	MovingMark = "moving"
	// ByHandMark is the trigger kind of a park a person started by hand.
	ByHandMark = "by hand"
	// EarlyUsageTrigger is a handoff short of the wall, at usage_move_before_pct: a move to an account with room, never a wait for a reset.
	EarlyUsageTrigger = "usage_early"
	// MoveBeforeWeekPct is the percent of its week at which an idle agent with work waiting moves early (while usage_move_before_pct is
	// on), and past which a new start goes elsewhere when another account fits.
	MoveBeforeWeekPct = 98
	// FoldKind is the event the tick writes when it closes an idle session fresh for its full context, and the cost-pilot switch for it.
	FoldKind = "idle_fold"
	// CompactAskedKind is the event session.compact writes: the tick hands the agent off as for a full context until a handoff or a new
	// session comes after it.
	CompactAskedKind = "compact_asked"
)

// ParkReason is why the daemon parked or queued row, from the column its writer set.
func ParkReason(row *db.AgentState) verdict.ParkReason {
	return verdict.ParseParkReason(row.ParkReason.String)
}

// The timing constants, in seconds.
const (
	LimitSettleS           = 30
	DegradedRetryS         = 600
	AssumedAccountRecheckS = 300
	LoginCheckCooldownS    = 300
	UsagePollS             = 120
	UsageMaxAgeS           = 900
	UsageFailBackoffCapS   = 1800
	MaxFailedAttempts      = 3
	MaxThrottleStreak      = 5
	IdleCloseS             = 600
	PromptCacheS           = 3600
	ResumeWarmS            = PromptCacheS - 300
	HandoffDeadlineS       = 90
	BackoffBaseS           = 60
	BackoffMaxS            = 3600
	MinHandoffIntervalS    = 120
)

// BackoffFor is backoff_for: seconds to wait before retrying after streak failed attempts.
func BackoffFor(streak int) time.Duration {
	if streak <= 0 {
		return 0
	}
	s := BackoffBaseS << (streak - 1)
	if streak > 20 || s > BackoffMaxS {
		s = BackoffMaxS
	}
	return time.Duration(s) * time.Second
}
