package taskwrites

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/attachments"
)

// A story's sub-task its assignee hands to the land_by agent is Ready for the founder: Redo sends it back with a count, the next finish is
// Ready again, and OK hands it to the land_by agent; after the OK, and for any other task, the handoff goes as asked.
func TestAStorysSubTaskWaitsForTheFoundersOKBeforeItGoesToLand(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker", "gp/lander")
	homes.Store(d, home)
	k := func(n string) string { return teamtest.Key(t, home, n) }
	worker, lander := k("gp/worker"), k("gp/lander")
	gp, _, _ := team.WorkspaceID(d, "gp")
	if err := team.SetRule(d, gp, team.RuleLandBy, lander, "p1", 1); err != nil {
		t.Fatal(err)
	}
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'story part', 'gp/worker', 'gp', 1), (2, 'plain', 'gp/worker', 'gp', 1)",
		`INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'gp', 'task_new', '{"task_id": 1, "draft": true}')`,
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('gp/worker', 'IDLE', 1, 1)")
	dir := paths.AgentDir(home, "gp/worker")
	for name, b := range map[string]string{"demo.png": "\x89PNG\r\n\x1a\nxx", "demo.mp4": "\x00\x00\x00\x18ftypisom\x00\x00", "fake.mov": "not a video"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeTeller{db: d}
	as := writesAs(t, d, home, f)
	hand := func(task int, to, note string, files ...string) map[string]any {
		t.Helper()
		var abs []string
		for _, f := range files {
			abs = append(abs, fmt.Sprintf("%q", filepath.Join(dir, f)))
		}
		return as(worker, "task.handoff", fmt.Sprintf(`"task":%d,"to":%q,"note":%q,"files":[%s],`, task, to, note, strings.Join(abs, ",")))
	}
	review := func() db.Review {
		t.Helper()
		r, err := db.TaskReview(d, 1)
		if err != nil || r == nil {
			t.Fatal(r, err)
		}
		return *r
	}
	said := func(r map[string]any) string {
		t.Helper()
		res, _ := r["result"].(map[string]any)
		if res == nil {
			t.Fatalf("%v", r)
		}
		return res["said"].(string)
	}

	if got := errText(hand(1, lander, "")); !strings.Contains(got, "say in note what you did") {
		t.Fatalf("a finish with no result: %q", got)
	}
	if errCode(hand(1, lander, "built", "fake.mov")) != "bad_request" {
		t.Fatal("a .mov that is not a video was taken")
	}
	if errCode(as("", "task.ok", `"task":1,`)) != "refused" {
		t.Fatal("OK on a task that is not Ready")
	}
	if got := said(hand(1, lander, "built it; before and after attached", "demo.png", "demo.mp4")); !strings.HasPrefix(got, "T1 is Ready for the founder (comment ") {
		t.Fatalf("%q", got)
	}
	r := review()
	if !r.Ready || r.LandBy != lander || r.Redo != 0 || r.OK {
		t.Fatalf("%+v", r)
	}
	if acting, _ := db.Acting(d, 1); acting != "p1" || count(t, d, "SELECT COUNT(*) FROM tasks WHERE id = 1 AND owner = 'gp/worker'") != 1 {
		t.Fatalf("the founder's turn is on the worker's task: acting %s", acting)
	}
	files, _ := attachments.CommentFiles(d, r.Comment)
	st := attachments.New(home)
	if len(files) != 2 || st.FileMap(files[1], false)["kind"] != "video" || st.FileMap(files[1], false)["poster"] != files[0].ID {
		t.Fatalf("the result carries the picture and the video, the picture its poster: %+v", files)
	}
	if errCode(hand(1, lander, "again")) != "refused" {
		t.Fatal("an agent moved a Ready task on")
	}
	f.deny = "no edge"
	if errCode(as("", "task.ok", `"task":1,`)) != "refused" || errCode(as("", "task.redo", `"task":1,"note":"x",`)) != "refused" {
		t.Fatal("a denied verdict went through")
	}
	f.deny = ""
	if r := review(); !r.Ready || r.Redo != 0 || r.OK {
		t.Fatalf("a refused verdict left its writes: %+v", r)
	}
	// The founder's comment on a Ready task is a question to its worker: still Ready, their turn still on the result. Only OK or Redo answers
	// it: Give back and a comment that hands a turn are refused.
	turns := func() string {
		t.Helper()
		ts, err := db.Turns(d, 1)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(ts)
	}
	before, comments := turns(), len(f.comments)
	if as("", "task.comment.add", `"task":1,"text":"why the yellow?",`)["error"] != nil {
		t.Fatal("the founder's comment")
	}
	if !review().Ready || turns() != before || len(f.comments) != comments+1 || !strings.Contains(f.comments[comments], `"why the yellow?"`) {
		t.Fatalf("the question ended Ready or was not told: %v %v", turns(), f.comments)
	}
	if acting, _ := db.Acting(d, 1); acting != "p1" {
		t.Fatalf("the founder's question passed their turn to %s", acting)
	}
	if as(worker, "task.comment.add", `"task":1,"text":"it marks the beta",`)["error"] != nil || !review().Ready {
		t.Fatal("the worker's answer")
	}
	for _, refused := range []string{
		errText(as("", "task.turn.pop", `"task":1,"outcome":"pass",`)),
		errText(as("", "task.comment.add", fmt.Sprintf(`"task":1,"text":"ask the lander","turn":%q,`, lander))),
	} {
		if refused != "T1 is Ready for you: answer it with OK, merge or Redo" {
			t.Fatalf("%q", refused)
		}
	}
	if !review().Ready || turns() != before {
		t.Fatal("a refused answer moved the Ready task")
	}
	if errCode(as(worker, "task.redo", `"task":1,"note":"x",`)) != "refused" || errCode(as(worker, "task.ok", `"task":1,`)) != "refused" {
		t.Fatal("OK and Redo are the founder's")
	}
	if errCode(as("", "task.redo", `"task":1,`)) != "bad_request" {
		t.Fatal("a redo with no note")
	}

	if got := said(as("", "task.redo", `"task":1,"note":"the badge overlaps on the phone",`)); got != "T1 sent back to gp/worker: Redo 1" {
		t.Fatalf("%q", named(d, got))
	}
	if r = review(); r.Ready || r.Redo != 1 {
		t.Fatalf("%+v", r)
	}
	if acting, _ := db.Acting(d, 1); acting != worker {
		t.Fatalf("redo goes back to the worker, not %s", acting)
	}
	if h := f.handed[len(f.handed)-1]; h.Task != 1 || h.To != "gp/worker" || h.RID == 0 {
		t.Fatalf("the worker is not moved on to the redo: %+v", h)
	}
	if got := errText(as(k("gp"), "task.handoff", fmt.Sprintf(`"task":1,"to":%q,"note":"ship it",`, lander))); !strings.Contains(got, "who works it, finishes it") {
		t.Fatal("the lead skipped the founder")
	}
	if count(t, d, "SELECT COUNT(*) FROM requests WHERE target = 'gp/worker' AND body LIKE 'T1 대표님 Redo 1 (댓글 %): the badge overlaps on the phone%First keep one lesson%hl lesson add \"<the lesson>\" --task T1%'") != 1 {
		t.Fatal("the worker is not told the redo")
	}

	if got := said(hand(1, lander, "fixed the badge")); !strings.HasPrefix(got, "T1 is Ready for the founder") {
		t.Fatalf("after a redo the finish is Ready again: %q", got)
	}
	if got := said(as("", "task.ok", `"task":1,`)); named(d, got) != "T1 is OK: handed to gp/lander to merge" {
		t.Fatalf("%q", named(d, got))
	}
	if r = review(); r.Ready || !r.OK || r.Redo != 1 {
		t.Fatalf("%+v", r)
	}
	if count(t, d, "SELECT COUNT(*) FROM tasks WHERE id = 1 AND owner = 'gp/lander'") != 1 ||
		count(t, d, "SELECT COUNT(*) FROM requests WHERE target = 'gp/lander' AND body LIKE '%OK, merge (the founder). fixed the badge%'") != 1 {
		t.Fatal("OK hands the task to the land_by agent with the result")
	}
	if count(t, d, "SELECT COUNT(*) FROM task_turns WHERE task_id = 1") != 0 {
		t.Fatal("the founder's turn stayed on")
	}

	// After the OK, the lander's send-back and the worker's next finish go as asked; so does a task that is not a story's.
	if as(lander, "task.handoff", fmt.Sprintf(`"task":1,"to":%q,"note":"rebase please",`, worker))["error"] != nil {
		t.Fatal("the lander could not send it back")
	}
	if got := said(hand(1, lander, "rebased")); !strings.HasPrefix(named(d, got), "T1 handed to gp/lander") {
		t.Fatalf("%q", named(d, got))
	}
	if got := said(hand(2, lander, "plain work")); !strings.HasPrefix(named(d, got), "T2 handed to gp/lander") {
		t.Fatalf("%q", named(d, got))
	}
	if errCode(hand(2, k("gp"), "x", "demo.png")) != "bad_request" {
		t.Fatal("files went with a handoff nobody reads them in")
	}
}

