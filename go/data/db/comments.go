package db

import (
	"database/sql"
	"regexp"
	"time"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
)

// Comment is one task_comments row.
type Comment struct {
	ID, TaskID   int64
	Author, Body string
	CreatedAt    float64
	EditedAt     sql.NullFloat64
}

// A comment's add, edit and delete are each kept as an event (comment_add, comment_edit, comment_delete) with the text before and after: they are the
// task's history (task.history), which keeps a deleted comment's words. q should be the transaction that changes the comment.

// AddComment adds a task's comment. The stamp is float seconds since the epoch, with the fraction kept.
func AddComment(q Q, tid int64, author, body string) (int64, error) {
	body, err := storedMentions(q, tid, author, body)
	if err != nil {
		return 0, err
	}
	res, err := q.Exec("INSERT INTO task_comments(task_id, author, body, created_at) VALUES (?, ?, ?, ?)", tid, author, body, float64(time.Now().UnixNano())/1e9)
	if err != nil {
		return 0, err
	}
	mid, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = AddEvent(q, author, "comment_add", KV{"task_id", tid}, KV{"comment", mid}, KV{"now", body})
	return mid, err
}

// GetComment is the comment with id mid, nil when there is no such comment.
func GetComment(q Q, mid int64) (*Comment, error) {
	m := &Comment{}
	err := q.QueryRow("SELECT id, task_id, author, body, created_at, edited_at FROM task_comments WHERE id = ?", mid).
		Scan(&m.ID, &m.TaskID, &m.Author, &m.Body, &m.CreatedAt, &m.EditedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// EditComment replaces comment mid's text, by by.
func EditComment(q Q, mid int64, by, body string) error {
	m, err := GetComment(q, mid)
	if err != nil || m == nil {
		return err
	}
	if body, err = storedMentions(q, m.TaskID, by, body); err != nil {
		return err
	}
	if _, err := q.Exec("UPDATE task_comments SET body = ?, edited_at = ? WHERE id = ?", body, Now(), mid); err != nil {
		return err
	}
	_, err = AddEvent(q, by, "comment_edit", KV{"task_id", m.TaskID}, KV{"comment", mid}, KV{"was", m.Body}, KV{"now", body})
	return err
}

// DeleteComment removes comment mid, by by.
func DeleteComment(q Q, mid int64, by string) error {
	m, err := GetComment(q, mid)
	if err != nil || m == nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM task_comments WHERE id = ?", mid); err != nil {
		return err
	}
	_, err = AddEvent(q, by, "comment_delete", KV{"task_id", m.TaskID}, KV{"comment", mid}, KV{"was", m.Body})
	return err
}

// LinkCommentTell keeps that request rid told reader of comment mid. A task's return that carried the comment stays its tell: an edit's later
// tell does not replace it, or the return would show again as a request of its own; one closed before it reached the reader is none.
func LinkCommentTell(q Q, mid int64, reader string, rid int64) error {
	_, err := q.Exec("INSERT INTO comment_tells(comment_id, reader, request_id) VALUES (?, ?, ?)"+keepReturnTell, mid, reader, rid)
	return err
}

// CarriesComment is whether request rid is reader's tell of a comment: the comment is in its body.
func CarriesComment(q Q, rid int64, reader string) (bool, error) {
	var n int
	err := q.QueryRow("SELECT COUNT(*) FROM comment_tells WHERE request_id = ? AND reader = ?", rid, reader).Scan(&n)
	return n > 0, err
}

// keepReturnTell is LinkCommentTell's rule for a reader already told: a return that reached it stays its tell.
var keepReturnTell = " ON CONFLICT(comment_id, reader) DO UPDATE SET request_id = excluded.request_id WHERE (SELECT kind != 'task' OR " +
	NeverReached("") + " FROM requests WHERE id = comment_tells.request_id) IS NOT 0"

// CommentCountBy is how many comments author has written on the task since the stamp, inclusive (seconds, as a comment's created_at).
func CommentCountBy(q Q, tid int64, author string, since float64) (n int64, err error) {
	err = q.QueryRow("SELECT COUNT(*) FROM task_comments WHERE task_id = ? AND author = ? AND created_at >= ?", tid, author, since).Scan(&n)
	return n, err
}

// ActedSince: the agent did something of its own at or after ts beyond one prompt and one stop (the turn a say woke): any other activity
// event, or a second prompt or stop.
func ActedSince(q Q, agent string, ts int64) (bool, error) {
	var other, prompts, stops int64
	err := q.QueryRow("SELECT COALESCE(SUM(kind NOT IN ('UserPromptSubmit','Stop')),0), COALESCE(SUM(kind='UserPromptSubmit'),0), COALESCE(SUM(kind='Stop'),0) "+
		"FROM events WHERE agent = ? AND ts >= ? AND kind IN ("+marks(len(ActivityKinds))+")", kindArgs([]any{agent, ts}, ActivityKinds)...).Scan(&other, &prompts, &stops)
	return other > 0 || prompts > 1 || stops > 1, err
}

// storedMentions is body as a comment keeps it: each @word that resolves for the author as <@key> (addr.Mentions), so a rename changes no
// stored text. A typed caller's handler may have done it already with the caller's folder; doing it again changes nothing.
func storedMentions(q Q, tid int64, author, body string) (string, error) {
	w, err := addr.Of(q, author, "")
	if err != nil {
		return body, err
	}
	w.Task = tid
	return addr.Mentions(q, w, body)
}

// founderMention is `@human` as a comment's first word, in a body written before mentions were stored as keys.
var founderMention = regexp.MustCompile(`^\s*@human(?:[^A-Za-z0-9_-]|$)`)

// ToPerson is whether a comment is a note to a person: a person's <@key> is its first token. It is news for them and moves no turn; a
// mention anywhere else (text to agents, a quote, code) is not.
func ToPerson(body string) bool {
	return actor.IsPerson(addr.Lead(body)) || founderMention.MatchString(body)
}

// CommentWriter is who wrote the comment that request rid tells its reader about, "" when rid tells no comment.
func CommentWriter(q Q, rid int64) (string, error) {
	var author string
	err := q.QueryRow("SELECT m.author FROM comment_tells t JOIN task_comments m ON m.id = t.comment_id WHERE t.request_id = ? LIMIT 1", rid).Scan(&author)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return author, err
}
