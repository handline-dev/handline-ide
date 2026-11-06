// Package pause is the founder's switch that holds new work (daemon.pause, daemon.resume): while it is on, the load gate holds every
// start and delivery it holds for the machine's load, with no ceiling and no force-start pass. It is kept in the meta table, so a
// restart keeps it, and it is off unless set.
package pause

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/timefmt"
	"github.com/handline/handline/go/scheduler/loadgate"
)

// Key is the meta row the pause is kept in.
const Key = db.PauseKey

// Prefix starts every reason Why gives. It starts with loadgate.HeldPrefix, so a caller that tells a hold from a failure sees one.
const Prefix = loadgate.HeldPrefix + "new work is paused"

// IsPaused is whether why is a reason Why gave.
func IsPaused(why string) bool { return strings.HasPrefix(why, Prefix) }

// State is a pause (db.Pause).
type State = db.Pause

// Get is the pause in force at now, nil when there is none (db.GetPause).
func Get(q db.Q, now time.Time) (*State, error) { return db.GetPause(q, now) }

// Set pauses from now until until (0 for until a resume). A pause in force keeps its since.
func Set(q db.Q, now time.Time, until int64) (State, error) {
	s := State{Since: now.Unix(), Until: until}
	if old, err := Get(q, now); err != nil {
		return s, err
	} else if old != nil {
		s.Since = old.Since
	}
	b, _ := json.Marshal(s)
	return s, db.SetMeta(q, Key, string(b))
}

// Clear ends the pause; none in force is fine.
func Clear(q db.Q) error { return db.ClearMeta(q, Key) }

// Why is the hold's reason at now, "" when nothing is paused. A read that fails holds nothing: the switch must never be what stalls work.
func Why(q db.Q, now time.Time) string {
	s, err := Get(q, now)
	if err != nil || s == nil {
		return ""
	}
	return Text(*s)
}

// Text is a pause's reason, as an agent's status shows it.
func Text(s State) string {
	if s.Until == 0 {
		return Prefix + ". It starts when new work is resumed."
	}
	return Prefix + " until " + timefmt.Stamp(s.Until, timefmt.Clock) + ". It starts by itself then, or when new work is resumed."
}
