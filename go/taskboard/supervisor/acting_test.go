package supervisor

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/actor"
)

// turnRig is a task bob owns and carol opened, queued, with alice's turn on top: every rule that asks who works it now must answer alice.
func turnRig(t *testing.T) (*rig, int64) {
	r := newRig(t)
	r.bigTeam()
	for _, a := range []string{r.k("alice"), r.k("bob"), r.k("carol")} {
		r.agentUp(a)
	}
	tid := r.newTask("bob", r.k("carol"), "the review")
	r.must(db.PushTurn(r.DB, tid, db.Turn{Agent: r.k("alice"), By: r.k("bob"), Since: r.clock}))
	return r, tid
}

// beginOnTop delivers tid to the agent on top and begins it there, as `hl task start` leaves it.
func (r *rig) beginOnTop(tid int64) {
	r.t.Helper()
	agent := r.k(r.acting(tid))
	_, _, err := r.AdvanceCurrentTask(r.DB, agent)
	r.must(err)
	if cur, ok, _ := db.CurrentTask(r.DB, agent); !ok || cur != tid {
		r.t.Fatalf("%s does not hold T%d: %d %v", agent, tid, cur, ok)
	}
	_, err = db.AddEvent(r.DB, agent, "task_begin", db.KV{K: "task_id", V: tid})
	r.must(err)
}

// The queue sweep delivers a queued task to the agent on top, whoever owns it, the founder included.
func TestTheQueueDeliversATaskToTheAgentOnTop(t *testing.T) {
	r, tid := turnRig(t)
	r.agentUp("erin")
	founders := r.newTask(actor.OwnerKey, r.k("carol"), "the founder's")
	r.must(db.PushTurn(r.DB, founders, db.Turn{Agent: r.k("erin"), By: actor.OwnerKey, Since: r.clock}))
	r.must(r.SweepTaskQueue(r.clock))
	for agent, want := range map[string]int64{r.k("alice"): tid, r.k("erin"): founders} {
		if cur, ok, _ := db.CurrentTask(r.DB, agent); !ok || cur != want {
			t.Fatalf("%s holds %d %v, want T%d", agent, cur, ok, want)
		}
	}
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("bob")); ok && cur == tid {
		t.Fatal("bob, the owner, holds it")
	}
}

// An agent that wrote to the founder and stopped with a task it acts on by a turn in doing is told.
func TestTheSaidAndStoppedNoticeReachesTheAgentOnTop(t *testing.T) {
	r, tid := turnRig(t)
	r.beginOnTop(tid)
	r.turn("alice", true)
	r.must(r.SweepSaidAndStopped())
	if n := r.saidStopNotices("alice", tid); n != 1 {
		t.Fatalf("told %d times: %v", n, r.saysTo("alice"))
	}
}

// Whoever opened a task waits on the agent acting on it, not on its owner.
func TestTheOpenerWaitsOnTheAgentOnTop(t *testing.T) {
	r, _ := turnRig(t)
	if who := r.waitingOn(r.DB, r.k("alice")); !slices.Contains(who, r.k("carol")) {
		t.Fatalf("waiting on alice: %v", who)
	}
	if who := r.waitingOn(r.DB, r.k("bob")); slices.Contains(who, r.k("carol")) {
		t.Fatalf("waiting on bob: %v", who)
	}
}

// A body edit is typed to the agent on top, even when the owner or the founder made it.
func TestABodyEditIsToldToTheAgentOnTop(t *testing.T) {
	r, tid := turnRig(t)
	r.beginOnTop(tid)
	r.turnCall("bob", "task.edit", fmt.Sprintf(`{"task":%d,"body":"check the cache too","request_id":"e1"}`, tid))
	if !strings.Contains(r.term.last(), fmt.Sprintf("T%d body edited by bob: check the cache too", tid)) {
		t.Fatalf("alice is not told: %v", r.term.typed)
	}
	r.agentUp("erin")
	founders := r.newTask(actor.OwnerKey, r.k("carol"), "the founder's")
	r.must(db.PushTurn(r.DB, founders, db.Turn{Agent: r.k("erin"), By: actor.OwnerKey, Since: r.clock}))
	r.beginOnTop(founders)
	r.term.typed = nil
	r.turnCall("human", "task.edit", fmt.Sprintf(`{"task":%d,"body":"and the docs","request_id":"e2"}`, founders))
	if len(r.term.typed) != 1 || !strings.Contains(r.term.last(), fmt.Sprintf("T%d body edited by human: and the docs", founders)) {
		t.Fatalf("erin is not told: %v", r.term.typed)
	}
}

