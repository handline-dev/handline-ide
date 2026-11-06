package outcome

import (
	"errors"
	"testing"
	"time"
)

func TestWaitAnswersOnceTheEffectEndedPastTheLimitOrWhenTheCallerHangsUp(t *testing.T) {
	n := 0
	if ok, err := Wait(time.Minute, nil, func() (bool, error) { n++; return n == 3, nil }); !ok || err != nil || n != 3 {
		t.Fatalf("ended on the third look: %v %v %d", ok, err, n)
	}
	start := time.Now()
	if ok, _ := Wait(50*time.Millisecond, nil, func() (bool, error) { return false, nil }); ok || time.Since(start) < 50*time.Millisecond {
		t.Fatalf("past the limit: %v after %s", ok, time.Since(start))
	}
	gone := make(chan struct{})
	close(gone)
	if ok, _ := Wait(time.Minute, gone, func() (bool, error) { return false, nil }); ok {
		t.Fatal("the caller hung up")
	}
	boom := errors.New("boom")
	if _, err := Wait(time.Minute, nil, func() (bool, error) { return false, boom }); err != boom {
		t.Fatalf("an error ends the wait: %v", err)
	}
}
