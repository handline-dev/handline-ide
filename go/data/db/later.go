package db

import "time"

// A later comment is a step only a person can do that the rest of the task does not wait on: it asks nothing when posted. It is open
// while its task is open and that person has not answered it, by a comment after it or by passing the turn that brought it to them.

// SetCommentLater marks comment mid a later step for person.
func SetCommentLater(q Q, mid int64, person string) error {
	_, err := q.Exec("UPDATE task_comments SET later = ? WHERE id = ?", person, mid)
	return err
}

// AnswerLater answers person's open later steps on task tid before comment before (0 for all of them).
func AnswerLater(q Q, tid int64, person string, before int64) error {
	_, err := q.Exec("UPDATE task_comments SET later_answered_at = ? WHERE task_id = ? AND later = ? AND later_answered_at IS NULL AND (? = 0 OR id < ?)",
		time.Now().Unix(), tid, person, before, before)
	return err
}

// LaterOpenSQL is whether the task_comments row named alias is an open later comment.
func LaterOpenSQL(alias string) string {
	return alias + ".later IS NOT NULL AND " + alias + ".later_answered_at IS NULL AND EXISTS (SELECT 1 FROM tasks t WHERE t.id = " + alias +
		".task_id AND t.done_evidence IS NULL AND t.discarded_by IS NULL)"
}

// LaterComment is an open later comment: its number and its text.
type LaterComment struct {
	ID   int64
	Body string
}

// LaterFor is the person the oldest open later comment on task tid is for, "" for none.
func LaterFor(q Q, tid int64) (string, error) {
	var p string
	err := q.QueryRow("SELECT COALESCE((SELECT m.later FROM task_comments m WHERE m.task_id = ? AND "+LaterOpenSQL("m")+" ORDER BY m.id LIMIT 1), '')", tid).Scan(&p)
	return p, err
}

// OpenLaterComments is task tid's open later comments for person, oldest first.
func OpenLaterComments(q Q, tid int64, person string) ([]LaterComment, error) {
	rows, err := q.Query("SELECT m.id, m.body FROM task_comments m WHERE m.task_id = ? AND m.later = ? AND "+LaterOpenSQL("m")+" ORDER BY m.id", tid, person)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LaterComment
	for rows.Next() {
		var m LaterComment
		if err := rows.Scan(&m.ID, &m.Body); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
