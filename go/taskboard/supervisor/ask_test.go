package supervisor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/addr"
)

func TestAskQueuesAndTellsOnlyAfterTheCommit(t *testing.T) {
	r := newRig(t)
	r.agentUp("bob")
	out, err := r.Ask(r.DB, AskIn{Target: r.k("bob"), Body: "please look", Requester: r.k("alice"), Notify: false})
	r.must(err)
	if !out.OK || out.RID == 0 || !strings.HasPrefix(r.ids(r.named(out.Message)), "queued as #") || !strings.HasSuffix(r.named(out.Message), "for bob") {
		t.Fatalf("%+v", out)
	}
	out.Finish()
	row, _ := db.GetRequest(r.DB, out.RID)
	if row.State != "queued" || row.EdgeOK.Int64 != 1 || row.ChainID.String != "1" {
		t.Fatalf("%+v", row)
	}
	if n, _, _ := db.LastEventID(r.DB, r.k("alice"), "ask"); n == 0 {
		t.Fatal("no ask event")
	}
}

func TestAskRefusesAnImpersonatedFounderAndAnUnknownAsker(t *testing.T) {
	r := newRig(t)
	agent := r.k("alice")
	out, _ := r.Ask(r.DB, AskIn{Target: r.k("bob"), Body: "x", Requester: r.k("human"), Speaker: &agent})
	if out.OK || !strings.Contains(r.named(out.Message), "alice may not ask as human") {
		t.Fatalf("%+v", out)
	}
	other := "p2" // another person is no more the owner than an agent is
	if out, _ := r.Ask(r.DB, AskIn{Target: r.k("bob"), Body: "x", Requester: r.k("human"), Speaker: &other}); out.OK ||
		!strings.Contains(out.Message, "p2 may not ask as p1") {
		t.Fatalf("%+v", out)
	}
	tid := r.newTask("bob", r.k("human"), "x")
	if out, _ := r.Ask(r.DB, AskIn{Target: r.k("bob"), Body: "x", Requester: r.k("human"), Speaker: &agent, OnBehalfOf: true,
		Forward: Forward{Task: tid, Carries: db.ForwardQueue}}); !out.OK {
		t.Fatalf("a system delivery on behalf of the founder: %+v", out)
	}
	if out, _ := r.Ask(r.DB, AskIn{Target: r.k("bob"), Body: "x"}); out.OK || !strings.Contains(r.named(out.Message), "cannot tell who is asking") {
		t.Fatalf("%+v", out)
	}
}

func TestADeniedAskKeepsItsTextOnceAndSaysWhy(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 2; i++ {
		out, err := r.Ask(r.DB, AskIn{Target: r.k("carol"), Body: "hi", Requester: r.k("alice")})
		r.must(err)
		if out.OK || r.named(out.Message) != "unknown target 'carol'" {
			t.Fatalf("%+v", out)
		}
	}
	var n int
	r.DB.QueryRow("SELECT COUNT(*) FROM requests WHERE state = 'denied'").Scan(&n)
	if n != 1 {
		t.Fatalf("the same refusal is a second row: %d", n)
	}
}

func TestACycleInAChainIsRaisedToTheFounderButASayBackIsNot(t *testing.T) {
	r := newRig(t)
	first, _ := r.Ask(r.DB, AskIn{Target: r.k("bob"), Body: "start", Requester: r.k("alice")})
	out, err := r.Ask(r.DB, AskIn{Target: r.k("alice"), Body: "back to you", Requester: r.k("bob"), ParentID: first.RID, Kind: "ask"})
	r.must(err)
	if out.OK || !strings.Contains(r.ids(r.named(out.Message)), fmt.Sprintf("cycle: alice already asked bob in this chain (request #%d), and this would send it straight back", first.RID)) ||
		!strings.Contains(r.ids(r.named(out.Message)), "Raised to the founder as #") {
		t.Fatalf("%+v", out)
	}
	out.Finish()
	if ids, _ := db.OpenDecisionCardIDs(r.DB, r.k("bob"), "[stopped] "+addr.Ref(r.k("bob"))+" -> "+addr.Ref(r.k("alice"))+" was stopped: cycle", nil); len(ids) != 1 {
		t.Fatalf("the runaway is not on the founder's board: %v", ids)
	}
	if reply, err := r.Say(r.DB, SayIn{Target: r.k("alice"), Body: "back to you", Requester: r.k("bob"), ParentID: first.RID}); err != nil || !reply.OK {
		t.Fatalf("a say back is the ordinary shape: %+v %v", reply, err)
	}
}

