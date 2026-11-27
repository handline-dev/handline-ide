package supervisor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/actor"
)

// While a seat of an agent holds a task, the agent only coordinates: once its own task is done and it is idle, a new task for it still goes to
// a seat, and one it queued for itself is still its own. Once no seat holds a task, the agent takes the next itself.
func TestWhileASeatWorksTheIdleAgentOnlyCoordinates(t *testing.T) {
	r, temps, first, second := tempRig(t, false)
	alice := r.k("alice")
	r.must(r.SweepTaskQueue(r.clock))
	seat := temps.hired[0]
	third := r.newTask("alice", "carol", "the footer") // waiting when her done moves her on
	r.turnCall("alice", "task.done", fmt.Sprintf(`{"task":%d,"evidence":"shipped","request_id":"d1"}`, first))
	r.must(db.SetAgentState(r.DB, alice, states.Idle, 0, ""))
	r.clock += HireRetryS
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 2 || r.acting(third) != temps.hired[1] {
		t.Fatalf("seats %v; T%d is %s's", temps.hired, third, r.acting(third))
	}
	if cur, ok, _ := db.CurrentTask(r.DB, alice); ok && cur != 0 {
		t.Fatalf("alice took T%d while her seat works", cur)
	}
	own := r.newTask("alice", "alice", "her own note")
	r.clock += QueueRetryS
	r.must(r.SweepTaskQueue(r.clock))
	if cur, _, _ := db.CurrentTask(r.DB, alice); cur != own {
		t.Fatalf("alice holds %d, not the task she queued for herself T%d", cur, own)
	}
	r.turnCall("alice", "task.done", fmt.Sprintf(`{"task":%d,"evidence":"noted","request_id":"d2"}`, own))
	for _, s := range []struct {
		seat string
		tid  int64
	}{{seat, second}, {temps.hired[1], third}} {
		r.turnCall(s.seat, "task.done", fmt.Sprintf(`{"task":%d,"evidence":"fixed","request_id":"d%d"}`, s.tid, s.tid))
	}
	fourth := r.newTask("alice", "carol", "the header")
	r.clock += HireRetryS
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 2 || r.acting(fourth) != alice {
		t.Fatalf("with no seat at work, T%d went to %s (seats %v)", fourth, r.acting(fourth), temps.hired)
	}
	if cur, _, _ := db.CurrentTask(r.DB, alice); cur != fourth {
		t.Fatalf("alice holds %d, want T%d", cur, fourth)
	}
}

// Manual keeps the old way: an idle agent takes its next task itself, whatever its seats hold.
func TestManualTempsLeaveTheIdleAgentItsTasks(t *testing.T) {
	r, temps, first, _ := tempRig(t, false)
	alice := r.k("alice")
	r.must(r.SweepTaskQueue(r.clock))
	temps.manual = true
	r.turnCall("alice", "task.done", fmt.Sprintf(`{"task":%d,"evidence":"shipped","request_id":"d1"}`, first))
	third := r.newTask("alice", "carol", "the footer")
	r.clock += HireRetryS
	r.must(r.SweepTaskQueue(r.clock))
	if cur, _, _ := db.CurrentTask(r.DB, alice); cur != third {
		t.Fatalf("alice holds %d, want T%d", cur, third)
	}
}

// A seat's question to the founder goes to its agent, with how to answer it; the agent's own question to the founder on it leaves the
// founder's turn right above the seat, so the founder's answer goes back to the seat.
func TestASeatAsksItsAgentAndTheFoundersAnswerGoesBackToTheSeat(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	alice := r.k("alice")
	r.must(r.SweepTaskQueue(r.clock))
	seat := temps.hired[0]
	r.turnCall(seat, "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"Blue or red?","turn":"human","request_id":"c1"}`, second))
	if got := r.acting(second); got != alice {
		t.Fatalf("the seat's question went to %s, not alice", got)
	}
	var body string
	r.must(r.DB.QueryRow(`SELECT q.body FROM requests q JOIN task_requests l ON l.request_id = q.id WHERE q.target = ? AND l.task_id = ? ORDER BY q.id DESC LIMIT 1`,
		alice, second).Scan(&body))
	if !strings.Contains(body, fmt.Sprintf("Your temp on T%d asks the founder (comment 1)", second)) || !strings.HasSuffix(body, "Comment 1:\nBlue or red?") || !strings.Contains(body, "--turn human") {
		t.Fatalf("alice was asked %q", body)
	}
	r.turnCall("alice", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"Blue or red for the badge? I recommend blue","turn":"human","request_id":"c2"}`, second))
	turns, err := db.Turns(r.DB, second)
	r.must(err)
	if len(turns) != 1 || turns[0].Agent != actor.OwnerKey {
		t.Fatalf("turns %+v: want the founder's alone, right above the seat", turns)
	}
	r.turnCall(seat, "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"Blue or red, for the badge only?","turn":"human","request_id":"c2b"}`, second))
	if turns, _ = db.Turns(r.DB, second); len(turns) != 1 || turns[0].Agent != actor.OwnerKey || !strings.Contains(turns[0].Ask, "badge only") {
		t.Fatalf("turns %+v: the seat asking again rewrites the founder's question", turns)
	}
	r.turnCall("human", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"blue","request_id":"c3"}`, second))
	if got := r.acting(second); got != seat {
		t.Fatalf("the founder's answer went to %s, not the seat", got)
	}
}

// The agent answers its seat itself with a pass: the task goes back to the seat with the answer.
func TestTheAgentAnswersItsSeatWithAPass(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	r.must(r.SweepTaskQueue(r.clock))
	seat := temps.hired[0]
	r.turnCall(seat, "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"Blue or red?","turn":"human","request_id":"c1"}`, second))
	r.turnCall("alice", "task.turn.pop", fmt.Sprintf(`{"task":%d,"outcome":"pass","note":"blue, as the brief says","request_id":"p1"}`, second))
	if got := r.acting(second); got != seat {
		t.Fatalf("the answer went to %s, not the seat", got)
	}
}

