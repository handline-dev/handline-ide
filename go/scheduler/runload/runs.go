package runload

import (
	"cmp"
	"slices"
	"time"

	"github.com/handline/handline/go/scheduler/session"
)

// Run is one command an agent runs now.
type Run struct {
	Agent      string // the agent's key
	ID         string // its Bash call's tool_use_id, or its background shell's job id
	Shape      string
	Started    int64 // epoch seconds
	Background bool
	// Heavy is whether it counts as a heavy run now: its shape is heavy from its start (Shapes.Heavy), or, in the foreground, it
	// has run session.HeavyAfter.
	Heavy  bool
	UsualS int64 // what its shape usually takes, 0 when not known
	// Late is whether it has run past LateFactor of that: the direct sign that runs are slower than they are.
	Late bool
	// What its processes use, when they were found (Found): summed over the shell Claude Code started for it and all under it.
	CPU   float64
	RSS   int64
	Procs int
	Found bool
}

const (
	// DefaultLimit is how long a Bash call may run when it names no limit, as Claude Code's Bash tool says of itself.
	DefaultLimit = 2 * time.Minute
	// limitGrace is how long past its limit a foreground call's start note still counts: the hook that takes it away comes after.
	limitGrace = 30 * time.Second
	// LateFactor is how far past its usual time, in tenths, a heavy run counts as late: 15 is one and a half times.
	LateFactor = 15
	// backgroundFor is how long a background shell whose usual time is not known counts as running.
	backgroundFor = 30 * time.Minute
	// backgroundTimes is how many times its usual time a background shell counts as running: one that never ends (a server, a
	// watcher that was once a long test) is not a run forever.
	backgroundTimes = 4
)

// Background is a background shell that still runs, by its agent's key.
type Background struct {
	Agent, Job, Shape string
	Started           int64
}

// Active is the runs going at now, the oldest first: the foreground calls whose start note stands and whose time limit has not
// passed, and the background shells of a heavy shape that still write. A foreground call of a shape that never counts (a wait,
// Handline's own command) is left out, as is one the processes say has ended (ended, by ID).
func Active(starts []session.OpenStart, backgrounds []Background, shapes Shapes, ended map[string]bool, now time.Time) []Run {
	var out []Run
	for _, s := range starts {
		limit := s.Limit
		if limit <= 0 {
			limit = DefaultLimit
		}
		age := now.Sub(s.At)
		if s.Shape == "" || age < 0 || age > limit+limitGrace || ended[s.ID] {
			continue
		}
		r := Run{Agent: s.Agent, ID: s.ID, Shape: s.Shape, Started: s.At.Unix(), UsualS: shapes.Usual(s.Shape)}
		r.Heavy = shapes.Heavy(s.Shape) || age >= session.HeavyAfter
		r.Late = late(r, shapes, now.Unix())
		out = append(out, r)
	}
	for _, b := range backgrounds {
		if !shapes.Heavy(b.Shape) || ended[b.Job] {
			continue
		}
		usual := shapes.Usual(b.Shape)
		most := int64(backgroundFor.Seconds())
		if usual > 0 {
			most = max(most, backgroundTimes*usual)
		}
		if age := now.Unix() - b.Started; age < 0 || age > most {
			continue
		}
		r := Run{Agent: b.Agent, ID: b.Job, Shape: b.Shape, Started: b.Started, Background: true, Heavy: true, UsualS: usual}
		r.Late = late(r, shapes, now.Unix())
		out = append(out, r)
	}
	slices.SortStableFunc(out, func(a, b Run) int { return cmp.Or(cmp.Compare(a.Started, b.Started), cmp.Compare(a.ID, b.ID)) })
	return out
}

// late is whether r, of a shape that is heavy from its start, has run past LateFactor of its usual time. A run that only became
// heavy by running long has no usual time to be late against.
func late(r Run, shapes Shapes, now int64) bool {
	return r.Heavy && shapes.Heavy(r.Shape) && r.UsualS > 0 && (now-r.Started)*10 > r.UsualS*LateFactor
}
