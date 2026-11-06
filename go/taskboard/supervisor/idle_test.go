package supervisor

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// idleRig: alice runs a parent task waiting on a child it gave bob. The child's ordinary result is not told to the parent's owner (it is on
// the parent's board), so only the idle rule can reach alice.
func idleRig(t *testing.T) (*rig, int64) {
	r := newRig(t)
	r.agentUp("bob")
	r.agentUp("alice")
	parent := r.newTask("alice", "alice", "parent")
	child, err := db.NewTask(r.DB, "child", "bob", "alice", parent, db.DefaultPriority, "", 0, "")
	r.must(err)
	_, err = db.SetTaskQueued(r.DB, child, "alice")
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
		out = append(out, b)
	}
	return out
}

func TestADoneThatLeavesItsAssigneeIdleIsToldToTheReporterAtOnce(t *testing.T) {
	r, child := idleRig(t)
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.term.typed = nil
	r.Done(child, "bob", "bob", nil, true)
	want := "T2 done by bob: built it - bob has nothing it can start now."
	if says := idleSays(r, "alice"); len(says) != 1 || says[0] != want {
		t.Fatalf("alice is told once: %q", says)
	}
	if !strings.Contains(strings.Join(r.term.typed, "\n"), want) {
		t.Fatalf("typed at once, not held for a digest: %q", r.term.typed)
	}
}

func TestADoneWithMoreWorkQueuedForTheAssigneeStaysOutOfTheReportersWay(t *testing.T) {
	r, child := idleRig(t)
	r.newTask("bob", "alice", "next")
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, "bob", "bob", nil, true)
	if says := idleSays(r, "alice"); len(says) != 0 {
		t.Fatalf("bob still has work: %q", says)
	}
}

func TestAQueuedTaskBlockedByAnOpenTaskIsNothingToStart(t *testing.T) {
	r, child := idleRig(t)
	next := r.newTask("bob", "alice", "next")
	other := r.newTask("alice", "alice", "other")
	r.must(db.AddBlock(r.DB, next, other))
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, "bob", "bob", nil, true)
	if says := idleSays(r, "alice"); len(says) != 1 {
		t.Fatalf("bob's only next task waits on someone else, so alice is woken: %q", says)
	}
}

func TestATaskThisDoneFreesIsSomethingToStart(t *testing.T) {
	r, child := idleRig(t)
	next := r.newTask("bob", "alice", "next")
	r.must(db.AddBlock(r.DB, next, child))
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, "bob", "bob", nil, true)
	if says := idleSays(r, "alice"); len(says) != 0 {
		t.Fatalf("the done freed bob's next task: %q", says)
	}
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != next {
		t.Fatalf("the daemon starts the freed task: current %d", cur)
	}
}

func TestNoIdleSayWhenTheReporterClosedItOrWasAlreadyToldAtOnce(t *testing.T) {
	r, child := idleRig(t)
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, "alice", "bob", nil, true)
	if says := idleSays(r, "alice"); len(says) != 0 {
		t.Fatalf("the reporter closed it itself: %q", says)
	}

	r, child = idleRig(t)
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	rid, err := db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: "alice", Body: "T2 done by bob: built it", Kind: "say"})
	r.must(err)
	r.Done(child, "bob", "bob", []db.Notice{{RID: rid, To: "alice", Text: "T2 done by bob: built it"}}, true)
	if says := idleSays(r, "alice"); len(says) != 0 {
		t.Fatalf("the done itself was already typed to the reporter: %q", says)
	}
}

func TestALeftoverSettledOnADoneTaskSaysNothingAboutIdleness(t *testing.T) {
	r, child := idleRig(t)
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	r.Done(child, "bob", "bob", nil, false)
	if says := idleSays(r, "alice"); len(says) != 0 {
		t.Fatalf("not a finish: %q", says)
	}
}

func TestNoIdleSayToTheFounderOrToAnAssigneeAboutItself(t *testing.T) {
	for _, reporter := range []string{db.Human, "bob"} {
		r := newRig(t)
		r.agentUp("bob")
		tid, err := db.NewTask(r.DB, "job", "bob", reporter, 0, db.DefaultPriority, "", 0, "")
		r.must(err)
		db.MarkTaskDone(r.DB, tid, "built it", "reason")
		r.Done(tid, "alice", "bob", nil, true)
		if says := idleSays(r, reporter); len(says) != 0 {
			t.Fatalf("%s: %q", reporter, says)
		}
	}
}

func TestNoIdleSayAboutTheFounderOrAnAgentNotOnTheTeam(t *testing.T) {
	for _, assignee := range []string{db.Human, "carol"} {
		r := newRig(t)
		r.agentUp("alice")
		tid, err := db.NewTask(r.DB, "job", assignee, "alice", 0, db.DefaultPriority, "", 0, "")
		r.must(err)
		db.MarkTaskDone(r.DB, tid, "built it", "reason")
		r.Done(tid, assignee, assignee, nil, true)
		if says := idleSays(r, "alice"); len(says) != 0 {
			t.Fatalf("%s: %q", assignee, says)
		}
	}
}
