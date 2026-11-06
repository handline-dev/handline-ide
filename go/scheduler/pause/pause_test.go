package pause

import (
	"testing"
	"time"

	"github.com/handline/handline/go/kit/storetest"
)

func TestAPauseHoldsUntilItsUntilAndASecondKeepsItsSince(t *testing.T) {
	q := storetest.Open(t)
	at := time.Unix(1000, 0)
	if p, err := Get(q, at); err != nil || p != nil || Why(q, at) != "" {
		t.Fatalf("off by default: %v %v", p, err)
	}
	if _, err := Set(q, at, 2000); err != nil {
		t.Fatal(err)
	}
	if why := Why(q, at.Add(time.Second)); !IsPaused(why) {
		t.Fatalf("not held: %q", why)
	}
	s, err := Set(q, at.Add(500*time.Second), 0)
	if err != nil || s != (State{Since: 1000}) {
		t.Fatalf("a pause while paused: %+v %v", s, err)
	}
	if p, _ := Get(q, time.Unix(5000, 0)); p == nil {
		t.Fatal("a pause with no until ended by itself")
	}
	Set(q, time.Unix(5000, 0), 6000)
	if p, _ := Get(q, time.Unix(5999, 0)); p == nil {
		t.Fatal("ended before its until")
	}
	if p, _ := Get(q, time.Unix(6000, 0)); p != nil {
		t.Fatalf("held at its until: %+v", p)
	}
	if s, _ := Set(q, time.Unix(7000, 0), 0); s.Since != 7000 {
		t.Fatalf("a pause after one that ended took its since: %+v", s)
	}
	if err := Clear(q); err != nil {
		t.Fatal(err)
	}
	if p, _ := Get(q, time.Unix(7000, 0)); p != nil {
		t.Fatal("still paused after Clear")
	}
}

func TestTheReasonIsAHoldAndSaysUntilWhen(t *testing.T) {
	open, timed := Text(State{Since: 1}), Text(State{Since: 1, Until: 1000})
	for _, why := range []string{open, timed} {
		if !IsPaused(why) {
			t.Fatalf("%q", why)
		}
	}
	if open == timed {
		t.Fatal("the until is not said")
	}
	if IsPaused("Waiting: the computer is busy right now.") {
		t.Fatal("the load's reason reads as a pause")
	}
}
