package taskwrites

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/protocol/rpc"
)

func TestProgressIsKeptOnTheTaskTheAgentHoldsOrTheAgentAndReadBack(t *testing.T) {
	t.Parallel()
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'worker', 'manager', 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(2, 'u', 'worker', 'manager', 1)",
	} {
		if _, err := db.Exec(keyed(db, q)); err != nil {
			t.Fatal(err)
		}
	}
	mk := func(agent string) *rpc.Server {
		s := rpc.NewServer(auth{agent}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = db
		Register(s, t.TempDir(), nil, nil)
		return s
	}
	prog := func(id int, params, rid string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"task.progress","params":{%s"request_id":%q}}`, id, params, rid)
	}
	get := `{"jsonrpc":"2.0","id":9,"method":"progress.get","params":{}}`
	note := func(r map[string]any) any { return r["result"].(map[string]any)["note"] }
	code := func(r map[string]any) any { return r["error"].(map[string]any)["data"].(map[string]any)["code"] }

	r := call(t, mk("worker"),
		prog(2, `"note":"on the agent",`, "p1"), get,
		`{"jsonrpc":"2.0","id":4,"method":"task.start","params":{"task":1,"request_id":"s1"}}`,
		prog(5, `"note":"on one",`, "p2"), get,
		prog(7, `"task":2,"note":"on two",`, "p3"), get,
		prog(9, `"note":"",`, "p4"), get)
	if note(r[1]) != "on the agent" {
		t.Fatalf("with no task the note is the agent's: %v", r[1])
	}
	if note(r[4]) != "on one" || note(r[6]) != "on one" {
		t.Fatalf("holding T1, the note is on T1 and a note on T2 leaves it alone: %v %v", r[4], r[6])
	}
	if said := r[5]["result"].(map[string]any)["said"]; said != "progress on T2 kept; the handoff and the wake prompt carry it" {
		t.Fatalf("%v", r[5])
	}
	var kept string
	db.QueryRow(keyed(db, "SELECT value FROM meta WHERE name = 'progress-task:2'")).Scan(&kept)
	if kept != "on two" {
		t.Fatalf("T2's note: %q", kept)
	}
	if note(r[8]) != nil {
		t.Fatalf("an empty note clears it: %v", r[8])
	}

	r = call(t, mk("manager"), prog(2, `"task":1,"note":"x",`, "q1"), prog(3, `"task":9,"note":"x",`, "q2"))
	if code(r[0]) != "refused" || code(r[1]) != "not_found" {
		t.Fatalf("another agent's task is refused, an unknown one is not_found: %v %v", r[0], r[1])
	}
	r = call(t, mk("worker"), prog(2, `"note":5,`, "q3"), prog(3, `"note":"`+strings.Repeat("x", 1501)+`",`, "q4"), prog(4, `"task":"a","note":"x",`, "q5"))
	for i, got := range r {
		if code(got) != "bad_request" {
			t.Fatalf("call %d: %v", i, got)
		}
	}
}

func TestProgressKeepsAStepCountOnTheTaskUntilANoteWithoutOne(t *testing.T) {
	d, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	clock, was := int64(1000), db.Now
	db.Now = func() int64 { return clock }
	t.Cleanup(func() { db.Now = was })
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'worker', 'manager', 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(2, 'u', 'worker', 'manager', 1)",
	} {
		if _, err := d.Exec(keyed(d, q)); err != nil {
			t.Fatal(err)
		}
	}
	s := rpc.NewServer(auth{"worker"}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	s.DB = d
	Register(s, t.TempDir(), nil, nil)
	prog := func(params, rid string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.progress","params":{%s"request_id":%q}}`, params, rid)
	}
	todo := func(tid int64) *db.TaskTodo {
		t.Helper()
		got, err := db.ReadTaskTodo(d, tid)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	call(t, s, prog(`"task":2,"note":"before holding","done":1,"total":2,`, "p0"))
	if got := todo(2); got == nil || got.Done != 1 {
		t.Fatalf("a named task takes its count though the caller holds none: %+v", got)
	}
	call(t, s, `{"jsonrpc":"2.0","id":1,"method":"task.start","params":{"task":1,"request_id":"s1"}}`, prog(`"note":"wiring the gate","done":3,"total":5,`, "p1"))
	worker := keyed(d, "worker")
	if got := todo(1); got == nil || *got != (db.TaskTodo{By: worker, Done: 3, Total: 5, Current: "wiring the gate", UpdatedAt: 1000}) {
		t.Fatalf("held task: %+v (worker %s)", got, worker)
	}
	clock = 1060
	call(t, s, prog(`"note":"the tests","done":4,"total":5,`, "p2"))
	if got := todo(1); got == nil || got.Done != 4 || got.Current != "the tests" || got.UpdatedAt != 1060 {
		t.Fatalf("a new count moves updated_at: %+v", got)
	}
	call(t, s, prog(`"task":2,"note":"on two","done":0,"total":2,`, "p3"))
	if got := todo(2); got == nil || got.Total != 2 || todo(1).Done != 4 {
		t.Fatalf("a named task keeps its own count: %+v", got)
	}
	call(t, s, prog(`"note":"plain words",`, "p4"))
	if got := todo(1); got != nil {
		t.Fatalf("a note without a count clears it: %+v", got)
	}
	call(t, s, prog(`"task":2,"note":"",`, "p5"))
	if got := todo(2); got != nil {
		t.Fatalf("an empty note clears it: %+v", got)
	}
	call(t, s, prog(`"note":"again","done":1,"total":3,`, "p6"))
	if _, err := d.Exec("UPDATE tasks SET done_evidence = 'shipped', done_at = 1 WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	m := rpc.NewServer(auth{"manager"}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	m.DB = d
	Register(m, t.TempDir(), nil, nil)
	if r := call(t, m, `{"jsonrpc":"2.0","id":3,"method":"task.reopen","params":{"task":1,"note":"more","request_id":"o1"}}`); r[0]["error"] != nil {
		t.Fatalf("reopen: %v", r[0])
	}
	if got := todo(1); got != nil {
		t.Fatalf("a reopened task starts with no count: %+v", got)
	}
	for i, bad := range []string{`"done":1,`, `"total":1,`, `"done":3,"total":2,`, `"done":-1,"total":2,`, `"done":1.5,"total":2,`, `"done":"1","total":2,`, `"done":0,"total":0,`} {
		r := call(t, s, prog(`"note":"x",`+bad, fmt.Sprintf("b%d", i)))
		e, _ := r[0]["error"].(map[string]any)
		if e == nil || e["data"].(map[string]any)["code"] != "bad_request" {
			t.Fatalf("%s: %v", bad, r[0])
		}
	}
}
