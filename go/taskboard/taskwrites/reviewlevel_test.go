package taskwrites

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
)

// Below every, a finish whose check passed, whose tests passed and that was never sent back may go on without the founder: at
// one_in_three every third still goes to the founder, at auto none; a miss, failed tests or an earlier Redo always go to the founder; the
// founder can still Redo one that went on, and a Redo moves the level up one.
func TestAReviewLevelBelowEveryLetsACheckedFinishSkipTheFounder(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker", "gp.lander")
	homes.Store(d, home)
	worker, lander := teamtest.Key(t, home, "gp.worker"), teamtest.Key(t, home, "gp.lander")
	gp, _, _ := team.WorkspaceID(d, "gp")
	if err := team.SetRule(d, gp, team.RuleLandBy, lander, "p1", 1); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 9; id++ {
		exec(t, d, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(%d, 'part', %q, 'gp', 1)", id, worker),
			fmt.Sprintf(`INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'gp', 'task_new', '{"task_id": %d, "draft": true}')`, id))
	}
	f := &fakeTeller{db: d}
	as := writesAs(t, d, home, f)
	checked := `"checked":"both themes, what is left out","tests":"pass",`
	finish := func(task int, report string) {
		t.Helper()
		if got := errText(as(worker, "task.handoff", fmt.Sprintf(`"task":%d,"to":%q,"note":"built it",%s`, task, lander, report))); got != "" {
			t.Fatalf("T%d: %s", task, got)
		}
	}
	review := func(task int64) db.Review {
		t.Helper()
		r, err := db.TaskReview(d, task)
		if err != nil || r == nil {
			t.Fatal(r, err)
		}
		return *r
	}
	skipped := func(task int64) bool { r := review(task); return !r.Ready && r.OK && r.Auto }
	if err := db.SetReviewLevel(d, worker, db.LevelOneInThree, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	for task := 1; task <= 6; task++ {
		finish(task, checked)
	}
	// The first clean finish since the level was set comes to the founder, then every third.
	for task, want := range map[int64]bool{1: false, 2: true, 3: true, 4: false, 5: true, 6: true} {
		if skipped(task) != want || review(task).Ready == want {
			t.Errorf("T%d at one in three: skipped %v, want %v", task, skipped(task), want)
		}
	}
	if count(t, d, fmt.Sprintf("SELECT COUNT(*) FROM tasks WHERE id = 2 AND owner = %q", lander)) != 1 {
		t.Error("a skipped finish did not go to its land_by agent")
	}
	if err := db.SetReviewLevel(d, worker, db.LevelAuto, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	finish(7, `"checked":"both themes","misses":"the dark theme cuts the label","tests":"pass",`)
	finish(8, `"checked":"both themes","tests":"fail",`)
	finish(9, `"tests":"pass",`)
	for _, task := range []int64{7, 8, 9} {
		if !review(task).Ready {
			t.Errorf("T%d went on without the founder at auto: %+v", task, review(task))
		}
	}
	exec(t, d, "UPDATE tasks SET done_evidence = 'landed', done_at = 2 WHERE id = 5")
	if got := errText(as("", "task.redo", `"task":5,"note":"x",`)); !strings.Contains(got, "is closed now") {
		t.Errorf("a Redo of a skipped task since closed: %q", got)
	}
	if got := errText(as(lander, "task.comment.add", `"task":6,"text":"merge now or after T7?","turn":"p1",`)); got != "" {
		t.Fatalf("the lander's question: %s", got)
	}
	if got := errText(as("", "task.redo", `"task":6,"note":"x",`)); !strings.Contains(got, "holds it now") {
		t.Errorf("a Redo over the lander's question to the founder: %q", got)
	}
	if got := errText(as("", "task.redo", `"task":2,"note":"the badge overlaps",`)); got != "" {
		t.Fatalf("the founder's Redo of a skipped task: %s", got)
	}
	if r := review(2); r.Redo != 1 || r.Auto {
		t.Fatalf("after its Redo: %+v", r)
	}
	if count(t, d, fmt.Sprintf("SELECT COUNT(*) FROM tasks WHERE id = 2 AND owner = %q", worker)) != 1 {
		t.Error("the Redo did not send it back to its worker")
	}
	if l, _ := db.ReviewLevel(d, worker); l.Level != db.LevelOneInThree {
		t.Errorf("a Redo at auto left the level at %s", l.Level)
	}
	finish(2, checked) // sent back once: the founder sees it, whatever the level
	if !review(2).Ready {
		t.Error("a finish after a Redo skipped the founder")
	}
	if got := errText(as("", "task.redo", `"task":2,"note":"again",`)); got != "" {
		t.Fatalf("a Redo on the Ready path: %s", got)
	}
	if l, _ := db.ReviewLevel(d, worker); l.Level != db.LevelEvery {
		t.Errorf("a Redo at one in three left the level at %s", l.Level)
	}
	if got := errText(as(worker, "task.handoff", fmt.Sprintf(`"task":2,"to":%q,"note":"x","tests":"maybe",`, lander))); !strings.Contains(got, "tests is pass or fail") {
		t.Errorf("tests maybe: %q", got)
	}
}

// A finish that would skip the founder but whose land_by agent cannot be asked rolls back whole, its automatic OK too.
func TestADeniedSkipLeavesNoAutomaticOK(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker", "gp.lander")
	homes.Store(d, home)
	worker, lander := teamtest.Key(t, home, "gp.worker"), teamtest.Key(t, home, "gp.lander")
	gp, _, _ := team.WorkspaceID(d, "gp")
	if err := team.SetRule(d, gp, team.RuleLandBy, lander, "p1", 1); err != nil {
		t.Fatal(err)
	}
	exec(t, d, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'part', %q, 'gp', 1)", worker),
		`INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'gp', 'task_new', '{"task_id": 1, "draft": true}')`)
	if err := db.SetReviewLevel(d, worker, db.LevelAuto, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	f := &fakeTeller{db: d, deny: "no edge"}
	s := rpc.NewServer(auth{worker}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	s.DB = d
	Register(s, home, f, nil)
	r := call(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.handoff","params":{"task":1,"to":%q,"note":"built","checked":"all","tests":"pass","request_id":"r1"}}`, lander))[0]
	if errText(r) == "" {
		t.Fatal("a denied ask went through")
	}
	if rv, _ := db.TaskReview(d, 1); rv != nil {
		t.Fatalf("the denied skip left %+v", rv)
	}
	if count(t, d, "SELECT COUNT(*) FROM events WHERE kind = 'task_verdict'") != 0 {
		t.Fatal("an automatic OK stayed")
	}
}

// Only the founder sets a review level, and only an agent's.
func TestOnlyTheFounderSetsAReviewLevel(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker")
	worker := teamtest.Key(t, home, "gp.worker")
	set := func(as, params string) map[string]any {
		s := rpc.NewServer(auth{as}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		RegisterReviewLevels(s)
		return call(t, s, `{"jsonrpc":"2.0","id":2,"method":"agent.review_set","params":{`+params+`"request_id":"r1"}}`)[0]
	}
	if errText(set(worker, fmt.Sprintf(`"agent":%q,"level":"auto",`, worker))) == "" {
		t.Error("an agent set its own level")
	}
	if errText(set("", `"agent":"p1","level":"auto",`)) == "" {
		t.Error("a level for a person")
	}
	if got := errText(set("", fmt.Sprintf(`"agent":%q,"level":"auto",`, worker))); got != "" {
		t.Fatal(got)
	}
	if l, _ := db.ReviewLevel(d, worker); l.Level != db.LevelAuto {
		t.Fatalf("%+v", l)
	}
}
