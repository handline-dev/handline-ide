package taskwrites

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// A task's plan (task.plan), the founder's OK of it (task.plan_ok), and the asks for a plan (task.plan_ask) or for the core logic
// (task.core_ask). The asks are only recorded here: the board sends each to its agent once that agent is done with what it was doing
// (supervisor.SweepTaskAsks).

// openTask is the task, refused when there is none or it has ended.
func openTask(tx *sql.Tx, tid int64) (*task, error) {
	t, err := getTask(tx, tid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, refusal(fmt.Sprintf("no task T%d", tid))
	}
	if t.done() || t.DiscardedBy.String != "" {
		return nil, refusal(fmt.Sprintf("T%d has ended", tid))
	}
	return t, nil
}

// agentTask is openTask for a task an agent works: a plan or core logic is its assignee's to write.
func agentTask(tx *sql.Tx, tid int64) (*task, error) {
	t, err := openTask(tx, tid)
	if err != nil {
		return nil, err
	}
	if actor.IsPerson(t.Owner) || t.Owner == db.Gate {
		return nil, refusal(fmt.Sprintf("T%d is not an agent's task: only its assignee writes its plan", tid))
	}
	return t, nil
}

// planMethod is task.plan: the assignee keeps a new version of the task's plan. A new version clears the founder's OK.
func planMethod(c *rpc.Conn, p map[string]any) (any, error) {
	tid, err := taskID(p)
	if err != nil {
		return nil, err
	}
	raw, err := text(p, "text")
	if err != nil {
		return nil, err
	}
	plan := strings.TrimSpace(textsafe.Text(raw))
	me := c.Caller.Actor()
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		t, err := agentTask(tx, tid)
		if err != nil {
			return nil, err
		}
		if me != t.Owner {
			return nil, refusal(fmt.Sprintf("T%d's plan is written by its assignee, %s", tid, addr.Ref(t.Owner)))
		}
		was, err := db.TaskPlan(tx, tid)
		if err != nil {
			return nil, err
		}
		if was != nil && was.Current().Text == plan {
			return nil, refusal(fmt.Sprintf("T%d's plan already reads so", tid))
		}
		if _, err := db.AddPlan(tx, tid, me, plan); err != nil {
			return nil, err
		}
		n := 1
		if was != nil {
			n += len(was.Versions)
		}
		return accepted(fmt.Sprintf("T%d's plan is kept (version %d)", tid, n), map[string]any{"version": n}), nil
	})
}

// planOKMethod is task.plan_ok: the founder marks the task's plan as it stands OK, or (ok false) takes that back.
func planOKMethod(c *rpc.Conn, p map[string]any) (any, error) {
	tid, err := taskID(p)
	if err != nil {
		return nil, err
	}
	ok := true
	if v, given := p["ok"]; given {
		b, isBool := v.(bool)
		if !isBool {
			return nil, badParam("ok is true or false", "ok")
		}
		ok = b
	}
	version, hasVersion := rpc.AsInt(p["version"])
	if p["version"] != nil && !hasVersion {
		return nil, badParam("version is a plan version number", "version")
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		if _, err := openTask(tx, tid); err != nil {
			return nil, err
		}
		plan, err := db.TaskPlan(tx, tid)
		if err != nil {
			return nil, err
		}
		if plan == nil {
			return nil, refusal(fmt.Sprintf("T%d has no plan yet", tid))
		}
		if hasVersion && version != int64(len(plan.Versions)) {
			return nil, refusal(fmt.Sprintf("T%d's plan is at version %d now, not %d: read it again", tid, len(plan.Versions), version))
		}
		if (plan.OK != nil) == ok {
			word := "OK"
			if !ok {
				word = "not OK"
			}
			return nil, refusal(fmt.Sprintf("T%d's plan is already %s", tid, word))
		}
		if err := db.MarkPlan(tx, tid, c.Caller.Actor(), plan.Current().ID, ok); err != nil {
			return nil, err
		}
		if ok {
			return accepted(fmt.Sprintf("T%d's plan is OK", tid), nil), nil
		}
		return accepted(fmt.Sprintf("T%d's plan is no longer OK", tid), nil), nil
	})
}

// AskPlan records an ask for the task's plan to its assignee, which waits behind what that agent is doing. by is who asked.
func AskPlan(q db.Q, tid int64, by string) error {
	t, err := db.GetTask(q, tid)
	if err != nil || t == nil {
		return err
	}
	behind, err := db.DoingTask(q, t.Owner)
	if err != nil {
		return err
	}
	_, err = db.AddTaskAsk(q, db.PlanAskKind, tid, by, t.Owner, "", behind)
	return err
}

// planAskMethod is task.plan_ask: the founder asks the task's assignee for a plan (Plans: Manual's button, or a plan asked again).
func planAskMethod(c *rpc.Conn, p map[string]any) (any, error) {
	tid, err := taskID(p)
	if err != nil {
		return nil, err
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		t, err := agentTask(tx, tid)
		if err != nil {
			return nil, err
		}
		state, _, _, err := db.PlanState(tx, tid)
		if err != nil {
			return nil, err
		}
		if state == "asked" { // a sent ask may be asked again: the agent can have lost it
			return nil, refusal(fmt.Sprintf("%s is already asked for T%d's plan, and has not been sent it yet", addr.Ref(t.Owner), tid))
		}
		if err := AskPlan(tx, tid, c.Caller.Actor()); err != nil {
			return nil, err
		}
		return accepted(fmt.Sprintf("%s is asked for T%d's plan, after what it is doing now", addr.Ref(t.Owner), tid), nil), nil
	})
}

// coreAskMethod is task.core_ask: the founder asks the task's assignee to write only its key function and that function's test, on the
// task's own branch, not merged; note says which part, when given.
func coreAskMethod(c *rpc.Conn, p map[string]any) (any, error) {
	tid, err := taskID(p)
	if err != nil {
		return nil, err
	}
	note := ""
	if v, given := p["note"]; given && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, badParam("note is text", "note")
		}
		note = strings.TrimSpace(textsafe.Text(s))
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		t, err := agentTask(tx, tid)
		if err != nil {
			return nil, err
		}
		asks, err := db.TaskAsks(tx, db.CoreAskKind, tid)
		if err != nil {
			return nil, err
		}
		for _, a := range asks {
			if a.Sent == 0 {
				return nil, refusal(fmt.Sprintf("%s is already asked for T%d's core logic, and has not been sent it yet", addr.Ref(t.Owner), tid))
			}
		}
		behind, err := db.DoingTask(tx, t.Owner)
		if err != nil {
			return nil, err
		}
		if _, err := db.AddTaskAsk(tx, db.CoreAskKind, tid, c.Caller.Actor(), t.Owner, note, behind); err != nil {
			return nil, err
		}
		return accepted(fmt.Sprintf("%s is asked for T%d's core logic, after what it is doing now", addr.Ref(t.Owner), tid), nil), nil
	})
}
