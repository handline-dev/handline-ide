package supervisor

import (
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
	want := "Plan, version 2 (OK by the founder):\nchange badge.tsx and its test\n\nIn your result, say what you changed from the plan."
	if !strings.HasPrefix(sent.Body, strings.TrimSpace(brief)+"\n\n") || !strings.HasSuffix(sent.Body, want) {
		t.Fatalf("%q", sent.Body)
	}
	if got := tellText(r.DB, sent, 1, nil, false, nil, ""); !strings.Contains(got, want) {
		t.Fatalf("the plan was cut off the typed delivery: %q", got)
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