// With the review flow on, any task that serves the founder is Ready for them when its assignee finishes it to the land_by agent, as a
// story's sub-task is: OK hands it on, Redo sends it back. Off, it goes as asked; a task of agents only goes as asked either way.
func TestWithTheReviewFlowAFinishOfTheFoundersWorkIsReady(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker", "gp/lander")
	homes.Store(d, home)
	k := func(n string) string { return teamtest.Key(t, home, n) }
	worker, lander := k("gp/worker"), k("gp/lander")
	gp, _, _ := team.WorkspaceID(d, "gp")
	if err := team.SetRule(d, gp, team.RuleLandBy, lander, "p1", 1); err != nil {
		t.Fatal(err)
	}
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, parent_task, created_at) VALUES(1, 'asked', 'gp/worker', 'p1', NULL, 1), "+
		"(2, 'part', 'gp/worker', 'gp', 1, 1), (3, 'below it', 'gp/worker', 'gp', 2, 1), (4, 'agents only', 'gp/worker', 'gp', NULL, 1)")
	for tid := range int64(4) {
		if err := db.SetTaskRequest(d, tid+1); err != nil { // the request task.new gives each
			t.Fatal(err)
		}
	}
	as := writesAs(t, d, home, &fakeTeller{db: d})
	hand := func(task int) string {
		t.Helper()
		r := as(worker, "task.handoff", fmt.Sprintf(`"task":%d,"to":%q,"note":"done",`, task, lander))
		res, _ := r["result"].(map[string]any)
		if res == nil {
			t.Fatalf("%v", r)
		}
		return named(d, res["said"].(string))
	}

	if got := hand(2); !strings.HasPrefix(got, "T2 handed to gp/lander") {
		t.Fatalf("with the flow off the founder's work goes as asked: %q", got)
	}
	cfg, err := os.OpenFile(paths.ConfigFile(home), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.WriteString("\n" + team.ReviewFlowKey + ": true\n"); err != nil || cfg.Close() != nil {
		t.Fatal(err)
	}
	if got := hand(3); !strings.HasPrefix(got, "T3 is Ready for the founder") {
		t.Fatalf("a task below the founder's request: %q", got)
	}
	if got := named(d, as("", "task.ok", `"task":3,`)["result"].(map[string]any)["said"].(string)); got != "T3 is OK: handed to gp/lander to merge" {
		t.Fatalf("%q", got)
	}
	if got := hand(1); !strings.HasPrefix(got, "T1 is Ready for the founder") {
		t.Fatalf("the founder's own request: %q", got)
	}
	if got := named(d, as("", "task.redo", `"task":1,"note":"not yet",`)["result"].(map[string]any)["said"].(string)); got != "T1 sent back to gp/worker: Redo 1" {
		t.Fatalf("%q", got)
	}
	if got := hand(4); !strings.HasPrefix(got, "T4 handed to gp/lander") {
		t.Fatalf("a task of agents only goes as asked: %q", got)
	}
}

// writesAs calls one task write as an agent ("" is the founder) on a server of its own over d, each call with a request id of its own.
func writesAs(t *testing.T, d *sql.DB, home string, f Teller) func(agent, method, params string) map[string]any {
	n := 0
	return func(agent, method, params string) map[string]any {
		t.Helper()
		n++
		s := rpc.NewServer(auth{agent}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		Register(s, home, f, nil)
		return call(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":%q,"params":{%s"request_id":"r%d"}}`, method, params, n))[0]
	}
}
