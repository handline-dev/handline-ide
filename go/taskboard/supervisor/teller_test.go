package supervisor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

var _ taskwrites.Teller = (*Env)(nil)

func TestTheTellerTypesTheMemoSayAndSendsAQueuedTaskToAFreeOwner(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	tid, _ := db.NewTask(r.DB, "job", r.k(("bob")), r.k(("alice")), 0, db.DefaultPriority, "", 0, "")
	db.SetTaskQueued(r.DB, tid, r.k(("human")))
	msg := r.namedQ(tid, r.k(("human")), true)
	if r.named(msg) != "task T1 is queued (human) - bob was free, so it went out now" {
		t.Fatal(msg)
	}
	r.term.typed = nil
	r.MemoChanged(tid, 0, r.k(("alice")), "added", "mind the cache", true, nil)
	if !strings.Contains(r.term.last(), "T1 comment by alice: mind the cache") {
		t.Fatalf("%q", r.term.last())
	}
	if msg := r.namedQ(9, r.k(("human")), true); r.named(msg) != "task T9 is queued (human)" {
		t.Fatalf("a missing task: %q", msg)
	}
}

func TestTheTellerAfterADiscardAdvancesTheOwnerAndTypesTheSays(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.agentUp(r.k("alice"))
	first := r.newTask(r.k("bob"), r.k(("alice")), "current")
	second := r.newTask(r.k("bob"), r.k(("alice")), "next")
	r.AdvanceCurrentTask(r.DB, r.k(("bob")))
	db.DiscardTask(r.DB, first, r.k(("alice")), "not needed")
	r.term.typed = nil
	say, _ := db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: r.k(("bob")), Body: "T1 discarded by alice: not needed", Kind: "say"})
	r.Discarded(r.k("bob"), []db.Notice{{RID: say, To: r.k(("bob")), Text: "T1 discarded by alice: not needed"}}, nil)
	if cur, _, _ := db.CurrentTask(r.DB, r.k(("bob"))); cur != second {
		t.Fatalf("the owner was not moved on: %d", cur)
	}
	typed := strings.Join(r.term.typed, "\n")
	if !strings.Contains(r.named(typed), "T1 discarded by alice: not needed") {
		t.Fatalf("%q", typed)
	}
}

func TestTheTellerTypesTheNextTaskAtTheOwnerAfterADiscard(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	first := r.newTask(r.k("bob"), r.k(("alice")), "current")
	second := r.newTask(r.k("bob"), r.k(("alice")), "the next one")
	r.AdvanceCurrentTask(r.DB, r.k(("bob")))
	// the next task already has its request out, so the advance only promotes it and the Teller's push is what types it
	rid, err := db.AddRequest(r.DB, db.NewRequest{Requester: r.k(("alice")), Target: r.k(("bob")), Body: "the next one", Kind: "task"})
	r.must(err)
	r.must(db.LinkRequest(r.DB, second, rid))
	db.DiscardTask(r.DB, first, r.k(("alice")), "not needed")
	reqs, _ := db.TaskRequests(r.DB, first)
	for _, q := range reqs {
		db.CloseIfQueued(r.DB, q.ID, "dismissed", "")
	}
	r.clock += BatchEveryS
	r.term.typed = nil
	r.Discarded(r.k("bob"), nil, nil)
	if typed := strings.Join(r.term.typed, "\n"); !strings.Contains(r.named(typed), "the next one") {
		t.Fatalf("the next task was not typed at the owner: %q", typed)
	}
}

func TestAFailedStatementAfterTheCommitIsLoggedByTheTeller(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.DB.Close()
	r.Discarded(r.k("bob"), nil, nil)
	if !r.logged("moving bob on after a discard failed") {
		t.Fatalf("%v", r.logs)
	}
	if msg := r.namedQ(1, r.k(("human")), true); r.named(msg) != "task T1 is queued (human)" || !r.logged("delivering T1 after it was queued failed") {
		t.Fatalf("%q %v", msg, r.logs)
	}
	r.MemoChanged(1, 0, r.k(("alice")), "added", "x", false, nil)
	if !r.logged("telling about the comment on T1 failed") {
		t.Fatalf("%v", r.logs)
	}
}

func TestTheTellerAfterAReleaseTypesTheStopAtAHolderAndRetiresAQueuedRequest(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	held := r.ask(r.k("alice"), r.k(("bob")), "x", "task")
	queued := r.ask(r.k("alice"), r.k(("bob")), "y", "task")
	r.term.typed = nil
	said := r.Released(1, "T1 released by alice", []taskwrites.Retired{{RID: held, Requester: r.k(("alice")), Target: r.k(("bob")), Held: true}})
	if len(said) != 1 || r.ids(said[0]) != "#"+fmt.Sprint(held)+" ended; bob told to stop" || !strings.Contains(strings.Join(r.term.typed, "\n"), "T1 released by alice - stop work on it") {
		t.Fatalf("%q %q", said, r.term.typed)
	}
	db.MarkDelivered(r.DB, queued) // the target has it, so it is told to stop
	db.CloseIfQueued(r.DB, queued, "rejected", "T1 released by alice")
	said = r.Released(1, "T1 released by alice", []taskwrites.Retired{{RID: queued, Requester: r.k(("alice")), Target: r.k(("bob"))}})
	if len(said) != 1 || r.ids(said[0]) != fmt.Sprintf("#%d to bob retired (bob was told to stop)", queued) {
		t.Fatalf("%q", said)
	}
}

