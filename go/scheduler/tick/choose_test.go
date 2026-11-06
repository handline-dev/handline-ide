package tick

import (
	"strings"
	"testing"
)

func TestChooseTriggerWeighsTheReasonsInOrder(t *testing.T) {
	fired := Trigger{true, "context", "full"}
	wall := &Limit{Text: "Weekly limit reached", Source: "stream"}
	for _, c := range []struct {
		name     string
		in       triggerInput
		fired    bool
		kind     string
		contains string
	}{
		{"nothing", triggerInput{}, false, "", ""},
		{"the reading's own trigger stands", triggerInput{Eval: fired}, true, "context", "full"},
		{"limit with no trigger is the wall", triggerInput{AtWall: &Limit{Text: "5-hour limit"}}, true, "usage", "read from ?"},
		{"a week limit is the weekly kind", triggerInput{AtWall: wall}, true, "usage_7d", "read from stream"},
		{"a movable trigger beats the wall", triggerInput{Eval: fired, EvalMovable: true, AtWall: wall}, true, "context", "full"},
		{"an unmovable trigger yields to the wall", triggerInput{Eval: fired, AtWall: wall}, true, "usage_7d", "Weekly limit"},
		{"fresh after a done task", triggerInput{FreshDue: true}, true, FreshKind, "fresh session"},
		{"the trigger beats fresh", triggerInput{Eval: fired, FreshDue: true}, true, "context", "full"},
		{"the note is written", triggerInput{NoteKept: true, NoteKind: "cache"}, true, "cache", "note is written"},
		{"the note is still held", triggerInput{NoteKept: true, NoteHeld: true}, false, "", ""},
		{"a compact ask", triggerInput{CompactBy: "ron"}, true, "context", "compact asked by ron"},
		{"the trigger beats a compact ask", triggerInput{Eval: Trigger{true, "usage", "wall"}, CompactBy: "ron"}, true, "usage", "wall"},
		{"a compact ask beats fresh", triggerInput{CompactBy: "ron", FreshDue: true}, true, "context", "compact asked"},
		{"fresh beats the note", triggerInput{FreshDue: true, NoteKept: true, NoteKind: "cache"}, true, FreshKind, "fresh session"},
	} {
		got := chooseTrigger(c.in)
		if got.Fired != c.fired || got.Kind != c.kind || (c.contains != "" && !strings.Contains(got.Reason, c.contains)) {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
}

func TestOpenIdleNeedsAnOpenIdleRowWithNothingBehindIt(t *testing.T) {
	ok := openIdleInput{HasRow: true, State: "IDLE"}
	if !openIdle(ok) {
		t.Fatal("open idle row")
	}
	for name, in := range map[string]openIdleInput{
		"no row":          {State: "IDLE"},
		"closed":          {HasRow: true, Closed: true, State: "IDLE"},
		"running":         {HasRow: true, State: "RUNNING"},
		"at the limit":    {HasRow: true, State: "IDLE", AtLimit: true},
		"background work": {HasRow: true, State: "IDLE", BackgroundWork: true},
	} {
		if openIdle(in) {
			t.Errorf("%s", name)
		}
	}
}
