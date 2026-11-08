package taskwrites

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// task.turn.push and task.turn.pop (docs/design/turns.md): the agent acting on a task, or the founder, puts another on top of it; the one
// on top, or the founder, takes its turn off with a pass or a send-back, and the task goes back to the one below.

// turnPushMethod is task.turn.push.
func turnPushMethod(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		to, err := text(p, "to")
		if err != nil {
			return nil, err
		}
		ask, err := optText(p, "ask")
		if err != nil {
			return nil, err
		}
		onPass, err := optText(p, "on_pass")
		if err != nil {
			return nil, err
		}
		evidence, err := optText(p, "evidence")
		if err != nil {
			return nil, err
		}
		if onPass != "" && onPass != db.TurnOnPassDone {
			return nil, badParam(`on_pass is "done" or absent`, "on_pass")
		}
		if (onPass == "") != (evidence == "") {
			return nil, badParam(`evidence comes with on_pass "done", and only with it`, "evidence")
		}
		agents := paths.ListAgents(home)
		if to != human && !contains(agents, to) {
			e := rpc.NotFoundError("no agent named " + to)
			e.Data = map[string]any{"what": "agent", "name": to}
			return nil, e
		}
		me := c.Caller.Actor()
		var h Handover
		var asked *Asked
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			var err error
			if h, asked, err = pushTurn(tx, tell, tid, me, to, ask, onPass, evidence); err != nil {
				return nil, err
			}
			if !asked.OK {
				return nil, &rpc.CommitThen{Err: refusal(asked.Msg)}
			}
			if asked.Reask {
				return accepted(fmt.Sprintf("T%d: %s", tid, asked.Msg), nil), nil
			}
			return accepted(fmt.Sprintf("T%d is %s's turn now: %s", tid, to, asked.Msg), nil), nil
		})
		if asked != nil && !asked.OK && asked.After != nil {
			asked.After()
		}
		if err != nil || asked.Reask {
			return out, err
		}
		h.After = asked.After
		if tail := tell.HandedOver(h); tail != "" {
			out["said"] = out["said"].(string) + "\n" + tail
		}
		return out, nil
	}
}

// optText is an optional text param, trimmed: "" when absent.
func optText(p map[string]any, key string) (string, error) {
	v, ok := p[key]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", badParam(key+" is text", key)
	}
	return strings.TrimSpace(textsafe.Text(s)), nil
}

// pushTurn puts `to` on top of task tid in tx, by the agent acting on it or the founder, and delivers the task to `to` (the founder is not
// delivered to: the task is in front of them as theirs to act on).
func pushTurn(tx *sql.Tx, tell Teller, tid int64, by, to, ask, onPass, evidence string) (Handover, *Asked, error) {
	t, err := getTask(tx, tid)
	if err != nil {
		return Handover{}, nil, err
	}
	turns, err := db.Turns(tx, tid)
	if err != nil {
		return Handover{}, nil, err
	}
	if to == human && ask != "" && turnRefusal(t, tid, human) == "" && t.Acting == human && len(turns) > 0 && by == below(turns, t.Owner) {
		// the founder's turn is their one question slot on the task: asking again rewrites it
		changed, err := db.ReaskTopTurn(tx, tid, by, ask)
		if err != nil || !changed {
			return Handover{}, &Asked{OK: true, Msg: "the founder's question is unchanged", Reask: true}, err
		}
		return Handover{}, &Asked{OK: true, Msg: "the founder's question now reads: " + ask, Reask: true}, nil
	}
	if why := turnRefusal(t, tid, by); why != "" {
		return Handover{}, nil, refusal(why)
	}
	if in, err := db.InStack(tx, tid, to); err != nil {
		return Handover{}, nil, err
	} else if in {
		return Handover{}, nil, refusal(fmt.Sprintf("%s already acts on T%d or has a turn below", to, tid))
	}
	if len(turns) >= db.MaxTurns {
		return Handover{}, nil, refusal(fmt.Sprintf("T%d carries %d turns, the most it may: pop one first", tid, db.MaxTurns))
	}
	h := Handover{Task: tid, By: by, From: t.Acting, To: to, Title: t.Title}
	push := func() error {
		return db.PushTurn(tx, tid, db.Turn{Agent: to, Ask: ask, By: by, Since: time.Now().Unix(), OnPass: onPass, Evidence: evidence})
	}
	if to == human {
		return h, &Asked{OK: true, Msg: "it waits on the founder"}, shiftTask(tx, tell, &h, "the founder's turn", true, push)
	}
	if tell == nil {
		return Handover{}, nil, fmt.Errorf("task.turn.push needs the supervisor to route its ask")
	}
	body := fmt.Sprintf("T%d is your turn, from %s: %s", tid, by, t.Title)
	if ask != "" {
		body += "\n\n" + ask
	}
	asked, err := tell.TaskAsk(tx, AskReq{From: by, To: to, Kind: "task", Body: body, Within: func(rid int64) error {
		h.RID = rid
		return shiftTask(tx, tell, &h, "turn to "+to, true, push)
	}})
	return h, asked, err
}

