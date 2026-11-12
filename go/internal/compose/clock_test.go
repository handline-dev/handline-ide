package compose

import (
	"testing"
	"time"

	"github.com/handline/handline/go/taskboard/supervisor"
)

func TestOneClockReachesTheUsageReaderTheManagerAndTheLauncher(t *testing.T) {
	at := time.Unix(1_800_000_000, 500_000_000)
	p, home := rigWith(t, func(c *Config) { c.Now = func() time.Time { return at } })
	lc := p.Lifecycle(cfgOf(p), LifecycleConfig{Env: &supervisor.Env{DB: p.db, Home: home, Log: func(string, ...any) {}}})
	if got := lc.usage.Now(); got != 1_800_000_000.5 {
		t.Fatalf("usage reads %v", got)
	}
	if got := lc.Manager.Now(); got != 1_800_000_000 {
		t.Fatalf("the manager reads %v", got)
	}
	if got := lc.Launch.Now(); !got.Equal(at) {
		t.Fatalf("the launcher reads %v", got)
	}
}
