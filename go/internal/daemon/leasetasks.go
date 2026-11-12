package daemon

import (
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/scheduler/leases"
	"github.com/handline/handline/go/taskboard/supervisor"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// leaseTasks is leases.Tasks on the daemon's task board: an agent's ask waits as a gate on its current task.
type leaseTasks struct{ d *Daemon }

var _ leases.Tasks = leaseTasks{}

func (t leaseTasks) Current(agent string) int64 {
	tid, ok, err := db.CurrentTask(t.d.db, agent)
	if err != nil || !ok || !t.Open(agent, tid) {
		return 0
	}
	return tid
}

func (t leaseTasks) Open(agent string, task int64) bool {
	got, err := db.GetTask(t.d.db, task)
	return err == nil && got != nil && !got.Ended() && got.Acting == agent
}

func (t leaseTasks) Wait(agent string, task int64, kind string) (int64, error) {
	return taskwrites.LeaseWait(t.d.db, t.d.cfg.Home, t.d.teller(), agent, task, leases.Noun(kind))
}

func (t leaseTasks) Granted(gate int64, kind string) {
	if err := taskwrites.LeaseGranted(t.d.db, t.d.cfg.Home, t.d.teller(), gate, leases.Noun(kind)); err != nil {
		t.d.cfg.Log("  leases: opening gate T%d: %v", gate, err)
	}
}

func (t leaseTasks) CloseStray(keep map[int64]bool) {
	gates, err := db.OpenLeaseGates(t.d.db, db.Now()-60)
	if err != nil {
		t.d.cfg.Log("  leases: reading the lease gates: %v", err)
	}
	for _, g := range gates {
		if !keep[g] {
			t.Dropped(g, "no ask waits on it")
		}
	}
}

func (t leaseTasks) Dropped(gate int64, why string) {
	if err := taskwrites.LeaseDropped(t.d.db, t.d.cfg.Home, t.d.teller(), gate, why); err != nil {
		t.d.cfg.Log("  leases: closing gate T%d: %v", gate, err)
	}
}

// tellLease says text to agent at once, a session closed being woken; a failure is logged, the lease's change stands.
func (d *Daemon) tellLease(agent, text string) {
	e := d.env()
	if !e.Ports.Wired() {
		return
	}
	var err error
	defer func() {
		if err != nil {
			d.cfg.Log("  leases: telling %s: %v", agent, err)
		}
	}()
	defer supervisor.Catch(&err)
	e.SayNow(d.db, agent, text, "lease")
}
