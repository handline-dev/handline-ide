package tick

import (
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/states"
)

// triggerInput is what chooseTrigger judges: the reading's own trigger, and the other reasons to hand an agent off that the tick finds
// beside it.
type triggerInput struct {
	Eval        Trigger // Evaluate of the reading
	EvalMovable bool    // the agent can be moved for Eval.Kind (only asked when Eval fired and the turn ended on the limit)
	AtWall      *Limit  // the limit the last turn ended on, nil when it did not
	FreshDue    bool    // its task is done and it starts fresh after each (only asked when nothing else fired)
	CompactBy   string  // who asked it to compact (session.compact) with nothing handed off since, "" for no one
	NoteKind    string  // the kind of the handoff whose note is waited for
	NoteKept    bool    // such a wait is kept
	NoteHeld    bool    // the note's turn is not over yet
}

// chooseTrigger is the reason the agent is handed off now, Fired false for none. A turn that ended on the usage limit means the agent is
// at the wall whatever the reading says (it can lag, or be the other window): it moves the same way a trigger would. An agent a person
// asked to compact, or one between tasks that starts fresh after each, is handed off the way a full context is. A handoff that asked for
// the agent's note goes on once the note's turn is over: the reading that fired it may not be trusted yet.
func chooseTrigger(in triggerInput) Trigger {
	trig := in.Eval
	if in.AtWall != nil && !(trig.Fired && in.EvalMovable) {
		kind := "usage"
		if strings.Contains(strings.ToLower(in.AtWall.Text), "week") {
			kind = "usage_7d"
		}
		src := in.AtWall.Source
		if src == "" {
			src = "?"
		}
		trig = Trigger{true, kind, fmt.Sprintf("turn ended on its usage limit: %s (read from %s)", in.AtWall.Text, src)}
	}
	if !trig.Fired && in.CompactBy != "" {
		trig = Trigger{true, "context", "compact asked by " + in.CompactBy}
	}
	if !trig.Fired && in.FreshDue {
		trig = Trigger{true, FreshKind, "its task is done and it starts a fresh session after each"}
	}
	if !trig.Fired && in.NoteKept && !in.NoteHeld {
		trig = Trigger{true, in.NoteKind, "its handoff note is written"}
	}
	return trig
}

// openIdleInput is what openIdle judges.
type openIdleInput struct {
	HasRow         bool
	Closed         bool
	State          string
	AtLimit        bool
	BackgroundWork bool
}

// openIdle is an open session that can take a turn now and runs nothing behind its prompt.
func openIdle(in openIdleInput) bool {
	return in.HasRow && !in.Closed && in.State == states.Idle && !in.AtLimit && !in.BackgroundWork
}
