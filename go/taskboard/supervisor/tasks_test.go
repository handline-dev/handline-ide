package supervisor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// newTask opens an approved task with no request out: queued, never delivered.
func (r *rig) newTask(owner, opener, title string) int64 {
	r.t.Helper()
	tid, err := db.NewTask(r.DB, title, r.k(owner), r.k(opener), 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	_, err = db.SetTaskQueued(r.DB, tid, r.k(opener))
	r.must(err)
	return tid
}

func TestAdvanceDeliversTheNextQueuedTaskAndMakesItCurrent(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.newTask("bob", "alice", "write the doc")
	changed, refused, err := r.AdvanceCurrentTask(r.DB, "bob")
	r.must(err)
	if !changed || len(refused) != 0 {
		t.Fatalf("%v %v", changed, refused)
	}
	if cur, ok, _ := db.CurrentTask(r.DB, "bob"); !ok || cur != tid {
		t.Fatalf("current %d %v", cur, ok)
	}
	reqs, _ := db.TaskRequests(r.DB, tid)
	if len(reqs) != 1 || reqs[0].Requester != "alice" || reqs[0].Target != "bob" || reqs[0].Kind.String != "task" || reqs[0].Body != "write the doc" {
		t.Fatalf("%+v", reqs)
	}
	task, _ := db.GetTask(r.DB, tid)
	if !task.SentAt.Valid {
		t.Fatal("sent_at not set")
	}
	// the same task with its request out is still being asked: nothing more to advance
	if changed, _, _ := r.AdvanceCurrentTask(r.DB, "bob"); changed {
		t.Fatal("advanced past a task still asking")
	}
	// the founder holds no current task
	if changed, _, _ := r.AdvanceCurrentTask(r.DB, "human"); changed {
		t.Fatal("advanced the founder")
	}
}

// newUnroutableTask queues a task for eng.bob that its opener ops.carol, an agent of another workspace, may not ask of it; eng leads its
// workspace.
func (r *rig) newUnroutableTask() int64 {
	r.t.Helper()
	r.register("eng", "eng.bob", "ops.carol")
	return r.newTask("eng.bob", "ops.carol", "unroutable")
}

func TestAdvanceDeliversATaskHandedBackToItsOpenerAsItsOwnWork(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.newTask("bob", "bob", "finish the doc")
	changed, refused, err := r.AdvanceCurrentTask(r.DB, "bob")
	r.must(err)
	if !changed || len(refused) != 0 {
		t.Fatalf("%v %v", changed, refused)
	}
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != tid {
		t.Fatal(cur)
	}
	reqs, _ := db.TaskRequests(r.DB, tid)
	if len(reqs) != 1 || reqs[0].Target != "bob" || reqs[0].Kind.String != "task" || reqs[0].Body != "finish the doc" {
		t.Fatalf("%+v", reqs)
	}
}

func TestSelfDeliveryCarriesWhatItsRequestsClosedWith(t *testing.T) {
	r := newRig(t)
	tid := r.newTask("bob", "bob", "finish the doc")
	asked := r.ask("bob", "alice", "which format?", "ask")
	r.must(db.LinkRequest(r.DB, tid, asked))
	if ok, err := db.CloseIfQueued(r.DB, asked, "done", "pdf"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	task, _ := db.GetTask(r.DB, tid)
	row, _ := db.GetRequest(r.DB, r.selfDelivery(r.DB, "bob", task))
	if r.ids(row.Body) != fmt.Sprintf("finish the doc\n\nanswer to #%d: pdf", asked) {
		t.Fatal(row.Body)
	}
}

func TestAdvanceRecordsARefusedDeliveryOnceAndTellsTheManager(t *testing.T) {
	r := newRig(t)
	tid := r.newUnroutableTask()
	other := r.newTask("eng.bob", "eng", "fine")
	changed, refused, err := r.AdvanceCurrentTask(r.DB, "eng.bob")
	r.must(err)
	if !changed {
		t.Fatal("the second task was not delivered")
	}
	if refused[tid] == "" {
		t.Fatalf("%v", refused)
	}
	if cur, _, _ := db.CurrentTask(r.DB, "eng.bob"); cur != other {
		t.Fatal(cur)
	}
	var n int
	r.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'task_deliver_refused'").Scan(&n)
	r.AdvanceCurrentTask(r.DB, "eng.bob")
	var m int
	r.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'task_deliver_refused'").Scan(&m)
	if n != 1 || m != 1 {
		t.Fatalf("a permanent refusal fills the log: %d %d", n, m)
	}
	var told int
	r.DB.QueryRow("SELECT COUNT(*) FROM requests WHERE requester = ? AND target = 'eng' AND body LIKE ?", db.NoticeRequester, fmt.Sprintf("T%d could not be delivered%%", tid)).Scan(&told)
	if told != 1 {
		t.Fatalf("the refusal was told %d times", told)
	}
}

func TestAdvanceClearsACurrentTaskThatHasNothingLeft(t *testing.T) {
	r := newRig(t)
	tid := r.newTask("bob", "alice", "done soon")
	db.SetCurrentTask(r.DB, "bob", tid, true)
	db.MarkTaskDone(r.DB, tid, "ok", "reason")
	if changed, _, _ := r.AdvanceCurrentTask(r.DB, "bob"); !changed {
		t.Fatal("not cleared")
	}
	if _, ok, _ := db.CurrentTask(r.DB, "bob"); ok {
		t.Fatal("still current")
	}
	// a blocked current task is let go
	blocked := r.newTask("bob", "alice", "blocked")
	db.SetCurrentTask(r.DB, "bob", blocked, true)
	blocker := r.newTask("alice", "alice", "first")
	db.AddBlock(r.DB, blocked, blocker)
	if changed, _, _ := r.AdvanceCurrentTask(r.DB, "bob"); !changed {
		t.Fatal("kept a blocked current task")
	}
	if _, ok, _ := db.CurrentTask(r.DB, "bob"); ok {
		t.Fatal("still current")
	}
}

func TestQueueDeliverSaysWhetherTheTaskWentOut(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid, _ := db.NewTask(r.DB, "job", "bob", "alice", 0, db.DefaultPriority, "", 0, "")
	db.SetTaskQueued(r.DB, tid, "human")
	row, _ := db.GetTask(r.DB, tid)
	msg, err := r.QueueDeliver(r.DB, tid, "human", true, row)
	r.must(err)
	if msg != "task T1 is queued (human) - bob was free, so it went out now" {
		t.Fatal(msg)
	}
	// a busy owner: queued behind the one it holds
	busy := r.newTask("bob", "alice", "second")
	row, _ = db.GetTask(r.DB, busy)
	msg, _ = r.QueueDeliver(r.DB, busy, "human", true, row)
	if msg != "task T2 is queued (human)" {
		t.Fatal(msg)
	}
	// back to draft
	if msg, _ := r.QueueDeliver(r.DB, busy, "human", false, row); msg != "task T2 is draft (human)" {
		t.Fatal(msg)
	}
	// the founder's task: the card goes back on their board
	ft, _ := db.NewTask(r.DB, "decide", "human", "alice", 0, db.DefaultPriority, "decision", 0, "")
	row, _ = db.GetTask(r.DB, ft)
	msg, _ = r.QueueDeliver(r.DB, ft, "human", true, row)
	if msg != "task T3 is queued (human) - its card is on the founder's board" {
		t.Fatal(msg)
	}
}

func TestADiscardedBlockerReleasesWhatItBlockedOnce(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	blocker := r.newTask("alice", "alice", "first")
	waiting := r.newTask("bob", "alice", "second")
	db.AddBlock(r.DB, waiting, blocker)
	db.DiscardTask(r.DB, blocker, "alice", "not needed")
	freed, err := r.ReleaseEndedBlockers()
	r.must(err)
	if len(freed) != 1 || freed[0] != waiting {
		t.Fatalf("%v", freed)
	}
	if bs, _ := db.TaskBlockers(r.DB, waiting); len(bs) != 0 {
		t.Fatal("still blocked")
	}
	typed := strings.Join(r.term.typed, "\n")
	if !strings.Contains(typed, "second") || strings.Contains(typed, "is unblocked") {
		t.Fatalf("%q", r.term.typed)
	}
	if req, _ := db.GetRequest(r.DB, 1); req.State != "withdrawn" {
		t.Fatalf("the unblocked say left as %s", req.State)
	}
	// and it is the owner's current task now
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != waiting {
		t.Fatalf("current %d", cur)
	}
	again, _ := r.ReleaseEndedBlockers()
	if len(again) != 0 {
		t.Fatalf("released twice: %v", again)
	}
}

func TestADoneBlockerReleasesOncePerFinishAndAChildsResultIsNotTold(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	parent := r.newTask("bob", "alice", "parent")
	child, _ := db.NewTask(r.DB, "child", "alice", "bob", parent, db.DefaultPriority, "", 0, "")
	db.AddBlock(r.DB, parent, child)
	db.MarkTaskDone(r.DB, child, "built it", "reason")
	freed, err := r.ReleaseEndedBlockers()
	r.must(err)
	if len(freed) != 1 || freed[0] != parent {
		t.Fatalf("%v", freed)
	}
	var results int
	r.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'child_result_told'").Scan(&results)
	if results != 0 || strings.Contains(strings.Join(r.term.typed, "\n"), "done by alice") {
		t.Fatalf("a child's result that did not fail is not told to the parent's owner: %d %q", results, r.term.typed)
	}
	typed := strings.Join(r.term.typed, "\n")
	if left, _ := db.PendingNotices(r.DB, "bob"); len(left) != 0 {
		t.Fatalf("a shown notice stays pending: %+v", left)
	}
	if strings.Contains(typed, "is unblocked") {
		t.Fatalf("the task went out, the unblocked say is extra: %q", r.term.typed)
	}
	// the block row stays for a reopen: no second release for the same finish
	r.ReleaseEndedBlockers()
	var released int
	r.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'released_at_done'").Scan(&released)
	if released != 1 {
		t.Fatalf("released again for the same finish: %d", released)
	}
}

func TestAnOwnerBusyWithItsCurrentTaskIsStillToldTheUnblock(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	busy := r.newTask("bob", "alice", "first")
	r.must(func() error { _, _, err := r.AdvanceCurrentTask(r.DB, "bob"); return err }())
	blocker := r.newTask("alice", "alice", "blocker")
	waiting := r.newTask("bob", "alice", "second")
	db.AddBlock(r.DB, waiting, blocker)
	db.DiscardTask(r.DB, blocker, "carol", "not needed") // not the blocker's owner, so the event's agent is the closer
	r.term.typed = nil
	if _, err := r.ReleaseEndedBlockers(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(r.term.typed, "\n"), "task T3 is unblocked - T2, its last open blocker, discarded") {
		t.Fatalf("%q", r.term.typed)
	}
	// chat.transcript draws this event as the freed task's card: the closer is its agent and closed_task the blocker.
	var events int
	r.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'task_unblocked' AND agent = 'carol' AND json_extract(payload, '$.task_id') = ? AND json_extract(payload, '$.closed_task') = ?",
		waiting, blocker).Scan(&events)
	if events != 1 {
		t.Fatalf("task_unblocked event: %d", events)
	}
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != busy {
		t.Fatalf("current %d", cur)
	}
}

func TestTheUnblockIsStillTypedWhenTheAdvanceSendsSomethingElseOrNothing(t *testing.T) {
	for _, c := range []struct {
		name  string
		draft bool
		setup func(r *rig) int64
	}{
		{"a draft goes nowhere", true, func(r *rig) int64 { return 0 }},
		{"a higher task goes first", false, func(r *rig) int64 {
			tid, _ := db.NewTask(r.DB, "urgent", "bob", "alice", 0, 1, "", 0, "")
			db.SetTaskQueued(r.DB, tid, "alice")
			return tid
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			r.agentUp("bob")
			blocker := r.newTask("alice", "alice", "blocker")
			waiting, _ := db.NewTask(r.DB, "second", "bob", "alice", 0, db.DefaultPriority, "", 0, "")
			if !c.draft {
				db.SetTaskQueued(r.DB, waiting, "alice")
			}
			db.AddBlock(r.DB, waiting, blocker)
			other := c.setup(r)
			db.DiscardTask(r.DB, blocker, "alice", "not needed")
			if _, err := r.ReleaseEndedBlockers(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(r.term.typed, "\n"), fmt.Sprintf("task T%d is unblocked - T%d, its last open blocker, discarded", waiting, blocker)) {
				t.Fatalf("%q", r.term.typed)
			}
			if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != other {
				t.Fatalf("current %d, want %d", cur, other)
			}
			var refusals int
			r.must(r.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'task_deliver_refused'").Scan(&refusals))
			if refusals != 0 {
				t.Fatal("the unblock notice made the draft look sent, and its delivery on nobody's behalf was refused")
			}
		})
	}
}

// The stall this guards against: a memo with no @ on a task never reached the agent holding it, and it sat idle.
func TestAMemoWithNoAtTellsTheAssigneeWhateverTheTaskState(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.newTask("bob", "alice", "job")
	r.must(r.TellMemo(r.DB, tid, "alice", "added", "check the cache", "", false))
	if says := r.saysTo("bob"); len(says) != 1 || !strings.Contains(says[0], "T1 comment by alice: check the cache") {
		t.Fatalf("the assignee of a task not yet doing was not told of a memo with no @: %q", says)
	}
	r.AdvanceCurrentTask(r.DB, "bob")
	r.term.typed = nil
	r.must(r.TellMemo(r.DB, tid, "alice", "added", "check it twice", "", false))
	if says := r.saysTo("bob"); len(says) != 2 {
		t.Fatalf("a memo with no @ did not reach the doing assignee: %q", says)
	}
	if says := r.saysTo("alice"); len(says) != 0 {
		t.Fatalf("the writer was told: %q", says)
	}
}

// The stall this guards against: the founder commented on a task it holds and nobody was told. Its reporter is; the founder, who reads the
// board, gets no inbox row.
func TestTheFoundersMemoOnATaskItHoldsReachesTheReporter(t *testing.T) {
	r := newRig(t)
	tid := r.newTask(db.Human, "alice", "decide")
	r.must(r.TellMemo(r.DB, tid, db.Human, "added", "go with the reassign model", "", false))
	if says := r.saysTo("alice"); len(says) != 1 || says[0] != "T1 comment by human: go with the reassign model" {
		t.Fatalf("the reporter was not told: %q", says)
	}
	if says := r.saysTo(db.Human); len(says) != 0 {
		t.Fatalf("the founder was told: %q", says)
	}
}

func TestSweepTaskQueueDeliversAWaitingTaskToAFreeOwnerAndBacksOffWhenNothingLands(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.newTask("bob", "alice", "waiting job")
	r.must(r.SweepTaskQueue(r.clock))
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != tid {
		t.Fatalf("current %d", cur)
	}
	if !r.logged("bob: a queued task was waiting and it is free - delivered") {
		t.Fatal(r.logs)
	}
	// a refusing task sets the retry clock
	r2 := newRig(t)
	r2.newUnroutableTask()
	r2.must(r2.SweepTaskQueue(r2.clock))
	if n, _ := db.Cursor(r2.DB, "taskqueue:eng.bob"); n != r2.clock {
		t.Fatalf("retry clock %d", n)
	}
	r2.logs = nil
	r2.must(r2.SweepTaskQueue(r2.clock + 10))
	var events int
	r2.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'task_deliver_refused'").Scan(&events)
	if events != 1 {
		t.Fatalf("retried inside the minute: %d", events)
	}
}

// The deadlock that held an agent's whole queue for a day: its current task, begun, waits on a task of its own through a block that went
// dormant (one agent holds both), and that task waits behind it.
func TestSweepDeliversTheDormantBlockerOfABegunTaskBeforeTheRestOfTheQueue(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	held := r.newTask("bob", "alice", "the graph")
	_, _, err := r.AdvanceCurrentTask(r.DB, "bob")
	r.must(err)
	_, err = db.AddEvent(r.DB, "bob", "task_begin", db.KV{K: "task_id", V: held})
	r.must(err)
	blocker := r.newTask("bob", "alice", "the frame the graph follows")
	other := r.newTask("bob", "alice", "something else")
	_, err = r.DB.Exec("INSERT INTO task_blocks(task_id, blocks_on, dormant, created_at) VALUES(?, ?, 1, 1)", held, blocker)
	r.must(err)
	r.must(r.SweepTaskQueue(r.clock))
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != blocker {
		t.Fatalf("current is T%d, want its dormant blocker T%d (not T%d)", cur, blocker, other)
	}
	if !r.logged("bob: its current task waits on a task of its own behind it - delivered that one first") {
		t.Fatal(r.logs)
	}
	// the blocker done, the task that waited on it is current again, ahead of the rest of the queue
	_, err = db.MarkTaskDone(r.DB, blocker, "framed", "test")
	r.must(err)
	_, _, err = r.AdvanceCurrentTask(r.DB, "bob")
	r.must(err)
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != held {
		t.Fatalf("current is T%d, want T%d back once its blocker is done (not T%d)", cur, held, other)
	}
}

// register gives each name an agent row, so the daemon knows it.
func (r *rig) register(names ...string) { teamtest.Seed(r.t, r.home, names...) }

// saysTo is the bodies of the says written to agent's inbox.
func (r *rig) saysTo(agent string) []string {
	r.t.Helper()
	rows, err := r.DB.Query("SELECT body FROM requests WHERE target=? AND kind='say' ORDER BY id", r.k(agent))
	r.must(err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		r.must(rows.Scan(&b))
		out = append(out, r.named(b))
	}
	return out
}

// The stall this guards against: the holder of a task asked, in a memo on it, an agent with no part in the task to discard a duplicate, and
// nobody was told. Anyone named with @ is told, and the assignee; the writer and the opener are not.
func TestAMemoReachesTheAgentsItNamesWhateverTheTaskState(t *testing.T) {
	r := newRig(t)
	r.register("carol", "dave")
	r.agentUp("bob")
	tid := r.newTask("bob", "alice", "job")
	r.must(r.TellMemo(r.DB, tid, "bob", "added", "@carol please discard T2, it duplicates this; mail x@dave.example and @nobody", "", false)) // local-ref-ok: an address names nobody
	if says := r.saysTo("carol"); len(says) != 1 || !strings.Contains(says[0], "T1 comment by bob: @carol please discard T2") {
		t.Fatalf("the agent named was not told: %q", says)
	}
	for _, a := range []string{"alice", "dave", "nobody", db.Human} {
		if says := r.saysTo(a); len(says) != 0 {
			t.Fatalf("%s was told: %q", a, says)
		}
	}
	r.must(r.TellMemo(r.DB, tid, "alice", "edited", "@alice @carol mind the cache", "", false))
	if len(r.saysTo("alice")) != 0 || len(r.saysTo("carol")) != 2 || len(r.saysTo("bob")) != 1 {
		t.Fatalf("the writer was told, or the agent named or the assignee was not: %q %q %q", r.saysTo("alice"), r.saysTo("carol"), r.saysTo("bob"))
	}
}

// Told at once: the holder of the task, the reader a say named, a reader the memo names with @, and anyone when the memo is sent now. The
// task's opener, with no @ or say, is not told.
func TestAMemoIsToldAtOnceOnlyToWhomItIsAimedAt(t *testing.T) {
	r := newRig(t)
	r.register("carol")
	r.agentUp("carol")
	r.agentUp("bob")
	r.must(db.SetAgentState(r.DB, "carol", states.Idle, 0, ""))
	tid := r.newTask("bob", "carol", "job")
	r.AdvanceCurrentTask(r.DB, "bob")
	r.term.typed = nil
	r.must(r.TellMemo(r.DB, tid, "alice", "added", "plain", "", false))
	if len(r.saysTo("carol")) != 0 || len(r.saysTo("bob")) != 1 {
		t.Fatalf("a memo with no @ is told to the holder only: carol %q, bob %q", r.saysTo("carol"), r.saysTo("bob"))
	}
	r.term.typed = nil
	r.must(r.TellMemo(r.DB, tid, "alice", "added", "@carol look", "", false))
	if len(r.term.typed) != 2 || !strings.Contains(r.term.last(), "T1 comment by alice: @carol look") {
		t.Fatalf("the reader named with @ was not told at once: %q", r.term.typed)
	}
	r.term.typed = nil
	r.must(r.TellMemo(r.DB, tid, "alice", "added", "said", "bob", false))
	if len(r.term.typed) != 1 || !strings.Contains(r.term.last(), "T1 comment by alice: said") {
		t.Fatalf("the reader a say named was not told at once: %q", r.term.typed)
	}
	r.term.typed = nil
	r.must(r.TellMemo(r.DB, tid, "alice", "added", "urgent", "", true))
	if len(r.term.typed) != 2 {
		t.Fatalf("a memo sent now was not told at once to both readers: %q", r.term.typed)
	}
}

// beginCurrent makes a new task of owner's current and begun, as `hl task start` leaves it.
func (r *rig) beginCurrent(owner, opener, title string) int64 {
	r.t.Helper()
	tid := r.newTask(owner, opener, title)
	owner = r.k(owner)
	_, _, err := r.AdvanceCurrentTask(r.DB, owner)
	r.must(err)
	_, err = db.AddEvent(r.DB, owner, "task_begin", db.KV{K: "task_id", V: tid})
	r.must(err)
	return tid
}

// The stall this guards against: a lead's one current task waits on another agent's task, and its queued tasks never reached it.
func TestSweepGivesAnAgentItsNextTaskWhileItsBegunTaskWaits(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	held := r.beginCurrent("bob", "alice", "the review")
	blocker := r.newTask("carol", "bob", "the list to review")
	_, err := r.DB.Exec("INSERT INTO task_blocks(task_id, blocks_on, dormant, created_at) VALUES(?, ?, 0, 1)", held, blocker)
	r.must(err)
	next := r.newTask("bob", "alice", "the next one")
	r.newTask("bob", "alice", "the one after")

	r.must(db.SetAgentState(r.DB, "bob", states.Running, 0, ""))
	r.must(r.SweepTaskQueue(r.clock))
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != next {
		t.Fatalf("current is T%d, want the next queued T%d", cur, next)
	}
	if st, _ := db.TaskState(r.DB, must(db.GetTask(r.DB, held))); st != "queued" {
		t.Fatalf("the blocked task reads %s", st)
	}
	if begun, _ := db.Begun(r.DB, held); !begun {
		t.Fatal("the blocked task lost its begin")
	}
}

// A begun task past its timeout that nobody answered pings its idle holder once per overrun notice when nothing else is queued for it.
func TestSweepPingsTheIdleHolderOfAnOverdueTaskOncePerNotice(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.beginCurrent("bob", "alice", "the long one")
	r.must(db.AdvanceTaskTimeout(r.DB, tid, r.clock, db.DefaultTaskTimeoutS))
	row, err := db.GetTask(r.DB, tid)
	r.must(err)
	r.must(db.SetAgentState(r.DB, "bob", states.Idle, 0, ""))

	r.must(r.SweepTaskQueue(r.clock))
	if says := r.saysTo("bob"); len(says) != 0 {
		t.Fatalf("a task within its timeout pinged its holder: %q", says)
	}
	// the clock runs past its end, and its holder was told
	_, err = r.DB.Exec("UPDATE tasks SET timeout_at = ? WHERE id = ?", row.TimeoutAt.Int64-db.DefaultTaskTimeoutS-60, tid)
	r.must(err)
	_, err = db.AddEvent(r.DB, "bob", "task_timeout_exceeded", db.KV{K: "task_id", V: tid}, db.KV{K: "over_s", V: 60})
	r.must(err)
	r.must(r.SweepTaskQueue(r.clock))
	r.must(r.SweepTaskQueue(r.clock + 1))
	if says := r.saysTo("bob"); len(says) != 1 || !strings.HasPrefix(says[0], fmt.Sprintf("T%d (the long one) is past its timeout, unanswered. Do it", tid)) {
		t.Fatalf("says to bob = %q, want one overdue ping", says)
	}

	next := r.newTask("bob", "alice", "the next one")
	r.must(r.SweepTaskQueue(r.clock + 2))
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != next {
		t.Fatalf("current is T%d, want T%d once something is queued behind the overdue one", cur, next)
	}
}

func TestABlockedTaskFreesItsOwnerAndComesBackWithoutPreempting(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	held := r.beginCurrent("bob", "alice", "the review")
	blocker := r.newTask("carol", "bob", "the list to review")
	next := r.newTask("bob", "alice", "the next one")
	r.must(db.AddBlock(r.DB, held, blocker))
	r.must(db.SetCurrentTask(r.DB, "bob", 0, false)) // as task.edit leaves it
	r.Edited(taskwrites.Edited{Task: held, By: "bob", Freed: "bob"})
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != next {
		t.Fatalf("current is T%d, want T%d at once", cur, next)
	}
	doing := 0
	for _, tid := range []int64{held, blocker, next} {
		if st, _ := db.TaskState(r.DB, must(db.GetTask(r.DB, tid))); st == "doing" && must(db.GetTask(r.DB, tid)).Owner == "bob" {
			doing++
		}
	}
	if doing != 1 {
		t.Fatalf("bob has %d doing", doing)
	}
	must(db.RemoveBlock(r.DB, held, blocker))
	if changed, _, _ := r.AdvanceCurrentTask(r.DB, "bob"); changed {
		t.Fatal("an unblocked task took bob off the one it is on")
	}
	must(db.MarkTaskDone(r.DB, next, "ok", "reason"))
	_, _, err := r.AdvanceCurrentTask(r.DB, "bob")
	r.must(err)
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != held {
		t.Fatalf("current is T%d, want T%d back", cur, held)
	}
	if begun, _ := db.Begun(r.DB, held); !begun {
		t.Fatal("it came back no longer begun")
	}
}

func TestAMemoNoticeToTheNewOwnerDoesNotCountAsTheTaskDelivered(t *testing.T) {
	r := newRig(t)
	r.register("bob")
	tid := r.newTask("human", "bob", "decide")
	bob := "bob"
	r.must(db.EditTask(r.DB, tid, "human", db.TaskEdit{Owner: &bob}))
	r.must(r.TellMemo(r.DB, tid, "human", "added", "go ahead", "", false)) // held for bob's digest, linked to the task
	r.must(r.SweepTaskQueue(r.clock))
	if cur, ok, _ := db.CurrentTask(r.DB, "bob"); !ok || cur != tid {
		t.Fatalf("the task was not delivered: current %d %v", cur, ok)
	}
}

// The founder's comment on a task an agent holds is that agent's to answer: the owner who opened it is not typed to as well, so two agents
// do not both answer it.
func TestTheFoundersMemoOnATaskAnAgentHoldsReachesOnlyTheHolder(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	r.agentUp("alice")
	tid := r.newTask("alice", "bob", "decide") // held by alice, opened by bob
	r.AdvanceCurrentTask(r.DB, "alice")
	r.term.typed = nil
	r.must(r.TellMemo(r.DB, tid, db.Human, "added", "go with the first", "", false))
	if got := r.saysTo("alice"); len(got) != 1 {
		t.Fatalf("the holder was not told: %v", got)
	}
	if got := r.saysTo("bob"); len(got) != 0 {
		t.Fatalf("the owner was typed to as well: %v", got)
	}
}

// A retired holder keeps the tasks it owned, and cannot answer: the founder's comment on one reaches who the task answers to.
func TestTheFoundersMemoOnARetiredHoldersTaskReachesTheOwner(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.newTask("gone", "bob", "decide")
	r.term.typed = nil
	r.must(r.TellMemo(r.DB, tid, db.Human, "added", "who picks this up?", "", false))
	if got := r.saysTo("bob"); len(got) != 1 {
		t.Fatalf("the owner was not told: %v", got)
	}
}

// The founder's comment on a task they hold is an answer someone waits for: it is typed at once to the task's owner and its assignee, not held
// for a digest as an agent's memo is (T6340, T6370).
func TestTheFoundersMemoReachesTheTaskOwnerAtOnceWhileAnAgentsMemoWithNoAtIsNotTold(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	r.agentUp("alice")
	tid := r.newTask(db.Human, "bob", "decide") // assigned to the founder, answered to bob
	r.must(r.TellMemo(r.DB, tid, db.Human, "added", "go with the first", "", false))
	if got := r.saysTo("bob"); len(got) != 1 || got[0] != "T1 comment by human: go with the first" {
		t.Fatalf("the founder's memo did not reach its owner: %v", got)
	}
	if left, _ := db.PendingNotices(r.DB, "bob"); len(left) != 0 {
		t.Fatalf("it also waits in the digest: %v", left)
	}
	typed := len(r.term.typed)
	r.must(r.TellMemo(r.DB, tid, "alice", "added", "fyi", "", false))
	if len(r.term.typed) != typed {
		t.Fatalf("an agent's memo was typed at once: %q", r.term.typed)
	}
	if left, _ := db.PendingNotices(r.DB, "bob"); len(left) != 0 {
		t.Fatalf("an agent's memo with no @ was held for a digest: %v", left)
	}
}

// A memo keeps which request told each reader, held for a digest or typed at once, so the memo can show who has read it.
func TestAMemoKeepsTheTellItSentToEachReader(t *testing.T) {
	r := newRig(t)
	r.register("carol")
	r.agentUp("carol")
	r.agentUp("bob")
	tid := r.newTask("bob", "carol", "job")
	r.AdvanceCurrentTask(r.DB, "bob")
	r.must(r.TellMemoOf(r.DB, tid, 8, "dave", "added", "@bob look", "", false))
	count := func(mid int64) int {
		var n int
		r.must(r.DB.QueryRow("SELECT COUNT(*) FROM memo_tells WHERE memo_id = ?", mid).Scan(&n))
		return n
	}
	if count(8) == 0 {
		t.Fatalf("a typed tell is kept: %d", count(8))
	}
	r.must(r.TellMemoOf(r.DB, tid, 0, "dave", "added", "unlinked", "", false))
	var n int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM memo_tells WHERE memo_id = 0").Scan(&n))
	if n != 0 {
		t.Fatal("memo 0 keeps nothing")
	}
}

func TestSayNowWithNoticesCarriesTheWaitingNoticesAndMarksThemDigested(t *testing.T) {
	r := newRig(t)
	db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: "bob", Body: "T9 done by alice: ok", Kind: "say", Notice: true})
	r.Env.SayNowWithNotices(r.DB, "bob", "You hold T1.", "held")
	var body string
	if err := r.DB.QueryRow("SELECT body FROM requests WHERE target = 'bob' AND body LIKE 'You hold T1.%'").Scan(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "T9 · alice · ok") {
		t.Fatalf("the say carries the digest: %q", body)
	}
	if left, _ := db.PendingNotices(r.DB, "bob"); len(left) != 0 {
		t.Fatal("the carried notices stay pending")
	}
}
