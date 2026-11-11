package taskwrites

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	hdb "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/protocol/rpc"
)

// task.send_now lets one task the pause holds go to the front of its agent's queue, passing the pause, and only such a task.
func TestSendNowLetsOneTaskThePauseHoldsGoFirst(t *testing.T) {
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	home := t.TempDir()
	teamtest.Seed(t, home, "w")
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', 'w', 'boss', 1), (2, 'b', 'w', 'boss', 1), (3, 'c', 'w', 'boss', 1)",
		"INSERT INTO requests(id, requester, target, kind, state, ts, body, queue_order, delivered_at) VALUES(10, 'boss', 'w', 'task', 'queued', 1, 'x', 10, NULL), " +
			"(20, 'boss', 'w', 'task', 'queued', 2, 'x', 20, NULL), (30, 'boss', 'w', 'task', 'queued', 3, 'x', 30, 6)",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 10), (2, 20), (3, 30)",
	} {
		if _, err := db.Exec(keyed(db, q)); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	s := rpc.NewServer(auth{""}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	s.DB = db
	Register(s, home, &fakeTeller{}, nil)
	n := 0
	send := func(task int) map[string]any {
		n++
		return call(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.send_now","params":{"task":%d,"request_id":"s%d"}}`, task, n))[0]
	}
	code := func(r map[string]any) any {
		e, _ := r["error"].(map[string]any)
		d, _ := e["data"].(map[string]any)
		return d["code"]
	}
	if r := send(2); code(r) != "refused" || !strings.Contains(fmt.Sprint(r["error"]), "new work is not paused") {
		t.Fatalf("not paused: %v", r)
	}
	if err := hdb.SetMeta(db, hdb.PauseKey, `{"since": 1}`); err != nil {
		t.Fatal(err)
	}
	if r := send(9); code(r) != "not_found" {
		t.Fatalf("no such task: %v", r)
	}
	if r := send(3); code(r) != "refused" {
		t.Fatalf("a task told already: %v", r)
	}
	agent := rpc.NewServer(auth{"w"}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	agent.DB = db
	Register(agent, home, &fakeTeller{}, nil)
	r := callHello(t, agent, `{"client":"t","protocol":1}`, `{"jsonrpc":"2.0","id":2,"method":"task.send_now","params":{"task":2,"request_id":"a1"}}`)[0]
	if code(r) != "refused" || !strings.Contains(fmt.Sprint(r["error"]), "role") {
		t.Fatalf("an agent let its own task past the founder's pause: %v", r)
	}
	if r := send(2); r["error"] != nil {
		t.Fatalf("held: %v", r)
	}
	var passed, sendNow int
	var o10, o20 float64
	db.QueryRow(keyed(db, "SELECT COUNT(*) FROM events WHERE kind = 'pause_passed' AND json_extract(payload, '$.request_id') = 20")).Scan(&passed)
	db.QueryRow(keyed(db, "SELECT COUNT(*) FROM requests WHERE id = 20 AND send_now IS NOT NULL")).Scan(&sendNow)
	db.QueryRow(keyed(db, "SELECT queue_order FROM requests WHERE id = 10")).Scan(&o10)
	db.QueryRow(keyed(db, "SELECT queue_order FROM requests WHERE id = 20")).Scan(&o20)
	if passed != 1 || sendNow != 1 || o20 >= o10 {
		t.Fatalf("passed %d, send_now %d, order %v before %v", passed, sendNow, o20, o10)
	}
	if held, _ := hdb.PauseHeldTasks(db, "w", time.Now()); !slices.Equal(held, []int64{1}) {
		t.Fatalf("still held: %v", held)
	}
	if r := send(2); code(r) != "refused" {
		t.Fatalf("a second send of a task the pause no longer holds: %v", r)
	}
}
