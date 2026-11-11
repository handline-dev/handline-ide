package supervisor

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

// turn records one turn of agent: a prompt, a say to the founder when said, and the Stop.
func (r *rig) turn(agent string, said bool) {
	r.t.Helper()
	agent = r.k(agent)
	r.clock++
	_, err := db.AddEvent(r.DB, agent, "UserPromptSubmit")
	r.must(err)
	if said {
		r.clock++
		_, err = db.AddRequest(r.DB, db.NewRequest{Requester: agent, Target: actor.OwnerKey, Body: "may I open the PR?", Kind: "say"})
		r.must(err)
	}
	r.clock++
	_, err = db.AddEvent(r.DB, agent, "Stop")
	r.must(err)
}

func (r *rig) saidStopNotices(agent string, tid int64) int {
	n := 0
	for _, b := range r.saysTo(agent) {
		if b == SaidStopText(tid) {
			n++
		}
	}
	return n
}

func TestAnAgentThatWroteToTheFounderAndStoppedWithADoingTaskIsToldOncePerTurn(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	tid := r.beginCurrent(r.k("bob"), r.k(("alice")), "the PR")
	r.turn(r.k("bob"), true)
	r.must(r.SweepSaidAndStopped())
	r.must(r.SweepSaidAndStopped())
	if n := r.saidStopNotices(r.k("bob"), tid); n != 1 {
		t.Fatalf("told %d times, want once: %v", n, r.saysTo(r.k("bob")))
	}
	r.turn(r.k("bob"), false) // a turn with no say: nothing new
	r.must(r.SweepSaidAndStopped())
	r.turn(r.k("bob"), true) // it asks again in a later turn: told again
	r.must(r.SweepSaidAndStopped())
	if n := r.saidStopNotices(r.k("bob"), tid); n != 2 {
		t.Fatalf("told %d times over three turns, want 2", n)
	}
}

func TestNoNoticeOnceTheTaskIsTheFoundersOrWithNoDoingTask(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	tid := r.beginCurrent(r.k("bob"), r.k(("alice")), "the PR")
	r.turn(r.k("bob"), true)
	mid, err := db.AddMemo(r.DB, tid, r.k(("bob")), "`@human` may I open the PR?") // the task waits on the founder in a memo ask
	r.must(err)
	ask, err := db.AddRequest(r.DB, db.NewRequest{Requester: r.k(("bob")), Target: actor.OwnerKey, Body: "may I open the PR?", Kind: "ask"})
	r.must(err)
	r.must(db.LinkRequest(r.DB, tid, ask))
	_, err = r.DB.Exec("UPDATE requests SET memo_id = ? WHERE id = ?", mid, ask)
	r.must(err)
	r.must(r.SweepSaidAndStopped())
	if n := r.saidStopNotices(r.k("bob"), tid); n != 0 {
		t.Fatalf("told about a task that waits on the founder in a memo ask: %v", r.saysTo(r.k("bob")))
	}

	r.agentUp(r.k("alice"))
	r.turn(r.k("alice"), true) // a say with no task held
	r.must(r.SweepSaidAndStopped())
	if says := r.saysTo(r.k("alice")); len(says) != 0 {
		t.Fatalf("told with no doing task: %v", says)
	}
}

// The notice names the one route to the founder the board has: their turn on the task, not a handoff or an @human comment.
func TestTheSaidStopNoticeDoesNotTellAnAgentToHandTheTaskToTheFounder(t *testing.T) {
	text := SaidStopText(7)
	if strings.Contains(text, "handoff") || strings.Contains(text, "@human") || !strings.Contains(text, "hl task comment T7 add") || !strings.Contains(text, "--turn human") {
		t.Fatalf("the notice must point at the founder's turn: %s", text)
	}
}
