package taskwrites

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/protocol/rpc"
)

// oldAsk is comment mid on task tid by worker with the ask to the founder it raised when comments still asked.
func oldAsk(t *testing.T, db *sql.DB, tid, mid int64, body string) {
	t.Helper()
	for _, q := range []string{
		fmt.Sprintf("INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(%d, %d, 'worker', %q, 1)", mid, tid, body),
		fmt.Sprintf("INSERT INTO requests(id, requester, target, body, state, kind, memo_id, ts) VALUES(%d, 'worker', 'p1', %q, 'queued', 'ask', %d, 1)", 100+mid, body, mid),
		fmt.Sprintf("INSERT INTO task_requests(task_id, request_id) VALUES(%d, %d)", tid, 100+mid),
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

// A comment asks the founder nothing, @human or not: their question is their turn on the task.
func TestACommentRaisesNoAsk(t *testing.T) {
	t.Parallel()
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'worker', 'manager', 1)")
	srv := serverFor(t, db, "worker")
	for i, text := range []string{"@human which one?", "  \n@human which?", "thanks @human for the answer"} {
		r := call(t, srv, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.comment.add","params":{"task":1,"text":%q,"request_id":"q%d"}}`, text, i))
		if r[0]["error"] != nil {
			t.Fatalf("%q: %v", text, r[0])
		}
	}
	var asks int
	db.QueryRow("SELECT COUNT(*) FROM requests WHERE target = 'p1'").Scan(&asks)
	if asks != 0 {
		t.Fatalf("comments asked the founder %d times", asks)
	}
}

// An ask an older comment raised follows it: an edit that keeps `@human` first rewrites it, one that does not, or a delete, withdraws it,
// and the founder writing on the task answers it.
func TestAnOldCommentsAskFollowsItsComment(t *testing.T) {
	t.Parallel()
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'worker', 'manager', 1)"); err != nil {
		t.Fatal(err)
	}
	mk := func(agent string) *rpc.Server { return serverFor(t, db, agent) }
	n := 0
	rid := func() string { n++; return fmt.Sprintf("r%d", n) }
	state := func(mid int64) (s, body string) {
		db.QueryRow("SELECT state, body FROM requests WHERE memo_id = ?", mid).Scan(&s, &body)
		return s, body
	}
	edit := func(mid int64, text string) {
		call(t, mk("worker"), fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.comment.edit","params":{"task":1,"memo":%d,"text":%q,"request_id":%q}}`, mid, text, rid()))
	}
	oldAsk(t, db, 1, 1, "@human which one?")
	edit(1, "@human which one, A or B?")
	if s, body := state(1); s != "queued" || body != "@human which one, A or B?" {
		t.Fatalf("an edit that keeps `@human` keeps the one ask: %q %q", s, body)
	}
	edit(1, "which, @human?")
	if s, _ := state(1); s != "withdrawn" {
		t.Fatalf("an edit that moves `@human` off the start: %q", s)
	}
	oldAsk(t, db, 1, 2, "@human again")
	call(t, mk("worker"), fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.comment.delete","params":{"task":1,"memo":2,"request_id":%q}}`, rid()))
	if s, _ := state(2); s != "withdrawn" {
		t.Fatalf("a deleted comment: %q", s)
	}
	oldAsk(t, db, 1, 3, "@human third")
	call(t, mk(""), fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.comment.add","params":{"task":1,"text":"my answer","request_id":%q}}`, rid()))
	if s, _ := state(3); s != "landed" {
		t.Fatalf("a founder comment on the task answers it: %q", s)
	}
}

// An ask an older comment raised does not outlive its task: done or discarded closes it.
func TestTheAskOfACommentEndsWithItsTask(t *testing.T) {
	t.Parallel()
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at, expects) VALUES(1, 'a', 'worker', 'manager', 1, 'report'), (2, 'b', 'worker', 'manager', 1, 'report')"); err != nil {
		t.Fatal(err)
	}
	srv := rpc.NewServer(auth{"worker"}, "x", store.LatestVersion(), "local", t.Logf)
	srv.DB = db
	Register(srv, t.TempDir(), &fakeTeller{}, nil)
	for i, c := range []struct {
		task           int64
		method, params string
	}{
		{1, "task.done", `"evidence":"shipped"`},
		{2, "task.discard", `"reason":"not needed"`},
	} {
		oldAsk(t, db, c.task, c.task, "@human which?")
		r := call(t, srv, fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"%s","params":{"task":%d,%s,"request_id":"e%d"}}`, c.method, c.task, c.params, i))
		if r[0]["error"] != nil {
			t.Fatalf("%s: %v", c.method, r[0])
		}
		var state string
		db.QueryRow("SELECT state FROM requests WHERE memo_id = ?", c.task).Scan(&state)
		if state != "withdrawn" {
			t.Fatalf("%s left the ask %q, not withdrawn", c.method, state)
		}
	}
}

func TestWithdrawUnaddressedMemoAsksTakesBackOnlyThoseNotStartingWithHumanOnce(t *testing.T) {
	t.Parallel()
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("INSERT INTO requests(id, requester, target, body, state, kind, memo_id, ts) VALUES(1, 'a', 'p1', '@human which?', 'queued', 'ask', 1, 1), (2, 'a', 'p1', 'thanks @human', 'queued', 'ask', 2, 1)")
	if n, err := WithdrawUnaddressedMemoAsks(db); err != nil || n != 1 {
		t.Fatalf("one stale ask: %d %v", n, err)
	}
	var st1, st2 string
	db.QueryRow("SELECT state FROM requests WHERE id = 1").Scan(&st1)
	db.QueryRow("SELECT state FROM requests WHERE id = 2").Scan(&st2)
	if st1 != "queued" || st2 != "withdrawn" {
		t.Fatalf("%s %s", st1, st2)
	}
	db.Exec("UPDATE requests SET state = 'queued' WHERE id = 2")
	if n, _ := WithdrawUnaddressedMemoAsks(db); n != 0 {
		t.Fatalf("once: %d", n)
	}
}
