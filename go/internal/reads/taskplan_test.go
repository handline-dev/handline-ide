package reads

import (
	"encoding/json"
	"testing"

	"github.com/handline/handline/go/data/db"
)

func TestATaskGetCarriesItsPlanItsOKAndTheAsksForThem(t *testing.T) {
	e := testEnv(t) // not parallel: it sets db.Now
	was := db.Now
	t.Cleanup(func() { db.Now = was })
	addAgent(t, e, "bob", "")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'story', 'amy', 'p1', 1), (2, 'a', 'bob', 'amy', 1), (3, 'b', 'bob', 'amy', 1)")
	run(t, e, "UPDATE tasks SET parent_task = 1 WHERE id IN (2, 3)")
	get := func(tid int64) map[string]any { return taskDetail(t, e, tid) }
	if g := get(2); g["plan_state"] != "none" || g["plan"] != nil || g["plan_ask"] != nil || g["core_ask"] != nil {
		t.Fatalf("%v %v %v %v", g["plan_state"], g["plan"], g["plan_ask"], g["core_ask"])
	}
	ask, err := db.AddTaskAsk(e.DB, db.PlanAskKind, 2, "sys", "bob", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if g := get(2); g["plan_state"] != "asked" || g["plan_ask"].(map[string]any)["sent_at"] != nil {
		t.Fatalf("%v %v", g["plan_state"], g["plan_ask"])
	}
	// A million haiku input tokens is a dollar: bob's calls between the ask's delivery and the plan are what the plan cost.
	run(t, e, "UPDATE events SET ts = 100 WHERE id = ?", ask)
	db.Now = func() int64 { return 100 }
	if err := db.MarkAskSent(e.DB, &db.TaskAsk{ID: ask, Task: 2, By: "sys"}, 7); err != nil {
		t.Fatal(err)
	}
	if g := get(2); g["plan_state"] != "writing" || g["plan_ask"].(map[string]any)["sent_at"] != "1970-01-01T00:01:40Z" {
		t.Fatalf("%v %v", g["plan_state"], g["plan_ask"])
	}
	db.Now = func() int64 { return 200 }
	for _, text := range []string{"first", "second"} {
		if _, err := db.AddPlan(e.DB, 2, "bob", text); err != nil {
			t.Fatal(err)
		}
	}
	run(t, e, "INSERT INTO token_calls(account, session, req, ts, model, cwd, inp, cc, cr, out, thinking, agent) "+
		"VALUES('g', 's', 'r1', 150, 'claude-haiku-5-5', '/w', 2000000, 0, 0, 0, 0, 'bob'), ('g', 's', 'r2', 300, 'claude-haiku-5-5', '/w', 5000000, 0, 0, 0, 0, 'bob')")
	if cost, err := e.planCost(); err != nil || cost != 2.0 {
		t.Fatalf("plan cost %v %v", cost, err)
	}
	p, _ := db.TaskPlan(e.DB, 2)
	if err := db.MarkPlan(e.DB, 2, "p1", p.Current().ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddTaskAsk(e.DB, db.CoreAskKind, 2, "p1", "bob", "the parser", 0); err != nil {
		t.Fatal(err)
	}
	g := get(2)
	plan := g["plan"].(map[string]any)
	if g["plan_state"] != "ok" || g["plan_ask"] != nil || plan["text"] != "second" || plan["version"] != 2 || plan["ok"].(map[string]any)["by"] != "p1" ||
		len(plan["versions"].([]any)) != 2 || plan["versions"].([]any)[0].(map[string]any)["text"] != "first" {
		t.Fatalf("%v %v", g["plan_state"], plan)
	}
	if core := g["core_ask"].(map[string]any); core["note"] != "the parser" || core["to"] != "bob" || core["sent_at"] != nil {
		t.Fatalf("%v", core)
	}
	subs := get(1)["subtasks"].([]any)
	if subs[0].(map[string]any)["plan_state"] != "ok" || subs[1].(map[string]any)["plan_state"] != "none" {
		t.Fatalf("%v", subs)
	}
}

// taskDetail is task.get of task tid as the founder reads it.
func taskDetail(t *testing.T, e *Env, tid int64) map[string]any {
	t.Helper()
	got, err := e.taskGet(nil, map[string]any{"task": json.Number(itoa(tid))})
	if err != nil {
		t.Fatal(err)
	}
	return got.(map[string]any)
}
