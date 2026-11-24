package daemon

import (
	"cmp"
	"sync"
	"time"

	"github.com/handline/handline/go/internal/status"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/scheduler/pause"
)

// pauseClock tells the status topic when the founder's pause changes, and once more when its until comes and it ends by itself.
type pauseClock struct {
	mu     sync.Mutex
	timer  *time.Timer
	closed bool // the daemon is closing: no clock is set, and one that fires tells nothing
}

// pauseChanged tells status.changed and sets the clock for the pause's until, and when the pause was lifted has the work it held go on;
// daemon.pause and daemon.resume call it.
func (d *Daemon) pauseChanged(lifted bool) {
	if lifted {
		d.pauseLifted() // first, so the views a client reads on status.changed no longer say paused
	}
	d.statusChanged()
	d.armPause()
}

// pauseLifted has the agents the pause may have held start or take their work now, not at the next tick: those whose views say it held
// them, and those with work not yet told (held since the last view was stored).
func (d *Daemon) pauseLifted() {
	act := d.env().ActNow
	if act == nil {
		return
	}
	rows, err := d.db.Query("SELECT agent FROM agent_view WHERE json_extract(view, '$.status.reason') = ? "+
		"UNION SELECT target FROM requests WHERE state = 'queued' AND delivered_at IS NULL", status.Paused)
	if err != nil {
		d.cfg.Log("the agents the pause held could not be read: %v", err)
		return
	}
	var agents []string
	for rows.Next() {
		var a string
		if rows.Scan(&a) == nil && actor.KindOf(a) == actor.Agent {
			agents = append(agents, a)
		}
	}
	if err := cmp.Or(rows.Err(), rows.Close()); err != nil {
		d.cfg.Log("the agents the pause held could not be read: %v", err)
		return
	}
	act(agents...)
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
	d.pauseLifted()
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
