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
	var back string
	r.must(r.DB.QueryRow("SELECT body FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob' ORDER BY r.id DESC LIMIT 1", held).Scan(&back))
	if back != "T1 질문에 대표님 댓글 1: now / 지금 질문 칸: beta now or after the fix? I recommend now" {
		t.Fatalf("%q", back)
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

// The founder ending their own turn tells the holder the question it answers: with the pass's note as the answer, or that they passed it.
func TestTheFoundersPassTellsTheHolderTheQuestion(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.beginCurrent("bob", "alice", "ship it")
	last := func() string {
		var body string
		r.must(r.DB.QueryRow("SELECT body FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob' ORDER BY r.id DESC LIMIT 1", tid).Scan(&body))
		return body
	}
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","ask":"beta now?","request_id":"p1"}`, tid))
	r.turnCall("human", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","note":"좋아 진행","request_id":"o1"}`, tid))
	if got, want := last(), fmt.Sprintf("T%d 질문에 대표님 답: 좋아 진행 / 지금 질문 칸: beta now?", tid); got != want {
		t.Fatalf("%q, want %q", got, want)
	}
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","ask":"and the phone?","request_id":"p2"}`, tid))
	r.turnCall("human", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","request_id":"o2"}`, tid))
	if got, want := last(), fmt.Sprintf("T%d 대표님이 질문 칸을 넘김 / 지금 질문 칸: and the phone?", tid); got != want {
		t.Fatalf("%q, want %q", got, want)
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

// The founder's comment passing a turn is told the agent below by the task's return, so the task's page and the chat show the comment and
// not that delivery again; it passes a pause as their own request, and the task still moves on a team with no edge but the founder's.
func TestTheFoundersPassIsTheirCommentsTellAndTheTaskStillMoves(t *testing.T) {
	r := newRig(t)
	r.founderOnlyTeam()
	r.agentUp("bob")
	held := r.beginCurrent("bob", "alice", "ship it")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","ask":"now?","request_id":"p1"}`, held))
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"now","request_id":"c1"}`, held))
	var rid, told int64
	var from string
	r.must(r.DB.QueryRow("SELECT r.id, r.requester FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob' ORDER BY r.id DESC LIMIT 1", held).Scan(&rid, &from))
	toldBob := func() int64 {
		r.must(r.DB.QueryRow("SELECT COALESCE(MAX(t.request_id), 0) FROM memo_tells t JOIN task_memos m ON m.id = t.memo_id WHERE m.task_id = ? AND m.author = 'human' AND t.reader = 'bob'", held).Scan(&told))
		return told
	}
	var comments int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM task_memos WHERE task_id = ? AND author = 'human'", held).Scan(&comments))
	if toldBob() != rid || from != db.Human || comments != 1 {
		t.Fatalf("the pass #%d from %q; the comment's tell to bob #%d; %d comments", rid, from, told, comments)
	}
	// an edit tells bob again, and the return stays the comment's tell
	var mid int64
	r.must(r.DB.QueryRow("SELECT MAX(id) FROM task_memos WHERE task_id = ? AND author = 'human'", held).Scan(&mid))
	r.turnCall("human", "task.comment.edit", fmt.Sprintf(`{"task":%d,"memo":%d,"text":"now, please","request_id":"c2"}`, held, mid))
	if toldBob() != rid {
		t.Fatalf("an edit replaced the return #%d as the comment's tell: #%d", rid, told)
	}
	req, err := db.GetRequest(r.DB, rid)
	r.must(err)
	if passes, err := db.PassesPause(r.DB, req); err != nil || !passes {
		t.Fatalf("the founder's pass is held by a pause: %v %v", passes, err)
	}
	r.turnCall("human", "task.edit", fmt.Sprintf(`{"task":%d,"owner":"alice","request_id":"e1"}`, held))
	if task, _ := db.GetTask(r.DB, held); task.Owner != "alice" {
		t.Fatalf("the task did not move: %+v", task)
	}
}

// An agent's pass with a note writes the note as its comment, and the task's return is that comment's tell to the one below.
func TestAnAgentsNotedPassIsToldByTheTasksReturn(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	r.agentUp("alice")
	tid := r.beginCurrent("bob", "amy", "ship it")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"alice","request_id":"p1"}`, tid))
	r.turnCall("alice", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"send_back","note":"the test fails","request_id":"p2"}`, tid))
	var rid, told int64
	r.must(r.DB.QueryRow("SELECT MAX(r.id) FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob'", tid).Scan(&rid))
	r.must(r.DB.QueryRow("SELECT COALESCE(MAX(t.request_id), 0) FROM memo_tells t JOIN task_memos m ON m.id = t.memo_id WHERE m.task_id = ? AND m.author = 'alice' AND m.body = 'the test fails' AND t.reader = 'bob'", tid).Scan(&told))
	if told == 0 || told != rid {
		t.Fatalf("the return #%d; alice's comment told bob by #%d", rid, told)
	}
}

// When the founder's comment passes a review turn whose done is refused, the refusal is news to the one below: the pop writes it as a
// comment of its own, and the task's return tells that one.
func TestARefusedDoneOnTheFoundersPassIsAComment(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	held := r.beginCurrent("bob", "alice", "ship it")
	child := r.newTask("alice", "alice", "first this")
	_, err := r.DB.Exec("UPDATE tasks SET parent_task = ? WHERE id = ?", held, child) // an open child refuses the done
	r.must(err)
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","ask":"review","on_pass":"done","evidence":"PR #9","request_id":"p1"}`, held))
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"looks good","request_id":"c1"}`, held))
	var rid, told int64
	r.must(r.DB.QueryRow("SELECT MAX(r.id) FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob'", held).Scan(&rid))
	r.must(r.DB.QueryRow("SELECT COALESCE(MAX(t.request_id), 0) FROM memo_tells t JOIN task_memos m ON m.id = t.memo_id WHERE m.task_id = ? AND m.author = 'human' "+
		"AND m.body LIKE 'looks good (not closed: %' AND t.reader = 'bob'", held).Scan(&told))
	if told == 0 || told != rid {
		t.Fatalf("the return #%d; the refusal's comment told bob by #%d", rid, told)
	}
}

// The founder's turn is the task's one question: the agent under it asking again rewrites the ask, the old text kept in a task_turn_ask
// event; the same text writes nothing; anyone else, or an empty ask, is refused as before. Nothing moves and nobody is typed at.
func TestAskingTheFounderAgainRewritesTheirQuestion(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	held := r.beginCurrent("bob", "alice", "ship it")
	r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","ask":"A or B?","request_id":"p1"}`, held))
	typed := len(r.term.typed)
	out := r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","ask":"A, B or C? I recommend C","request_id":"p2"}`, held))
	if said := out["result"].(map[string]any)["said"]; said != fmt.Sprintf("T%d: the founder's question now reads: A, B or C? I recommend C", held) {
		t.Fatalf("%q", said)
	}
	turns, _ := db.Turns(r.DB, held)
	if len(turns) != 1 || turns[0].Agent != db.Human || turns[0].Ask != "A, B or C? I recommend C" || turns[0].By != "bob" {
		t.Fatalf("%+v", turns)
	}
	var from string
	r.must(r.DB.QueryRow("SELECT json_extract(payload, '$.from') FROM events WHERE kind = 'task_turn_ask' AND json_extract(payload, '$.task_id') = ?", held).Scan(&from))
	if from != "A or B?" {
		t.Fatalf("the old text is kept: %q", from)
	}
	out = r.turnCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","ask":"A, B or C? I recommend C","request_id":"p3"}`, held))
	if said := out["result"].(map[string]any)["said"]; said != fmt.Sprintf("T%d: the founder's question is unchanged", held) {
		t.Fatalf("%q", said)
	}
	var n int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'task_turn_ask'").Scan(&n))
	if n != 1 || len(r.term.typed) != typed {
		t.Fatalf("the same text writes nothing (%d events) and nobody is typed at (%d -> %d)", n, typed, len(r.term.typed))
	}
	r.refusedCall("alice", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","ask":"mine?","request_id":"p4"}`, held), "only it, or the founder")
	r.refusedCall("bob", "task.turn.push", fmt.Sprintf(`{"task":%d,"to":"human","request_id":"p5"}`, held), "only it, or the founder")
}

