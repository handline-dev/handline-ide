package taskwrites

import (
	"database/sql"
	"regexp"

	"github.com/handline/handline/go/data/db"
)

// humanMention is a comment addressed to the founder: `@human` is its first word. A mention anywhere else (text to agents, a quote, code) asks nothing.
var humanMention = regexp.MustCompile(`^\s*@human(?:[^A-Za-z0-9_-]|$)`)

// syncMemoAsk makes the ask to the founder that comment mid on task tid raised match the comment as it reads now: an agent's comment that
// starts with `@human` has one open ask (its body the comment), and one that does not has none. A done or discarded task raises none.
func syncMemoAsk(tx *sql.Tx, tid, mid int64, author, body string) error {
	var rid int64
	err := tx.QueryRow("SELECT id FROM requests WHERE memo_id = ? AND state = 'queued'", mid).Scan(&rid)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	open := err == nil
	switch want := author != human && humanMention.MatchString(body); {
	case want && !open:
		closed, err := taskEnded(tx, tid)
		if err != nil || closed {
			return err // a task that is over asks nothing
		}
		if rid, err = db.AddRequest(tx, db.NewRequest{Requester: author, Target: human, Body: body}); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE requests SET memo_id = ? WHERE id = ?", mid, rid); err != nil {
			return err
		}
		return db.LinkRequest(tx, tid, rid)
	case want:
		_, err = tx.Exec("UPDATE requests SET body = ? WHERE id = ?", body, rid)
		return err
	case open:
		_, err = db.CloseMemoAsks(tx, "withdrawn", "memo_id = ?", mid)
		return err
	}
	return nil
}

// answerMemoAsks closes the asks to the founder that comments on task tid raised: the founder has now written on the task.
func answerMemoAsks(tx *sql.Tx, tid int64) error {
	_, err := db.CloseMemoAsks(tx, "landed", "id IN (SELECT request_id FROM task_requests WHERE task_id = ?)", tid)
	return err
}

// taskEnded: task tid is done or discarded.
func taskEnded(tx *sql.Tx, tid int64) (bool, error) {
	var n int
	err := tx.QueryRow("SELECT COUNT(*) FROM tasks WHERE id = ? AND (COALESCE(done_evidence, '') != '' OR COALESCE(discarded_by, '') != '')", tid).Scan(&n)
	return n > 0, err
}

// WithdrawUnaddressedMemoAsks withdraws, once (behind a cursor), the open asks of comments that do not start with `@human`; it answers how many.
func WithdrawUnaddressedMemoAsks(q db.Q) (int64, error) {
	const key = "memo_asks_unaddressed_withdrawn"
	if done, err := db.Cursor(q, key); err != nil || done != 0 {
		return 0, err
	}
	rows, err := q.Query("SELECT memo_id, body FROM requests WHERE memo_id IS NOT NULL AND state = 'queued'")
	if err != nil {
		return 0, err
	}
	var stale []int64
	for rows.Next() {
		var mid int64
		var body string
		if err := rows.Scan(&mid, &body); err != nil {
			rows.Close()
			return 0, err
		}
		if !humanMention.MatchString(body) {
			stale = append(stale, mid)
		}
	}
	rows.Close()
	var closed int64
	for _, mid := range stale {
		n, err := db.CloseMemoAsks(q, "withdrawn", "memo_id = ?", mid)
		if err != nil {
			return 0, err
		}
		closed += n
	}
	return closed, db.SetCursor(q, key, 1)
}
