package db

import "testing"

// A resend carries the comment's tell to its reader over one that was withdrawn before it reached them, as LinkMemoTell would.
func TestAResendReplacesAWithdrawnTellOfItsReader(t *testing.T) {
	q := open(t)
	for _, s := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'alice', 'human', 1)",
		"INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(7, 1, 'human', 'now', 1)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 1, 'human', 'alice', 'r1', 'withdrawn', 'task'), " +
			"(2, 2, 'human', 'bob', 'r2', 'withdrawn', 'task'), (3, 3, 'human', 'alice', 'r3', 'queued', 'task')",
		"INSERT INTO memo_tells(memo_id, reader, request_id) VALUES(7, 'alice', 1), (7, 'bob', 2)",
	} {
		if _, err := q.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := RecordForward(q, 1, 3, ForwardResend, 2, "alice"); err != nil {
		t.Fatal(err)
	}
	var rid int64
	if err := q.QueryRow("SELECT request_id FROM memo_tells WHERE memo_id = 7 AND reader = 'alice'").Scan(&rid); err != nil || rid != 3 {
		t.Fatalf("alice's tell is %d %v", rid, err)
	}
}
