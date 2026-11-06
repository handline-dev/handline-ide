package db

import (
	"strings"
	"testing"
)

func TestMemosAreAddedReadEditedAndDeletedWithAFractionalStamp(t *testing.T) {
	q := open(t)
	tid := seedTask(t, q, "b", "a")
	mid, err := AddMemo(q, tid, "a", "first")
	if err != nil || mid == 0 {
		t.Fatal(mid, err)
	}
	m, err := GetMemo(q, mid)
	if err != nil || m == nil || m.TaskID != tid || m.Author != "a" || m.Body != "first" || m.EditedAt.Valid {
		t.Fatalf("%+v %v", m, err)
	}
	if m.CreatedAt <= 1e9 {
		t.Fatalf("%v", m.CreatedAt)
	}
	if err := EditMemo(q, mid, "w", "second"); err != nil {
		t.Fatal(err)
	}
	if m, _ := GetMemo(q, mid); m.Body != "second" || !m.EditedAt.Valid || m.EditedAt.Float64 != 1000 {
		t.Fatalf("%+v", m)
	}
	if s, _ := MemoSummary(q, tid, 500); s == "" {
		t.Fatal("a fractional stamp breaks the summary")
	}
	if err := DeleteMemo(q, mid, "w"); err != nil {
		t.Fatal(err)
	}
	if m, err := GetMemo(q, mid); m != nil || err != nil {
		t.Fatalf("%+v %v", m, err)
	}
}

// A memo's add, edit and delete are each an event with who and the text before and after: the task's history keeps a deleted memo's words.
func TestAMemoChangeIsKeptAsAnEventWithItsTextBeforeAndAfter(t *testing.T) {
	q := open(t)
	tid := seedTask(t, q, "b", "a")
	mid, _ := AddMemo(q, tid, "a", "first")
	EditMemo(q, mid, "b", "second")
	DeleteMemo(q, mid, "human")
	rows, err := q.Query("SELECT agent, kind, payload FROM events WHERE kind LIKE 'memo_%' ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var agent, kind, payload string
		rows.Scan(&agent, &kind, &payload)
		got = append(got, agent+" "+kind+" "+payload)
	}
	want := []string{
		`a memo_add {"task_id": 1, "memo": 1, "now": "first"}`,
		`b memo_edit {"task_id": 1, "memo": 1, "was": "first", "now": "second"}`,
		`human memo_delete {"task_id": 1, "memo": 1, "was": "second"}`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%q", got)
	}
	if err := EditMemo(q, 99, "a", "x"); err != nil {
		t.Fatal("a missing memo is not an error here: its caller says no memo")
	}
}

// A later tell replaces a say as a comment's tell, so its read mark follows the newest, and a link to no request; a task's return that
// carried the comment stays it while it may still reach its reader or once it has, and gives way once it was closed without reaching them.
func TestALaterTellReplacesASayButNotATaskReturnThatMayReachItsReader(t *testing.T) {
	q := open(t)
	if _, err := q.Exec("INSERT INTO requests(id, ts, requester, target, body, state, kind, landed_at, edge_ok) VALUES" +
		"(1, 1, 'task-board', 'bob', 'told', 'landed', 'say', 1, 1), (2, 2, 'task-board', 'bob', 'edited', 'landed', 'say', 2, 1), " +
		"(3, 3, 'human', 'bob', 'T1 is your turn again', 'queued', 'task', NULL, 1), (4, 4, 'task-board', 'bob', 'edited again', 'landed', 'say', 4, 1), " +
		"(5, 5, 'human', 'bob', 'T1 is your turn again', 'withdrawn', 'task', 5, 1)"); err != nil {
		t.Fatal(err)
	}
	tell := func(rid int64) int64 {
		t.Helper()
		if err := LinkMemoTell(q, 7, "bob", rid); err != nil {
			t.Fatal(err)
		}
		var got int64
		if err := q.QueryRow("SELECT request_id FROM memo_tells WHERE memo_id = 7 AND reader = 'bob'").Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if tell(99) != 99 || tell(1) != 1 || tell(2) != 2 {
		t.Fatal("a later tell did not replace a say, or a link to no request")
	}
	if tell(3) != 3 || tell(4) != 3 {
		t.Fatal("a say replaced the task's return")
	}
	for _, closed := range []string{"withdrawn", "denied", "replaced"} {
		if _, err := q.Exec("UPDATE requests SET state = ? WHERE id = 3", closed); err != nil {
			t.Fatal(err)
		}
		if tell(3) != 3 || tell(4) != 4 {
			t.Fatalf("a return %s before it reached bob kept its place", closed)
		}
	}
	if tell(5) != 5 || tell(4) != 5 {
		t.Fatal("a return withdrawn after it reached bob gave way")
	}
}