// A seat's word to the founder goes to its agent instead.
func TestASeatsSayToTheFounderGoesToItsAgent(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	r.must(r.SweepTaskQueue(r.clock))
	out := r.say(SayVerbIn{Requester: temps.hired[0], Target: actor.OwnerKey, Body: "Blue or red?"})
	if !out.OK {
		t.Fatalf("%+v", out)
	}
	said := r.says(r.k("alice"))
	if len(said) == 0 || !strings.Contains(said[len(said)-1], fmt.Sprintf("Your temp on T%d writes to the founder: Blue or red?", second)) {
		t.Fatalf("alice was told %q", said)
	}
}

// The founder's comment on a task a seat holds reaches the seat only, even when it names the agent the seat stands in for or that agent
// wrote there last: one conversation answers it.
func TestTheFoundersCommentOnASeatsTaskReachesTheSeatOnly(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	alice := r.k("alice")
	r.must(r.SweepTaskQueue(r.clock))
	seat := temps.hired[0]
	r.turnCall("alice", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"handed to my temp","request_id":"c1"}`, second))
	for _, text := range []string{"keep the old link", fmt.Sprintf("<@%s> keep the old link", alice)} {
		got, found := r.MemoRecipients(second, actor.OwnerKey, text, false)
		if !found || len(got) != 1 || got[0].Agent != seat {
			t.Fatalf("%q reaches %+v, want the seat %s alone", text, got, seat)
		}
	}
}

// A review the seat asked its agent for is not a question passed on: the agent asking the founder about it keeps its own turn under theirs.
func TestAnAgentsReviewTurnStaysUnderTheFoundersQuestion(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	alice := r.k("alice")
	r.must(r.SweepTaskQueue(r.clock))
	seat := temps.hired[0]
	r.turnCall(seat, "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"review it","turn":%q,"done":"fixed","request_id":"c1"}`, second, alice))
	r.turnCall("alice", "task.comment.add", fmt.Sprintf(`{"task":%d,"text":"Is red allowed?","turn":"human","request_id":"c2"}`, second))
	turns, err := db.Turns(r.DB, second)
	r.must(err)
	if len(turns) != 2 || turns[0].Agent != alice || turns[1].Agent != actor.OwnerKey {
		t.Fatalf("turns %+v: want alice's review under the founder's question", turns)
	}
}

// A seat's reply to the founder's own message goes to the founder.
func TestASeatsReplyToTheFounderGoesToTheFounder(t *testing.T) {
	r, temps, _, _ := tempRig(t, false)
	r.must(r.SweepTaskQueue(r.clock))
	seat := temps.hired[0]
	asked := r.founderSays(seat, "how is it going?")
	out := r.say(SayVerbIn{Requester: seat, Target: actor.OwnerKey, Body: "half done", Parent: asked})
	if !out.OK {
		t.Fatalf("%+v", out)
	}
	var target string
	r.must(r.DB.QueryRow("SELECT target FROM requests WHERE body = 'half done'").Scan(&target))
	if target != actor.OwnerKey {
		t.Fatalf("the reply went to %s", target)
	}
}

// A seat's long question reaches its agent whole, after how to answer it, and the footer names pass and back, not a handoff; the seat is
// told its question goes to its agent first.
func TestASeatsLongQuestionReachesItsAgentWholeWithPassAndBack(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	alice := r.k("alice")
	r.must(r.SweepTaskQueue(r.clock))
	seat := temps.hired[0]
	_, err := r.DB.Exec("UPDATE tasks SET title = ? WHERE id = ?", strings.Repeat("t", 190), second)
	r.must(err)
	question := strings.Repeat("q", 300) + "\n" + strings.Repeat("r", 299) + "END"
	out := r.turnCall(seat, "task.comment.add", fmt.Sprintf(`{"task":%d,"text":%q,"turn":"human","request_id":"c1"}`, second, question))
	said, _ := out["result"].(map[string]any)["said"].(string)
	if !strings.Contains(r.named(said), "goes first to") || !strings.Contains(said, "the agent you stand in for") {
		t.Fatalf("the seat was told %q", said)
	}
	var rid int64
	r.must(r.DB.QueryRow(`SELECT q.id FROM requests q JOIN task_requests l ON l.request_id = q.id WHERE q.target = ? AND l.task_id = ? ORDER BY q.id DESC LIMIT 1`,
		alice, second).Scan(&rid))
	got := tellText(r.DB, mustReq(r, rid), 1, nil, false, nil, "")
	for _, want := range []string{"rEND", fmt.Sprintf("`hl task pass T%d", second), "--turn human", fmt.Sprintf("`hl task back T%d", second)} {
		if !strings.Contains(got, want) {
			t.Fatalf("alice's delivery lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hl task handoff") || strings.Count(got, "rrrrEND") != 1 {
		t.Fatalf("alice's delivery names a handoff or repeats the question:\n%s", got)
	}
}
