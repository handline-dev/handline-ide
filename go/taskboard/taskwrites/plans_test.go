package taskwrites

import (
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/db"
)

func planCall(id int, method, params, rid string) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":{%s"request_id":%q}}`, id, method, params, rid)
}

func errCode(r map[string]any) any {
	e, _ := r["error"].(map[string]any)
	if e == nil {
		return nil
	}
	return e["data"].(map[string]any)["code"]
}

func TestThePlanIsItsAssigneesToWriteAndANewVersionClearsTheOK(t *testing.T) {
	d, mk := discardEnv(t)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'worker', 'manager', 1), (2, 'mine', 'p1', 'p1', 1), "+
		"(3, 'over', 'worker', 'manager', 1)", "UPDATE tasks SET done_evidence = 'x' WHERE id = 3")
	r := call(t, mk("worker"),
		planCall(2, "task.plan", `"task":1,"text":"  change a  ",`, "a1"),
		planCall(3, "task.plan", `"task":1,"text":"change a",`, "a2"),
		planCall(4, "task.plan", `"task":1,"text":" ",`, "a3"),
		planCall(5, "task.plan", `"task":2,"text":"x",`, "a4"),
		planCall(6, "task.plan", `"task":3,"text":"x",`, "a5"),
		planCall(7, "task.plan", `"task":9,"text":"x",`, "a6"),
		planCall(8, "task.plan_ok", `"task":1,`, "a7"))
	if res, _ := r[0]["result"].(map[string]any); res == nil || res["version"].(float64) != 1 {
		t.Fatalf("%v", r[0])
	}
	for i, want := range []string{"refused", "bad_request", "refused", "refused", "not_found", "refused"} {
		if errCode(r[i+1]) != want {
			t.Fatalf("call %d: %v, want %s", i+3, r[i+1], want)
		}
	}
	if r := call(t, mk("other"), planCall(2, "task.plan", `"task":1,"text":"mine now",`, "b1")); errCode(r[0]) != "refused" {
		t.Fatalf("another agent wrote the plan: %v", r[0])
	}

	r = call(t, mk(""),
		planCall(2, "task.plan_ok", `"task":1,`, "c1"),
		planCall(3, "task.plan_ok", `"task":1,"ok":true,`, "c2"),
		planCall(4, "task.plan_ok", `"task":2,`, "c3"))
	if r[0]["error"] != nil || errCode(r[1]) != "refused" || errCode(r[2]) != "refused" {
		t.Fatalf("%v", r)
	}
	p, err := db.TaskPlan(d, 1)
	if err != nil || p == nil || p.OK == nil || p.Current().Text != "change a" {
		t.Fatalf("%+v %v", p, err)
	}
	call(t, mk("worker"), planCall(2, "task.plan", `"task":1,"text":"change a and b",`, "d1"))
	if p, _ = db.TaskPlan(d, 1); len(p.Versions) != 2 || p.OK != nil {
		t.Fatalf("a new version keeps the old one and clears the OK: %+v", p)
	}
	r = call(t, mk(""), planCall(2, "task.plan_ok", `"task":1,`, "e1"), planCall(3, "task.plan_ok", `"task":1,"ok":false,`, "e2"))
	if r[0]["error"] != nil || r[1]["error"] != nil {
		t.Fatalf("%v", r)
	}
	if p, _ = db.TaskPlan(d, 1); p.OK != nil {
		t.Fatal("ok false took nothing back")
	}
	// The OK is of the version read: a click on version 1 after version 2 came is refused.
	r = call(t, mk(""), planCall(2, "task.plan_ok", `"task":1,"version":1,`, "f1"), planCall(3, "task.plan_ok", `"task":1,"version":2,`, "f2"))
	if errCode(r[0]) != "refused" || r[1]["error"] != nil {
		t.Fatalf("%v", r)
	}
	// A new brief clears it: the plan was written for the old one.
	exec(t, d, `INSERT INTO events(ts, agent, kind, payload) VALUES(9, 'manager', 'edit_task', '{"task_id": 1, "body": {"old": "a", "new": "b"}}')`)
	if p, _ = db.TaskPlan(d, 1); p.OK != nil {
		t.Fatal("the OK outlived the brief it was given for")
	}
}

