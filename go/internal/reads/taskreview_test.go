package reads

import (
	"testing"

	"github.com/handline/handline/go/data/db"
)

// task.get, and its parent's subtasks, say how a story's sub-task stands with the founder: Ready while their turn from the Ready is on top,
// the redos counted, OK once the newest verdict is.
func TestATaskGetCarriesItsReview(t *testing.T) {
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'story', 'amy', 'p1', 1), (2, 'a', 'bob', 'amy', 1)")
	run(t, e, "UPDATE tasks SET parent_task = 1 WHERE id = 2")
	get := func(tid int64) map[string]any { return taskDetail(t, e, tid) }
	if g := get(2); g["review"] != nil {
		t.Fatalf("%v", g["review"])
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.AddReady(e.DB, 2, "bob", "lander", 5))
	must(db.PushTurn(e.DB, 2, db.Turn{Agent: "p1", By: "bob", Since: 1, Memo: 5}))
	r := get(2)["review"].(map[string]any)
	if r["ready"] != true || r["land_by"] != "lander" || r["comment"] != int64(5) || r["redo"] != 0 || r["ok"] != false {
		t.Fatalf("%v", r)
	}
	if sub := get(1)["subtasks"].([]any)[0].(map[string]any)["review"].(map[string]any); sub["ready"] != true {
		t.Fatalf("%v", sub)
	}
	must(db.AddVerdict(e.DB, 2, "p1", db.VerdictRedo, 6))
	if _, _, _, err := db.PopTurn(e.DB, 2, "p1", db.TurnSendBack, "again", 0, 6); err != nil {
		t.Fatal(err)
	}
	if r := get(2)["review"].(map[string]any); r["ready"] != false || r["redo"] != 1 {
		t.Fatalf("%v", r)
	}
	must(db.AddReady(e.DB, 2, "bob", "lander", 7))
	must(db.PushTurn(e.DB, 2, db.Turn{Agent: "p1", By: "bob", Since: 1, Memo: 7}))
	must(db.AddVerdict(e.DB, 2, "p1", db.VerdictOK, 0))
	if r := get(2)["review"].(map[string]any); r["ready"] != false || r["ok"] != true || r["redo"] != 1 || r["comment"] != int64(7) {
		t.Fatalf("an OK ends the Ready even before its turn comes off: %v", r)
	}
}
