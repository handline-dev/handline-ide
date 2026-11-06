package daemon

import (
	"sync"
	"time"

	"github.com/handline/handline/go/scheduler/pause"
)

// pauseClock tells the status topic when the founder's pause changes, and once more when its until comes and it ends by itself.
type pauseClock struct {
	mu     sync.Mutex
	timer  *time.Timer
	closed bool // the daemon is closing: no clock is set, and one that fires tells nothing
}

// pauseChanged tells status.changed and sets the clock for the pause's until; daemon.pause and daemon.resume call it.
func (d *Daemon) pauseChanged() {
	d.statusChanged()
	d.armPause()
}

// armPause sets the clock for the pause in force: none when there is no pause or it has no until.
func (d *Daemon) armPause() {
	d.pause.mu.Lock()
	defer d.pause.mu.Unlock()
	d.armPauseLocked()
}

func (d *Daemon) armPauseLocked() {
	if d.pause.timer != nil {
		d.pause.timer.Stop()
		d.pause.timer = nil
	}
	if d.pause.closed {
		return
	}
	p, err := pause.Get(d.db, time.Now())
	if err != nil || p == nil || p.Until == 0 {
		return
	}
	d.pause.timer = time.AfterFunc(time.Until(time.Unix(p.Until, 0)), d.pauseCame)
}

// pauseCame tells that the pause ended at its until. The timer runs on the monotonic clock and the until is wall time, so it can come a little early: then the pause is still in force and the clock is set again.
func (d *Daemon) pauseCame() {
	d.pause.mu.Lock()
	defer d.pause.mu.Unlock() // held while it tells, so Close waits for it before the database closes
	if d.pause.closed {
		return
	}
	if p, _ := pause.Get(d.db, time.Now()); p != nil {
		d.armPauseLocked()
		return
	}
	d.statusChanged()
}

// stopPause stops the clock for good; Close calls it before the database closes.
func (d *Daemon) stopPause() {
	d.pause.mu.Lock()
	defer d.pause.mu.Unlock()
	d.pause.closed = true
	if d.pause.timer != nil {
		d.pause.timer.Stop()
	}
}
