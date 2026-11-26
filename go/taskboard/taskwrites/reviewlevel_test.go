package taskwrites

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
)

// Below every, a finish whose check passed, whose tests passed and that was never sent back may go on without the founder: at
// one_in_three every third still goes to the founder, at never none; a miss, failed tests or an earlier Redo always go to the founder; the
// founder can still Redo one that went on, and a Redo puts the level back to every at once.
func TestAReviewLevelBelowEveryLetsACheckedFinishSkipTheFounder(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker", "gp/lander")
	homes.Store(d, home)
	commit := workRepo(t, home, "gp/worker")
	worker, lander := teamtest.Key(t, home, "gp/worker"), teamtest.Key(t, home, "gp/lander")
	gp, _, _ := team.WorkspaceID(d, "gp")
	if err := team.SetRule(d, gp, team.RuleLandBy, lander, "p1", 1); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 9; id++ {
		commit(id, fmt.Sprintf("go/p%d.go", id))
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
	model, _ := team.NewRosterOn(d, home).Model(worker)
	level := func() string { l, _ := db.ReviewLevel(d, worker, db.WorkDaemon, model); return l.Level }
	if err := db.SetReviewLevel(d, worker, db.WorkDaemon, model, db.LevelOneInThree, "p1", "set", 0); err != nil {
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
	if err := db.SetReviewLevel(d, worker, db.WorkDaemon, model, db.LevelNever, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	finish(7, `"checked":"both themes","misses":"the dark theme cuts the label","tests":"pass",`)
	finish(8, `"checked":"both themes","tests":"fail",`)
	finish(9, `"tests":"pass",`)
	for _, task := range []int64{7, 8, 9} {
		if !review(task).Ready {
			t.Errorf("T%d went on without the founder at never: %+v", task, review(task))
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
	if level() != db.LevelEvery {
		t.Errorf("a Redo of a skipped task at never left the level at %s", level())
	}
	finish(2, checked) // sent back once: the founder sees it, whatever the level
	if !review(2).Ready {
		t.Error("a finish after a Redo skipped the founder")
	}
	if got := errText(as("", "task.redo", `"task":2,"note":"again",`)); got != "" {
		t.Fatalf("a Redo on the Ready path: %s", got)
	}
	if n := count(t, d, "SELECT COUNT(*) FROM review_levels WHERE why = 'redo'"); n != 1 {
		t.Errorf("a Redo at every wrote %d redo rows, want the one", n)
	}
	if got := errText(as(worker, "task.handoff", fmt.Sprintf(`"task":2,"to":%q,"note":"x","tests":"maybe",`, lander))); !strings.Contains(got, "tests is pass or fail") {
		t.Errorf("tests maybe: %q", got)
	}
}

// A finish that would skip the founder but whose land_by agent cannot be asked rolls back whole, its automatic OK too.
func TestADeniedSkipLeavesNoAutomaticOK(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker", "gp/lander")
	homes.Store(d, home)
	worker, lander := teamtest.Key(t, home, "gp/worker"), teamtest.Key(t, home, "gp/lander")
	gp, _, _ := team.WorkspaceID(d, "gp")
	if err := team.SetRule(d, gp, team.RuleLandBy, lander, "p1", 1); err != nil {
		t.Fatal(err)
	}
	workRepo(t, home, "gp/worker")(1, "go/a.go")
	exec(t, d, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'part', %q, 'gp', 1)", worker),
		`INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'gp', 'task_new', '{"task_id": 1, "draft": true}')`)
	model, _ := team.NewRosterOn(d, home).Model(worker)
	if err := db.SetReviewLevel(d, worker, db.WorkDaemon, model, db.LevelNever, "p1", "set", 0); err != nil {
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

// Only the founder sets a review level, only an agent's, at one kind of work, and never below every for contract and data; agent.review
// gives each kind the agent has done its level and record.
func TestOnlyTheFounderSetsAReviewLevel(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker")
	worker := teamtest.Key(t, home, "gp/worker")
	server := func(as string) *rpc.Server {
		s := rpc.NewServer(auth{as}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		RegisterReviewLevels(s, home)
		return s
	}
	set := func(as, params string) map[string]any {
		return call(t, server(as), `{"jsonrpc":"2.0","id":2,"method":"agent.review_set","params":{`+params+`"request_id":"r1"}}`)[0]
	}
	if errText(set(worker, fmt.Sprintf(`"agent":%q,"work":"screen","level":"never",`, worker))) == "" {
		t.Error("an agent set its own level")
	}
	if errText(set("", `"agent":"p1","work":"screen","level":"never",`)) == "" {
		t.Error("a level for a person")
	}
	if got := errText(set("", fmt.Sprintf(`"agent":%q,"level":"never",`, worker))); !strings.Contains(got, "work is") {
		t.Errorf("no work: %q", got)
	}
	if got := errText(set("", fmt.Sprintf(`"agent":%q,"work":"contract","level":"one_in_two",`, worker))); !strings.Contains(got, "always looked at") {
		t.Errorf("contract below every: %q", got)
	}
	if got := errText(set("", fmt.Sprintf(`"agent":%q,"work":"screen","level":"one_in_five",`, worker))); got != "" {
		t.Fatal(got)
	}
	model, _ := team.NewRosterOn(d, home).Model(worker)
	if l, _ := db.ReviewLevel(d, worker, db.WorkScreen, model); l.Level != db.LevelOneInFive {
		t.Fatalf("%+v", l)
	}
	if l, _ := db.ReviewLevel(d, worker, db.WorkDaemon, model); l.Level != db.LevelEvery {
		t.Fatalf("another kind moved: %+v", l)
	}
	got := call(t, server(""), fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"agent.review","params":{"agent":%q}}`, worker))[0]
	if got := fmt.Sprint(got["result"]); got != "map[agent:"+worker+" ladder:[map[level:one_in_five looks:0 record:[] suggested:<nil> work:screen]] lessons:0]" {
		t.Fatalf("agent.review %s", got)
	}
}

// The kinds of work a finish is come from the files of its commits in the agent's repo; it is looked at as the strictest of their levels,
// at 1 in n the first since that level and every n-th after; one of no known kind always goes to the founder; a Redo puts each of the
// task's kinds back to every and no other; a changed model starts every kind at every.
func TestAFinishIsSampledByTheStrictestLevelOfTheKindsItsCommitsAre(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker", "gp/lander")
	homes.Store(d, home)
	commit := workRepo(t, home, "gp/worker")
	worker, lander := teamtest.Key(t, home, "gp/worker"), teamtest.Key(t, home, "gp/lander")
	gp, _, _ := team.WorkspaceID(d, "gp")
	if err := team.SetRule(d, gp, team.RuleLandBy, lander, "p1", 1); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 20; id++ {
		exec(t, d, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(%d, 'part', %q, 'gp', 1)", id, worker),
			fmt.Sprintf(`INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'gp', 'task_new', '{"task_id": %d, "draft": true}')`, id))
	}
	as := writesAs(t, d, home, &fakeTeller{db: d})
	finish := func(task int) bool {
		t.Helper()
		if got := errText(as(worker, "task.handoff", fmt.Sprintf(`"task":%d,"to":%q,"note":"built it","checked":"all","tests":"pass",`, task, lander))); got != "" {
			t.Fatalf("T%d: %s", task, got)
		}
		r, _ := db.TaskReview(d, int64(task))
		return r != nil && r.Auto
	}
	model, _ := team.NewRosterOn(d, home).Model(worker)
	set := func(work, level string) {
		t.Helper()
		if err := db.SetReviewLevel(d, worker, work, model, level, "p1", "set", 0); err != nil {
			t.Fatal(err)
		}
	}
	level := func(work string) string { l, _ := db.ReviewLevel(d, worker, work, model); return l.Level }
	set(db.WorkScreen, db.LevelOneInTwo)
	set(db.WorkDocs, db.LevelOneInFive)
	var got []bool
	for task := 1; task <= 4; task++ { // screen alone
		commit(task, fmt.Sprintf("apps/a%d.tsx", task))
		got = append(got, finish(task))
	}
	for task := 5; task <= 10; task++ { // docs alone
		commit(task, fmt.Sprintf("docs/a%d.md", task))
		got = append(got, finish(task))
	}
	if want := []bool{false, true, false, true, false, true, true, true, true, false}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("skipped %v, want %v", got, want)
	}
	commit(11, "docs/b.md", "go/b.go") // docs at 1 in 5 and daemon at every: every
	if finish(11) {
		t.Fatal("a docs and daemon task skipped the founder at daemon's every")
	}
	commit(12, "docs/c.md", "contract/spec/x.json")
	set(db.WorkContract, db.LevelNever) // a row cannot lower the floor
	if finish(12) {
		t.Fatal("a contract task skipped the founder")
	}
	set(db.WorkDaemon, db.LevelNever)
	commit(13, "apps/m.tsx", "go/m.go")
	finish(13)
	if got := errText(as("", "task.redo", `"task":13,"note":"the badge overlaps",`)); got != "" {
		t.Fatalf("the founder's Redo: %s", got)
	}
	if level(db.WorkScreen) != db.LevelEvery || level(db.WorkDaemon) != db.LevelEvery || level(db.WorkDocs) != db.LevelOneInFive {
		t.Fatalf("after a Redo of a screen and daemon task: screen %s, daemon %s, docs %s", level(db.WorkScreen), level(db.WorkDaemon), level(db.WorkDocs))
	}
	set(db.WorkDaemon, db.LevelNever)
	if finish(14) { // no commit names it: its kinds are unknown
		t.Fatal("a task of no known kind skipped the founder")
	}
	teamtest.Set(t, home, "gp/worker", "model", "another-model")
	other, _ := team.NewRosterOn(d, home).Model(worker)
	if l, _ := db.ReviewLevel(d, worker, db.WorkDocs, other); other == model || l.Level != db.LevelEvery {
		t.Fatalf("after a model change docs is %s", l.Level)
	}
}

// workRepo gives agent a git repo to work in, and commit, which commits files naming task as its last parenthesis.
func workRepo(t *testing.T, home, agent string) (commit func(task int, files ...string)) {
	t.Helper()
	repo := t.TempDir()
	teamtest.Set(t, home, agent, "repo", repo)
	git := func(args ...string) {
		t.Helper()
		if r := gitrun.Run(repo, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...); r.RC != 0 {
			t.Fatalf("git %v: %s", args, r.Stderr)
		}
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "base")
	return func(task int, files ...string) {
		t.Helper()
		for _, f := range files {
			p := filepath.Join(repo, f)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(fmt.Sprint(task)), 0o644); err != nil {
				t.Fatal(err)
			}
			git("add", f)
		}
		git("commit", "-q", "-m", fmt.Sprintf("part (T%d)", task))
	}
}
