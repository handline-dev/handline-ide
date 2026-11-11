package db

import (
	"database/sql"
	"regexp"
	"time"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
)

// Memo is one task_memos row.
type Memo struct {
	ID, TaskID   int64
	Author, Body string
	CreatedAt    float64
	EditedAt     sql.NullFloat64
}

// A memo's add, edit and delete are each kept as an event (memo_add, memo_edit, memo_delete) with the text before and after: they are the
// task's history (task.history), which keeps a deleted memo's words. q should be the transaction that changes the memo.

// AddMemo is db.add_memo. The stamp is a float, as Python's time.time() writes it.
func AddMemo(q Q, tid int64, author, body string) (int64, error) {
	body, err := storedMentions(q, author, body)
	if err != nil {
		return 0, err
	}
	res, err := q.Exec("INSERT INTO task_memos(task_id, author, body, created_at) VALUES (?, ?, ?, ?)", tid, author, body, float64(time.Now().UnixNano())/1e9)
	if err != nil {
		return 0, err
	}
	mid, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = AddEvent(q, author, "memo_add", KV{"task_id", tid}, KV{"memo", mid}, KV{"now", body})
	return mid, err
}

// GetMemo is db.get_memo: nil when there is no such memo.
func GetMemo(q Q, mid int64) (*Memo, error) {
	m := &Memo{}
	err := q.QueryRow("SELECT id, task_id, author, body, created_at, edited_at FROM task_memos WHERE id = ?", mid).
		Scan(&m.ID, &m.TaskID, &m.Author, &m.Body, &m.CreatedAt, &m.EditedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// EditMemo is db.edit_memo: by replaces memo mid's text.
func EditMemo(q Q, mid int64, by, body string) error {
	m, err := GetMemo(q, mid)
	if err != nil || m == nil {
		return err
	}
	if body, err = storedMentions(q, by, body); err != nil {
		return err
	}
	if _, err := q.Exec("UPDATE task_memos SET body = ?, edited_at = ? WHERE id = ?", body, Now(), mid); err != nil {
		return err
	}
	_, err = AddEvent(q, by, "memo_edit", KV{"task_id", m.TaskID}, KV{"memo", mid}, KV{"was", m.Body}, KV{"now", body})
	return err
}

// DeleteMemo is db.delete_memo: by removes memo mid.
func DeleteMemo(q Q, mid int64, by string) error {
	m, err := GetMemo(q, mid)
	if err != nil || m == nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM task_memos WHERE id = ?", mid); err != nil {
		return err
	}
	_, err = AddEvent(q, by, "memo_delete", KV{"task_id", m.TaskID}, KV{"memo", mid}, KV{"was", m.Body})
	return err
}

// MemoTexts is the texts of T<tid>'s memos, oldest first.
func MemoTexts(q Q, tid int64) ([]string, error) {
	rows, err := q.Query("SELECT body FROM task_memos WHERE task_id = ? ORDER BY id", tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		out = append(out, body)
	}
	return out, rows.Err()
}

// LinkMemoTell keeps that request rid told reader of memo mid. A task's return that carried the comment stays its tell: an edit's later
// tell does not replace it, or the return would show again as a request of its own; one closed before it reached the reader is none.
func LinkMemoTell(q Q, mid int64, reader string, rid int64) error {
	_, err := q.Exec("INSERT INTO memo_tells(memo_id, reader, request_id) VALUES (?, ?, ?)"+keepReturnTell, mid, reader, rid)
	return err
}

// keepReturnTell is LinkMemoTell's rule for a reader already told: a return that reached it stays its tell.
var keepReturnTell = " ON CONFLICT(memo_id, reader) DO UPDATE SET request_id = excluded.request_id WHERE (SELECT kind != 'task' OR " +
	NeverReached("") + " FROM requests WHERE id = memo_tells.request_id) IS NOT 0"

// MemoCountBy is how many memos author has written on the task since the stamp, inclusive (seconds, as a memo's created_at).
func MemoCountBy(q Q, tid int64, author string, since float64) (n int64, err error) {
	err = q.QueryRow("SELECT COUNT(*) FROM task_memos WHERE task_id = ? AND author = ? AND created_at >= ?", tid, author, since).Scan(&n)
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

// storedMentions is body as a memo keeps it: each @word that resolves for the author as <@key> (addr.Mentions), so a rename changes no
// stored text. A typed caller's handler may have done it already with the caller's folder; doing it again changes nothing.
func storedMentions(q Q, author, body string) (string, error) {
	w, err := addr.Of(q, author, "")
	if err != nil {
		return body, err
	}
	return addr.Mentions(q, w, body)
}

// founderMention is `@human` as a comment's first word, in a body written before mentions were stored as keys.
var founderMention = regexp.MustCompile(`^\s*@human(?:[^A-Za-z0-9_-]|$)`)

// ToPerson is whether a comment is a note to a person: a person's <@key> is its first token. It is news for them and moves no turn; a
// mention anywhere else (text to agents, a quote, code) is not.
func ToPerson(body string) bool {
	return actor.IsPerson(addr.Lead(body)) || founderMention.MatchString(body)
}
