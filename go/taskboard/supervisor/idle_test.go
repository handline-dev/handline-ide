package supervisor

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

// idleRig: alice runs a parent task waiting on a child it gave bob. The child's ordinary result is not told to the parent's owner (it is on
// the parent's board), so only the idle rule can reach alice.
func idleRig(t *testing.T) (*rig, int64) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.agentUp(r.k("alice"))
	parent := r.newTask(r.k("alice"), r.k(("alice")), "parent")
	child, err := db.NewTask(r.DB, "child", r.k(("bob")), r.k(("alice")), parent, db.DefaultPriority, "", 0, "")
	r.must(err)
	_, err = db.SetTaskQueued(r.DB, child, r.k(("alice")))
	r.must(err)
	r.must(db.AddBlock(r.DB, parent, child))
	return r, child
}

func idleSays(r *rig, to string) []string {
	var out []string
	rows, err := r.DB.Query("SELECT body FROM requests WHERE target = ? AND body LIKE '%has nothing it can start now.'", to)
	r.must(err)
	defer rows.Close()
	for rows.Next() {
		var b string
		r.must(rows.Scan(&b))
		out = append(out, r.named(b))
	}
	return out
}

func TestADoneThatLeavesItsAssigneeIdleIsToldToTheReporterAtOnce(t *testing.T) {
	r, child := idleRig(t)
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.term.typed = nil
	r.Done(child, r.k(("bob")), r.k(("bob")), nil, true)
	want := "T2 done by bob: built it - bob has nothing it can start now."
	if says := idleSays(r, r.k(("alice"))); len(says) != 1 || says[0] != want {
		t.Fatalf("alice is told once: %q", says)
	}
	if !strings.Contains(strings.Join(r.term.typed, "\n"), "T2 done by @bob: built it - @bob has nothing it can start now.") { // as typed
		t.Fatalf("typed at once, not held for a digest: %q", r.term.typed)
	}
}

func TestADoneWithMoreWorkQueuedForTheAssigneeStaysOutOfTheReportersWay(t *testing.T) {
	r, child := idleRig(t)
	r.newTask(r.k("bob"), r.k(("alice")), "next")
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, r.k(("bob")), r.k(("bob")), nil, true)
	if says := idleSays(r, r.k(("alice"))); len(says) != 0 {
		t.Fatalf("bob still has work: %q", says)
	}
}

func TestAQueuedTaskBlockedByAnOpenTaskIsNothingToStart(t *testing.T) {
	r, child := idleRig(t)
	next := r.newTask(r.k("bob"), r.k(("alice")), "next")
	other := r.newTask(r.k("alice"), r.k(("alice")), "other")
	r.must(db.AddBlock(r.DB, next, other))
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, r.k(("bob")), r.k(("bob")), nil, true)
	if says := idleSays(r, r.k(("alice"))); len(says) != 1 {
		t.Fatalf("bob's only next task waits on someone else, so alice is woken: %q", says)
	}
}

func TestATaskThisDoneFreesIsSomethingToStart(t *testing.T) {
	r, child := idleRig(t)
	next := r.newTask(r.k("bob"), r.k(("alice")), "next")
	r.must(db.AddBlock(r.DB, next, child))
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, r.k(("bob")), r.k(("bob")), nil, true)
	if says := idleSays(r, r.k(("alice"))); len(says) != 0 {
		t.Fatalf("the done freed bob's next task: %q", says)
	}
	if cur, _, _ := db.CurrentTask(r.DB, r.k(("bob"))); cur != next {
		t.Fatalf("the daemon starts the freed task: current %d", cur)
	}
}

func TestNoIdleSayWhenTheReporterClosedItOrWasAlreadyToldAtOnce(t *testing.T) {
	r, child := idleRig(t)
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, r.k(("alice")), r.k(("bob")), nil, true)
	if says := idleSays(r, r.k(("alice"))); len(says) != 0 {
		t.Fatalf("the reporter closed it itself: %q", says)
	}

	r, child = idleRig(t)
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	rid, err := db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: r.k(("alice")), Body: "T2 done by bob: built it", Kind: "say"})
	r.must(err)
	r.Done(child, r.k(("bob")), r.k(("bob")), []db.Notice{{RID: rid, To: r.k(("alice")), Text: "T2 done by bob: built it"}}, true)
	if says := idleSays(r, r.k(("alice"))); len(says) != 0 {
		t.Fatalf("the done itself was already typed to the reporter: %q", says)
	}
}

func TestALeftoverSettledOnADoneTaskSaysNothingAboutIdleness(t *testing.T) {
	r, child := idleRig(t)
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, r.k(("bob")), r.k(("bob")), nil, false)
	if says := idleSays(r, r.k(("alice"))); len(says) != 0 {
		t.Fatalf("not a finish: %q", says)
	}
}

func TestNoIdleSayToTheFounderOrToAnAssigneeAboutItself(t *testing.T) {
	for _, reporter := range []string{actor.OwnerKey, "bob"} {
		r := newRig(t)
		reporter = r.k(reporter)
		r.agentUp(r.k("bob"))
		tid, err := db.NewTask(r.DB, "job", r.k(("bob")), reporter, 0, db.DefaultPriority, "", 0, "")
		r.must(err)
		db.MarkTaskDone(r.DB, tid, "built it", "reason")
		r.Done(tid, r.k(("alice")), r.k(("bob")), nil, true)
		if says := idleSays(r, reporter); len(says) != 0 {
			t.Fatalf("%s: %q", reporter, says)
		}
	}
}

func TestNoIdleSayAboutTheFounderOrAnAgentNotOnTheTeam(t *testing.T) {
	for _, assignee := range []string{actor.OwnerKey, "carol"} {
		r := newRig(t)
		assignee = r.k(assignee)
		r.agentUp(r.k("alice"))
		tid, err := db.NewTask(r.DB, "job", assignee, r.k(("alice")), 0, db.DefaultPriority, "", 0, "")
		r.must(err)
		db.MarkTaskDone(r.DB, tid, "built it", "reason")
		r.Done(tid, assignee, assignee, nil, true)
		if says := idleSays(r, r.k(("alice"))); len(says) != 0 {
			t.Fatalf("%s: %q", assignee, says)
		}
	}
}
