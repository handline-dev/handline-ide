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
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"beta now or after the fix? I recommend now","turn":"human","request_id":"p1"}`, held))
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
	// while it waits on the founder they answer, not hand it on; nobody hands a turn to themself; the comment it hangs on stays
	r.refusedCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"alice","request_id":"p2"}`, held), "waits on you")
	r.refusedCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"mine","turn":"human","request_id":"p3"}`, held), "to yourself")
	r.refusedCall("bob", "task.comment.delete", fmt.Sprintf(`{"task":%d,"memo":%d,"request_id":"d1"}`, held, turns[0].Memo), "what a turn")
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
	if back != "T1 질문에 대표님 댓글 2: now / 지금 질문 칸: beta now or after the fix? I recommend now" {
		t.Fatalf("%q", back)
	}
}

// A review turn pushed with on_pass done closes the task when the reviewer passes it; a send-back returns it with the note as a comment.
func TestAReviewTurnClosesTheTaskOnAPassAndASendBackReturnsItWithTheNote(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	r.agentUp("alice")
	tid := r.beginCurrent("bob", "alice", "the parser")
	push := fmt.Sprintf(`{"task":%d,"text":"review the parser","turn":"alice","done":"PR #9, tests green","request_id":"%%s"}`, tid)
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(push, "p1"))
	if r.acting(tid) != "alice" {
		t.Fatalf("acting %q", r.acting(tid))
	}
	// the turn's delivery carries the comment: alice is not told it a second time
	var delivered, told int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM requests WHERE target = 'alice' AND kind = 'task' AND body LIKE '%review the parser%'").Scan(&delivered))
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM memo_tells WHERE reader = 'alice'").Scan(&told))
	if turns, _ := db.Turns(r.DB, tid); delivered != 1 || told != 0 || len(turns) != 1 || turns[0].Memo == 0 || turns[0].OnPass != db.TurnOnPassDone {
		t.Fatalf("delivered %d, told %d, %+v", delivered, told, turns)
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
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(push, "p2"))
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
	for _, a := range []string{"alice", "bob", "carol", "dave", "erin"} {
		r.agentUp(a)
	}
	tid := r.beginCurrent("bob", "alice", "the job")
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"alice","request_id":"p1"}`, tid))
	r.turnCall("alice", "task.handoff", fmt.Sprintf(`{"task":%d,"to":"carol","note":"yours to review","request_id":"h1"}`, tid))
	if task, _ := db.GetTask(r.DB, tid); task.Owner != "bob" || task.Acting != "carol" {
		t.Fatalf("the handoff replaced the top turn: %+v", task)
	}
	r.refusedCall("carol", "task.handoff", fmt.Sprintf(`{"task":%d,"to":"bob","request_id":"h2"}`, tid), "under its turns")
	r.refusedCall("bob", "task.release", fmt.Sprintf(`{"task":%d,"reason":"mine","request_id":"r0"}`, tid), "carol's is on top")
	r.turnCall("carol", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"dave","request_id":"p2"}`, tid))
	r.turnCall("dave", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"erin","request_id":"p3"}`, tid))
	r.refusedCall("erin", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"human","request_id":"p4"}`, tid), "the most it may")
	r.turnCall("erin", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","request_id":"o1"}`, tid))
	r.turnCall("dave", "task.release", fmt.Sprintf(`{"task":%d,"reason":"not mine to do","request_id":"r1"}`, tid))
	if r.acting(tid) != "carol" {
		t.Fatalf("dave's release sent it back to carol: %q", r.acting(tid))
	}
	later := r.newTask("bob", "alice", "later")
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"erin","request_id":"p5"}`, later))
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
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"beta now?","turn":"human","request_id":"p1"}`, tid))
	r.turnCall("human", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","note":"좋아 진행","request_id":"o1"}`, tid))
	if got, want := last(), fmt.Sprintf("T%d 질문에 대표님 답: 좋아 진행 / 지금 질문 칸: beta now?", tid); got != want {
		t.Fatalf("%q, want %q", got, want)
	}
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"and the phone?","turn":"human","request_id":"p2"}`, tid))
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
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"carol","request_id":"p1"}`, top))
	under := r.newTask("alice", "bob", "under")
	r.turnCall("alice", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"carol","request_id":"p2"}`, under))
	r.turnCall("carol", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"human","request_id":"p3"}`, under))
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
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"alice","done":"PR #9","request_id":"p1"}`, tid))
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
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"alice","request_id":"p1"}`, tid))
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
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"alice","request_id":"p1"}`, tid))
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
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"now?","turn":"human","request_id":"p1"}`, held))
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
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"over to you","turn":"alice","request_id":"p1"}`, tid))
	r.turnCall("alice", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"send_back","note":"the test fails","request_id":"p2"}`, tid))
	var rid, told int64
	r.must(r.DB.QueryRow("SELECT MAX(r.id) FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob'", tid).Scan(&rid))
	r.must(r.DB.QueryRow("SELECT COALESCE(MAX(t.request_id), 0) FROM memo_tells t JOIN task_memos m ON m.id = t.memo_id WHERE m.task_id = ? AND m.author = 'alice' AND m.body = 'the test fails' AND t.reader = 'bob'", tid).Scan(&told))
	if told == 0 || told != rid {
		t.Fatalf("the return #%d; alice's comment told bob by #%d", rid, told)
	}
}

