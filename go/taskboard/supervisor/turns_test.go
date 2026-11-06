package supervisor

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// turnCall is one method call as agent, failing the test on an error reply.
func (r *rig) turnCall(agent, method, params string) map[string]any {
	r.t.Helper()
	out := r.callAs(agent, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":%q,"params":%s}`, method, params))
	if out["error"] != nil {
		r.t.Fatalf("%s %s by %s: %v", method, params, agent, out["error"])
	}
	return out
}

// refusedCall is one method call as agent that must be refused, with words containing want.
func (r *rig) refusedCall(agent, method, params, want string) {
	r.t.Helper()
	out := r.callAs(agent, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":%q,"params":%s}`, method, params))
	if out["error"] == nil || !strings.Contains(fmt.Sprint(out["error"]), want) {
		r.t.Fatalf("%s %s by %s: want refused with %q, got %v", method, params, agent, want, out)
	}
}

// bigTeam is the rig's team with carol, dave and erin beside alice and bob, any of whom may ask any other.
func (r *rig) bigTeam() {
	r.t.Helper()
	r.register("carol", "dave", "erin")
	team := strings.NewReplacer("eng: [alice, bob]", "eng: [alice, bob, carol, dave, erin]", "  bob: {}\n", "  bob: {}\n  carol: {}\n  dave: {}\n  erin: {}\n").Replace(teamYAML)
	r.must(os.WriteFile(filepath.Join(r.home, "team.yaml"), []byte(team), 0o644))
}

func (r *rig) acting(tid int64) string {
	r.t.Helper()
	a, err := db.Acting(r.DB, tid)
	r.must(err)
	return a
}

// An agent that pushes the founder's turn frees its slot: its next queued task starts, the time box waits on the founder, and the
// founder's comment passes the turn back, the task returning to the agent's queue with the comment.
func TestTheFoundersTurnFreesTheAgentsSlotAndTheirCommentGivesItBack(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	held := r.beginCurrent("bob", "alice", "ship it")
	next := r.newTask("bob", "alice", "the next one")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","ask":"beta now or after the fix? I recommend now","request_id":"p1"}`, held))
	if r.acting(held) != db.Human {
		t.Fatalf("acting %q", r.acting(held))
	}
	if task, _ := db.GetTask(r.DB, held); task.Owner != "bob" {
		t.Fatalf("the task stays bob's: %+v", task)
	}
	if cur, ok, _ := db.CurrentTask(r.DB, "bob"); !ok || cur != next {
		t.Fatalf("bob's next task starts: current %d %v", cur, ok)
	}
	turns, _ := db.Turns(r.DB, held)
	if len(turns) != 1 || turns[0].Ask != "beta now or after the fix? I recommend now" || turns[0].By != "bob" {
		t.Fatalf("%+v", turns)
	}
	// a second founder turn, or the owner, is refused
	r.refusedCall("human", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"bob","request_id":"p2"}`, held), "already acts")
	// the founder's comment passes it: bob acts again, told with the comment
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"now","request_id":"c1"}`, held))
	if r.acting(held) != "bob" {
		t.Fatalf("acting %q", r.acting(held))
	}
	if turns, _ := db.Turns(r.DB, held); len(turns) != 0 {
		t.Fatalf("%+v", turns)
	}
	var back, from string
	r.must(r.DB.QueryRow("SELECT body, requester FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob' ORDER BY r.id DESC LIMIT 1", held).Scan(&back, &from))
	if !strings.Contains(back, "your turn again") || !strings.Contains(back, "now") {
		t.Fatalf("%q", back)
	}
	// the delivery is the board's, so the founder's chat does not show it as a request of theirs
	if from != db.NoticeRequester {
		t.Fatalf("the pass is delivered from %q", from)
	}
	var rid int64
	r.must(r.DB.QueryRow("SELECT MAX(r.id) FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob'", held).Scan(&rid))
	req, err := db.GetRequest(r.DB, rid)
	r.must(err)
	if passes, err := db.PassesPause(r.DB, req); err != nil || !passes {
		t.Fatalf("the founder's pass is held by a pause: %v %v", passes, err)
	}
}

