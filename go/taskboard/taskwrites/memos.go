// Package taskwrites serves the write methods of the task board (handline/daemon/rpc_tasks.py, rpc_taskverbs.py and the
// actions behind them): each makes its change and records its result in one transaction (rpc.Conn.Effect).
package taskwrites

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/handline/handline/go/kit/addr"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/attachments"
)

// CancelVerdict is what a request the founder cancels says it was closed under.
const CancelVerdict = "withdrawn by the founder"

// Register adds the task writes to the server's table.
// Teller is what follows a write once it has committed: the typed says and the deliveries the Python write did after its commit
// (the supervisor's telling layer). A nil Teller tells nothing.
type Teller interface {
	// Background runs fn after the caller returns, as one of the tellings the daemon waits on before it closes.
	Background(name string, fn func())
	// MemoChanged tells whom a memo concerns (the agents it names with @, only those; naming no one, the task's holder and who it answers
	// to) that it was added, edited or deleted (what), with the memo's text: at once when now, else a reader mid-turn hears at the end of its turn.
	// skip leaves those agents out of the telling (a comment's skip); the memo stays on the task.
	MemoChanged(tid, mid int64, by, what, text string, now bool, skip []string)
	// MemoRecipients is who MemoChanged would tell of a comment by `by` with this text, and why, without telling anyone. A missing task is nil, false.
	MemoRecipients(tid int64, by, text string, now bool) ([]Recipient, bool)
	// PassedPause has the agent take the task task.send_now let through at once.
	PassedPause(agent string)
	// QueueDelivered sends a queued task to its free owner, or raises the founder's card, and answers what happened.
	QueueDelivered(tid int64, by string, queued bool) string
	// Discarded types the says a discard wrote, moves the owner on to its next task, advances the owners it freed, and removes the inbox
	// files of the requests it closed.
	Discarded(owner string, told []db.Notice, freed []string)
	// Released stops the owner on the held requests a release ended, tells the requesters of the queued ones it closed (why is the reason on
	// them), moves the queues up, and answers a phrase for each ("#12 ended; w told to stop").
	Released(tid int64, why string, retired []Retired) []string
	// Cancelled is what the founder's cancel leaves before the discard's own telling: each request it closed told like any cancel, and the
	// owner typed a stop when it held the task without a delivery to stop it (stop).
	Cancelled(tid int64, by, owner string, stop bool, closed []Retired)
	// Done is what a done task leaves after the commit: the owner moves on to its next task, and
	// (finished) the tasks it was the last blocker of are released; the says it wrote are typed. finished is false when only a leftover request
	// of an already done task was settled.
	Done(tid int64, by, owner string, told []db.Notice, finished bool)
	// TaskAsk routes the request that delivers a task to its owner, in the transaction of the write that opens or moves it: the request row (a
	// refusal's denial row too, which stands), then within once the ask is allowed. The ask's telling waits for the commit
	// (Asked.After).
	TaskAsk(tx *sql.Tx, in AskReq) (*Asked, error)
	// HandoffRetire closes a request of the old owner's quietly (done, nobody told, the decision task it is the card of not settled), in the
	// transaction of the move. told, nil when the request was already closed, runs after the commit.
	HandoffRetire(tx *sql.Tx, rid int64, verdict string) (told func(), err error)
	// HandedOver is what a committed handoff leaves: the requests it closed told, the old owner moved on, the new owner typed the task. It
	// answers the lines the reply adds.
	HandedOver(h Handover) string
	// Edited is what a committed task.edit leaves: a changed body told to the holder of a doing task, a task that changed hands sent on to its new
	// owner, an owner whose last blocker cleared or whose current task got one moved on.
	Edited(ed Edited)
	// Notified delivers the notices a write left for the people a task's change concerns (tellTask).
	Notified(told []db.Notice)
	// ForTemps is whether a task by opens for agent now waits, queued and unsent, for a temp of it (agent mid-turn, or a temp of it at work),
	// and the words that say where it went; false when it is asked of agent at once.
	ForTemps(q db.Q, by, agent string) (hold bool, where string)
}

