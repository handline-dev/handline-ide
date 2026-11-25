package rulesfold

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/storetest"
)

const person = "p1"

func board(t *testing.T) *sql.DB {
	t.Helper()
	return storetest.Open(t)
}

func task(t *testing.T, q db.Q, title string) int64 {
	t.Helper()
	tid, err := db.NewTask(q, title, "a1", person, 0, db.DefaultPriority, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return tid
}

// comment writes a comment by author on tid said at ts.
func comment(t *testing.T, q db.Q, tid int64, author, body string, ts int64) int64 {
	t.Helper()
	res, err := q.Exec("INSERT INTO task_memos(task_id, author, body, created_at) VALUES (?, ?, ?, ?)", tid, author, body, float64(ts)+0.25)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// chat writes a say from requester to target at ts.
func chat(t *testing.T, q db.Q, requester, target, body string, ts int64) int64 {
	t.Helper()
	res, err := q.Exec("INSERT INTO requests(ts, requester, target, body, kind, state) VALUES (?, ?, ?, ?, 'say', 'landed')", ts, requester, target, body)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// answer is a fake Model that returns rules and records what it was asked.
type answer struct {
	rules  []map[string]any
	asked  []string
	during func()
}

func (a *answer) model(_ context.Context, prompt, _ string) ([]byte, error) {
	a.asked = append(a.asked, prompt)
	if a.during != nil {
		a.during()
	}
	return json.Marshal(map[string]any{"rules": a.rules})
}

func rule(id int64, area, text string, count int, quote, ref string, merges ...int64) map[string]any {
	return map[string]any{"id": id, "area": area, "text": text, "count": count, "quote": quote, "quote_ref": ref, "refs": []string{ref},
		"merges": merges}
}

func live(t *testing.T, q db.Q) []db.Lesson {
	t.Helper()
	ls, err := db.LiveLessons(q, person)
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// ref is the ref of request rid.
func ref(t *testing.T, q db.Q, rid int64) string {
	t.Helper()
	r, err := db.GetRequest(q, rid)
	if err != nil {
		t.Fatal(err)
	}
	return r.Ref
}
