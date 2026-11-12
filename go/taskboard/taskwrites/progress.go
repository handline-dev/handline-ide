package taskwrites

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// progressMax is handoff.PROGRESS_MAX: the note is typed into a fresh session, so it stays short.
const progressMax = 1500

func taskProgressKey(tid int64) string    { return fmt.Sprintf("progress-task:%d", tid) }
func requestProgressKey(rid int64) string { return fmt.Sprintf("progress:%d", rid) }

// setProgress is handoff.set_progress: the note goes on the agent's delivered request (its effective head), else on the task it holds,
// else on the agent. An empty note clears it. It answers where it was kept.
func setProgress(tx *sql.Tx, agent, note string) (string, error) {
	head, err := db.EffectiveHead(tx, agent)
	if err != nil {
		return "", err
	}
	if head == nil || !head.DeliveredAt.Valid || head.DeliveredAt.Int64 == 0 {
		tid, held, err := db.CurrentTask(tx, agent)
		if err != nil {
			return "", err
		}
		if held {
			return fmt.Sprintf("T%d (no request in progress)", tid), db.SetMeta(tx, taskProgressKey(tid), note)
		}
		return "you (no request in progress)", db.SetMeta(tx, db.AgentProgressKey(agent), note)
	}
	return db.MsgName(tx, head.ID), db.SetMeta(tx, requestProgressKey(head.ID), note)
}

// progress is task.progress: the caller's note on what is done and what is left, kept where the handoff and the wake prompt read it.
func progress(c *rpc.Conn, p map[string]any) (any, error) {
	var tid int64
	named := p["task"] != nil
	if named {
		var err error
		if tid, err = taskID(p); err != nil {
			return nil, err
		}
	}
	note, ok := p["note"].(string)
	if !ok {
		return nil, badParam("note is text (empty clears it)", "note")
	}
	note = strings.TrimSpace(textsafe.Text(note))
	if utf8.RuneCountInString(note) > progressMax {
		return nil, badParam(fmt.Sprintf("note is at most %d characters: keep what is done and what is left", progressMax), "note")
	}
	todo, err := progressCount(p, note)
	if err != nil {
		return nil, err
	}
	me := c.Caller.Actor()
	if todo != nil {
		todo.By = me
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		// count is where the step count goes: the task named, else the one the caller holds; the note's own place may be a request.
		count := func(tid int64) error { return db.SetTaskTodo(tx, tid, todo) }
		kept := func(target int64) (map[string]any, error) {
			where, err := setProgress(tx, me, note)
			if err != nil {
				return nil, err
			}
			if target == 0 {
				if cur, held, err := db.CurrentTask(tx, me); err != nil {
					return nil, err
				} else if held {
					target = cur
				}
			}
			said := fmt.Sprintf("progress on %s kept; the handoff and the wake prompt carry it", where)
			if target != 0 {
				if err := count(target); err != nil {
					return nil, err
				}
			} else if todo != nil {
				said += " (no task held, so no step count)"
			}
			return accepted(said, nil), nil
		}
		if !named { // what `hl progress` does: the note goes where the caller's own state says, task held or not
			return kept(0)
		}
		t, err := getTask(tx, tid)
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, refusal(fmt.Sprintf("no task T%d", tid))
		}
		if t.Acting != me {
			return nil, rpc.Refused(fmt.Sprintf("task T%d is %s's: progress is kept by whoever holds it", tid, t.Acting), "state", "")
		}
		if cur, held, err := db.CurrentTask(tx, me); err != nil {
			return nil, err
		} else if !held || cur == tid {
			return kept(tid) // the same place `hl progress` keeps the note
		}
		if err := db.SetMeta(tx, taskProgressKey(tid), note); err != nil {
			return nil, err
		}
		if err := count(tid); err != nil {
			return nil, err
		}
		return accepted(fmt.Sprintf("progress on T%d kept; the handoff and the wake prompt carry it", tid), nil), nil
	})
}

// progressCount is the step count given beside the note: done and total both or neither, whole numbers with 0 <= done <= total and
// total >= 1; nil for none, and for an empty note, which clears the count with the note.
func progressCount(p map[string]any, note string) (*db.TaskTodo, error) {
	if p["done"] == nil && p["total"] == nil {
		return nil, nil
	}
	done, okDone := rpc.AsInt(p["done"])
	total, okTotal := rpc.AsInt(p["total"])
	switch {
	case !okDone || done < 0:
		return nil, badParam("done is a whole number of steps, given with total", "done")
	case !okTotal || total < 1 || total < done:
		return nil, badParam("total is a whole number of steps, at least 1, given with done and not under it", "total")
	case note == "":
		return nil, nil
	}
	return &db.TaskTodo{Done: int(done), Total: int(total), Current: note}, nil
}

// progressGet is progress.get: the note kept on the caller's effective head, else on its current task, else on the agent.
func progressGet(c *rpc.Conn, _ map[string]any) (any, error) {
	if c.Caller.Role != "agent" {
		return nil, rpc.Refused("a progress note is an agent's own: only an agent has one", "", "")
	}
	me := c.Caller.Actor()
	q := c.Server().DB
	head, err := db.EffectiveHead(q, me)
	if err != nil {
		return nil, err
	}
	key := ""
	if head != nil && head.DeliveredAt.Valid && head.DeliveredAt.Int64 != 0 {
		key = requestProgressKey(head.ID)
	} else if tid, held, err := db.CurrentTask(q, me); err != nil {
		return nil, err
	} else if held {
		key = taskProgressKey(tid)
	} else {
		key = db.AgentProgressKey(me)
	}
	v, _, err := db.Meta(q, key)
	if err != nil {
		return nil, err
	}
	if v == "" {
		return map[string]any{"note": nil}, nil
	}
	return map[string]any{"note": v}, nil
}