// Recipient is an agent a comment is told to, and why: asked (named with @), last_writer, assignee or owner (Teller.MemoRecipients).
type Recipient struct {
	Agent string `json:"agent"`
	Why   string `json:"why"`
}

func Register(s *rpc.Server, home string, tell Teller, decide CardDecider) {
	s.Register("task.discard", rpc.Method{Handler: discardMethod(home, tell)})
	s.Register("task.withdraw", rpc.Method{Handler: withdrawMethod(home, tell)})
	s.Register("task.queue", rpc.Method{Handler: queueMethod(home, tell, true)})
	s.Register("task.draft", rpc.Method{Handler: queueMethod(home, tell, false)})
	s.Register("task.progress", rpc.Method{Handler: progress})
	s.Register("progress.get", rpc.Method{Handler: progressGet})
	s.Register("task.begin", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) { return begin(c, p) }})
	s.Register("task.handoff", rpc.Method{Handler: handoffMethod(home, tell)})
	s.Register("task.turn.pop", rpc.Method{Handler: turnPopMethod(home, tell)})
	s.Register("task.new", rpc.Method{Handler: newMethod(home, tell)})
	s.Register("task.edit", rpc.Method{Handler: editMethod(home, tell)})
	s.Register("task.plan", rpc.Method{Handler: planMethod})
	s.Register("task.plan_ok", rpc.Method{Handler: planOKMethod})
	s.Register("task.ok", rpc.Method{Handler: okMethod(home, tell)})
	s.Register("task.redo", rpc.Method{Handler: redoMethod(home, tell)})
	s.Register("task.plan_ask", rpc.Method{Handler: planAskMethod})
	s.Register("task.core_ask", rpc.Method{Handler: coreAskMethod})
	s.Register("task.start", rpc.Method{Handler: start})
	s.Register("task.reopen", rpc.Method{Handler: reopenMethod(home, tell)})
	s.Register("task.done", rpc.Method{Handler: doneMethod(home, tell, decide)})
	s.Register("task.request", rpc.Method{Handler: requestMethod(home)})
	s.Register("task.release", rpc.Method{Handler: releaseMethod(home, tell)})
	s.Register("task.wait", rpc.Method{Handler: waitMethod(home, tell)})
	s.Register("task.merge", rpc.Method{Handler: mergeMethod(home, tell)})
	s.Register("task.comment.add", rpc.Method{Handler: memoAdd(home, tell)})
	s.Register("task.comment.edit", rpc.Method{Handler: memoEdit(home, tell)})
	s.Register("task.comment.delete", rpc.Method{Handler: memoDelete(tell)})
	s.Register("task.comment.recipients", rpc.Method{Handler: memoRecipients(tell)})
	s.Register("task.seen", rpc.Method{Handler: seenMethod})
	s.Register("task.send_now", rpc.Method{Handler: sendNowMethod(tell)})
}

func claimImages(tx *sql.Tx, home, by, text, was, field string) error {
	return attachments.New(home).ClaimText(tx, text, was, field, by != actor.OwnerKey)
}

func badParam(message, field string) *rpc.Error {
	return rpc.BadRequestError(message, field, rpc.InvalidParams)
}

func taskID(p map[string]any) (int64, error) {
	n, ok := rpc.AsInt(p["task"])
	if !ok {
		return 0, badParam("task is a task number", "task")
	}
	return n, nil
}

func memoID(p map[string]any) (int64, error) {
	n, ok := rpc.AsInt(p["memo"])
	if !ok {
		return 0, badParam("memo is a memo number", "memo")
	}
	return n, nil
}

func text(p map[string]any, key string) (string, error) {
	s, ok := p[key].(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", badParam(key+" is text and not empty", key)
	}
	return s, nil
}

func accepted(said string, more map[string]any) map[string]any {
	out := map[string]any{"accepted": true, "said": said}
	for k, v := range more {
		out[k] = v
	}
	return out
}

// refusal is an action's "no": a missing task is not_found, anything else refused.
func refusal(msg string) error {
	if strings.HasPrefix(msg, "no task T") {
		e := rpc.NotFoundError(msg)
		e.Data = map[string]any{"what": "task"}
		return e
	}
	return rpc.Refused(msg, "", "")
}

