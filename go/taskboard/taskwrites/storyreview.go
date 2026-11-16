package taskwrites

import (
	"database/sql"
	"fmt"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// The founder's review of a story's sub-task: db.Review.

// readyFor is the land_by agent an agent's handoff of task tid to `to` is held back from, as a Ready for the founder; "" for a handoff
// that goes as asked. The land_by is the one of the task's owner's workspace, whoever hands it on, and only the agent working the task
// finishes it so. An agent's handoff of a task that is Ready now is refused: only the founder's verdict moves it on.
func readyFor(tx *sql.Tx, tid int64, by, to string) (string, error) {
	t, err := getTask(tx, tid)
	if err != nil || t == nil || actor.IsPerson(by) {
		return "", err // no task: handOff refuses it
	}
	if r, err := db.TaskReview(tx, tid); err != nil {
		return "", err
	} else if r != nil && r.Ready {
		return "", refusal(fmt.Sprintf("T%d is Ready for the founder: it waits for their OK, merge or Redo", tid))
	}
	if actor.IsPerson(to) {
		return "", nil
	}
	rules, _, err := team.AgentRules(tx, t.Owner)
	if err != nil {
		return "", err
	}
	landBy := false
	for _, r := range rules {
		landBy = landBy || r.Key == team.RuleLandBy && r.Value == to
	}
	if !landBy {
		return "", nil
	}
	if due, err := db.ReadyDue(tx, tid); err != nil || !due {
		return "", err
	}
	if t.Acting != by {
		return "", refusal(fmt.Sprintf("T%d is a story's sub-task: %s, who works it, finishes it, and it goes to the founder before %s",
			tid, addr.Ref(t.Acting), addr.Ref(to)))
	}
	return to, nil
}

// readyTask is task tid's review, refused unless the task is Ready now and by may answer the founder's turn on it.
func readyTask(tx *sql.Tx, tid int64, by string) (*db.Review, error) {
	t, err := openTask(tx, tid)
	if err != nil {
		return nil, err
	}
	r, err := db.TaskReview(tx, tid)
	if err != nil {
		return nil, err
	}
	if r == nil || !r.Ready {
		return nil, refusal(fmt.Sprintf("T%d is not Ready for you: only a story's sub-task its assignee finished waits for OK or Redo", tid))
	}
	if why := turnRefusal(t, tid, by); why != "" {
		return nil, refusal(why)
	}
	return r, nil
}

// okMethod is task.ok, "OK, merge": the founder's turn on a Ready task comes off, and the task goes to the land_by agent it waited for,
// with the assignee's result as the note, as the assignee's own handoff would have.
func okMethod(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		me := c.Caller.Actor()
		var (
			h     Handover
			asked *Asked
		)
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			r, err := readyTask(tx, tid, me)
			if err != nil {
				return nil, err
			}
			_, result, err := memoOf(tx, tid, r.Memo)
			if err != nil {
				return nil, err
			}
			if _, _, _, err := db.PopTurn(tx, tid, me, db.TurnPass, "", 0, 0); err != nil {
				return nil, err
			}
			if err := db.AddVerdict(tx, tid, me, db.VerdictOK, 0); err != nil {
				return nil, err
			}
			if h, asked, err = handOff(tx, tell, tid, me, r.LandBy, "OK, merge (the founder). "+result, paths.ListAgents(home), false); err != nil {
				return nil, err
			}
			if !asked.OK { // all of it rolls back: the task stays Ready
				return nil, refusal(asked.Msg)
			}
			return accepted(fmt.Sprintf("T%d is OK: handed to %s to merge", tid, addr.Ref(r.LandBy)), nil), nil
		})
		if err != nil {
			return nil, err
		}
		h.After = asked.After
		tell.HandedOver(h)
		return out, nil
	}
}

// redoMethod is task.redo: the founder sends a Ready task back to its assignee with a note, kept as their comment.
func redoMethod(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		note, err := optText(p, "note")
		if err != nil {
			return nil, err
		}
		if note == "" {
			return nil, badParam("a redo says what to change in note", "note")
		}
		me := c.Caller.Actor()
		var popped Popped
		var memo int64
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			r, err := readyTask(tx, tid, me)
			if err != nil {
				return nil, err
			}
			if err := claimImages(tx, home, me, note, "", "note"); err != nil {
				return nil, err
			}
			if memo, err = db.AddMemo(tx, tid, me, note); err != nil {
				return nil, err
			}
			if err := db.AddVerdict(tx, tid, me, db.VerdictRedo, memo); err != nil {
				return nil, err
			}
			told := fmt.Sprintf("T%d 대표님 Redo %d (댓글 %d): %s\n\nWork it again on your branch, then hand it to %s as before: it comes to the founder first again.",
				tid, r.Redo+1, memo, note, addr.Ref(r.LandBy))
			if popped, err = popTurnTold(tx, home, tell, tid, me, db.TurnSendBack, note, memo, told); err != nil {
				return nil, err
			}
			if popped.Asked != nil && !popped.Asked.OK { // all of it rolls back: the task stays Ready
				return nil, refusal(popped.Asked.Msg)
			}
			return accepted(fmt.Sprintf("T%d sent back to %s: Redo %d", tid, addr.Ref(popped.To), r.Redo+1), map[string]any{"memo": memo}), nil
		})
		if err != nil {
			return nil, err
		}
		popped.tell(tell)
		tell.MemoChanged(tid, memo, me, "added", note, false, []string{popped.To}) // the task's return carries the note
		return out, nil
	}
}