// `hl say human --task T<n>` from the agent the task's question is for lands in that slot: it puts the founder's turn on, then rewrites it.
// From the opener, who does not hold it, it is a message linked to the task as before.
func TestASayToTheFounderFromTheHolderIsTheTasksQuestion(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	held := r.beginCurrent("bob", "alice", "ship it")
	out := r.say(SayVerbIn{Requester: "bob", Target: db.Human, Body: "A or B?", Task: held})
	if !out.OK || out.Msg != fmt.Sprintf("T%d is the founder's turn now, asking: A or B?", held) || r.acting(held) != db.Human {
		t.Fatalf("%+v acting %s", out, r.acting(held))
	}
	if cur, ok, _ := db.CurrentTask(r.DB, "bob"); ok && cur == held {
		t.Fatal("bob's slot is freed as by a turn")
	}
	out = r.say(SayVerbIn{Requester: "bob", Target: db.Human, Body: "A, B or C?", Task: held})
	if turns, _ := db.Turns(r.DB, held); !out.OK || len(turns) != 1 || turns[0].Ask != "A, B or C?" {
		t.Fatalf("%+v %+v", out, turns)
	}
	var says int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM requests WHERE target = 'human'").Scan(&says))
	if says != 0 {
		t.Fatalf("no message to the founder: %d", says)
	}
	out = r.say(SayVerbIn{Requester: "alice", Target: db.Human, Body: "fyi", Task: held})
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'human' AND r.body = 'fyi'", held).Scan(&says))
	if !out.OK || says != 1 {
		t.Fatalf("the opener's say is a linked message: %+v %d", out, says)
	}
}

// A say with files is never the founder's question: the question holds text, so it is refused with the way to post the files first.
func TestASayWithFilesToTheFounderIsNotTheQuestion(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	held := r.beginCurrent("bob", "alice", "ship it")
	out := r.say(SayVerbIn{Requester: "bob", Target: db.Human, Body: "this one?", Task: held, Attached: true})
	if out.OK || !strings.Contains(out.Msg, fmt.Sprintf("hl task comment T%d add", held)) || r.acting(held) != "bob" {
		t.Fatalf("%+v acting %s", out, r.acting(held))
	}
}