func hasTask(tx *sql.Tx, tid int64) (bool, error) {
	var n int
	err := tx.QueryRow("SELECT COUNT(*) FROM tasks WHERE id = ?", tid).Scan(&n)
	return n > 0, err
}

// memoOf is the memo `mid` of task `tid`, or not_found.
func memoOf(tx *sql.Tx, tid, mid int64) (author, body string, err error) {
	var of int64
	switch err = tx.QueryRow("SELECT task_id, author, body FROM task_memos WHERE id = ?", mid).Scan(&of, &author, &body); {
	case err == sql.ErrNoRows || (err == nil && of != tid):
		e := rpc.NotFoundError(fmt.Sprintf("T%d has no comment %d", tid, mid))
		e.Data = map[string]any{"what": "memo", "name": fmt.Sprint(mid)}
		return "", "", e
	}
	return author, body, err
}

// guard: a memo is anyone's to add; only its author or the founder edits or deletes it.
func guard(by, author string, mid int64) error {
	if by == actor.OwnerKey || by == author {
		return nil
	}
	return refusal(fmt.Sprintf("comment %d is %s's: only its author or the founder edits or deletes it", mid, author))
}

// The holder of a doing task is not told of a memo change here: the telling is a typed say, the delivery layer's.

func memoAdd(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		body, err := text(p, "text")
		if err != nil {
			return nil, err
		}
		now, err := optionalBool(p, "now")
		if err != nil {
			return nil, err
		}
		skip, err := optionalNames(p, "skip")
		if err != nil {
			return nil, err
		}
		reply, hasReply, err := optionalTask(p, "reply")
		if err != nil {
			return nil, err
		}
		paths, err := attachments.FilePaths(p)
		if err != nil {
			return nil, err
		}
		to, evidence, err := turnParams(p, home)
		if err != nil {
			return nil, err
		}
		store := attachments.New(home)
		files, err := store.AddFiles(c.Server().DB, home, c.Caller.Actor(), c.Caller.Role == "owner", paths) // read before the write: a video is large
		if err != nil {
			return nil, err
		}
		var added int64
		var popped *Popped // the founder's comment that ended the founder's turn
		var pushed Handover
		var asked *Asked // the turn the comment hands over, when it carries one
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			if ok, err := hasTask(tx, tid); err != nil || !ok {
				if err != nil {
					return nil, err
				}
				return nil, refusal(fmt.Sprintf("no task T%d", tid))
			}
			if body, err = c.MentionsOn(tid, body); err != nil {
				return nil, err
			}
			if err = turnPerson(tx, to); err != nil {
				return nil, err
			}
			if err := claimImages(tx, home, c.Caller.Actor(), body, "", "text"); err != nil {
				return nil, err
			}
			if hasReply {
				if _, _, err := memoOf(tx, tid, reply); err != nil {
					return nil, err
				}
			}
			mid, err := db.AddMemo(tx, tid, c.Caller.Actor(), body)
			if err != nil {
				return nil, err
			}
			added = mid
			if hasReply {
				if _, err := tx.Exec("UPDATE task_memos SET reply_to = ? WHERE id = ?", reply, mid); err != nil {
					return nil, err
				}
			}
			if me := c.Caller.Actor(); actor.IsPerson(me) {
				switch {
				case to == me:
					return nil, refusal("you do not hand a turn to yourself: name an agent")
				case actor.IsPerson(to):
					return nil, refusal("a person's comment hands no turn to a person: name an agent")
				}
				if err = answerMemoAsks(tx, tid, me); err != nil {
					return nil, err
				}
				// while the founder's turn is on top, their comment passes it, the comment its note; on a task Ready for them it is a
				// question to its assignee, and the turn stays for their verdict
				acting, err := db.Acting(tx, tid)
				if err != nil {
					return nil, err
				}
				turns, err := db.Turns(tx, tid)
				if err != nil {
					return nil, err
				}
				// a person answers their own turn; the owner answers any person's
				if actor.IsPerson(acting) && len(turns) > 0 && (acting == me || c.Caller.Role == "owner") {
					ready, err := readyNow(tx, tid)
					switch {
					case err != nil:
						return nil, err
					case ready && to != "":
						return nil, refusal(readyRefusal(tid))
					case to != "":
						return nil, refusal(fmt.Sprintf("T%d waits on you: answer with a comment and no turn, and it goes back to the agent that asked", tid))
					case !ready:
						p, err := popTurn(tx, home, tell, tid, c.Caller.Actor(), db.TurnPass, body, mid)
						if err != nil {
							return nil, err
						}
						popped = &p
					}
				}
			}
			if err := store.AttachMemo(tx, mid, files); err != nil {
				return nil, err
			}
			said := fmt.Sprintf("comment %d added to T%d", mid, tid)
			if to != "" {
				onPass := ""
				if evidence != "" {
					onPass = db.TurnOnPassDone
				}
				ask, drop := body, ""
				if to, ask, drop, err = seatTurn(tx, tid, c.Caller.Actor(), to, body); err != nil {
					return nil, err
				}
				if pushed, asked, err = pushTurn(tx, tell, tid, c.Caller.Actor(), to, ask, mid, onPass, evidence); err != nil {
					return nil, err
				}
				if !asked.OK { // the comment and the denied ask roll back together: a refused comment is not left on the task
					return nil, refusal(asked.Msg)
				}
				if drop != "" && !asked.Reask {
					if err := dropRelay(tx, tid, drop, to); err != nil {
						return nil, err
					}
				}
				if asked.Reask {
					said += ": " + asked.Msg
				} else {
					said += fmt.Sprintf(", and T%d is %s's turn now: %s", tid, addr.Ref(to), asked.Msg)
				}
			}
			return accepted(said, map[string]any{"memo": mid}), nil
		})
		if err == nil && asked != nil && !asked.Reask && tell != nil {
			pushed.After = asked.After
			if tail := tell.HandedOver(pushed); tail != "" {
				out["said"] = out["said"].(string) + "\n" + tail
			}
			if !actor.IsPerson(to) {
				skip = append(skip, to) // the turn's delivery carries the comment
			}
		}
		if err == nil && tell != nil {
			told := body
			if lines := store.FileLines(files); lines != "" {
				told += "\n" + lines
			}
			if popped != nil {
				popped.tell(tell)
				if popped.Done == nil && popped.Asked != nil && popped.Asked.OK {
					skip = append(skip, popped.To) // the task's return to it carries the comment
				}
			}
			tell.MemoChanged(tid, added, c.Caller.Actor(), "added", told, now, skip)
		}
		return out, err
	}
}

