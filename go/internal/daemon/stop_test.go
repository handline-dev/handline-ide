package daemon

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// within fails the test unless f returns in time, so a stop that waits forever is a failure and not a ten-minute timeout.
func within(t *testing.T, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stop waited past the limit")
	}
}

// stuck is a started daemon whose Close cannot finish: something on its wait group never ends.
func stuck(t *testing.T) *Daemon {
	t.Helper()
	d := start(t, fakeAuth{key: "k"}, "")
	d.wg.Add(1)
	t.Cleanup(func() { d.wg.Done() })
	return d
}

func TestAStuckCloseAfterASignalWritesTheStacksAndExitsWithOne(t *testing.T) {
	var out strings.Builder
	var exited []int
	within(t, func() { stuck(t).Stop(50*time.Millisecond, &out, true, func(c int) { exited = append(exited, c) }) })
	if fmt.Sprint(exited) != "[1]" || !strings.Contains(out.String(), "daemon.(*Daemon).Close") {
		t.Fatalf("exit %v, dump %q", exited, out.String())
	}
}

// A dev-mode restart past a stuck Close returns, so its exec still runs.
func TestAStuckCloseBeforeARestartWritesTheStacksAndReturns(t *testing.T) {
	var out strings.Builder
	within(t, func() { stuck(t).Stop(50*time.Millisecond, &out, false, func(int) { t.Error("exit") }) })
	if !strings.Contains(out.String(), "daemon.(*Daemon).Close") {
		t.Fatalf("dump %q", out.String())
	}
}

func TestACloseThatFinishesWritesNoDump(t *testing.T) {
	var out strings.Builder
	within(t, func() { start(t, fakeAuth{key: "k"}, "").Stop(time.Minute, &out, true, func(int) { t.Error("exit") }) })
	if out.Len() != 0 {
		t.Fatalf("dump %q", out.String())
	}
}
