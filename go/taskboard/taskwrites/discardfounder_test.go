package taskwrites

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/protocol/rpc"
)

func TestTheFoundersDiscardWithdrawsTheRequestsOutOnTheTask(t *testing.T) {
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	home := t.TempDir()
	teamtest.Seed(t, home, "w", "w2")
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, sent_at) VALUES(1, 'a', 'w', 'boss', 1, 5), (2, 'b', 'w2', 'p1', 1, 5), (3, 'c', 'p1', 'p1', 1, 5)",
		"INSERT INTO requests(id, requester, target, kind, state, ts, body, delivered_at) VALUES(10, 'boss', 'w', 'task', 'queued', 1, 'x', 6), (20, 'p1', 'w2', 'task', 'queued', 1, 'x', NULL), " +
			"(30, 'p1', 'p1', 'task', 'queued', 1, 'x', NULL)",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 10), (2, 20), (3, 30)",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('w2', 'IDLE', 1, 2)",
	} {
		if _, err := db.Exec(keyed(db, q)); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	f := &fakeTeller{}
	mk := func(agent string) *rpc.Server {
		s := rpc.NewServer(auth{agent}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = db
		Register(s, home, f, nil)
		return s
	}
	discard := func(rid string, task int) map[string]any {
		return call(t, mk(""), fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.discard","params":{"task":%d,"request_id":%q}}`, task, rid))[0]
	}
	count := func(q string) (n int) { db.QueryRow(keyed(db, q)).Scan(&n); return }

	// An agent's discard is still refused while a request is out on the task.
	r := call(t, mk("boss"), `{"jsonrpc":"2.0","id":2,"method":"task.discard","params":{"task":1,"reason":"moot","request_id":"a1"}}`)[0]
	if e, _ := r["error"].(map[string]any); e == nil || !strings.Contains(fmt.Sprint(e["message"]), "still has a request open") {
		t.Fatalf("an agent's discard of a task with a request out: %v", r)
	}
	r = discard("c1", 1)
	if r["error"] != nil || r["result"].(map[string]any)["said"] != "task T1 discarded by "+rpc.Human+": Discarded" {
		t.Fatalf("discard: %v", r)
	}
	if count("SELECT COUNT(*) FROM requests WHERE id = 10 AND state = 'landed' AND queue_order IS NULL") != 1 ||
		count("SELECT COUNT(*) FROM events WHERE kind = 'cancel' AND payload LIKE '%\"request_id\": 10%'") != 1 {
		t.Fatal("the delivered request is closed and its cancel recorded")
	}
	if len(f.cancelled) != 1 || f.cancelled[0].RID != 10 || !f.cancelled[0].Delivered || f.stopped {
		t.Fatalf("the target of a delivered request is stopped by the cancel itself: %+v %v", f.cancelled, f.stopped)
	}

	r = discard("c2", 2)
	if r["error"] != nil {
		t.Fatalf("the founder's own undelivered request: %v", r)
	}
	if count("SELECT COUNT(*) FROM requests WHERE id = 20 AND state = 'withdrawn'") != 1 || len(f.cancelled) != 0 || !f.stopped {
		t.Fatalf("it is withdrawn and the holder is typed to stop: %+v %v", f.cancelled, f.stopped)
	}

	// The founder's own task: its request is its card, and withdrawing the card is part of this discard, not someone else's.
	r = discard("c3", 3)
	if r["error"] != nil || r["result"].(map[string]any)["said"] != "task T3 discarded by "+rpc.Human+": Discarded" {
		t.Fatalf("a founder-owned task: %v", r)
	}
	if count("SELECT COUNT(*) FROM tasks WHERE id = 3 AND discard_reason = 'Discarded'") != 1 ||
		count("SELECT COUNT(*) FROM requests WHERE id = 30 AND state = 'withdrawn'") != 1 {
		t.Fatal("the task is discarded and its card withdrawn")
	}
}
