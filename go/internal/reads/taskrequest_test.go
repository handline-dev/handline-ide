package reads

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// Task.request reads back on the row; tasks.list request=N is N's tree as requests.list counts it, ended tasks too; and requests.list
// takes a task whose request is itself as a request.
func TestTheTasksThatServeARequestAreItsTree(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	at := func(n int64) string { return itoa(now - n) }
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, parent_task, created_at, request, done_evidence, done_at) VALUES"+
		"(1, 'made from a message', 'lead', 'lead', NULL, "+at(500)+", 1, NULL, NULL), "+
		"(2, 'build', 'dev', 'lead', 1, "+at(400)+", 1, 'built', "+at(300)+"), "+
		"(3, 'audit', 'auditor', 'dev', NULL, "+at(350)+", 1, NULL, NULL), "+
		"(4, 'ops', 'dev', 'dev', NULL, "+at(200)+", NULL, NULL, NULL), "+
		"(5, 'made before the field', 'dev', 'lead', 2, "+at(390)+", NULL, NULL, NULL), "+
		"(6, 'under the audit', 'dev', 'auditor', 3, "+at(340)+", NULL, NULL, NULL)")
	got, err := e.tasksList(asFounder, map[string]any{"request": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	var ids, reqs []string
	for _, it := range got.(map[string]any)["items"].([]any) {
		row := it.(map[string]any)
		ids = append(ids, fmt.Sprint(row["id"]))
		reqs = append(reqs, fmt.Sprint(row["request"]))
	}
	if fmt.Sprint(ids) != "[6 5 3 2 1]" || fmt.Sprint(reqs) != "[<nil> <nil> 1 1 1]" {
		t.Errorf("request 1's tree: %v, requests %v", ids, reqs)
	}
	rows, err := e.RequestRows(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["task"] != int64(1) || rows[0]["open"] != 3 || rows[0]["done"] != 1 || rows[0]["now"] != "dev" {
		t.Errorf("one request, T1, with the build done and three open: %v", rows)
	}
}

// A message that gave a request its turn_origin is that request's row, not a message row of its own as well.
func TestAMessageATurnMadeARequestIsListedOnce(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	run(t, e, "INSERT INTO requests(id, ref, ts, requester, target, body, state, kind) VALUES(7, 'aaaaaaaaaaa7', "+itoa(now-100)+", 'p1', 'lead', 'new ask', 'landed', 'say')")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, request, turn_origin) VALUES(1, 'from the ask', 'dev', 'lead', "+itoa(now-90)+", 1, 7)")
	rows, err := e.RequestRows(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["task"] != int64(1) || rows[0]["ref"] != "msg_aaaaaaaaaaa7" || rows[0]["origin"] != "lead" { // local-ref-ok
		t.Errorf("one row, T1, from msg 7: %v", rows)
	}
}

// A request a founder's comment started has the comment's tell as turn_origin (a sys message, or the task's return that carried it): its row
// names no message.
func TestARequestAFounderCommentStartedNamesNoMessage(t *testing.T) {
	t.Parallel()
	for _, tell := range []string{"'sys', 'lead', 'tell', 'landed', 'say'", "'p1', 'lead', 'back to you', 'landed', 'task'"} {
		e := testEnv(t)
		now := time.Now().Unix()
		run(t, e, "INSERT INTO requests(id, ref, ts, requester, target, body, state, kind) VALUES(7, 'aaaaaaaaaaa7', "+itoa(now-100)+", "+tell+")")
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, request, turn_origin) VALUES(1, 'from the comment', 'dev', 'lead', "+itoa(now-90)+", 1, 7)")
		rows, err := e.RequestRows(now)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0]["task"] != int64(1) || rows[0]["ref"] != nil || rows[0]["origin"] != nil {
			t.Errorf("%s: one row, T1, no message: %v", tell, rows)
		}
	}
}
