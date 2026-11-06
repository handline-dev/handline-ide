package taskwrites

import (
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
)

// sendNow is task.send_now: while new work is paused, one task the pause holds (db.PauseHeldTasks) goes to its agent as one the founder
// sent. Its delivery passes the pause (db.PausePassed), goes to the front of the queue and is told at the next tick; the rest stay held.
func sendNow(c *rpc.Conn, p map[string]any) (any, error) {
	tid, err := taskID(p)
	if err != nil {
		return nil, err
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		t, err := db.GetTask(tx, tid)
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, rpc.NotFoundError(fmt.Sprintf("no task T%d", tid))
		}
		now := time.Now()
		if pz, err := db.GetPause(tx, now); err != nil || pz == nil {
			if err != nil {
				return nil, err
			}
			return nil, refusal(fmt.Sprintf("new work is not paused: nothing holds T%d", tid))
		}
		held, err := db.PauseHeldTasks(tx, t.Owner, now)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(held, tid) {
			return nil, refusal(fmt.Sprintf("the pause does not hold T%d: it is not queued for its agent, or is told already", tid))
		}
		r, err := db.LiveDelivery(tx, tid, t.Owner)
		if err != nil {
			return nil, err
		}
		if r == nil { // PauseHeldTasks found it through this delivery
			return nil, refusal(fmt.Sprintf("T%d has no delivery to send", tid))
		}
		if err := db.PromoteToFront(tx, r.ID); err != nil {
			return nil, err
		}
		if _, err := db.MarkSendNow(tx, r.ID); err != nil {
			return nil, err
		}
		if err := addEvent(tx, t.Owner, db.PausePassed, kv{"request_id", r.ID}, kv{"task_id", tid}); err != nil {
			return nil, err
		}
		return accepted(fmt.Sprintf("T%d goes to %s now; the pause holds the rest", tid, t.Owner), nil), nil
	})
}