func TestTheTellerAfterADraftParksTheHolderAndMovesItOnToItsNextTask(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	first := r.newTask(r.k("bob"), r.k(("alice")), "current")
	second := r.newTask(r.k("bob"), r.k(("alice")), "the next one")
	r.AdvanceCurrentTask(r.DB, r.k(("bob")))
	_, err := r.DB.Exec(r.sql("UPDATE tasks SET queued_by = NULL WHERE id = ?"), first) // the draft's own writes: unapproved, its delivery ended
	r.must(err)
	reqs, _ := db.TaskRequests(r.DB, first)
	_, err = r.DB.Exec(r.sql("UPDATE requests SET state = 'landed', landed_at = 1 WHERE id = ?"), reqs[0].ID)
	r.must(err)
	r.must(db.EndHeld(r.DB, first, r.k(("bob"))))
	r.must(db.RefreshSent(r.DB, first))
	r.term.typed = nil
	said := r.Released(first, fmt.Sprintf("T%d made draft by alice", first), []taskwrites.Retired{{RID: reqs[0].ID, Requester: r.k(("alice")), Target: r.k(("bob")), Held: true, Park: true}})
	if len(said) != 1 || r.ids(said[0]) != fmt.Sprintf("#%d ended; bob told to stop and moved on", reqs[0].ID) {
		t.Fatalf("%q", said)
	}
	if typed := strings.Join(r.term.typed, "\n"); !strings.Contains(r.named(typed), "it is parked: stop work on it and move on to your next task") {
		t.Fatalf("%q", typed)
	}
	if cur, _, _ := db.CurrentTask(r.DB, r.k(("bob"))); cur != second {
		t.Fatalf("the holder was not moved on: %d", cur)
	}
}

func TestTheTellerAfterACancelClosesTheTellsAndTypesTheStopToAnOwnerWithNoDelivery(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	rid := r.ask(r.k("alice"), r.k(("bob")), "x", "task")
	db.CloseIfQueued(r.DB, rid, "rejected", CancelVerdict)
	r.term.typed = nil
	r.Cancelled(1, r.k(("human")), r.k(("bob")), true, []taskwrites.Retired{{RID: rid, Requester: r.k(("alice")), Target: r.k(("bob")), Delivered: true}})
	if typed := strings.Join(r.term.typed, "\n"); !strings.Contains(r.named(typed), "T1 discarded by human - stop work on it") {
		t.Fatalf("%q", typed)
	}
}

func TestTheTellerAfterADoneMovesTheOwnerOnAndReleasesWhatWaitedOnTheTask(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.agentUp(r.k("alice"))
	first := r.newTask(r.k("bob"), r.k(("alice")), "current")
	waiting := r.newTask(r.k("alice"), r.k(("alice")), "waits on the first")
	r.must(db.AddBlock(r.DB, waiting, first))
	db.MarkTaskDone(r.DB, first, "done", "reason")
	r.term.typed = nil
	r.Done(first, r.k(("bob")), r.k(("bob")), nil, true)
	var n int
	r.DB.QueryRow(r.sql("SELECT COUNT(*) FROM requests WHERE target = 'alice' AND body LIKE 'task T2 is unblocked%'")).Scan(&n)
	if n != 1 {
		t.Fatalf("the owner of the task that waited is told once: %d", n)
	}
}

func TestTheTellerRoutesAHandoffInTheTransactionOfTheMoveAndTypesItAfterTheCommit(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.agentUp(r.k("alice"))
	tid := r.newTask(r.k("bob"), r.k(("alice")), "job")
	tx, err := r.DB.Begin()
	r.must(err)
	var moved int64
	asked, err := r.TaskAsk(tx, taskwrites.AskReq{From: r.k(("bob")), To: r.k(("alice")), Kind: "task", Body: "T1 handed to you by bob: job\n\ncheck it", Within: func(rid int64) error {
		moved = rid
		to := r.k(("alice"))
		return db.EditTask(tx, tid, r.k(("bob")), db.TaskEdit{Owner: &to})
	}})
	r.must(err)
	if !asked.OK || asked.RID == 0 || moved != asked.RID {
		t.Fatalf("%+v", asked)
	}
	var n int
	r.DB.QueryRow(r.sql("SELECT COUNT(*) FROM requests")).Scan(&n)
	if n != 0 {
		t.Fatal("nothing is visible before the commit")
	}
	r.must(tx.Commit())
	r.term.typed = nil
	r.must(db.LinkRequest(r.DB, tid, asked.RID))
	db.MarkSendNow(r.DB, asked.RID)
	asked.After()
	r.HandedOver(taskwrites.Handover{Task: tid, By: r.k(("bob")), From: r.k(("bob")), To: r.k(("alice")), Title: "job", RID: asked.RID})
	if !strings.Contains(strings.Join(r.term.typed, "\n"), "check it") {
		t.Fatalf("the new owner is typed the handoff: %q", r.term.typed)
	}

	tx, err = r.DB.Begin()
	r.must(err)
	denied, err := r.TaskAsk(tx, taskwrites.AskReq{From: r.k(("bob")), To: "nobody.such", Kind: "task", Body: "x", Within: func(int64) error { return fmt.Errorf("not reached") }})
	r.must(err)
	r.must(tx.Commit())
	if denied.OK || denied.Msg == "" {
		t.Fatalf("an agent the routing table does not know is refused: %+v", denied)
	}
}

