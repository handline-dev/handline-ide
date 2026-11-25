package rulesfold

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/handline/handline/go/data/db"
)

// Item is one thing the person wrote, as the model reads it: what was said, the agent text it answered, and on a task, the task's title
// and kind of work when it is known.
type Item struct {
	Ref      string `json:"ref"`
	Task     string `json:"task,omitempty"`
	Kind     string `json:"kind,omitempty"` // an area, when the task's kind of work is known
	Answered string `json:"answered,omitempty"`
	Said     string `json:"said"`

	source string
	row    int64
	at     int64
	task   int64
}

// The two sources a person's words come from, each with a cursor of its own.
const (
	sourceComments = "comments"
	sourceChat     = "chat"
	sourceTried    = "tried" // not a source: when a fold last failed
)

const (
	minSaid      = 8    // shorter than this is an ack ("OK", "좋아"), never a rule
	maxSaid      = 1200 // the most of one item the model reads
	maxChat      = 1500 // a longer chat message is pasted text, not the person's words
	maxAnswered  = 250
	rowsPerQuery = 400
)

var (
	marksBlock = regexp.MustCompile("(?s)```marks\\s*(\\[.*?\\])\\s*```")
	picture    = regexp.MustCompile(`!\[[^\]]*\]\(attachment:[^)]*\)`)
)

// clean is text as the model reads it: a picture's marks become the words written on them, an attached picture a placeholder.
func clean(s string) string {
	s = marksBlock.ReplaceAllStringFunc(s, func(m string) string {
		var marks []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(marksBlock.FindStringSubmatch(m)[1]), &marks) != nil {
			return ""
		}
		var out []string
		for _, k := range marks {
			out = append(out, "[mark: "+k.Text+"]")
		}
		return strings.Join(out, " ")
	})
	return strings.TrimSpace(picture.ReplaceAllString(s, "[picture]"))
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Cursors are the last row of each source the fold has read.
type Cursors struct{ Comments, Chat int64 }

func cursorName(person, source string) string { return "rules_fold:" + person + ":" + source }

// ReadCursors are person's cursors, zero for a source never read.
func ReadCursors(q db.Q, person string) (Cursors, error) {
	var c Cursors
	for src, p := range map[string]*int64{sourceComments: &c.Comments, sourceChat: &c.Chat} {
		err := q.QueryRow("SELECT row_id FROM cursors WHERE name = ?", cursorName(person, src)).Scan(p)
		if err != nil && err != sql.ErrNoRows {
			return c, err
		}
	}
	return c, nil
}

func writeCursors(q db.Q, person string, c Cursors) error {
	for src, row := range map[string]int64{sourceComments: c.Comments, sourceChat: c.Chat} {
		if _, err := q.Exec("INSERT INTO cursors(name, row_id, updated) VALUES(?, ?, ?) ON CONFLICT(name) DO UPDATE SET row_id = excluded.row_id, "+
			"updated = excluded.updated", cursorName(person, src), row, db.Now()); err != nil {
			return err
		}
	}
	return nil
}

// Batch is what one step reads: the items to send, in the order they were said, and the cursors once they are folded in (rows too short
// to send move the cursor too).
type Batch struct {
	Items []Item
	Next  Cursors
	// Full is whether more was there to read than this batch took.
	Full bool
}

// ReadBatch is person's words after c, oldest first, up to budget bytes of items. kind is the area of a task's kind of work, "" when
// unknown.
func ReadBatch(q db.Q, person string, c Cursors, budget int, kind func(task int64) string) (Batch, error) {
	comments, err := readComments(q, person, c.Comments)
	if err != nil {
		return Batch{}, err
	}
	chat, err := readChat(q, person, c.Chat)
	if err != nil {
		return Batch{}, err
	}
	fullComments, fullChat := len(comments) == rowsPerQuery, len(chat) == rowsPerQuery
	// Each source is in row order, so its cursor only moves forward; the two are merged by the time said, the one whose next row was said
	// first going next. A source that filled its query may have rows said before the other's next one: once it runs out, stop.
	b := Batch{Next: c}
	size, areas := 0, map[int64]string{}
	for len(comments)+len(chat) > 0 {
		var it Item
		switch {
		case len(comments) == 0 && fullComments, len(chat) == 0 && fullChat:
			b.Full = true
			return b, nil
		case len(chat) == 0 || len(comments) > 0 && comments[0].at <= chat[0].at:
			it, comments = comments[0], comments[1:]
		default:
			it, chat = chat[0], chat[1:]
		}
		if len(it.Said) >= minSaid {
			n := len(it.Said) + len(it.Answered) + len(it.Task) + 40
			if len(b.Items) > 0 && size+n > budget {
				b.Full = true
				return b, nil
			}
			size += n
			if it.task != 0 && kind != nil {
				if _, ok := areas[it.task]; !ok {
					areas[it.task] = kind(it.task)
				}
				it.Kind = areas[it.task]
			}
			b.Items = append(b.Items, it)
		}
		if it.source == sourceComments {
			b.Next.Comments = it.row
		} else {
			b.Next.Chat = it.row
		}
	}
	b.Full = fullComments || fullChat
	return b, nil
}

// readComments are person's task comments after row, each with the agent comment it answered: the one it replies to, or else the newest
// one before it on the task.
func readComments(q db.Q, person string, after int64) ([]Item, error) {
	return db.Collect[Item](q.Query(`SELECT m.id, CAST(m.created_at AS INTEGER), m.task_id, t.title, m.body,
		COALESCE((SELECT a.body FROM task_memos a WHERE a.task_id = m.task_id AND a.id < m.id AND a.author != m.author
			AND (m.reply_to IS NULL OR a.id = m.reply_to) ORDER BY a.id DESC LIMIT 1), '')
		FROM task_memos m JOIN tasks t ON t.id = m.task_id WHERE m.author = ? AND m.id > ? ORDER BY m.id LIMIT ?`, person, after, rowsPerQuery))(
		func(rows *sql.Rows) (it Item, err error) {
			var title, body, answered string
			err = rows.Scan(&it.row, &it.at, &it.task, &title, &body, &answered)
			it.source, it.Ref = sourceComments, fmt.Sprintf("comment %d", it.row)
			it.Task, it.Said, it.Answered = cut(title, 100), cut(clean(body), maxSaid), cut(clean(answered), maxAnswered)
			return it, err
		})
}

// readChat are the chat messages person typed after row, each with the message it answered: its parent, or else the newest one its
// target sent the person before it. A message longer than maxChat is pasted, and read as nothing.
func readChat(q db.Q, person string, after int64) ([]Item, error) {
	return db.Collect[Item](q.Query(`SELECT r.id, r.ts, COALESCE(r.ref, ''), r.body,
		COALESCE((SELECT p.body FROM requests p WHERE p.id = r.parent_id),
			(SELECT a.body FROM requests a WHERE a.requester = r.target AND a.target = r.requester AND a.id < r.id ORDER BY a.id DESC LIMIT 1), '')
		FROM requests r WHERE r.requester = ? AND r.kind = 'say' AND r.id > ? ORDER BY r.id LIMIT ?`, person, after, rowsPerQuery))(
		func(rows *sql.Rows) (it Item, err error) {
			var ref, body, answered string
			err = rows.Scan(&it.row, &it.at, &ref, &body, &answered)
			it.source, it.Ref = sourceChat, db.MsgPrefix+ref
			if body = clean(body); len(body) <= maxChat {
				it.Said = cut(body, maxSaid)
			}
			it.Answered = cut(clean(answered), maxAnswered)
			return it, err
		})
}
