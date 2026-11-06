package db

import (
	"database/sql"
)

// Turns (docs/design/turns.md): a task keeps its owner and carries a stack of turns above it. The top turn's agent acts on the task now;
// with no turn its owner does. task_turns is the truth; task_turn_push and task_turn_pop events are its history, written in the same step.

// MaxTurns is how many turns a task may carry at once.
const MaxTurns = 3

// Pop outcomes: the turn's work is accepted, or it goes back to the turn below with a note. Cleared is a stack emptied by a done, a draft,
// a discard or a reopen.
const (
	TurnPass     = "pass"
	TurnSendBack = "send_back"
	TurnCleared  = "cleared"
)

// TurnOnPassDone is a turn whose pass closes the task (a review of finished work).
const TurnOnPassDone = "done"

// Turn is one turn on a task: who acts, what they were asked, who put them there and when, and what a pass does.
type Turn struct {
	Agent, Ask, By string
	Since          int64
	OnPass         string
	Evidence       string
}

// ActingSQL is the agent acting on the task row named alias: its top turn's, else its owner.
func ActingSQL(alias string) string {
	return "COALESCE((SELECT agent FROM task_turns WHERE task_id = " + alias + ".id ORDER BY pos DESC LIMIT 1), " + alias + ".owner)"
}

// Turns is task tid's stack, bottom first.
func Turns(q Q, tid int64) ([]Turn, error) {
	rows, err := q.Query("SELECT agent, ask, pushed_by, since, COALESCE(on_pass, ''), COALESCE(evidence, '') FROM task_turns WHERE task_id = ? ORDER BY pos", tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Turn
	for rows.Next() {
		var t Turn
		if err := rows.Scan(&t.Agent, &t.Ask, &t.By, &t.Since, &t.OnPass, &t.Evidence); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Acting is the agent acting on task tid now, "" for no such task.
func Acting(q Q, tid int64) (string, error) {
	var a string
	err := q.QueryRow("SELECT "+ActingSQL("tasks")+" FROM tasks WHERE id = ?", tid).Scan(&a)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return a, err
}

// PushTurn puts t on top of task tid's stack. The caller has checked the task is open, it carries fewer than MaxTurns, and t.Agent is
// neither its owner nor in the stack.
func PushTurn(q Q, tid int64, t Turn) error {
	var n int
	if err := q.QueryRow("SELECT COUNT(*) FROM task_turns WHERE task_id = ?", tid).Scan(&n); err != nil {
		return err
	}
	if _, err := q.Exec("INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since, on_pass, evidence) VALUES(?, ?, ?, ?, ?, ?, ?, ?)",
		tid, n+1, t.Agent, t.Ask, t.By, t.Since, nullStr(t.OnPass), nullStr(t.Evidence)); err != nil {
		return err
	}
	_, err := AddEvent(q, t.By, "task_turn_push", KV{K: "task_id", V: tid}, KV{K: "to", V: t.Agent}, KV{K: "ask", V: t.Ask},
		KV{K: "on_pass", V: t.OnPass}, KV{K: "depth", V: n + 1})
	return err
}

// PopTurn takes the top turn off task tid by `by` with outcome and note, and answers it and the agent that acts now; ok false with no turn.
// rid is the request that carries the task back to its agent, 0 for none.
func PopTurn(q Q, tid int64, by, outcome, note string, rid int64) (top Turn, acting string, ok bool, err error) {
	turns, err := Turns(q, tid)
	if err != nil || len(turns) == 0 {
		return Turn{}, "", false, err
	}
	top = turns[len(turns)-1]
	if _, err := q.Exec("DELETE FROM task_turns WHERE task_id = ? AND pos = ?", tid, len(turns)); err != nil {
		return Turn{}, "", false, err
	}
	if acting, err = Acting(q, tid); err != nil {
		return Turn{}, "", false, err
	}
	kv := []KV{{K: "task_id", V: tid}, {K: "agent", V: top.Agent}, {K: "outcome", V: outcome}, {K: "note", V: note}, {K: "to", V: acting}}
	if rid != 0 {
		kv = append(kv, KV{K: "request_id", V: rid})
	}
	_, err = AddEvent(q, by, "task_turn_pop", kv...)
	return top, acting, true, err
}

// ClearTurns empties task tid's stack by `by` (a done, a draft, a discard) and answers the turns it took off, bottom first. The top one's
// event says top (TurnPass for the review whose pass closed the task), the others' TurnCleared.
func ClearTurns(q Q, tid int64, by, top string) ([]Turn, error) {
	turns, err := Turns(q, tid)
	if err != nil || len(turns) == 0 {
		return nil, err
	}
	if _, err := q.Exec("DELETE FROM task_turns WHERE task_id = ?", tid); err != nil {
		return nil, err
	}
	for i := len(turns) - 1; i >= 0; i-- {
		outcome := TurnCleared
		if i == len(turns)-1 && top != "" {
			outcome = top
		}
		if _, err := AddEvent(q, by, "task_turn_pop", KV{K: "task_id", V: tid}, KV{K: "agent", V: turns[i].Agent}, KV{K: "outcome", V: outcome}); err != nil {
			return nil, err
		}
	}
	return turns, nil
}

// InStack is whether agent is task tid's owner or holds one of its turns.
func InStack(q Q, tid int64, agent string) (bool, error) {
	var n int
	err := q.QueryRow("SELECT (SELECT COUNT(*) FROM tasks WHERE id = ?1 AND owner = ?2) + (SELECT COUNT(*) FROM task_turns WHERE task_id = ?1 AND agent = ?2)",
		tid, agent).Scan(&n)
	return n > 0, err
}

// ReplaceTopTurn gives task tid's top turn to agent, as a handoff does while turns are open; false when it has none.
func ReplaceTopTurn(q Q, tid int64, agent string) (bool, error) {
	res, err := q.Exec("UPDATE task_turns SET agent = ? WHERE task_id = ? AND pos = (SELECT MAX(pos) FROM task_turns WHERE task_id = ?)", agent, tid, tid)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RemoveTurn takes agent's turn out from under the top of task tid's stack, the ones above moving down one.
func RemoveTurn(q Q, tid int64, agent string) error {
	var pos int
	if err := q.QueryRow("SELECT pos FROM task_turns WHERE task_id = ? AND agent = ?", tid, agent).Scan(&pos); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM task_turns WHERE task_id = ? AND pos = ?", tid, pos); err != nil {
		return err
	}
	if _, err := q.Exec("UPDATE task_turns SET pos = pos - 1 WHERE task_id = ? AND pos > ?", tid, pos); err != nil {
		return err
	}
	_, err := AddEvent(q, agent, "task_turn_pop", KV{K: "task_id", V: tid}, KV{K: "agent", V: agent}, KV{K: "outcome", V: TurnSendBack}, KV{K: "note", V: "retired"})
	return err
}