// Asker is who may put the founder's question on open task tid, or rewrite it: the agent acting on it, or the one under the founder's turn.
// "" for none (no such task, or it is over).
func Asker(tx *sql.Tx, tid int64) (string, error) {
	t, err := getTask(tx, tid)
	if err != nil || t == nil || t.done() || t.DiscardedBy.Valid && t.DiscardedBy.String != "" {
		return "", err
	}
	if t.Acting != human {
		return t.Acting, nil
	}
	turns, err := db.Turns(tx, tid)
	if err != nil || len(turns) == 0 {
		return "", err // the founder owns it: nobody asks them
	}
	return below(turns, t.Owner), nil
}

// PutQuestion is the founder's turn on task tid with ask, by its Asker: put on, or its question rewritten when it is already theirs. The
// caller runs tell.HandedOver on the Handover after the commit unless Asked.Reask.
func PutQuestion(tx *sql.Tx, tell Teller, tid int64, by, ask string) (Handover, *Asked, error) {
	return pushTurn(tx, tell, tid, by, human, ask, "", "")
}

// below is who acts on a task once its top turn comes off: the turn under it, else its owner.
func below(turns []db.Turn, owner string) string {
	if len(turns) > 1 {
		return turns[len(turns)-2].Agent
	}
	return owner
}

// turnRefusal is why by may not push or pop on task t now, or "": the task is open and by acts on it or is the founder.
func turnRefusal(t *task, tid int64, by string) string {
	switch {
	case t == nil:
		return fmt.Sprintf("no task T%d", tid)
	case t.done() || t.DiscardedBy.Valid && t.DiscardedBy.String != "":
		return fmt.Sprintf("task T%d is already finished", tid)
	case by != human && by != t.Acting:
		return fmt.Sprintf("T%d is %s's turn: only it, or the founder, moves its turns", tid, t.Acting)
	}
	return ""
}

// Popped is a pop that committed: the move back to the agent below, or the done a passing review closed the task with.
type Popped struct {
	Handover
	Asked *Asked
	Done  *DoneResult
}

// turnPopMethod is task.turn.pop.
func turnPopMethod(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		outcome, err := text(p, "outcome")
		if err != nil {
			return nil, err
		}
		note, err := optText(p, "note")
		if err != nil {
			return nil, err
		}
		if outcome != db.TurnPass && outcome != db.TurnSendBack {
			return nil, badParam(`outcome is "pass" or "send_back"`, "outcome")
		}
		me := c.Caller.Actor()
		if outcome == db.TurnSendBack && note == "" && me != human {
			return nil, badParam("a send-back says why in note", "note")
		}
		var popped Popped
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			var err error
			if popped, err = popTurn(tx, home, tell, tid, me, outcome, note, 0); err != nil {
				return nil, err
			}
			if popped.Asked != nil && !popped.Asked.OK {
				return nil, &rpc.CommitThen{Err: refusal(popped.Asked.Msg)}
			}
			return accepted(popped.said(), nil), nil
		})
		popped.tell(tell) // a refused return runs its After
		if err != nil {
			return nil, err
		}
		return out, nil
	}
}

func (p Popped) said() string {
	if p.Done != nil {
		return p.Done.Said
	}
	return fmt.Sprintf("T%d is %s's turn again", p.Task, p.To)
}

// tell is what a committed pop leaves: the task's done told, or the agent below moved on to it.
func (p Popped) tell(tell Teller) {
	if p.Done != nil {
		tell.Done(p.Task, p.By, p.Done.Owner, p.Done.Told, p.Done.Finished)
		return
	}
	if p.Asked != nil {
		p.After = p.Asked.After
		if !p.Asked.OK {
			if p.After != nil {
				p.After()
			}
			return
		}
	}
	tell.HandedOver(p.Handover)
}

