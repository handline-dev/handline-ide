package supervisor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/gitrun"
)

// A story's sub-task with a plan goes out with its brief and that plan, marked OK or not with its version, typed whole; another queued task
// goes out as its title.
func TestRunDeliversAStorysSubTaskWithItsPlan(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	main := func() string { return "main" }
	tid, _ := db.NewTask(r.DB, "story part", r.k("bob"), r.k("alice"), 0, db.DefaultPriority, "", 0, "")
	brief := "Draw the badge. " + strings.Repeat("It sits on the card's corner. ", 20)
	rid, err := db.AddRequest(r.DB, db.NewRequest{Requester: r.k("alice"), Target: r.k("bob"), Body: brief, Kind: "task", State: "withdrawn"})
	r.must(err)
	r.must(db.LinkRequest(r.DB, tid, rid))
	_, err = db.AddEvent(r.DB, r.k("alice"), "task_new", db.KV{K: "task_id", V: tid}, db.KV{K: "draft", V: true})
	r.must(err)
	_, err = db.AddPlan(r.DB, tid, r.k("bob"), "change badge.tsx")
	r.must(err)
	v2, err := db.AddPlan(r.DB, tid, r.k("bob"), "change badge.tsx and its test")
	r.must(err)
	r.must(db.MarkPlan(r.DB, tid, r.k("human"), v2, true))
	db.SetTaskQueued(r.DB, tid, r.k("human"))
	row, _ := db.GetTask(r.DB, tid)
	_, err = r.QueueDeliver(r.DB, tid, r.k("human"), true, row)
	r.must(err)
	sent, _ := db.LiveDelivery(r.DB, tid, r.k("bob"))
	if sent == nil {
		t.Fatal("not delivered")
	}
	want := "Plan, version 2 (OK by the founder):\nchange badge.tsx and its test\n\nCommit on branch lead-T1 (made from origin/main; if it exists, " +
		"carry on from it). In your result, say what you changed from the plan."
	if !strings.HasPrefix(sent.Body, strings.TrimSpace(brief)+"\n\n") || !strings.HasSuffix(sent.Body, want) {
		t.Fatalf("%q", sent.Body)
	}
	if got := tellText(r.DB, sent, 1, nil, false, nil, ""); !strings.Contains(got, want) {
		t.Fatalf("the plan was cut off the typed delivery: %q", got)
	}

	// a story's sub-task with no plan goes out as its title and its branch, now its new assignee's; its core ask names that branch too
	bare, _ := db.NewTask(r.DB, "bare part", r.k("bob"), r.k("alice"), 0, db.DefaultPriority, "", 0, "")
	_, err = db.AddEvent(r.DB, r.k("alice"), "task_new", db.KV{K: "task_id", V: bare}, db.KV{K: "draft", V: true})
	r.must(err)
	_, err = r.DB.Exec("UPDATE tasks SET owner = ? WHERE id = ?", r.k("alice"), bare)
	r.must(err)
	row, _ = db.GetTask(r.DB, bare)
	branch := fmt.Sprintf("lead-T%d", bare) // alice leads its workspace, and its agent is named lead
	if body, plan := queuedBody(r.DB, row, main); body != "bare part\n\nCommit on branch "+branch+" (made from origin/main; if it exists, carry on from it)." || plan != 0 {
		t.Fatalf("%q %d", body, plan)
	}
	if got := askText(&db.TaskAsk{Kind: db.CoreAskKind}, row, branch, "main"); !strings.Contains(got, "Commit on branch "+branch+" (made from origin/main; "+
		"if it exists, carry on from it); do not merge or push it.") || strings.Contains(got, "leave the worktree") {
		t.Fatalf("%q", got)
	}
	if got := askText(&db.TaskAsk{Kind: db.CoreAskKind}, row, "", "master"); !strings.Contains(got, fmt.Sprintf("Commit them on a branch of T%d's own, made from origin/master; do not", bare)) {
		t.Fatalf("a task that is not a story's sub-task: %q", got)
	}

	// the branch named is the default branch of the agent's repository: main for an agent with none on record, master in a master repository
	if got := r.landsOn(r.DB, r.k("alice")); got != "main" {
		t.Fatalf("no repository on record: %q", got)
	}
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "master"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "x"}} {
		if g := gitrun.Run(repo, args...); g.RC != 0 {
			t.Fatalf("git %v: %+v", args, g)
		}
	}
	teamtest.Set(t, r.home, "alice", "repo", repo)
	if body, _ := queuedBody(r.DB, row, func() string { return r.landsOn(r.DB, r.k("alice")) }); body != "bare part\n\nCommit on branch "+branch+" (made from origin/master; if it exists, carry on from it)." {
		t.Fatalf("in a master repository: %q", body)
	}

	// a task not opened as a draft goes out as its title, plan or not
	plain, _ := db.NewTask(r.DB, "plain", r.k("bob"), r.k("alice"), 0, db.DefaultPriority, "", 0, "")
	_, err = db.AddPlan(r.DB, plain, r.k("bob"), "x")
	r.must(err)
	row, _ = db.GetTask(r.DB, plain)
	if body, plan := queuedBody(r.DB, row, main); body != "plain" || plan != 0 {
		t.Fatalf("%q %d", body, plan)
	}
}
