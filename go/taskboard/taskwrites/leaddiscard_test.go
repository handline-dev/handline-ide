package taskwrites

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/protocol/rpc"
)

// leadEnv is lead, w1, w2 and ops: T1 and T2 are w1's, opened by lead, each with a request still queued from w2; T3 is ops's own. Leading
// gives no say over a discard: only the founder's discard withdraws an open request.
func leadEnv(t *testing.T) (*sql.DB, func(agent string, tell Teller) *rpc.Server) {
	t.Helper()
	d, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	home := t.TempDir()
	teamtest.Seed(t, home, "lead", "w1", "w2", "ops")
	homes.Store(d, home)
	exec(t, d,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'first', 'w1', 'lead', 1), (2, 'second', 'w1', 'lead', 1), (3, 'theirs', 'ops', 'ops', 1)",
		"INSERT INTO requests(id, ts, requester, target, body, kind, state, edge_ok) VALUES(5, 1, 'w2', 'w1', 'look at t', 'ask', 'queued', 1), "+
			"(6, 1, 'w2', 'w1', 'and u', 'ask', 'queued', 1)",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 5), (2, 6)")
	return d, func(agent string, tell Teller) *rpc.Server {
		return rigServer(t, d, home, agent, tell)
	}
}

func TestTheFounderDiscardsATaskWithdrawingItsOpenRequest(t *testing.T) {
	t.Parallel()
	d, mk := leadEnv(t)
	f := &fakeTeller{}
	r := call(t, mk("human", f), `{"jsonrpc":"2.0","id":2,"method":"task.discard","params":{"task":1,"reason":"PR merged","request_id":"l1"}}`)
	if r[0]["error"] != nil {
		t.Fatal(r[0])
	}
	if count(t, d, "SELECT COUNT(*) FROM requests WHERE id = 5 AND state != 'queued'") != 1 {
		t.Fatal("the open request is withdrawn")
	}
	if len(f.cancelled) != 1 || f.cancelled[0].RID != 5 || f.cancelled[0].Verdict != CancelVerdict {
		t.Fatalf("its request closes as the founder's cancel: %+v", f.cancelled)
	}
	told := false
	for _, n := range f.told {
		told = told || n.To == "w1" && n.Text == "T1 discarded by the founder: PR merged"
	}
	if !told {
		t.Fatalf("the owner is told the task was closed and why: %v", f.told)
	}
	if count(t, d, "SELECT COUNT(*) FROM events WHERE kind = 'discard_task' AND agent = 'p1' AND json_extract(payload, '$.withdrew[0]') = 5") != 1 {
		t.Fatal("the record keeps which requests were withdrawn")
	}
}

func TestOnlyTheFounderDiscardsOverAnOpenRequest(t *testing.T) {
	t.Parallel()
	d, mk := leadEnv(t)
	for _, who := range []string{"lead", "w2", "ops"} {
		r := call(t, mk(who, nil), `{"jsonrpc":"2.0","id":2,"method":"task.discard","params":{"task":1,"reason":"x","request_id":"n1"}}`)
		if r[0]["error"] == nil {
			t.Fatalf("%s, its opener or not, is refused while a request is open: %v", who, r[0])
		}
	}
	if count(t, d, "SELECT COUNT(*) FROM requests WHERE id = 5 AND state = 'queued'") != 1 {
		t.Fatal("a refused discard withdraws nothing")
	}
}

func TestABatchDiscardIsOneEventWithEachTasksReason(t *testing.T) {
	t.Parallel()
	d, mk := leadEnv(t)
	r := call(t, mk("human", &fakeTeller{}), `{"jsonrpc":"2.0","id":2,"method":"task.discard","params":{"tasks":[1,2],"reason":"done elsewhere",`+
		`"reasons":{"T2":"merged upstream"},"request_id":"b1"}}`)
	if r[0]["error"] != nil {
		t.Fatal(r[0])
	}
	var payload string
	if err := d.QueryRow(keyed(d, "SELECT payload FROM events WHERE kind = 'discard_batch' AND agent = 'p1'")).Scan(&payload); err != nil {
		t.Fatal("one discard_batch event: ", err)
	}
	var ev struct {
		Batch string
		Tasks []struct {
			TaskID   int64 `json:"task_id"`
			Title    string
			Reason   string
			Withdrew []int64
		}
	}
	json.Unmarshal([]byte(payload), &ev)
	if len(ev.Tasks) != 2 || ev.Tasks[0].Reason != "done elsewhere" || ev.Tasks[1].Reason != "merged upstream" || ev.Tasks[1].Title != "second" ||
		len(ev.Tasks[1].Withdrew) != 1 {
		t.Fatalf("each task with its title, reason and withdrawn requests: %s", payload)
	}
	if count(t, d, "SELECT COUNT(*) FROM events WHERE kind = 'discard_task' AND json_extract(payload, '$.batch') = '"+ev.Batch+"'") != 2 {
		t.Fatal("each task's own record names the batch")
	}
	if count(t, d, "SELECT COUNT(*) FROM tasks WHERE id = 2 AND discard_reason = 'merged upstream'") != 1 {
		t.Fatal("the reason of its own is the task's")
	}
}

func TestABatchWithATaskItCannotDiscardDiscardsNone(t *testing.T) {
	d, mk := leadEnv(t)
	r := call(t, mk("ops", nil), `{"jsonrpc":"2.0","id":2,"method":"task.discard","params":{"tasks":[3,1],"reason":"x","request_id":"b2"}}`)
	if r[0]["error"] == nil {
		t.Fatalf("T1 has an open request: %v", r[0])
	}
	if count(t, d, "SELECT COUNT(*) FROM tasks WHERE id = 3 AND discarded_by IS NULL") != 1 {
		t.Fatal("a refused batch discards none of it")
	}
}

func TestAReopenedDiscardedTaskIsADraftAgain(t *testing.T) {
	t.Parallel()
	d, mk := leadEnv(t)
	call(t, mk("human", nil), `{"jsonrpc":"2.0","id":2,"method":"task.discard","params":{"tasks":[1,2],"reason":"x","request_id":"b4"}}`)
	r := call(t, mk("human", &fakeTeller{}), `{"jsonrpc":"2.0","id":2,"method":"task.reopen","params":{"task":2,"note":"still needed","request_id":"r1"}}`)
	if r[0]["error"] != nil {
		t.Fatal(r[0])
	}
	if said := r[0]["result"].(map[string]any)["said"]; said != "task T2 reopened as a draft by human: still needed - `hl task queue T2` sends it again" {
		t.Fatal(said)
	}
	if count(t, d, "SELECT COUNT(*) FROM tasks WHERE id = 2 AND discarded_by IS NULL AND discard_reason IS NULL AND queued_by IS NULL") != 1 ||
		count(t, d, "SELECT COUNT(*) FROM tasks WHERE id = 1 AND discarded_by = 'p1'") != 1 {
		t.Fatal("only the reopened task comes back")
	}
	if count(t, d, "SELECT COUNT(*) FROM events WHERE kind = 'reopen_task' AND json_extract(payload, '$.was') = 'discarded'") != 1 {
		t.Fatal("the record says it was reopened from a discard")
	}
}
