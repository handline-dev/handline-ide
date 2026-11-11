package taskwrites

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// Asked is the answer to a handoff's ask. A refusal has OK false and its writes (a denied row) stand. After runs once the transaction has
// committed (the telling) and answers a line for the reply.
type Asked struct {
	OK    bool
	Msg   string
	RID   int64
	After func() string
	// Reask is a push that rewrote the founder's open question instead of putting a turn on: nothing moved.
	Reask bool
}

// AskReq is a task's delivery to be routed. Kind "" is the routing edge's own; NoCard keeps a request to the founder from raising a decision
// card (task.new opens its own task); Notify types the target after the commit, which a handoff leaves to HandedOver.
type AskReq struct {
	From, To, Body, Kind string
	NoCard, Notify       bool
	// System is the daemon's own step, a start-up migration: no team edge is checked. Only code in the daemon sets it, never a caller.
	System bool
	Within func(rid int64) error
}

// Handover is a handoff that committed.
type Handover struct {
	Task         int64
	By, From, To string
	Title        string
	RID          int64
	WasCurrent   bool // the old owner held it as its current task
	Retired      []func()
	After        func() string
}

// handoffRefusal is why the task may not go to `to`, or "": anyone hands an open task on, with a note (its asker takes it back so).
func handoffRefusal(t *task, to string, agents []string) string {
	switch {
	case t.done() || t.DiscardedBy.Valid && t.DiscardedBy.String != "":
		return fmt.Sprintf("task T%d is already finished - nothing to hand over", t.ID)
	case to != actor.OwnerKey && !contains(agents, to):
		return "no agent named " + to
	case to == t.Acting:
		return fmt.Sprintf("T%d is already %s's", t.ID, to)
	case to == t.Owner:
		return fmt.Sprintf("T%d is %s's already, under its turns: pass or send back the turns above it (`hl task pass` or `hl task back`)", t.ID, to)
	}
	return ""
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// handoffMethod is task.handoff (actions.task_handoff): anyone passes a task along with a note, the history keeping who. The ask to the new owner, the move and the
// record of the result are one transaction, so a crash between them leaves nothing half done and a retry needs no intent to find its way.
func handoffMethod(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		note, ok := p["note"].(string)
		if p["note"] != nil && !ok {
			return nil, badParam("note is text", "note")
		}
		to, err := text(p, "to")
		if err != nil {
			return nil, err
		}
		agents := paths.ListAgents(home)
		if to != actor.OwnerKey && !contains(agents, to) {
			e := rpc.NotFoundError("no agent named " + to)
			e.Data = map[string]any{"what": "agent", "name": to}
			return nil, e
		}
		me := c.Caller.Actor()
		said := strings.TrimSpace(textsafe.Text(note))
		toFounder := actor.IsPerson(to) && me != actor.OwnerKey // an agent's handoff to the founder is the founder's turn, hung on the note as a comment
		if toFounder && said == "" {
			return nil, badParam("a handoff to the founder says what they decide, and what you recommend", "note")
		}
		var (
			h     Handover
			asked *Asked
			msg   string
			memo  int64
		)
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			if err := claimImages(tx, home, me, said, "", "note"); err != nil {
				return nil, err
			}
			var err error
			if toFounder { // the task stays the agent's
				if ok, err := hasTask(tx, tid); err != nil || !ok {
					if err != nil {
						return nil, err
					}
					return nil, refusal(fmt.Sprintf("no task T%d", tid))
				}
				if memo, err = db.AddMemo(tx, tid, me, said); err != nil {
					return nil, err
				}
				h, asked, err = pushTurn(tx, tell, tid, me, actor.OwnerKey, said, memo, "", "")
			} else {
				h, asked, err = handOff(tx, tell, tid, me, to, said, agents, false)
			}
			if err != nil {
				return nil, err
			}
			if !asked.OK {
				return nil, &rpc.CommitThen{Err: refusal(asked.Msg)}
			}
			msg = asked.Msg
			if asked.Reask { // already on the founder's turn: it moved to this note, nothing is handed over
				return accepted(fmt.Sprintf("T%d: %s", tid, msg), nil), nil
			}
			return accepted(fmt.Sprintf("T%d handed to %s: %s", tid, to, msg), nil), nil
		})
		if asked != nil && !asked.OK && asked.After != nil {
			asked.After() // the refusal's writes stood: what follows them still runs
		}
		if err != nil {
			return nil, err
		}
		if !asked.Reask {
			h.After = asked.After
			if tail := tell.HandedOver(h); tail != "" {
				out["said"] = fmt.Sprintf("T%d handed to %s: %s\n%s", tid, to, msg, tail)
			}
		}
		if memo != 0 {
			tell.MemoChanged(tid, memo, me, "added", said, false, nil)
		}
		return out, nil
	}
}