// turnPerson refuses a turn to a removed person; human was resolved to the task's person with the turn's other words.
func turnPerson(tx *sql.Tx, to string) error {
	if !actor.IsPerson(to) {
		return nil
	}
	var removed sql.NullInt64
	if err := tx.QueryRow("SELECT removed_at FROM people WHERE id = ?", to[1:]).Scan(&removed); errors.Is(err, sql.ErrNoRows) || removed.Valid {
		return refusal(to + " is no person here")
	} else if err != nil {
		return err
	}
	return nil
}

// turnParams are a comment's turn: who it hands the task to ("" for none) and, for a review, the evidence a pass of it closes the task with.
func turnParams(p map[string]any, home string) (to, evidence string, err error) {
	if to, err = optText(p, "turn"); err != nil {
		return "", "", err
	}
	if evidence, err = optText(p, "done"); err != nil {
		return "", "", err
	}
	switch {
	case evidence != "" && (to == "" || actor.IsPerson(to)):
		return "", "", badParam("done goes with a turn to an agent: a review whose pass closes the task", "done")
	case to != "" && !actor.IsPerson(to) && !contains(paths.ListAgents(home), to):
		e := rpc.NotFoundError("no agent named " + to)
		e.Data = map[string]any{"what": "agent", "name": to}
		return "", "", e
	}
	return to, evidence, nil
}

