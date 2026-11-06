package daemon

import (
	"maps"
	"sync"
	"time"

	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/internal/reads"
)

// healthEvery is how often the daemon looks at DaemonStatus.health and keep_awake, so status.changed is told within 10 s of a change
// of either: the load, the account waits, config.yaml and the sleep lock change with no write the daemon makes.
const healthEvery = 5 * time.Second

// healthWatch is the health and keep_awake the status topic was last told. mu is held from the read to the append, so two tellers
// cannot leave an older status last.
type healthWatch struct {
	mu    sync.Mutex
	told  reads.Health
	awake map[string]any
}

// healthLoop looks at health every healthEvery (Config.HealthEvery) until the daemon closes.
func (d *Daemon) healthLoop() {
	defer d.wg.Done()
	every := d.cfg.HealthEvery
	if every <= 0 {
		every = healthEvery
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-t.C:
			d.checkHealth()
		}
	}
}

// checkHealth tells status.changed when health's level or text, or keep_awake, is not what the topic was last told; it says whether it
// told. Only those are read until there is something to tell.
func (d *Daemon) checkHealth() bool { return d.tell(false) }

// statusChanged tells the status topic daemon.status as it stands.
func (d *Daemon) statusChanged() { d.tell(true) }

// seedHealth keeps the health and keep_awake a client reads at the start as told, so the first check tells only a change.
func (d *Daemon) seedHealth() {
	renv := d.readsEnv()
	d.health.mu.Lock()
	defer d.health.mu.Unlock()
	d.health.awake = renv.KeepAwakeView()
	if h, err := renv.Health(); err == nil {
		d.health.told = h
	}
}

// tell tells status.changed, always or only when health's level or text or keep_awake moved, and keeps what it told once it is appended.
func (d *Daemon) tell(always bool) bool {
	d.health.mu.Lock()
	defer d.health.mu.Unlock()
	renv := d.readsEnv()
	if !always {
		h, err := renv.Health()
		if err != nil || h.Level == d.health.told.Level && h.Text == d.health.told.Text && maps.Equal(renv.KeepAwakeView(), d.health.awake) {
			return false
		}
	}
	st, h, err := renv.StatusHealth(d.local)
	if err != nil {
		d.cfg.Log("  stream: status.changed: %v", err)
		return false
	}
	if _, err := d.hub.Append(stream.Entry{Topic: "status", Type: "status.changed", Data: st}); err != nil {
		d.cfg.Log("  stream: status.changed: %v", err)
		return false
	}
	d.health.told = h
	d.health.awake, _ = st["keep_awake"].(map[string]any)
	return true
}