// handOff routes task tid to `to` in tx and, once the ask is allowed, moves it (moveTask). A refusal is the error; asked.OK false is a denied
// ask, whose writes stand. The caller runs asked.After and tell.HandedOver after the commit.
func handOff(tx *sql.Tx, tell Teller, tid int64, by, to, note string, agents []string, system bool) (Handover, *Asked, error) {
	t, err := getTask(tx, tid)
	if err != nil {
		return Handover{}, nil, err
	}
	if t == nil {
		return Handover{}, nil, refusal(fmt.Sprintf("no task T%d", tid))
	}
	if why := handoffRefusal(t, to, agents); why != "" {
		return Handover{}, nil, refusal(why)
	}
	if in, err := db.InStack(tx, tid, to); err != nil {
		return Handover{}, nil, err
	} else if in {
		return Handover{}, nil, refusal(fmt.Sprintf("%s already has a turn on T%d", to, tid))
	}
	if tell == nil {
		return Handover{}, nil, fmt.Errorf("task.handoff needs the supervisor to route its ask")
	}
	h := Handover{Task: tid, By: by, From: t.Acting, To: to, Title: t.Title}
	asked, err := tell.TaskAsk(tx, AskReq{From: by, To: to, Kind: "task", System: system, Body: fmt.Sprintf("T%d handed to you by %s: %s\n\n%s", tid, by, t.Title, note),
		Within: func(rid int64) error {
			h.RID = rid
			return moveTask(tx, tell, &h, note)
		}})
	return h, asked, err
}

// moveTask is what follows an allowed ask, in its transaction: the old owner's open requests close, the task changes hands (or, with turns
// on it, its top turn does) and links the delivery.
func moveTask(tx *sql.Tx, tell Teller, h *Handover, note string) error {
	return shiftTask(tx, tell, h, "handed to "+h.To, true, func() error {
		replaced, err := db.ReplaceTopTurn(tx, h.Task, h.To)
		if err != nil {
			return err
		}
		if !replaced {
			if err := db.EditTask(tx, h.Task, h.By, db.TaskEdit{Owner: &h.To}); err != nil {
				return err
			}
			went, err := db.SyncSelfBlocks(tx, h.Task, h.From)
			if err != nil {
				return err
			}
			if err := reportDormant(tx, h.By, h.To, went); err != nil {
				return err
			}
		}
		at := float64(time.Now().UnixNano()) / 1e9
		return addEvent(tx, h.By, "task_handoff", kv{"task_id", h.Task}, kv{"from", h.From}, kv{"to", h.To}, kv{"note", note},
			kv{"request_id", h.RID}, kv{"at", at})
	})
}

// shiftTask moves who acts on task h.Task from h.From to h.To in tx, change being the write that does it (a new owner, a turn pushed or
// popped): h.From's open requests for it close (why) and its slot frees, and h.To gets the task as its delivery h.RID (none for the
// founder), typed at once (now) or at the front of its queue.
func shiftTask(tx *sql.Tx, tell Teller, h *Handover, why string, now bool, change func() error) error {
	reqs, err := taskRequests(tx, h.Task)
	if err != nil {
		return err
	}
	for _, r := range reqs {
		if r.State == "queued" && r.Target == h.From && r.ID != h.RID {
			told, err := tell.HandoffRetire(tx, r.ID, why)
			if err != nil {
				return err
			}
			if told != nil {
				h.Retired = append(h.Retired, told)
			}
		}
	}
	if err := db.EndHeld(tx, h.Task, h.From); err != nil {
		return err
	}
	if err := change(); err != nil {
		return err
	}
	if h.RID != 0 {
		if err := db.LinkRequest(tx, h.Task, h.RID); err != nil {
			return err
		}
		if now {
			if _, err := db.MarkSendNow(tx, h.RID); err != nil { // typed on arrival, whatever its holder is doing
				return err
			}
		} else if err := db.PromoteToFront(tx, h.RID); err != nil {
			return err
		}
	}
	if cur, held, err := db.CurrentTask(tx, h.From); err != nil {
		return err
	} else if held && cur == h.Task {
		h.WasCurrent = true
		if err := db.SetCurrentTask(tx, h.From, 0, false); err != nil {
			return err
		}
	}
	if actor.IsPerson(h.To) { // a task the founder acts on is asked of them, so it is queued: a draft is outside "for you"
		_, err := db.SetTaskQueued(tx, h.Task, h.By)
		return err
	}
	if _, held, err := db.CurrentTask(tx, h.To); err != nil {
		return err
	} else if !held {
		return db.SetCurrentTask(tx, h.To, h.Task, true)
	}
	return nil
}

// HandOffAsDaemon hands task tid to `to` on the daemon's own account, with the note: the timebox's step when a holder leaves a task unchanged
// after it was asked to act.
func HandOffAsDaemon(d *sql.DB, home string, tell Teller, tid int64, to, note string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	h, asked, err := handOff(tx, tell, tid, "daemon", to, note, paths.ListAgents(home), true)
	if err == nil && !asked.OK {
		err = fmt.Errorf("handing T%d to %s was refused: %s", tid, to, asked.Msg)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	if asked.After != nil {
		h.After = asked.After
	}
	tell.HandedOver(h)
	return nil
}