// optionalNames is a list of names (strings) in p[key], or none.
func optionalNames(p map[string]any, key string) ([]string, error) {
	if p[key] == nil {
		return nil, nil
	}
	list, ok := p[key].([]any)
	if !ok || len(list) > 20 {
		return nil, badParam(key+" is at most 20 names", key)
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		name, ok := v.(string)
		if !ok {
			return nil, badParam(key+" is a list of names", key)
		}
		out = append(out, name)
	}
	return out, nil
}

// memoRecipients is task.comment.recipients: who task.comment.add would tell, by the same rule (Teller.MemoRecipients), nothing written.
func memoRecipients(tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		body, err := text(p, "text")
		if err != nil {
			return nil, err
		}
		if body, err = c.MentionsOn(tid, body); err != nil { // as add would store it
			return nil, err
		}
		now, err := optionalBool(p, "now")
		if err != nil {
			return nil, err
		}
		out := []Recipient{}
		if tell != nil {
			got, found := tell.MemoRecipients(tid, c.Caller.Actor(), body, now)
			if !found {
				return nil, refusal(fmt.Sprintf("no task T%d", tid))
			}
			out = append(out, got...)
		}
		return map[string]any{"recipients": out}, nil
	}
}

func memoEdit(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		body, err := text(p, "text")
		if err != nil {
			return nil, err
		}
		mid, err := memoID(p)
		if err != nil {
			return nil, err
		}
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			if ok, err := hasTask(tx, tid); err != nil || !ok {
				if err != nil {
					return nil, err
				}
				return nil, refusal(fmt.Sprintf("no task T%d", tid))
			}
			author, was, err := memoOf(tx, tid, mid)
			if err != nil {
				return nil, err
			}
			if err := guard(c.Caller.Actor(), author, mid); err != nil {
				return nil, err
			}
			if body, err = c.MentionsOn(tid, body); err != nil {
				return nil, err
			}
			if body == was {
				return nil, refusal(fmt.Sprintf("comment %d already reads that way - nothing to edit", mid))
			}
			if err := claimImages(tx, home, c.Caller.Actor(), body, was, "text"); err != nil {
				return nil, err
			}
			if err := db.EditMemo(tx, mid, c.Caller.Actor(), body); err != nil {
				return nil, err
			}
			if err := syncMemoAsk(tx, mid, body); err != nil {
				return nil, err
			}
			return accepted(fmt.Sprintf("comment %d on T%d edited", mid, tid), nil), nil
		})
		if err == nil && tell != nil {
			tell.MemoChanged(tid, mid, c.Caller.Actor(), "edited", body, false, nil)
		}
		return out, err
	}
}

func memoDelete(tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		mid, err := memoID(p)
		if err != nil {
			return nil, err
		}
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			if ok, err := hasTask(tx, tid); err != nil || !ok {
				if err != nil {
					return nil, err
				}
				return nil, refusal(fmt.Sprintf("no task T%d", tid))
			}
			author, _, err := memoOf(tx, tid, mid)
			if err != nil {
				return nil, err
			}
			if err := guard(c.Caller.Actor(), author, mid); err != nil {
				return nil, err
			}
			var hung int
			if err := tx.QueryRow("SELECT COUNT(*) FROM task_turns WHERE memo = ?", mid).Scan(&hung); err != nil {
				return nil, err
			}
			if hung > 0 {
				return nil, refusal(fmt.Sprintf("comment %d is what a turn on T%d asks: edit it instead; the turn comes off with its answer", mid, tid))
			}
			if err := db.DeleteMemo(tx, mid, c.Caller.Actor()); err != nil {
				return nil, err
			}
			if _, err := db.CloseMemoAsks(tx, "withdrawn", "memo_id = ?", mid); err != nil {
				return nil, err
			}
			if err := attachments.UnlinkMemo(tx, mid); err != nil {
				return nil, err
			}
			return accepted(fmt.Sprintf("comment %d deleted from T%d", mid, tid), nil), nil
		})
		if err == nil && tell != nil {
			tell.MemoChanged(tid, mid, c.Caller.Actor(), "deleted", "", false, nil)
		}
		return out, err
	}
}
