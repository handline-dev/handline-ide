package db

import (
	"database/sql"

	"github.com/handline/handline/go/kit/actor"
)

// A task's request (tasks.request) is the founder request it serves: the number of that request's own task, NULL for an agent's own work.
// It is set from the founder's words only: when the task is made, and when a task of the request is made to wait on it. tasks.turn_origin
// is the founder's message a task became a request from in that message's turn (turnRequest).

// SetTaskRequest gives the new task tid its request, by the first rule that holds: a task with no parent that answers to the founder (its
// reporter, else its opener) is a request of its own; a task with a parent serves its parent's; a task an agent opens serves the request
// its turn's founder message or comment makes (turnRequest). The task the agent holds gives none: an agent's own work done while holding a
// request is not that request's.
func SetTaskRequest(q Q, tid int64) error {
	var parent sql.NullInt64
	var openedBy, owner string
	var reporter sql.NullString
	if err := q.QueryRow("SELECT parent_task, opened_by, owner, reporter FROM tasks WHERE id = ?", tid).Scan(&parent, &openedBy, &owner, &reporter); err != nil {
		return err
	}
	var req sql.NullInt64
	switch {
	case !parent.Valid && (reporter.String == actor.OwnerKey || (!reporter.Valid && openedBy == actor.OwnerKey)):
		req = sql.NullInt64{Int64: tid, Valid: true}
	case parent.Valid:
		if err := q.QueryRow("SELECT request FROM tasks WHERE id = ?", parent.Int64).Scan(&req); err != nil {
			return err
		}
	case openedBy != actor.OwnerKey && owner != actor.OwnerKey && owner != Gate:
		var err error
		if req, err = turnRequest(q, tid, openedBy); err != nil {
			return err
		}
	}
	if !req.Valid {
		return nil
	}
	_, err := q.Exec("UPDATE tasks SET request = ? WHERE id = ?", req, tid)
	return err
}

// turnStart is when agent's turn began: its last Stop, 0 for none.
const turnStart = "COALESCE((SELECT MAX(ts) FROM events WHERE agent = ?1 AND kind IN ('Stop', 'StopFailure')), 0)"

// turnRequest is the request of task tid, opened by agent in a turn that brought the founder's words (typed to it since its last Stop), by
// the newest of them. A comment of the founder's is its task's request, none when that task serves none. A message is the request of the
// task that holds its thread (its top message made a task, or gave a task its turn_origin); else, when the message is a thread's top, tid's
// own, with the message as its turn_origin; else none.
func turnRequest(q Q, tid int64, agent string) (sql.NullInt64, error) {
	var req sql.NullInt64
	var rid, top int64
	var memoTask sql.NullInt64
	const memoTaskSQL = "(SELECT m.task_id FROM memo_tells t JOIN task_memos m ON m.id = t.memo_id WHERE t.request_id = r.id AND t.reader = ?1 " +
		"AND m.author = ?2 ORDER BY m.id DESC LIMIT 1)"
	err := q.QueryRow("SELECT r.id, "+memoTaskSQL+" FROM requests r WHERE r.target = ?1 AND r.landed_at >= "+turnStart+
		" AND ((r.requester = ?2 AND r.kind = 'say' AND r.founder_auth >= 1) OR "+memoTaskSQL+" IS NOT NULL) ORDER BY r.landed_at DESC, r.id DESC LIMIT 1",
		agent, actor.OwnerKey).Scan(&rid, &memoTask)
	if err == sql.ErrNoRows {
		return req, nil
	} else if err != nil {
		return req, err
	}
	if memoTask.Valid {
		err = q.QueryRow("SELECT request FROM tasks WHERE id = ?", memoTask.Int64).Scan(&req)
		return req, err
	}
	if err := q.QueryRow("WITH RECURSIVE up(id, parent) AS (SELECT id, parent_id FROM requests WHERE id = ? "+
		"UNION SELECT r.id, r.parent_id FROM requests r JOIN up ON r.id = up.parent) SELECT id FROM up WHERE parent IS NULL", rid).Scan(&top); err != nil {
		return req, err
	}
	err = q.QueryRow("SELECT request FROM tasks WHERE id = COALESCE((SELECT id FROM tasks WHERE from_request = ?1 ORDER BY id LIMIT 1), "+
		"(SELECT task_id FROM task_requests WHERE request_id = ?1 ORDER BY task_id LIMIT 1), (SELECT id FROM tasks WHERE turn_origin = ?1 ORDER BY id LIMIT 1))", top).Scan(&req)
	if err != sql.ErrNoRows {
		return req, err
	}
	if top != rid {
		return req, nil
	}
	if _, err := q.Exec("UPDATE tasks SET turn_origin = ? WHERE id = ?", rid, tid); err != nil {
		return req, err
	}
	return sql.NullInt64{Int64: tid, Valid: true}, nil
}

// inheritRequest: task tid, which serves a request, made to wait on blocksOn, a later task of none (an audit, a landing), brings it in.
func inheritRequest(q Q, tid, blocksOn int64) error {
	_, err := q.Exec("UPDATE tasks SET request = (SELECT w.request FROM tasks w WHERE w.id = ?) WHERE id = ? AND request IS NULL "+
		"AND id > ? AND EXISTS (SELECT 1 FROM tasks w WHERE w.id = ? AND w.request IS NOT NULL)", tid, blocksOn, tid, tid)
	return err
}