// A review turn pushed with on_pass done closes the task when the reviewer passes it; a send-back returns it with the note as a comment.
func TestAReviewTurnClosesTheTaskOnAPassAndASendBackReturnsItWithTheNote(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	r.agentUp("alice")
	tid := r.beginCurrent("bob", "alice", "the parser")
	push := fmt.Sprintf(`{"task":%d,"to":"alice","ask":"review the parser","on_pass":"done","evidence":"PR #9, tests green","request_id":"%%s"}`, tid)
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(push, "p1"))
	if r.acting(tid) != "alice" {
		t.Fatalf("acting %q", r.acting(tid))
	}
	r.refusedCall("bob", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","request_id":"x1"}`, tid), "alice's turn")
	r.refusedCall("alice", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"send_back","request_id":"x2"}`, tid), "says why")
	r.turnCall("alice", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"send_back","note":"the empty list fails","request_id":"b1"}`, tid))
	if r.acting(tid) != "bob" {
		t.Fatalf("acting %q", r.acting(tid))
	}
	var memo string
	r.must(r.DB.QueryRow("SELECT body FROM task_memos WHERE task_id = ? AND author = 'alice'", tid).Scan(&memo))
	if memo != "the empty list fails" {
		t.Fatalf("%q", memo)
	}
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(push, "p2"))
	r.turnCall("alice", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","request_id":"a1"}`, tid))
	task, _ := db.GetTask(r.DB, tid)
	if task.DoneEvidence.String != "PR #9, tests green" {
		t.Fatalf("a pass closes it with the evidence: %+v", task)
	}
	if turns, _ := db.Turns(r.DB, tid); len(turns) != 0 {
		t.Fatalf("%+v", turns)
	}
	var pops int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'task_turn_pop' AND json_extract(payload, '$.outcome') = 'pass'").Scan(&pops))
	if pops != 1 {
		t.Fatalf("the pass is in the history: %d", pops)
	}
}

// With turns open a handoff replaces the one on top, a release by it sends the task back, the owner may not release it, a draft clears the
// stack, and at most three turns stand.
func TestTurnsUnderAHandoffAReleaseADraftAndTheLimit(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	for _, a := range []string{"alice", "bob", "carol", "dave"} {
		r.agentUp(a)
	}
	tid := r.beginCurrent("bob", "alice", "the job")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"alice","request_id":"p1"}`, tid))
	r.turnCall("alice", "task.handoff", fmt.Sprintf(`{"task":%d,"to":"carol","note":"yours to review","request_id":"h1"}`, tid))
	if task, _ := db.GetTask(r.DB, tid); task.Owner != "bob" || task.Acting != "carol" {
		t.Fatalf("the handoff replaced the top turn: %+v", task)
	}
	r.refusedCall("carol", "task.handoff", fmt.Sprintf(`{"task":%d,"to":"bob","request_id":"h2"}`, tid), "under its turns")
	r.refusedCall("bob", "task.release", fmt.Sprintf(`{"task":%d,"reason":"mine","request_id":"r0"}`, tid), "carol's is on top")
	r.turnCall("carol", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"dave","request_id":"p2"}`, tid))
	r.turnCall("dave", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","request_id":"p3"}`, tid))
	r.refusedCall("human", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"erin","request_id":"p4"}`, tid), "the most it may")
	r.turnCall("human", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","request_id":"o1"}`, tid))
	r.turnCall("dave", "task.release", fmt.Sprintf(`{"task":%d,"reason":"not mine to do","request_id":"r1"}`, tid))
	if r.acting(tid) != "carol" {
		t.Fatalf("dave's release sent it back to carol: %q", r.acting(tid))
	}
	later := r.newTask("bob", "alice", "later")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"erin","request_id":"p5"}`, later))
	r.turnCall("alice", "task.draft", fmt.Sprintf(`{"tasks":[%d],"request_id":"d1"}`, later))
	if turns, _ := db.Turns(r.DB, later); len(turns) != 0 || r.acting(later) != "bob" {
		t.Fatalf("a draft goes back to its owner with no turns: %+v %q", turns, r.acting(later))
	}
}

// Retiring an agent takes its turns off: the top one goes back to the agent below, a lower one just goes.
func TestRetiringAnAgentTakesItsTurnsOff(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	for _, a := range []string{"alice", "bob", "carol"} {
		r.agentUp(a)
	}
	top := r.beginCurrent("bob", "alice", "on top")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"carol","request_id":"p1"}`, top))
	under := r.newTask("alice", "bob", "under")
	r.turnCall("alice", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"carol","request_id":"p2"}`, under))
	r.turnCall("carol", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","request_id":"p3"}`, under))
	tx, err := r.DB.Begin()
	r.must(err)
	if _, err := taskwrites.RetireTurns(tx, r.home, r.Env, "carol"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	r.must(tx.Commit())
	if r.acting(top) != "bob" {
		t.Fatalf("top: %q", r.acting(top))
	}
	if turns, _ := db.Turns(r.DB, under); len(turns) != 1 || turns[0].Agent != db.Human {
		t.Fatalf("under: %+v", turns)
	}
}