// A comment tells the agent on top at once, as the task's assignee; its owner hears of a plain comment only through the board.
func TestACommentTellsTheAgentOnTop(t *testing.T) {
	r, tid := turnRig(t)
	got, ok := r.MemoRecipients(tid, r.k("dave"), "plain", false)
	if !ok || len(got) != 1 || got[0].Agent != r.k("alice") || got[0].Why != "assignee" {
		t.Fatalf("%+v", got)
	}
}

// The agent on top may start the task and keep progress on it; its owner may do neither.
func TestTheAgentOnTopStartsTheTaskAndKeepsItsProgress(t *testing.T) {
	r, tid := turnRig(t)
	r.refusedCall("bob", "task.start", fmt.Sprintf(`{"task":%d,"request_id":"s0"}`, tid), "alice's")
	r.turnCall("alice", "task.start", fmt.Sprintf(`{"task":%d,"request_id":"s1"}`, tid))
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("alice")); !ok || cur != tid {
		t.Fatalf("alice holds %d %v", cur, ok)
	}
	r.refusedCall("bob", "task.progress", fmt.Sprintf(`{"task":%d,"note":"x","request_id":"g0"}`, tid), "alice's")
	r.turnCall("alice", "task.progress", fmt.Sprintf(`{"task":%d,"note":"done: read it","request_id":"g1"}`, tid))
}

// Blocking the task frees the slot of the agent on top, which moves on to its next task; its owner keeps the task it holds. A task the
// founder owns frees the agent on top the same way.
func TestABlockFreesTheAgentOnTop(t *testing.T) {
	r, tid := turnRig(t)
	bobs := r.beginCurrent("bob", r.k("carol"), "bob's own")
	r.beginOnTop(tid)
	next := r.newTask("alice", r.k("carol"), "next")
	blocker := r.newTask("dave", r.k("carol"), "first this")
	r.turnCall("alice", "task.edit", fmt.Sprintf(`{"task":%d,"blocked_by":%d,"request_id":"e1"}`, tid, blocker))
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("alice")); !ok || cur != next {
		t.Fatalf("alice moved on: %d %v", cur, ok)
	}
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("bob")); !ok || cur != bobs {
		t.Fatalf("bob keeps his own: %d %v", cur, ok)
	}
	r.agentUp("erin")
	founders := r.newTask(actor.OwnerKey, r.k("carol"), "the founder's")
	r.must(db.PushTurn(r.DB, founders, db.Turn{Agent: r.k("erin"), By: actor.OwnerKey, Since: r.clock}))
	r.beginOnTop(founders)
	erins := r.newTask("erin", r.k("carol"), "erin's next")
	r.turnCall("erin", "task.edit", fmt.Sprintf(`{"task":%d,"blocked_by":%d,"request_id":"e2"}`, founders, blocker))
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("erin")); !ok || cur != erins {
		t.Fatalf("erin moved on: %d %v", cur, ok)
	}
}

// A lead's discard of a task the founder owns, with alice on top holding it before its delivery reached her, tells alice to stop and moves
// her on to her next task.
func TestADiscardStopsTheAgentOnTopAndMovesItOn(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	for _, a := range []string{r.k("alice"), r.k("carol")} {
		r.agentUp(a)
	}
	// carol leads a workspace, so her discard stops the requests out on the task instead of refusing
	teamtest.Set(t, r.home, "alice", "reports_to", "carol")
	tid := r.newTask(actor.OwnerKey, r.k("carol"), "the founder's")
	r.must(db.PushTurn(r.DB, tid, db.Turn{Agent: r.k("alice"), By: actor.OwnerKey, Since: r.clock}))
	r.beginOnTop(tid)
	// held, its delivery still queued: the stop is typed to alice, as no delivery will reach her
	_, err := r.DB.Exec("UPDATE requests SET delivered_at = NULL, state = 'queued' WHERE id IN (SELECT request_id FROM task_requests WHERE task_id = ?)", tid)
	r.must(err)
	next := r.newTask("alice", r.k("carol"), "next")
	r.turnCall("carol", "task.discard", fmt.Sprintf(`{"task":%d,"reason":"not needed","request_id":"d1"}`, tid))
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("alice")); !ok || cur != next {
		t.Fatalf("alice moved on: %d %v", cur, ok)
	}
	told := false
	for _, s := range r.term.typed {
		told = told || strings.Contains(s, fmt.Sprintf("T%d discarded by carol - stop work on it", tid))
	}
	if !told {
		t.Fatalf("alice is told to stop: %v", r.term.typed)
	}
}
