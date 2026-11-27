package taskwrites

import (
	"database/sql"

	"github.com/handline/handline/go/data/db"
)

// syncCommentAsk keeps an old @human comment's open ask in step with its edit, or withdraws it; a comment raises no new ask.
func syncCommentAsk(tx *sql.Tx, mid int64, body string) error {
	var rid int64
	err := tx.QueryRow("SELECT id FROM requests WHERE comment_id = ? AND state = 'queued'", mid).Scan(&rid)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if db.ToPerson(body) {
		_, err = tx.Exec("UPDATE requests SET body = ? WHERE id = ?", body, rid)
		return err
	}
	_, err = db.CloseCommentAsks(tx, "withdrawn", "comment_id = ?", mid)
	return err
}

// answerCommentAsks closes the asks to person that comments on task tid raised: they have now written on the task.
func answerCommentAsks(tx *sql.Tx, tid int64, person string) error {
	_, err := db.CloseCommentAsks(tx, "landed", "target = ? AND id IN (SELECT request_id FROM task_requests WHERE task_id = ?)", person, tid)
	return err
}

// taskEnded: task tid is done or discarded.
func taskEnded(tx db.Q, tid int64) (bool, error) {
	var n int
	err := tx.QueryRow("SELECT COUNT(*) FROM tasks WHERE id = ? AND (COALESCE(done_evidence, '') != '' OR COALESCE(discarded_by, '') != '')", tid).Scan(&n)
	return n > 0, err
}

// WithdrawUnaddressedCommentAsks withdraws, once (behind a cursor), the open asks of comments that do not start with `@human`; it answers how many.
func WithdrawUnaddressedCommentAsks(q db.Q) (int64, error) {
	const key = "memo_asks_unaddressed_withdrawn"
	if done, err := db.Cursor(q, key); err != nil || done != 0 {
		return 0, err
	}
	rows, err := q.Query("SELECT comment_id, body FROM requests WHERE comment_id IS NOT NULL AND state = 'queued'")
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
		if !db.ToPerson(body) {
			stale = append(stale, mid)
		}
	}
	rows.Close()
	var closed int64
	for _, mid := range stale {
		n, err := db.CloseCommentAsks(q, "withdrawn", "comment_id = ?", mid)
		if err != nil {
			return 0, err
		}
		closed += n
	}
	return closed, db.SetCursor(q, key, 1)
}