// A passing review closes the task as its reviewer: the reviewer is moved off it, and the builder below is told it closed.
func TestAPassingReviewMovesTheReviewerOnAndTellsTheBuilder(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	r.agentUp("alice")
	tid := r.beginCurrent("bob", "alice", "the parser")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"alice","on_pass":"done","evidence":"PR #9","request_id":"p1"}`, tid))
	if cur, ok, _ := db.CurrentTask(r.DB, "alice"); !ok || cur != tid {
		t.Fatalf("alice holds the review: %d %v", cur, ok)
	}
	r.turnCall("alice", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","request_id":"a1"}`, tid))
	if cur, ok, _ := db.CurrentTask(r.DB, "alice"); ok && cur == tid {
		t.Fatal("alice still holds the closed task")
	}
	told := false
	for _, s := range r.saysTo("bob") {
		told = told || strings.Contains(s, fmt.Sprintf("T%d done by alice", tid))
	}
	if !told {
		t.Fatalf("bob is told it closed: %v", r.saysTo("bob"))
	}
	var outcome string
	r.must(r.DB.QueryRow("SELECT json_extract(payload, '$.outcome') FROM events WHERE kind = 'task_turn_pop' ORDER BY id DESC LIMIT 1").Scan(&outcome))
	if outcome != "pass" {
		t.Fatalf("the review's turn ends as a pass: %q", outcome)
	}
}

// A turn sent back goes to the front of the queue of the agent below, not typed into the work it has started since; the founder's own
// send-back note is a comment; the founder's comment while an agent is on top is a comment only.
func TestASendBackWaitsAtTheFrontAndTheFoundersNoteIsAComment(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	r.agentUp("alice")
	tid := r.beginCurrent("bob", "alice", "the parser")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"alice","request_id":"p1"}`, tid))
	other := r.newTask("bob", "alice", "meanwhile")
	_, _, err := r.AdvanceCurrentTask(r.DB, "bob")
	r.must(err)
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != other {
		t.Fatalf("bob moved on: %d", cur)
	}
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"how is it going?","request_id":"c1"}`, tid))
	if r.acting(tid) != "alice" {
		t.Fatal("the founder's comment under an agent's turn is a comment only")
	}
	r.turnCall("human", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"send_back","note":"take it back, bob","request_id":"b1"}`, tid))
	var memos int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM task_memos WHERE task_id = ? AND author = 'human' AND body = 'take it back, bob'", tid).Scan(&memos))
	if memos != 1 {
		t.Fatal("the founder's send-back note is a comment")
	}
	var sendNow sql.NullInt64
	r.must(r.DB.QueryRow("SELECT r.send_now FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob' "+
		"ORDER BY r.id DESC LIMIT 1", tid).Scan(&sendNow))
	if sendNow.Int64 != 0 {
		t.Fatal("the return waits in bob's queue")
	}
	if cur, _, _ := db.CurrentTask(r.DB, "bob"); cur != other {
		t.Fatalf("bob keeps the task it started: %d", cur)
	}
}

// With a turn open, an owner edit replaces the agent acting, an unblock moves the agent acting on, and a done clears the stack and tells it.
func TestAnEditAnUnblockAndADoneUnderATurn(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	for _, a := range []string{"alice", "bob", "carol"} {
		r.agentUp(a)
	}
	tid := r.beginCurrent("bob", "alice", "the job")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"alice","request_id":"p1"}`, tid))
	r.turnCall("alice", "task.edit", fmt.Sprintf(`{"task":%d,"owner":"carol","request_id":"e1"}`, tid))
	if task, _ := db.GetTask(r.DB, tid); task.Owner != "bob" || task.Acting != "carol" {
		t.Fatalf("the edit moved the turn: %+v", task)
	}
	blocker := r.newTask("alice", "carol", "first this")
	r.turnCall("carol", "task.edit", fmt.Sprintf(`{"task":%d,"blocked_by":%d,"request_id":"e2"}`, tid, blocker))
	if cur, ok, _ := db.CurrentTask(r.DB, "carol"); ok && cur == tid {
		t.Fatal("the block frees carol")
	}
	r.turnCall("carol", "task.edit", fmt.Sprintf(`{"task":%d,"unblock":%d,"request_id":"e3"}`, tid, blocker))
	if cur, ok, _ := db.CurrentTask(r.DB, "carol"); !ok || cur != tid {
		t.Fatalf("the unblock gives it back to carol: %d %v", cur, ok)
	}
	r.turnCall("carol", "task.done", fmt.Sprintf(`{"task":%d,"evidence":"done it","request_id":"d1"}`, tid))
	if turns, _ := db.Turns(r.DB, tid); len(turns) != 0 {
		t.Fatalf("%+v", turns)
	}
	for _, who := range []string{"bob"} {
		told := false
		for _, s := range r.saysTo(who) {
			told = told || strings.Contains(s, "done by carol")
		}
		if !told {
			t.Fatalf("%s, below the turn, is told: %v", who, r.saysTo(who))
		}
	}
}
