package supervisor

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// draft opens a draft for owner, with the task_new event a plan's due-ness counts from.
func (r *rig) draft(owner, opener, title string) int64 {
	r.t.Helper()
	tid, err := db.NewTask(r.DB, title, r.k(owner), r.k(opener), 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	_, err = db.AddEvent(r.DB, r.k(opener), "task_new", db.KV{K: "task_id", V: tid}, db.KV{K: "draft", V: true})
	r.must(err)
	return tid
}

func (r *rig) planSays(agent string) []string {
	var out []string
	for _, s := range r.saysTo(agent) {
		if strings.HasPrefix(s, "Write the plan for ") || strings.HasPrefix(s, "Write the core logic of ") {
			out = append(out, s)
		}
	}
	return out
}

func TestADraftsPlanIsAskedOnceAndWaitsBehindWhatItsAssigneeIsDoing(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	busy := r.newTask("bob", "alice", "the work bob is on")
	_, _, err := r.AdvanceCurrentTask(r.DB, r.k("bob"))
	r.must(err)
	tid := r.draft("bob", "alice", "the draft")

	r.must(r.SweepTaskAsks(true))
	r.must(r.SweepTaskAsks(true))
	asks, err := db.TaskAsks(r.DB, db.PlanAskKind, tid)
	r.must(err)
	if len(asks) != 1 || asks[0].Behind != busy || asks[0].Sent != 0 || asks[0].By != db.NoticeRequester {
		t.Fatalf("one ask, waiting behind T%d: %+v", busy, asks)
	}
	if says := r.planSays("bob"); len(says) != 0 {
		t.Fatalf("sent while bob is still doing T%d: %q", busy, says)
	}
	if st, _, _, _ := db.PlanState(r.DB, tid); st != "asked" {
		t.Fatalf("plan state %q", st)
	}

	_, err = db.MarkTaskDone(r.DB, busy, "sha", "done")
	r.must(err)
	r.must(r.SweepTaskAsks(true))
	r.must(r.SweepTaskAsks(true))
	says := r.planSays("bob")
	if len(says) != 1 || !strings.HasPrefix(says[0], "Write the plan for T2 (the draft). Read its brief (`hl task show T2`)") ||
		!strings.Contains(says[0], "`hl task plan T2 --file <path>`") {
		t.Fatalf("one ask once bob is done: %q", says)
	}
	if st, _, _, _ := db.PlanState(r.DB, tid); st != "writing" {
		t.Fatalf("plan state %q", st)
	}

	_, err = db.AddPlan(r.DB, tid, r.k("bob"), "change x")
	r.must(err)
	r.must(r.SweepTaskAsks(true))
	if st, _, _, _ := db.PlanState(r.DB, tid); st != "written" || len(r.planSays("bob")) != 1 {
		t.Fatalf("a plan answers the ask: %q, %q", st, r.planSays("bob"))
	}

	// A new brief asks again, for this task only; a draft not opened as one (sent back to draft) is not asked about.
	other := r.draft("bob", "alice", "another draft")
	_, err = db.AddPlan(r.DB, other, r.k("bob"), "plan y")
	r.must(err)
	back, err := db.NewTask(r.DB, "sent back to draft", r.k("bob"), r.k("alice"), 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	_, err = db.AddEvent(r.DB, r.k("alice"), "task_new", db.KV{K: "task_id", V: back})
	r.must(err)
	_, err = db.AddEvent(r.DB, r.k("alice"), "edit_task", db.KV{K: "task_id", V: tid}, db.KV{K: "body", V: map[string]any{"old": "a", "new": "b"}})
	r.must(err)
	r.must(r.SweepTaskAsks(true))
	if says := r.planSays("bob"); len(says) != 2 || !strings.HasPrefix(says[1], "Write the plan for T2 ") {
		t.Fatalf("the edited draft's plan is asked again: %q", says)
	}
	for _, tid := range []int64{other, back} {
		if asks, _ := db.TaskAsks(r.DB, db.PlanAskKind, tid); len(asks) != 0 {
			t.Fatalf("T%d was asked: %+v", tid, asks)
		}
	}
}

func TestPlansManualAsksNothingAndAStaleAskIsDropped(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.draft("bob", "alice", "the draft")
	r.must(r.SweepTaskAsks(false))
	if asks, _ := db.TaskAsks(r.DB, db.PlanAskKind, tid); len(asks) != 0 {
		t.Fatalf("manual asked: %+v", asks)
	}
	// An ask made for bob whose task went to alice before it was sent is dropped, not typed to either.
	_, err := db.AddTaskAsk(r.DB, db.PlanAskKind, tid, "human", r.k("bob"), "", 0)
	r.must(err)
	_, err = db.AddTaskAsk(r.DB, db.CoreAskKind, tid, "human", r.k("bob"), "the parser", 0)
	r.must(err)
	_, err = r.DB.Exec("UPDATE tasks SET owner = ? WHERE id = ?", r.k("alice"), tid)
	r.must(err)
	r.must(r.SweepTaskAsks(false))
	for _, kind := range []string{db.PlanAskKind, db.CoreAskKind} {
		asks, _ := db.TaskAsks(r.DB, kind, tid)
		if len(asks) != 1 || !asks[0].Dropped {
			t.Fatalf("%s not dropped: %+v", kind, asks)
		}
	}
	if len(r.planSays("bob"))+len(r.planSays("alice")) != 0 {
		t.Fatal("a dropped ask was sent")
	}
	// A plan written while its ask waited answers the ask: it is dropped.
	answered := r.draft("bob", "alice", "planned early")
	_, err = db.AddTaskAsk(r.DB, db.PlanAskKind, answered, "human", r.k("bob"), "", 0)
	r.must(err)
	_, err = db.AddPlan(r.DB, answered, r.k("bob"), "the plan")
	r.must(err)
	r.must(r.SweepTaskAsks(false))
	if asks, _ := db.TaskAsks(r.DB, db.PlanAskKind, answered); len(asks) != 1 || !asks[0].Dropped || len(r.planSays("bob")) != 0 {
		t.Fatalf("an answered ask was not dropped: %+v %q", asks, r.planSays("bob"))
	}
}

func TestACoreLogicAskIsSentWithItsNoteOnePerAgentASweep(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.draft("bob", "alice", "the parser")
	_, err := db.AddTaskAsk(r.DB, db.CoreAskKind, tid, "human", r.k("bob"), "the tokenizer", 0)
	r.must(err)
	_, err = db.AddTaskAsk(r.DB, db.PlanAskKind, tid, "human", r.k("bob"), "", 0)
	r.must(err)
	r.must(r.SweepTaskAsks(false))
	says := r.planSays("bob")
	if len(says) != 1 || !strings.HasPrefix(says[0], "Write the core logic of T1 (the parser): only its key function and that function's test") ||
		!strings.Contains(says[0], "Which part: the tokenizer") || !strings.Contains(says[0], "Commit on branch bob-T1 ") {
		t.Fatalf("the core ask goes first, alone: %q", says)
	}
	r.must(r.SweepTaskAsks(false))
	if says := r.planSays("bob"); len(says) != 2 || !strings.HasPrefix(says[1], "Write the plan for T1 ") {
		t.Fatalf("the plan ask follows at the next sweep: %q", says)
	}
}

func TestAPlanAskIsDroppedOnceItsDraftIsQueuedAndACoreAskOnceItsTaskIsBeingDone(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.draft("bob", "alice", "the draft")
	_, err := db.AddTaskAsk(r.DB, db.PlanAskKind, tid, "human", r.k("bob"), "", 0)
	r.must(err)
	_, err = db.SetTaskQueued(r.DB, tid, r.k("alice"))
	r.must(err)
	_, _, err = r.AdvanceCurrentTask(r.DB, r.k("bob"))
	r.must(err)
	_, err = db.AddTaskAsk(r.DB, db.CoreAskKind, tid, "human", r.k("bob"), "", 0)
	r.must(err)
	r.must(r.SweepTaskAsks(true))
	for _, kind := range []string{db.PlanAskKind, db.CoreAskKind} {
		if asks, _ := db.TaskAsks(r.DB, kind, tid); len(asks) != 1 || !asks[0].Dropped {
			t.Fatalf("%s not dropped: %+v", kind, asks)
		}
	}
	if says := r.planSays("bob"); len(says) != 0 {
		t.Fatalf("an ask reached bob while he works the task: %q", says)
	}
}

func TestAnAgentWritingAPlanGetsNoOtherAskUntilItAnswersOrAnHourPasses(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	first := r.draft("bob", "alice", "one")
	r.draft("bob", "alice", "two")
	for range 3 {
		r.must(r.SweepTaskAsks(true))
	}
	if says := r.planSays("bob"); len(says) != 1 || !strings.HasPrefix(says[0], "Write the plan for T1 ") {
		t.Fatalf("one ask while the first plan is being written: %q", says)
	}
	_, err := db.AddPlan(r.DB, first, r.k("bob"), "plan one")
	r.must(err)
	r.must(r.SweepTaskAsks(true))
	if says := r.planSays("bob"); len(says) != 2 || !strings.HasPrefix(says[1], "Write the plan for T2 ") {
		t.Fatalf("the next ask once the first is answered: %q", says)
	}
	r.draft("bob", "alice", "three")
	r.clock += PlanWaitS + 1
	r.must(r.SweepTaskAsks(true))
	r.must(r.SweepTaskAsks(true))
	if says := r.planSays("bob"); len(says) != 3 || !strings.HasPrefix(says[2], "Write the plan for T3 ") {
		t.Fatalf("an unanswered ask holds the next one an hour at most: %q", says)
	}
}
