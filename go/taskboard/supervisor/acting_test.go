package supervisor

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/taskboard/taskwrites"
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
	if !strings.Contains(r.term.last(), fmt.Sprintf("T%d body edited by @bob/lead: check the cache too", tid)) {
		t.Fatalf("alice is not told: %v", r.term.typed)
	}
	r.agentUp("erin")
	founders := r.newTask(actor.OwnerKey, r.k("carol"), "the founder's")
	r.must(db.PushTurn(r.DB, founders, db.Turn{Agent: r.k("erin"), By: actor.OwnerKey, Since: r.clock}))
	r.beginOnTop(founders)
	r.term.typed = nil
	r.turnCall("human", "task.edit", fmt.Sprintf(`{"task":%d,"body":"and the docs","request_id":"e2"}`, founders))
	if len(r.term.typed) != 1 || !strings.Contains(r.term.last(), fmt.Sprintf("T%d body edited by @human: and the docs", founders)) {
		t.Fatalf("erin is not told: %v", r.term.typed)
	}
}

// A comment tells the agent on top at once, as the task's assignee; its owner hears of a plain comment only through the board.
func TestACommentTellsTheAgentOnTop(t *testing.T) {
	r, tid := turnRig(t)
	got, ok := r.CommentRecipients(tid, r.k("dave"), "plain", false)
	if !ok || len(got) != 1 || got[0].Agent != r.k("alice") || got[0].Why != "assignee" {
		t.Fatalf("%+v", got)
	}
}

// foundersRecipients is whom the founder's comment text on task tid is told, as "agent:why" each.
func (r *rig) foundersRecipients(tid int64, text string) string {
	r.t.Helper()
	got, ok := r.CommentRecipients(tid, actor.OwnerKey, text, false)
	if !ok {
		r.t.Fatal("no task")
	}
	var out []string
	for _, g := range got {
		out = append(out, g.Agent+":"+g.Why)
	}
	return strings.Join(out, " ")
}

// The founder's comment naming no one on a task they hold themselves (a story) is told to the last agent that commented there, the one who
// spoke to them; one naming @x is told to x only; with no other writer it reaches no one.
func TestTheFoundersCommentOnTheirOwnTaskGoesToTheLastAgentThatWroteThere(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	tid, err := db.NewTask(r.DB, "the story", actor.OwnerKey, actor.OwnerKey, 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	who := func(text string) string { t.Helper(); return r.foundersRecipients(tid, text) }
	if got := who("plain"); got != "" {
		t.Fatalf("no one else wrote, yet %q", got)
	}
	for _, w := range []string{r.k("alice"), r.k("bob"), actor.OwnerKey} {
		_, err := db.AddComment(r.DB, tid, w, "a word")
		r.must(err)
	}
	if got := who("plain"); got != r.k("bob")+":last_writer" {
		t.Fatalf("%q", got)
	}
	if got := who("ask <@" + r.k("alice") + ">"); got != r.k("alice")+":asked" {
		t.Fatalf("%q", got)
	}
	// An agent's comment on a task it holds stays the holder's to answer: its progress notes reach no one who once wrote there.
	held := r.newTask("alice", r.k("carol"), "alice's work")
	_, err = db.AddComment(r.DB, held, r.k("bob"), "a word")
	r.must(err)
	if got, ok := r.CommentRecipients(held, r.k("alice"), "halfway", false); !ok || len(got) != 0 {
		t.Fatalf("an agent's progress note was told: %+v", got)
	}
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"why is it paused?","request_id":"c1"}`, tid))
	if says := fmt.Sprint(r.saysTo("bob")); !strings.Contains(says, "why is it paused?") {
		t.Fatalf("bob is not told: %s", says)
	}
}

// The founder's comment naming no one on a draft is told to the agent that opened it, now, and not to its assignee, who hears of the draft
// only when it is queued; one naming an agent still reaches that agent.
func TestTheFoundersCommentOnADraftReachesItsOpener(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	tid, err := db.NewTask(r.DB, "a draft", r.k("bob"), r.k("carol"), 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	who := func(text string) string { t.Helper(); return r.foundersRecipients(tid, text) }
	if got := who("why this?"); got != r.k("carol")+":owner" {
		t.Fatalf("the draft's opener, not its assignee: %q", got)
	}
	if got := who("<@" + r.k("alice") + "> look"); got != r.k("alice")+":asked" {
		t.Fatalf("a named agent: %q", got)
	}
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"which git hosts?","request_id":"d1"}`, tid))
	if says := fmt.Sprint(r.saysTo("carol")); !strings.Contains(says, "which git hosts?") {
		t.Fatalf("carol is not told: %s", says)
	}
	if says := fmt.Sprint(r.saysTo("bob")); strings.Contains(says, "which git hosts?") {
		t.Fatalf("the draft's assignee is told: %s", says)
	}
	r.must(db.PushTurn(r.DB, tid, db.Turn{Agent: r.k("alice"), By: actor.OwnerKey, Since: r.clock}))
	if got := who("over to you"); got != r.k("alice")+":assignee" {
		t.Fatalf("a draft that carries a turn: %q", got)
	}
	_, err = r.DB.Exec("DELETE FROM task_turns WHERE task_id = ?", tid)
	r.must(err)
	_, err = db.SetTaskQueued(r.DB, tid, r.k("carol"))
	r.must(err)
	if got := who("and now?"); got != r.k("bob")+":assignee" {
		t.Fatalf("once queued, its assignee: %q", got)
	}
	// a draft the founder opened has no one to answer for it but them: their comment is told to no one
	mine, err := db.NewTask(r.DB, "my draft", r.k("bob"), actor.OwnerKey, 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	if got, _ := r.CommentRecipients(mine, actor.OwnerKey, "a note", false); len(got) != 0 {
		t.Fatalf("the founder's own draft: %+v", got)
	}
}