// The founder's turn is the task's one question: the agent under it handing it over again with a new comment moves the turn to that comment,
// the old text kept in a task_turn_ask event; anyone else is refused. Nothing moves and nobody is typed at.
func TestANewCommentMovesTheFoundersTurnToIt(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	held := r.beginCurrent("bob", "alice", "ship it")
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"A or B?","turn":"human","request_id":"p1"}`, held))
	typed := len(r.term.typed)
	out := r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"A, B or C? I recommend C","turn":"human","request_id":"p2"}`, held))
	res := out["result"].(map[string]any)
	mid, _ := res["memo"].(float64)
	if said := res["said"]; said != fmt.Sprintf("comment %d added to T%d: the founder's turn now hangs on this comment", int64(mid), held) {
		t.Fatalf("%q", said)
	}
	turns, _ := db.Turns(r.DB, held)
	if len(turns) != 1 || turns[0].Agent != db.Human || turns[0].Ask != "A, B or C? I recommend C" || turns[0].Memo != int64(mid) || turns[0].By != "bob" {
		t.Fatalf("%+v", turns)
	}
	var from, stored string
	r.must(r.DB.QueryRow("SELECT json_extract(payload, '$.from') FROM events WHERE kind = 'task_turn_ask' AND json_extract(payload, '$.task_id') = ?", held).Scan(&from))
	r.must(r.DB.QueryRow("SELECT ask FROM task_turns WHERE task_id = ?", held).Scan(&stored))
	if from != "A or B?" || stored != "" {
		t.Fatalf("the old text is kept (%q) and the turn stores no ask of its own (%q)", from, stored)
	}
	if len(r.term.typed) != typed {
		t.Fatalf("nobody is typed at (%d -> %d)", typed, len(r.term.typed))
	}
	r.refusedCall("alice", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"mine?","turn":"human","request_id":"p4"}`, held), "only it, or the founder")
	var n int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM task_memos WHERE task_id = ? AND body = 'mine?'", held).Scan(&n))
	if n != 0 {
		t.Fatal("a refused turn leaves no comment: the two are one write")
	}
}

// forwarded is whether request rid forwards a comment or a turn's return (db.ForwardedSQL): the task's page and the chat draw that instead.
func (r *rig) forwarded(rid int64) bool {
	r.t.Helper()
	var n int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM ("+db.ForwardedSQL+") WHERE id = ?", rid).Scan(&n))
	return n == 1
}

// The founder's comment that gives a turn back is forwarded to the holder, and when the task then moves to a new holder the delivery sent
// again carries the comment on: both are the comment's, the withdrawn one too, and the new holder is one of its readers. A pass with no
// comment is forwarded as the turn's return.
func TestAForwardedCommentStaysTheCommentsWhenTheTaskMovesAndAPassIsTheTurns(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	r.agentUp("alice")
	tid := r.beginCurrent("bob", "alice", "ship it")
	r.turnCall("bob", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"now?","turn":"human","request_id":"p1"}`, tid))
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"now","request_id":"c1"}`, tid))
	var back int64
	r.must(r.DB.QueryRow("SELECT r.id FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'bob' ORDER BY r.id DESC LIMIT 1", tid).Scan(&back))
	r.turnCall("human", "task.edit", fmt.Sprintf(`{"task":%d,"owner":"alice","request_id":"e1"}`, tid))
	var again int64
	r.must(r.DB.QueryRow("SELECT r.id FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'alice' ORDER BY r.id DESC LIMIT 1", tid).Scan(&again))
	if !r.forwarded(back) || !r.forwarded(again) {
		t.Fatalf("the return %d and its resend %d are the comment's: %v %v", back, again, r.forwarded(back), r.forwarded(again))
	}
	var reader string
	r.must(r.DB.QueryRow("SELECT reader FROM memo_tells WHERE request_id = ?", again).Scan(&reader))
	if reader != "alice" {
		t.Fatalf("the resend tells %q", reader)
	}

	r.turnCall("alice", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"ok?","turn":"human","request_id":"p2"}`, tid))
	r.turnCall("human", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","request_id":"x1"}`, tid))
	var passed int64
	r.must(r.DB.QueryRow("SELECT r.id FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = 'alice' ORDER BY r.id DESC LIMIT 1", tid).Scan(&passed))
	if passed == again || !r.forwarded(passed) {
		t.Fatalf("the pass's return %d is the turn's", passed)
	}
}
