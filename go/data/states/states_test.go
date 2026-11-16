package states

import (
	"testing"
	"time"
)

func TestBackoffDoublesFromAMinuteAndStopsAtAnHour(t *testing.T) {
	for streak, want := range map[int]time.Duration{-1: 0, 0: 0, 1: time.Minute, 2: 2 * time.Minute, 3: 4 * time.Minute, 7: time.Hour, 80: time.Hour} {
		if got := BackoffFor(streak); got != want {
			t.Errorf("BackoffFor(%d) = %v, want %v", streak, got, want)
		}
	}
}

func TestEveryWritableStateHasAWaker(t *testing.T) {
	for _, s := range []string{Idle, Queued, Running, Handoff, Degraded, Parked, Throttled, Stalled} {
		if Waker[s] == "" {
			t.Errorf("%s has no way back", s)
		}
	}
}