// A comment that names someone reaches only them, not the task's holder.
func TestACommentThatNamesSomeoneSkipsTheHolder(t *testing.T) {
	r, tid := turnRig(t)
	got, ok := r.CommentRecipients(tid, r.k("dave"), "<@"+r.k("carol")+"> look", false)
	if !ok || len(got) != 1 || got[0].Agent != r.k("carol") || got[0].Why != "asked" {
		t.Fatalf("%+v", got)
	}
}

// The founder's comment that gives their turn back reaches the holder it goes back to, whomever else it names.
func TestATurnGivenBackReachesItsHolderWhomeverItNames(t *testing.T) {
	r, tid := turnRig(t)
	r.must(db.PushTurn(r.DB, tid, db.Turn{Agent: actor.OwnerKey, By: r.k("alice"), Since: r.clock}))
	got, ok := r.CommentRecipients(tid, actor.OwnerKey, "yes, and <@"+r.k("carol")+"> see this", false)
	if !ok || fmt.Sprint(got) != fmt.Sprint([]taskwrites.Recipient{{Agent: r.k("alice"), Why: "assignee"}, {Agent: r.k("carol"), Why: "asked"}}) {
		t.Fatalf("%+v", got)
	}
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"yes, and <@%s> see this","request_id":"c1"}`, tid, r.k("carol")))
	var back string // the task's return to alice carries the comment
	r.must(r.DB.QueryRow(r.sql("SELECT body FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = ? ORDER BY r.id DESC LIMIT 1"), tid, r.k("alice")).Scan(&back))
	if !strings.Contains(back, "see this") {
		t.Fatalf("alice is not told: %q", back)
	}
	if says := fmt.Sprint(r.saysTo("carol")); !strings.Contains(says, "see this") {
		t.Fatalf("carol is not told: %s", says)
	}
}

// The founder's question on a task Ready for them is told to the agent below their turn, which answers it in a comment; their turn stays.
func TestTheFoundersQuestionOnAReadyTaskIsToldToItsAssignee(t *testing.T) {
	r, tid := turnRig(t)
	mid, err := db.AddComment(r.DB, tid, r.k("alice"), "built it")
	r.must(err)
	r.must(db.AddReady(r.DB, tid, r.k("alice"), r.k("carol"), mid))
	r.must(db.PushTurn(r.DB, tid, db.Turn{Agent: actor.OwnerKey, By: r.k("alice"), Since: r.clock, Comment: mid}))
	if got, ok := r.CommentRecipients(tid, actor.OwnerKey, "why the yellow?", false); !ok || len(got) != 1 || got[0].Agent != r.k("alice") {
		t.Fatalf("%+v", got)
	}
	if got, ok := r.CommentRecipients(tid, actor.OwnerKey, "<@"+r.k("carol")+"> why?", false); !ok || len(got) != 2 || got[0].Agent != r.k("alice") {
		t.Fatalf("a question naming someone else must still reach the holder: %+v", got)
	}
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"why the yellow?","request_id":"c1"}`, tid))
	says := fmt.Sprint(r.saysTo("alice"))
	if !strings.Contains(says, "why the yellow?") || !strings.Contains(says, fmt.Sprintf("T%d stays Ready for", tid)) {
		t.Fatalf("alice is not asked: %s", says)
	}
	if a := r.acting(tid); a != actor.OwnerKey {
		t.Fatalf("the question passed the founder's turn to %s", a)
	}
}

// The agent on top may start the task and keep progress on it; its owner may do neither.
func TestTheAgentOnTopStartsTheTaskAndKeepsItsProgress(t *testing.T) {
	r, tid := turnRig(t)
	r.refusedCall("bob", "task.start", fmt.Sprintf(`{"task":%d,"request_id":"s0"}`, tid), "<@alice>'s")
	r.turnCall("alice", "task.start", fmt.Sprintf(`{"task":%d,"request_id":"s1"}`, tid))
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("alice")); !ok || cur != tid {
		t.Fatalf("alice holds %d %v", cur, ok)
	}
	r.refusedCall("bob", "task.progress", fmt.Sprintf(`{"task":%d,"note":"x","request_id":"g0"}`, tid), "<@alice>'s")
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

// The founder's discard of a task they own, with alice on top holding it before its delivery reached her, tells alice to stop and moves
// her on to her next task.
func TestADiscardStopsTheAgentOnTopAndMovesItOn(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	for _, a := range []string{r.k("alice"), r.k("carol")} {
		r.agentUp(a)
	}
	tid := r.newTask(actor.OwnerKey, r.k("carol"), "the founder's")
	r.must(db.PushTurn(r.DB, tid, db.Turn{Agent: r.k("alice"), By: actor.OwnerKey, Since: r.clock}))
	r.beginOnTop(tid)
	// held, its delivery still queued: the stop is typed to alice, as no delivery will reach her
	_, err := r.DB.Exec("UPDATE requests SET delivered_at = NULL, state = 'queued' WHERE id IN (SELECT request_id FROM task_requests WHERE task_id = ?)", tid)
	r.must(err)
	next := r.newTask("alice", r.k("carol"), "next")
	r.turnCall(actor.OwnerKey, "task.discard", fmt.Sprintf(`{"task":%d,"reason":"not needed","request_id":"d1"}`, tid))
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("alice")); !ok || cur != next {
		t.Fatalf("alice moved on: %d %v", cur, ok)
	}
	told := false
	for _, s := range r.term.typed {
		told = told || strings.Contains(s, fmt.Sprintf("T%d discarded by @human - stop work on it", tid))
	}
	if !told {
		t.Fatalf("alice is told to stop: %v", r.term.typed)
	}
}
