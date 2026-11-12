package taskwrites

import (
	"database/sql"
	"fmt"

	"github.com/handline/handline/go/data/db"
)

// LeaseWait blocks task tid of agent on a new gate task while its ask for a lease of noun waits in line, releasing tid when doing,
// in one transaction, as task.wait does for a watch; the leases manager marks the gate done when a lease is reserved for tid
// (LeaseGranted). It answers the gate.
func LeaseWait(d *sql.DB, home string, tell Teller, agent string, tid int64, noun string) (int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	gate, _, err := OpenTask(tx, tell, agent, NewIn{Owner: db.Gate, Title: fmt.Sprintf("T%d waits on a %s", tid, noun), Prio: db.DefaultPriority,
		Expects: "report"})
	if err != nil {
		return 0, err
	}
	t, err := getTask(tx, tid)
	if err != nil {
		return 0, err
	}
	if t == nil {
		return 0, refusal(fmt.Sprintf("no task T%d", tid))
	}
	var r released
	freed := false
	if st, err := state(tx, t); err != nil { // read before the gate blocks it: a blocked task never reads doing
		return 0, err
	} else if st == "doing" {
		if r, err = releaseTask(tx, home, agent, tid, fmt.Sprintf("waits on gate T%d for a %s", gate, noun)); err != nil {
			return 0, err
		}
		freed = true
	}
	if why, err := recordBlock(tx, tid, gate, agent); err != nil || why != "" {
		if err == nil {
			err = refusal(why)
		}
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if tell != nil && freed {
		r.tell(tell, tid)
	}
	return gate, nil
}

// LeaseGranted marks gate done: a lease of noun is reserved for the task it blocks, which comes back to its agent.
func LeaseGranted(d *sql.DB, home string, tell Teller, gate int64, noun string) error {
	return closeLeaseGate(d, home, tell, gate, "a "+noun+" is reserved for it")
}

// LeaseDropped closes gate: the ask it stood for ended (why), so it blocks nothing any more.
func LeaseDropped(d *sql.DB, home string, tell Teller, gate int64, why string) error {
	return closeLeaseGate(d, home, tell, gate, "dropped: "+why)
}

func closeLeaseGate(d *sql.DB, home string, tell Teller, gate int64, words string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	done, err := MarkDone(tx, home, db.Gate, DoneIn{Task: gate, Words: words})
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if tell != nil {
		tell.Done(gate, db.Gate, done.Owner, done.Told, done.Finished)
	}
	return nil
}