func TestTheDecisionRaisedForARunawayIsOneCardPerShape(t *testing.T) {
	r := newRig(t)
	after := &After{}
	a := r.raiseDecision(r.DB, after, r.k("alice"), "need a call\nmore", "why", "", "", 0, 0)
	b := r.raiseDecision(r.DB, after, r.k("alice"), "need a call\nmore", "why", "", "", 0, 0)
	if a != b {
		t.Fatalf("the same text still waiting is a second card: %d %d", a, b)
	}
	c := r.raiseDecision(r.DB, after, r.k("alice"), "a different call", "why", "", "", 0, 0)
	if c == a {
		t.Fatal("a different text shares a card")
	}
	card, _ := db.GetRequest(r.DB, a)
	if card.Target != r.k("human") || card.Kind.String != "task" {
		t.Fatalf("%+v", card)
	}
	tid, _ := db.TaskOfCard(r.DB, a)
	task, _ := db.GetTask(r.DB, tid)
	if task.Owner != r.k("human") || task.OpenedBy != r.k("alice") || task.Title != "need a call" || task.Expects.String != "decision" || task.QueuedBy.String != r.k("human") {
		t.Fatalf("%+v", task)
	}
	after.Run()
}

func TestReportToTheFounderIsBornLandedAndAsksNothing(t *testing.T) {
	r := newRig(t)
	after := &After{}
	rid := r.report(r.DB, after, r.k("alice"), "all quiet", "fyi")
	row, _ := db.GetRequest(r.DB, rid)
	if row.State != "landed" || row.Kind.String != "say" || row.ReadAt.Valid {
		t.Fatalf("%+v", row)
	}
}

func TestAnAskToTheFounderOpensADecisionTask(t *testing.T) {
	r := newRig(t)
	out, err := r.Ask(r.DB, AskIn{Target: r.k("human"), Body: "which db?\nlong text", Requester: r.k("alice")})
	r.must(err)
	if !out.OK {
		t.Fatal(r.named(out.Message))
	}
	tid, _ := db.TaskOfCard(r.DB, out.RID)
	if tid == 0 {
		t.Fatal("no decision task")
	}
	// task new says NoCard and opens its own
	out2, _ := r.Ask(r.DB, AskIn{Target: r.k("human"), Body: "other", Requester: r.k("alice"), NoCard: true})
	if tid, _ := db.TaskOfCard(r.DB, out2.RID); tid != 0 {
		t.Fatal("NoCard still opened a task")
	}
}

func TestASayReplyLinksIntoTheThreadAndClosesNothing(t *testing.T) {
	r := newRig(t)
	q, _ := r.Ask(r.DB, AskIn{Target: r.k("bob"), Body: "what time?", Requester: r.k("alice")})
	out, err := r.Say(r.DB, SayIn{Target: r.k("alice"), Body: "noon", Requester: r.k("bob"), ParentID: q.RID})
	r.must(err)
	if !out.OK {
		t.Fatal(r.named(out.Message))
	}
	if question, _ := db.GetRequest(r.DB, q.RID); question.State != "queued" {
		t.Fatalf("a reply settles nothing: %+v", question)
	}
	if reply, _ := db.GetRequest(r.DB, out.RID); reply.ParentID.Int64 != q.RID || reply.Kind.String != "say" {
		t.Fatalf("the reply is a say in the question's thread: %+v", reply)
	}
}

// A request the daemon sends on someone's behalf must say what it forwards, so a screen can draw that and not a request nobody sent.
func TestARequestSentOnBehalfOfSomeoneNamesWhatItForwards(t *testing.T) {
	r := newRig(t)
	tid := r.newTask("bob", r.k("human"), "x")
	if _, err := r.Ask(r.DB, AskIn{Target: r.k("bob"), Body: "x", Requester: r.k("human"), Kind: "task", OnBehalfOf: true}); err == nil {
		t.Fatal("sent on the founder's behalf with nothing it forwards")
	}
	out, err := r.Ask(r.DB, AskIn{Target: r.k("bob"), Body: "x", Requester: r.k("human"), Kind: "task", OnBehalfOf: true, Forward: Forward{Task: tid, Carries: db.ForwardQueue}})
	if err != nil || !out.OK {
		t.Fatalf("%+v %v", out, err)
	}
	var carries string
	if err := r.DB.QueryRow("SELECT json_extract(payload, '$.carries') FROM events WHERE kind = 'task_forward' AND json_extract(payload, '$.request_id') = ?", out.RID).Scan(&carries); err != nil || carries != db.ForwardQueue {
		t.Fatalf("forward %q %v", carries, err)
	}
}
