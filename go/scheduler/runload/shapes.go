// Package runload watches what the agents' commands cost the machine, by the command that runs and not by the agent that runs it:
// which heavy commands run now, how long each has run against what its shape usually takes, and whether the machine has become slow.
// It is level-triggered and has no model. While nothing is slow it does nothing at all; once runs take much longer than they
// usually do it says so, and the policy the person picked (Policy) decides what else happens: nothing, the runs' priority lowered,
// or new heavy runs taking turns. A run already going is never stopped.
package runload

import (
	"slices"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/policy"
)

// Shape is what the last week says of one command shape (session.HeavyShape).
type Shape struct {
	Runs, Long int   // its timed runs, and those of them that ran session.HeavyAfter or longer
	UsualS     int64 // the median of its recorded heavy runs, in seconds; 0 with none
}

// Shapes is every shape with a run in the last week.
type Shapes map[string]Shape

// minRuns is how many timed runs a shape needs before it is called heavy from its start.
const minRuns = 3

// Heavy is whether a run of shape is a heavy run from its start: at least half of its runs last week were long ones. A shape seen
// fewer than minRuns times is not, and counts only once a run of it has run long.
func (s Shapes) Heavy(shape string) bool {
	x := s[shape]
	return x.Runs >= minRuns && x.Long*2 >= x.Runs
}

// Usual is how long a run of shape usually takes, in seconds; 0 when not known.
func (s Shapes) Usual(shape string) int64 { return s[shape].UsualS }

// ReadShapes reads the shapes of the last week at now (epoch seconds): the counts from shape_days, the usual time from heavy_runs.
func ReadShapes(q db.Q, now int64) (Shapes, error) {
	since := now - int64(policy.HeavyKeep.Seconds())
	out := Shapes{}
	counts, err := q.Query("SELECT shape, SUM(runs), SUM(long) FROM shape_days WHERE day >= ? GROUP BY shape", since/(24*3600))
	if err != nil {
		return nil, err
	}
	for counts.Next() {
		var name string
		var s Shape
		if err := counts.Scan(&name, &s.Runs, &s.Long); err != nil {
			counts.Close()
			return nil, err
		}
		out[name] = s
	}
	if err := counts.Close(); err != nil {
		return nil, err
	}
	took, err := q.Query("SELECT shape, seconds FROM heavy_runs WHERE ended >= ?", since)
	if err != nil {
		return nil, err
	}
	by := map[string][]int64{}
	for took.Next() {
		var name string
		var secs int64
		if err := took.Scan(&name, &secs); err != nil {
			took.Close()
			return nil, err
		}
		by[name] = append(by[name], secs)
	}
	if err := took.Close(); err != nil {
		return nil, err
	}
	for name, secs := range by {
		slices.Sort(secs)
		s := out[name]
		s.UsualS = secs[len(secs)/2]
		if len(secs)%2 == 0 {
			s.UsualS = (secs[len(secs)/2-1] + secs[len(secs)/2]) / 2
		}
		out[name] = s
	}
	return out, nil
}