// popTurn takes the top turn off task tid in tx: a pass or a send-back (with a note) by the agent whose turn it is or the founder. The task
// goes back to the one below, at the front of its queue, with the note as a comment: memo, the founder's comment the note already is, or
// one the pop writes (0). That return is the comment's tell to the one below. A pass of a turn pushed with on_pass "done" closes the task
// instead, by that turn's agent through the done gates; when a gate refuses, the turn still comes off with the refusal added to its note.
// Popped.Asked not OK is a refused return: nothing changed.
func popTurn(tx *sql.Tx, home string, tell Teller, tid int64, by, outcome, note string, memo int64) (Popped, error) {
	t, err := getTask(tx, tid)
	if err != nil {
		return Popped{}, err
	}
	if why := turnRefusal(t, tid, by); why != "" {
		return Popped{}, refusal(why)
	}
	turns, err := db.Turns(tx, tid)
	if err != nil {
		return Popped{}, err
	}
	if len(turns) == 0 {
		return Popped{}, refusal(fmt.Sprintf("T%d has no turn on it: %s acts on it as its owner", tid, t.Owner))
	}
	top := turns[len(turns)-1]
	below := below(turns, t.Owner)
	h := Handover{Task: tid, By: by, From: top.Agent, To: below, Title: t.Title}
	if outcome == db.TurnPass && top.OnPass == db.TurnOnPassDone {
		res, err := MarkDone(tx, home, top.Agent, DoneIn{Task: tid, Words: top.Evidence, TurnPassed: true})
		var refused *rpc.Error
		switch {
		case err == nil:
			return Popped{Handover: h, Done: res}, nil
		case errors.As(err, &refused) && refused.Code == "refused":
			note = strings.TrimSpace(note + " (not closed: " + refused.Message + ")")
			memo = 0 // the refusal is news to the one below
		default:
			return Popped{}, err
		}
	}
	pop := func() error {
		if _, _, _, err := db.PopTurn(tx, tid, by, outcome, note); err != nil {
			return err
		}
		if note != "" && memo == 0 {
			if memo, err = db.AddMemo(tx, tid, by, note); err != nil {
				return err
			}
		}
		if memo == 0 || h.RID == 0 {
			return nil
		}
		return db.LinkMemoTell(tx, memo, below, h.RID) // the note reaches the one below on the task's return: its tell
	}
	word := "passed"
	if outcome == db.TurnSendBack {
		word = "sent back"
	}
	if below == human {
		return Popped{Handover: h, Asked: &Asked{OK: true}}, shiftTask(tx, tell, &h, "turn "+word, false, pop)
	}
	if tell == nil {
		return Popped{}, fmt.Errorf("task.turn.pop needs the supervisor to route its ask")
	}
	body := fmt.Sprintf("T%d is your turn again: %s %s it: %s", tid, top.Agent, word, t.Title)
	if note != "" {
		body += "\n\n" + note
	}
	// System: the task goes back to an agent that already had it, so no team edge is asked for
	asked, err := tell.TaskAsk(tx, AskReq{From: by, To: below, Kind: "task", System: true, Body: body, Within: func(rid int64) error {
		h.RID = rid
		return shiftTask(tx, tell, &h, "turn "+word, false, pop)
	}})
	return Popped{Handover: h, Asked: asked}, err
}

// clearStack empties task tid's turns as an end, a draft or a reopen does, and answers the agents in it, to be told.
func clearStack(tx *sql.Tx, tid int64, by, top string) ([]string, error) {
	turns, err := db.ClearTurns(tx, tid, by, top)
	var who []string
	for _, t := range turns {
		who = append(who, t.Agent)
	}
	return who, err
}

// RetireTurns takes off, in tx, every turn agent holds on an open task, as its retiring does: one on top is sent back to the agent below it
// with the note "retired" (that delivery waits in its queue for the tick), and one lower down, or any with no tell, just goes. It answers
// the tasks it changed.
func RetireTurns(tx *sql.Tx, home string, tell Teller, agent string) ([]int64, error) {
	ids, err := db.IDs(tx, "SELECT DISTINCT u.task_id FROM task_turns u JOIN tasks t ON t.id = u.task_id WHERE u.agent = ? AND "+
		"t.done_evidence IS NULL AND t.discarded_by IS NULL ORDER BY u.task_id", agent)
	if err != nil {
		return nil, err
	}
	for _, tid := range ids {
		acting, err := db.Acting(tx, tid)
		if err != nil {
			return nil, err
		}
		if acting == agent && tell != nil {
			var p Popped
			if p, err = popTurn(tx, home, tell, tid, agent, db.TurnSendBack, "retired", 0); err == nil && p.Asked != nil && !p.Asked.OK {
				err = db.RemoveTurn(tx, tid, agent) // the way back refused: the turn goes all the same, its agent leaving
			}
		} else {
			err = db.RemoveTurn(tx, tid, agent)
		}
		if err != nil {
			return nil, err
		}
	}
	return ids, nil
}
