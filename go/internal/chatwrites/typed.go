package chatwrites

import (
	"fmt"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/outcome"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// typeWait is how long a person's message call waits for its message to be typed; 0 is outcome.Limit. A test shortens it.
var typeWait time.Duration

// typesNow is whether the delivery types a message as soon as it is written: only once its ports are wired (supervisor.Ports.Wired);
// before that the telling is left to the tick, and the call is not held. A test sets it.
var typesNow = func(e *supervisor.Env) bool { return e.Wired() }

// awaitTyped holds a person's chat.send, chat.send_now or inbox.reply to a terminal agent, which takes the person's words mid-turn too,
// until the delivery has typed the message (delivered_at), and answers out once it has. Past the limit, or when the caller hangs up, a
// message still untyped is taken back (withdrawn) and the call is refused, so nothing is typed later that the caller did not hear of.
// Not held: an agent's call (its message is queued until the target takes it), and a message to a structured agent, whose receipt is
// already the outcome: written, or queued behind the turn running there (a queued bubble that send_now and chat.cancel act on).
func (w *writes) awaitTyped(c *rpc.Conn, out any, err error) (any, error) {
	m, isMap := out.(map[string]any)
	if err != nil || !isMap || !c.Caller.Allows("member") || !typesNow(w.e) {
		return out, err
	}
	rid, _ := m["request"].(int64)
	if rid == 0 {
		return out, nil
	}
	if first, err := db.GetRequest(w.e.DB, rid); err != nil || first == nil || w.e.StructuredAgent(first.Target) {
		return out, err
	}
	var row *db.Request
	typed := func() (bool, error) {
		var err error
		row, err = db.GetRequest(w.e.DB, rid)
		return err != nil || row == nil || row.DeliveredAt.Valid || row.State != "queued" || actor.IsPerson(row.Target), err
	}
	ended, err := outcome.Wait(typeWait, c.Done(), typed)
	if err != nil {
		return nil, err
	}
	if !ended {
		if err := w.withdrawHeld(rid); err != nil {
			return nil, err
		}
		took, err := db.RetireIfUndelivered(w.e.DB, rid, "withdrawn", "withdrawn: not typed while the sender waited")
		if err != nil {
			return nil, err
		}
		if took {
			return nil, rpc.Refused(fmt.Sprintf("message %d was not typed into %s's session in time, so it was taken back; send it again", rid, row.Target), "state", "")
		}
		if _, err := typed(); err != nil { // typed in the gap: it went
			return nil, err
		}
	}
	if row == nil {
		return out, nil
	}
	if !row.DeliveredAt.Valid && !actor.IsPerson(row.Target) {
		return nil, rpc.Refused(fmt.Sprintf("message %d was closed (%s) before it was typed", rid, row.State), "state", "")
	}
	return m, nil
}
