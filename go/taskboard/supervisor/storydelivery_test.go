package supervisor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// A story's sub-task with a plan goes out with its brief and that plan, marked OK or not with its version, typed whole; another queued task
// goes out as its title.
func TestRunDeliversAStorysSubTaskWithItsPlan(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
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
	want := "Plan, version 2 (OK by the founder):\nchange badge.tsx and its test\n\nCommit on branch bob-T1 (made from origin/main; if it exists, " +
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
	branch := fmt.Sprintf("alice-T%d", bare)
	if body, plan := queuedBody(r.DB, row); body != "bare part\n\nCommit on branch "+branch+" (made from origin/main; if it exists, carry on from it)." || plan != 0 {
		t.Fatalf("%q %d", body, plan)
	}
	if got := askText(&db.TaskAsk{Kind: db.CoreAskKind}, row, branch); !strings.Contains(got, "Commit on branch "+branch+" (made from origin/main; "+
		"if it exists, carry on from it); do not merge or push it.") || strings.Contains(got, "leave the worktree") {
		t.Fatalf("%q", got)
	}
	if got := askText(&db.TaskAsk{Kind: db.CoreAskKind}, row, ""); !strings.Contains(got, "Commit them on a branch of T") {
		t.Fatalf("a task that is not a story's sub-task: %q", got)
	}

	// a task not opened as a draft goes out as its title, plan or not
	plain, _ := db.NewTask(r.DB, "plain", r.k("bob"), r.k("alice"), 0, db.DefaultPriority, "", 0, "")
	_, err = db.AddPlan(r.DB, plain, r.k("bob"), "x")
	r.must(err)
	row, _ = db.GetTask(r.DB, plain)
	if body, plan := queuedBody(r.DB, row); body != "plain" || plan != 0 {
		t.Fatalf("%q %d", body, plan)
	}
}
