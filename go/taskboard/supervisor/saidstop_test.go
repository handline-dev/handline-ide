package supervisor

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// turn records one turn of agent: a prompt, a say to the founder when said, and the Stop.
func (r *rig) turn(agent string, said bool) {
	r.t.Helper()
	r.clock++
	_, err := db.AddEvent(r.DB, agent, "UserPromptSubmit")
	r.must(err)
	if said {
		r.clock++
		_, err = db.AddRequest(r.DB, db.NewRequest{Requester: agent, Target: db.Human, Body: "may I open the PR?", Kind: "say"})
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
	r.agentUp("bob")
	tid := r.beginCurrent("bob", "alice", "the PR")
	r.turn("bob", true)
	r.must(r.SweepSaidAndStopped())
	r.must(r.SweepSaidAndStopped())
	if n := r.saidStopNotices("bob", tid); n != 1 {
		t.Fatalf("told %d times, want once: %v", n, r.saysTo("bob"))
	}
	r.turn("bob", false) // a turn with no say: nothing new
	r.must(r.SweepSaidAndStopped())
	r.turn("bob", true) // it asks again in a later turn: told again
	r.must(r.SweepSaidAndStopped())
	if n := r.saidStopNotices("bob", tid); n != 2 {
		t.Fatalf("told %d times over three turns, want 2", n)
	}
}

func TestNoNoticeOnceTheTaskIsTheFoundersOrWithNoDoingTask(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	tid := r.beginCurrent("bob", "alice", "the PR")
	r.turn("bob", true)
	mid, err := db.AddMemo(r.DB, tid, "bob", "`@human` may I open the PR?") // the task waits on the founder in a memo ask
	r.must(err)
	ask, err := db.AddRequest(r.DB, db.NewRequest{Requester: "bob", Target: db.Human, Body: "may I open the PR?", Kind: "ask"})
	r.must(err)
	r.must(db.LinkRequest(r.DB, tid, ask))
	_, err = r.DB.Exec("UPDATE requests SET memo_id = ? WHERE id = ?", mid, ask)
	r.must(err)
	r.must(r.SweepSaidAndStopped())
	if n := r.saidStopNotices("bob", tid); n != 0 {
		t.Fatalf("told about a task that waits on the founder in a memo ask: %v", r.saysTo("bob"))
	}

	r.agentUp("alice")
	r.turn("alice", true) // a say with no task held
	r.must(r.SweepSaidAndStopped())
	if says := r.saysTo("alice"); len(says) != 0 {
		t.Fatalf("told with no doing task: %v", says)
	}
}

// The notice names a route the board accepts: a task cannot be handed to the founder, so it must not say to.
func TestTheSaidStopNoticeDoesNotTellAnAgentToHandTheTaskToTheFounder(t *testing.T) {
	text := SaidStopText(7)
	if strings.Contains(text, "handoff") || !strings.Contains(text, "@human") {
		t.Fatalf("the notice must point at an @human memo, not a handoff: %s", text)
	}
}
