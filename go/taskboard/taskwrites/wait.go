package taskwrites

import (
	"database/sql"
	"fmt"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/watch"
)

// waitMethod is task.wait {task, watch, time_box?, then?}: a gate on watch, the task blocked by it and, when doing, released, all in one
// transaction, so the task never waits on a gate that blocks nothing. It comes back to its holder's queue when the gate opens. A watch
// on a pull request also names it as the task's (Task.pr, Task.pr_repo), in the same write. With then "done" the gate closes the task
// when its watch passes (package gates), in place of giving it back.
func waitMethod(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		secs, err := timeBox(p, "time_box")
		if err != nil {
			return nil, err
		}
		w, err := parseWatch(p["watch"])
		if err != nil {
			return nil, err
		}
		if secs == 0 {
			secs = gateDeadline(home, w)
		}
		title := fmt.Sprintf("T%d waits on %s", tid, w.Label)
		if p["then"] != nil {
			if then, _ := p["then"].(string); then != "done" {
				return nil, badParam("then is done: the task is closed when the watch passes", "then")
			}
			if !watch.Lands(*w) {
				return nil, badParam("then done goes with a pull request waited on until landed: only its merge names the commit the task is closed with", "then")
			}
			w.Closes, title = tid, title+", then is done"
		}
		if why, err := waitRefusal(c.Server().DB, tid); err != nil || why != "" {
			if err == nil {
				err = refusal(why)
			}
			return nil, err
		}
		if err := watchReady(w); err != nil { // last: a vendor may read the outside to say it is set up
			return nil, err
		}
		me := c.Caller.Actor()
		var (
			r     released
			gate  int64
			freed bool
		)
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			r, freed = released{}, false
			var err error
			gate, _, err = OpenTask(tx, tell, me, NewIn{Owner: db.Gate, Title: title, Secs: secs, Prio: db.DefaultPriority, Expects: "report", Watch: w})
			if err != nil {
				return nil, err
			}
			said := fmt.Sprintf("T%d waits on gate T%d (watching %s)", tid, gate, w.Label)
			if w.Closes != 0 {
				said = fmt.Sprintf("T%d waits on gate T%d (watching %s; a merge closes T%d with its commit)", tid, gate, w.Label, tid)
			}
			t, err := getTask(tx, tid)
			if err != nil {
				return nil, err
			}
			// Read before the gate blocks it: a blocked task never reads doing.
			if st, err := state(tx, t); err != nil {
				return nil, err
			} else if st == "doing" {
				if r, err = releaseTask(tx, home, me, tid, fmt.Sprintf("waits on gate T%d", gate)); err != nil {
					return nil, err
				}
				freed = true
				said += "; " + r.said()
			}
			if why, err := recordBlock(tx, tid, gate, me); err != nil || why != "" {
				if err == nil {
					err = refusal(why)
				}
				return nil, err
			}
			// A pull request the task waits on is the task's pull request: named here, by the one who knows, never guessed from a branch.
			if repo, pr, ok := watch.PullOf(*w); ok {
				if _, _, err := linkPR(tx, t, me, pr, repo); err != nil {
					return nil, err
				}
			}
			return accepted(said, map[string]any{"gate": gate}), nil
		})
		if err == nil && tell != nil && freed {
			r.tell(tell, tid)
		}
		return out, err
	}
}

// gateDeadline is a gate's time box when none is given: a day, or for a pull request waited on until it lands the founder's
// landed_deadline_s (a week unless set), since a review can take days and a gate that timed out daily would bring its task back each
// time to no use.
func gateDeadline(home string, w *db.Watch) int64 {
	if watch.Lands(*w) {
		return team.LandedDeadline(home)
	}
	return db.GateDeadlineS
}

// waitRefusal is why task tid cannot wait, "" when it can: it must be there and open.
func waitRefusal(sdb *sql.DB, tid int64) (string, error) {
	tx, err := sdb.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	t, err := getTask(tx, tid)
	switch {
	case err != nil:
		return "", err
	case t == nil:
		return fmt.Sprintf("no task T%d", tid), nil
	case t.ended():
		return fmt.Sprintf("task T%d is %s", tid, t.endWord()), nil
	}
	return "", nil
}