func TestTheTellerSendsATaskThatAnEditGaveToANewOwnerAndTellsTheOldOneToStop(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.agentUp(r.k("alice"))
	tid, err := db.NewTask(r.DB, "job", r.k(("bob")), r.k(("human")), 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	rid := r.ask(r.k("human"), r.k(("bob")), "the brief", "task")
	r.must(db.LinkRequest(r.DB, tid, rid))
	r.must(db.SetCurrentTask(r.DB, r.k(("bob")), tid, true))
	to := r.k(("alice"))
	r.must(db.EditTask(r.DB, tid, r.k(("human")), db.TaskEdit{Owner: &to}))
	r.term.typed = nil
	r.Edited(taskwrites.Edited{Task: tid, By: r.k(("human")), WasOwner: r.k(("bob"))})
	var old, fresh int
	r.DB.QueryRow(r.sql("SELECT COUNT(*) FROM requests WHERE id = ? AND state = 'withdrawn'"), rid).Scan(&old)
	r.DB.QueryRow(r.sql("SELECT COUNT(*) FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'alice' AND r.body = 'the brief'"), tid).Scan(&fresh)
	if old != 1 || fresh != 1 {
		t.Fatalf("the old request is taken back (%d) and alice is asked the same brief (%d)", old, fresh)
	}
	if cur, held, _ := db.CurrentTask(r.DB, r.k(("bob"))); held && cur == tid {
		t.Fatal("bob still holds the task")
	}
	if cur, _, _ := db.CurrentTask(r.DB, r.k(("alice"))); cur != tid {
		t.Fatalf("alice holds %d", cur)
	}
	if !strings.Contains(strings.Join(r.term.typed, "\n"), "T1 moved to alice by human - stop work on it") {
		t.Fatalf("the old owner is told to stop: %q", r.term.typed)
	}
}

// A team like the live one: agents reach the founder and the founder reaches them, and nothing else is declared, so no edge leads from the
// daemon to an agent, and none from one agent to another.
// The start-up hand-back of the founder's tasks is the daemon's own step: it moves each task to its agent though no edge declares daemon ->
// agent, and leaves those it must, once.
func TestFounderTasksGoBackWithNoTeamEdgeFromTheDaemon(t *testing.T) {
	r := newRig(t)
	r.founderOnlyTeam()
	handed := r.newTask(r.k("human"), "eng.alice", "handed by ops.bob")
	opened := r.newTask(r.k("human"), "eng.alice", "opened by eng.alice")
	asked := r.newTask(r.k("human"), r.k(("human")), "the founder's own")
	gone := r.newTask(r.k("human"), r.k(("carol")), "opened by a retired agent")
	_, err := db.AddEvent(r.DB, "ops.bob", "task_handoff", db.KV{K: "task_id", V: handed}, db.KV{K: "from", V: "ops.bob"}, db.KV{K: "to", V: r.k(("human"))},
		db.KV{K: "note", V: "which one?"})
	r.must(err)

	said, err := taskwrites.ReturnFounderTasks(r.DB, r.home)
	if err != nil || r.named(fmt.Sprint(said)) != fmt.Sprintf("[T%d goes back to ops.bob, the founder's turn on it T%d goes back to eng.alice, the founder's turn on it]", handed, opened) {
		t.Fatalf("%q %v", said, err)
	}
	for tid, want := range map[int64]string{handed: "ops.bob", opened: "eng.alice", asked: r.k(("human")), gone: r.k(("human"))} {
		if task, _ := db.GetTask(r.DB, tid); task == nil || task.Owner != want || task.Acting != r.k(("human")) {
			t.Errorf("T%d: owner %+v, want %s under the founder's turn", tid, task, want)
		}
	}
	if again, err := taskwrites.ReturnFounderTasks(r.DB, r.home); err != nil || len(again) != 0 {
		t.Fatalf("it runs once: %q %v", again, err)
	}

}