func TestAPlanOrCoreLogicAskWaitsBehindWhatTheAssigneeIsDoing(t *testing.T) {
	d, mk := discardEnv(t)
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at) VALUES(1, 'busy', 'worker', 'manager', 1, 'manager', 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(2, 'draft', 'worker', 'manager', 1), (3, 'mine', 'p1', 'p1', 1)",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('worker', 'IDLE', 1, 1)",
		"INSERT INTO requests(id, ts, requester, target, body, kind, state) VALUES(1, 1, 'manager', 'worker', 'busy', 'task', 'landed')",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 1)")
	r := call(t, mk(""),
		planCall(2, "task.plan_ask", `"task":2,`, "a1"),
		planCall(3, "task.plan_ask", `"task":2,`, "a2"),
		planCall(4, "task.core_ask", `"task":2,"note":" the parser ",`, "a3"),
		planCall(5, "task.core_ask", `"task":2,`, "a4"),
		planCall(6, "task.plan_ask", `"task":3,`, "a5"),
		planCall(7, "task.core_ask", `"task":2,"note":7,`, "a6"))
	if r[0]["error"] != nil || errCode(r[1]) != "refused" || r[2]["error"] != nil || errCode(r[3]) != "refused" || errCode(r[4]) != "refused" ||
		errCode(r[5]) != "bad_request" {
		t.Fatalf("%v", r)
	}
	plans, _ := db.TaskAsks(d, db.PlanAskKind, 2)
	cores, _ := db.TaskAsks(d, db.CoreAskKind, 2)
	draft, _ := db.GetTask(d, 2)
	worker := draft.Owner
	if len(plans) != 1 || plans[0].To != worker || plans[0].Behind != 1 || len(cores) != 1 || cores[0].Note != "the parser" || cores[0].Behind != 1 {
		t.Fatalf("plans %+v cores %+v", plans, cores)
	}
	if r := call(t, mk("worker"), planCall(2, "task.plan_ask", `"task":2,`, "b1")); errCode(r[0]) != "refused" {
		t.Fatalf("an agent asked: %v", r[0])
	}
	// Once sent, it may be asked again: the agent can have lost it.
	if err := db.MarkAskSent(d, plans[0], 5); err != nil {
		t.Fatal(err)
	}
	if r := call(t, mk(""), planCall(2, "task.plan_ask", `"task":2,`, "c1")); r[0]["error"] != nil {
		t.Fatalf("a sent ask was not asked again: %v", r[0])
	}
}

func TestADraftKeepsItsBriefAndAsksNobody(t *testing.T) {
	d, mk := discardEnv(t)
	f := &fakeTeller{}
	r := call(t, mk("manager", f),
		planCall(2, "task.new", `"owner":"worker","title":"later","body":"the brief","draft":true,`, "a1"),
		planCall(3, "task.new", `"owner":"human","title":"x","draft":true,`, "a2"),
		planCall(4, "task.new", `"owner":"worker","title":"y","draft":"yes",`, "a3"))
	if r[0]["error"] != nil || errCode(r[1]) != "refused" || errCode(r[2]) != "bad_request" {
		t.Fatalf("%v", r)
	}
	tid := int64(r[0]["result"].(map[string]any)["task"].(float64))
	task, err := db.GetTask(d, tid)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := db.TaskState(d, task); st != "draft" {
		t.Fatalf("state %q", st)
	}
	for q, want := range map[string]int{
		fmt.Sprintf("SELECT COUNT(*) FROM task_requests l JOIN requests r ON r.id = l.request_id WHERE l.task_id = %d AND r.body = 'the brief' AND r.state = 'withdrawn'", tid): 1,
		"SELECT COUNT(*) FROM requests WHERE state = 'queued'":                                         0, // the owner is not asked
		"SELECT COUNT(*) FROM agent_state WHERE current_task IS NOT NULL":                              0,
		fmt.Sprintf("SELECT COUNT(*) FROM tasks WHERE id = %d AND queued_by IS NULL", tid):             1,
		"SELECT COUNT(*) FROM events WHERE kind = 'task_new' AND json_extract(payload, '$.draft') = 1": 1, // a story's sub-task (db.OpenedAsDraft)
	} {
		if got := count(t, d, q); got != want {
			t.Errorf("%s: %d, want %d", q, got, want)
		}
	}
	// Queuing it runs it: it is no longer a draft.
	r = call(t, mk("manager", f), planCall(2, "task.queue", fmt.Sprintf(`"tasks":[%d],`, tid), "b1"))
	if r[0]["error"] != nil {
		t.Fatalf("%v", r[0])
	}
	task, _ = db.GetTask(d, tid)
	if st, _ := db.TaskState(d, task); st == "draft" {
		t.Fatal("still a draft after task.queue")
	}
	// A founder's draft is a draft too.
	r = call(t, mk("", f), planCall(2, "task.new", `"owner":"worker","title":"the founder's","draft":true,`, "c1"))
	tid = int64(r[0]["result"].(map[string]any)["task"].(float64))
	if task, _ = db.GetTask(d, tid); task.QueuedBy.Valid {
		t.Fatal("the founder's draft was queued")
	}
}
