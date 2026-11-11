package supervisor

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

func (r *rig) ask(from, to, body, kind string) int64 {
	r.t.Helper()
	out, err := r.Ask(r.DB, AskIn{Target: r.k(to), Body: body, Requester: r.k(from), Kind: kind})
	r.must(err)
	if !out.OK {
		r.t.Fatalf("ask refused: %s", r.named(out.Message))
	}
	out.Finish()
	return out.RID
}

func TestASayIsTypedWithItsLines(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	out, err := r.Say(r.DB, SayIn{Target: r.k(("bob")), Body: "Done:\n\n- one\x07\n- two\n", Requester: r.k(("alice"))})
	r.must(err)
	head := mustReq(r, out.RID)
	if got := r.ids(tellText(r.DB, head, 1, nil, false, nil, "")); !strings.HasPrefix(r.named(got), "#1 from alice, waiting for you:\nDone:\n\n- one\n- two\n`hl read #1`") {
		t.Fatalf("%q", got)
	}
}

func TestAHandoffNoteIsTypedWithItsLinesAndTheTitleOnItsOwn(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	for note, want := range map[string]string{
		"Step 1:\n- a\x07\n- b": "T1 handed to you. Note: Step 1:\n- a\n- b\nTitle: write the docs\n",
		"check it":              "T1 handed to you. Note: check it Title: write the docs\n",
	} {
		head := mustReq(r, r.ask(r.k("alice"), r.k(("bob")), "T1 handed to you by alice: write the docs\n\n"+note, "task"))
		if got := tellText(r.DB, head, 1, nil, false, nil, ""); !strings.Contains(r.named(got), "waiting for you:\n"+want) {
			t.Errorf("%q", got)
		}
	}
}

func TestDeliverQueuedTellsTheHeadTaskThenBatchesWhatArrivesBehindIt(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	head := r.task(r.k("bob"), r.k(("alice")), "first job")
	sent, err := r.DeliverQueued(r.DB, r.k(("bob")))
	r.must(err)
	if !sent {
		t.Fatal("not delivered")
	}
	if n, _ := db.Cursor(r.DB, "notify:"+r.k("bob")); n != head {
		t.Fatalf("notify cursor %d", n)
	}
	if got := r.term.last(); !strings.HasPrefix(r.named(got), "#1 from alice, waiting for you:\nfirst job\n`hl read #1` shows it whole; you own T1 now: hand it on with ") {
		t.Fatalf("%q", got)
	}
	// two more arrive behind the open head: told as a batch, once a minute
	r.task(r.k("bob"), r.k(("alice")), "second job")
	r.task(r.k("bob"), r.k(("alice")), "third job")
	r.term.typed = nil
	if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); sent {
		t.Fatal("a batch inside the minute")
	}
	r.clock += BatchEveryS
	sent, _ = r.DeliverQueued(r.DB, r.k(("bob")))
	if !sent || !strings.HasPrefix(r.term.last(), "2 more tasks queued behind #1, which is still open:\n#2 from alice: second job\n#3 from alice: third job\n") {
		t.Fatalf("%v %q", sent, r.term.last())
	}
	if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); sent {
		t.Fatal("told twice")
	}
}

func TestSeveralUntoldTasksGoOutAsOneBatchWithTheHeadFirst(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.task(r.k("bob"), r.k(("alice")), "one")
	r.task(r.k("bob"), r.k(("alice")), "two")
	sent, _ := r.DeliverQueued(r.DB, r.k(("bob")))
	if !sent || !strings.HasPrefix(r.term.last(), "2 tasks waiting for you:\n#1 from alice: one\n#2 from alice: two\n`hl read <id>` shows one whole.") {
		t.Fatalf("%v %q", sent, r.term.last())
	}
	if n, _ := db.Cursor(r.DB, "notify:"+r.k("bob")); n != 1 {
		t.Fatal(n)
	}
}

func TestDeliverQueuedExplainsWhyItIsHolding(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.task(r.k("bob"), r.k(("alice")), "job")
	r.Agents.(*sessionAgents).live[r.k(("bob"))] = false
	if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); sent || len(r.term.typed) != 0 {
		t.Fatal("told an agent with no session")
	}
	why, contestable, ok := r.HeldReason(r.k("bob"), r.clock)
	if !ok || why != "no live session" || contestable {
		t.Fatalf("%q %v %v", why, contestable, ok)
	}
	if !r.logged("request 1 is held - no live session") {
		t.Fatal(r.logs)
	}
	r.logs = nil
	r.DeliverQueued(r.DB, r.k(("bob")))
	if r.logged("is held") {
		t.Fatal("the same reason is logged on every tick")
	}
}

func TestTheLoadHoldsATaskForAnAgentAtItsPromptButNotOneMidTurn(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.task(r.k("bob"), r.k(("alice")), "job")
	r.LoadHeld = func(string) string { return "held: machine load - 3 of 3 agents mid-turn" }
	if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); sent || len(r.term.typed) != 0 {
		t.Fatal("a task started a turn past the load")
	}
	if why, _, ok := r.HeldReason(r.k("bob"), r.clock); !ok || why != "held: machine load - 3 of 3 agents mid-turn" {
		t.Fatalf("%q %v", why, ok)
	}
	r.must(db.SetAgentState(r.DB, r.k(("bob")), states.Running, 0, ""))
	if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); !sent {
		t.Fatal("an agent mid-turn was not told")
	}
}

func TestDeliverQueuedToATerminalAgentSaysWhyItHolds(t *testing.T) {
	t.Run("a live session through the peer inbox: delivered", func(t *testing.T) {
		r := newRig(t)
		r.agentUp(r.k("bob")) // peer on is the default: the port answers structured and live
		r.task(r.k("bob"), r.k(("alice")), "job")
		if sent, err := r.DeliverQueued(r.DB, r.k(("bob"))); err != nil || !sent || len(r.term.typed) != 1 {
			t.Fatalf("%v %v %v", sent, err, r.term.typed)
		}
	})
	t.Run("peer_inbox off with a live session: the reason names it", func(t *testing.T) {
		r := newRig(t)
		r.agentUp(r.k("bob"))
		r.Agents.(*sessionAgents).noPeer[r.k(("bob"))] = true
		r.task(r.k("bob"), r.k(("alice")), "job")
		if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); sent || len(r.term.typed) != 0 {
			t.Fatal("told an agent nothing reaches")
		}
		why, _, ok := r.HeldReason(r.k("bob"), r.clock)
		if !ok || why != "peer_inbox is off for this agent; nothing types any more" {
			t.Fatalf("%q %v", why, ok)
		}
	})
	t.Run("no live session: no live session", func(t *testing.T) {
		r := newRig(t)
		r.agentUp(r.k("bob"))
		r.Agents.(*sessionAgents).live[r.k(("bob"))] = false
		r.task(r.k("bob"), r.k(("alice")), "job")
		r.DeliverQueued(r.DB, r.k(("bob")))
		if why, _, ok := r.HeldReason(r.k("bob"), r.clock); !ok || why != "no live session" {
			t.Fatalf("%q %v", why, ok)
		}
	})
}

type noSession struct{ *fakeTerm }

func (noSession) SessionOnScreen(string) (bool, bool) { return false, true }

func TestEnforceEdgesDeniesWhatTheTableDoesNotAllow(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	rid := r.ask(r.k("alice"), r.k(("bob")), "x", "")
	// queued before the edges' relation, response and kind stopped being read: those are not the table's to check
	r.DB.Exec("UPDATE requests SET edge_ok = NULL, relation = 'supervisor', response = 'fire_and_forget', kind = 'verdict' WHERE id = ?", rid)
	deep := r.ask(r.k("alice"), r.k(("bob")), "y", "")
	r.DB.Exec("UPDATE requests SET edge_ok = NULL, chain_depth = 4 WHERE id = ?", deep)
	ok, err := r.EnforceEdges(r.DB, r.k(("bob")))
	r.must(err)
	if !ok {
		t.Fatal("untrusted")
	}
	if row, _ := db.GetRequest(r.DB, rid); row.State != "queued" || row.EdgeOK.Int64 != 1 {
		t.Fatalf("%+v", row)
	}
	if row, _ := db.GetRequest(r.DB, deep); row.State != "denied" || row.Reason.String != "refused at delivery: chain depth 4 reached the limit 4 (loop guard)" {
		t.Fatalf("%+v", row)
	}
	// a system name is not an agent's edge
	sys, _ := db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: r.k(("bob")), Body: "s", Kind: "say"})
	r.DB.Exec("UPDATE requests SET edge_ok = NULL WHERE id = ?", sys)
	r.EnforceEdges(r.DB, r.k(("bob")))
	if row, _ := db.GetRequest(r.DB, sys); !row.EdgeOK.Valid || row.State != "queued" {
		t.Fatalf("%+v", row)
	}
}

func TestResolvingAnAskTypesNothingAtItsAsker(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("alice"))
	q := r.ask(r.k("alice"), r.k(("bob")), "what time is it", "ask")
	ok, msg, err := r.Resolve(r.DB, ResolveIn{RID: q, Verdict: "noon"})
	r.must(err)
	if !ok || strings.Contains(msg, r.k(("alice"))) {
		t.Fatal(ok, msg)
	}
	if last := r.term.last(); last != "" {
		t.Fatalf("an answer is no longer typed at whoever asked: %q", last)
	}
}

func TestWaitingPromptIsTheTellTheNextTickWouldType(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	rid := r.ask(r.k("alice"), r.k(("bob")), "wake up", "ask")
	text, told, ok, err := r.WaitingPrompt(r.DB, r.k(("bob")))
	r.must(err)
	if !ok || !strings.HasPrefix(r.ids(text), "#1 from alice, waiting for you:\nwake up\n") {
		t.Fatalf("%q", text)
	}
	if row, _ := db.GetRequest(r.DB, rid); !row.DeliveredAt.Valid {
		t.Fatal("not marked delivered before the launch")
	}
	told("s7")
	if n, _ := db.Cursor(r.DB, "notify:"+r.k("bob")); n != rid {
		t.Fatal(n)
	}
	if s, _ := db.NotifiedSession(r.DB, r.k(("bob"))); s != "s7" {
		t.Fatal(s)
	}
	r.must(db.MarkLanded(r.DB, rid)) // the resumed session read it: it leaves the line
	rid = r.ask(r.k("alice"), r.k(("bob")), "start fresh", "ask")
	_, told, _, _ = r.WaitingPrompt(r.DB, r.k(("bob")))
	told("") // a fresh start: recorded as told to the session now live, so it is not told again
	if s, _ := db.NotifiedSession(r.DB, r.k(("bob"))); s != "s1" {
		t.Fatal(s)
	}
	r.must(db.MarkLanded(r.DB, rid))
	db.AddPending(r.DB, r.k(("bob")), "a bare line", 5)
	text, told, ok, _ = r.WaitingPrompt(r.DB, r.k(("bob")))
	if !ok || text != "a bare line" {
		t.Fatal(text)
	}
	told("s7")
	if rows, _ := db.ListPending(r.DB, r.k(("bob"))); len(rows) != 0 {
		t.Fatal("pending row kept")
	}
}

// The task bob holds stays at the head of his queue once it has landed; the say behind it is what waits for the session that wakes him.
func TestWaitingPromptPassesOverAHeldTaskToTheSayBehindIt(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	held := r.task(r.k("bob"), r.k(("alice")), "the one bob holds")
	if _, err := db.MarkDelivered(r.DB, held); err != nil {
		t.Fatal(err)
	}
	r.must(db.MarkLanded(r.DB, held))
	say := r.ask("sys", r.k(("bob")), "T9 is unblocked", "say")
	text, _, ok, err := r.WaitingPrompt(r.DB, r.k(("bob")))
	r.must(err)
	if !ok || !strings.Contains(text, "T9 is unblocked") {
		t.Fatalf("%v %q", ok, text)
	}
	if row, _ := db.GetRequest(r.DB, say); !row.DeliveredAt.Valid {
		t.Fatal("the say was not marked delivered")
	}
}

// task opens a queued task for owner with the delivery request, as task new does.
func (r *rig) task(owner, opener, title string) int64 {
	r.t.Helper()
	tid, err := db.NewTask(r.DB, title, r.k(owner), r.k(opener), 0, db.DefaultPriority, "", 0, "")
	r.must(err)
	db.SetTaskQueued(r.DB, tid, r.k(opener))
	rid := r.ask(opener, owner, title, "task")
	r.must(db.LinkRequest(r.DB, tid, rid))
	return rid
}

func TestThePauseHoldsAnAgentsMessageToAnAgentAtItsPromptButNotAPersonsOrOneMidTurn(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	paused := "Waiting: new work is paused."
	r.Paused = func() string { return paused }
	rid := r.ask(r.k("alice"), r.k(("bob")), "look at this", "ask")
	if got, _ := r.TellNow(r.DB, rid); !strings.Contains(got, paused) || len(r.term.typed) != 0 {
		t.Fatalf("an agent's message started a turn while paused: %q %v", got, r.term.typed)
	}
	if got, _ := r.TellNow(r.DB, r.ask(db.Human, r.k(("bob")), "from the founder", "ask")); got != "sent" {
		t.Fatalf("the founder's message was held: %q", got)
	}
	r.must(db.SetAgentState(r.DB, r.k(("bob")), states.Running, 0, ""))
	if got, _ := r.TellNow(r.DB, r.ask(r.k("alice"), r.k(("bob")), "mid-turn", "ask")); got != "sent" {
		t.Fatalf("an agent mid-turn was not told: %q", got)
	}
	r.must(db.SetAgentState(r.DB, r.k(("bob")), states.Idle, 0, ""))
	paused = ""
	r.must(r.TellUndelivered(r.DB, r.k(("bob"))))
	if row, _ := db.GetRequest(r.DB, rid); !row.DeliveredAt.Valid {
		t.Fatal("the held message was not told after the pause")
	}
}

func TestThePauseHoldsATaskMidTurnTooButNotOneTheFounderSent(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.must(db.SetAgentState(r.DB, r.k(("bob")), states.Running, 0, ""))
	paused := "Waiting: new work is paused."
	r.Paused = func() string { return paused }
	r.task(r.k("bob"), r.k(("alice")), "job")
	if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); sent || len(r.term.typed) != 0 {
		t.Fatalf("a task reached an agent mid-turn while paused: %v", r.term.typed)
	}
	paused = ""
	if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); !sent {
		t.Fatal("the held task was not told after the pause")
	}
	r2 := newRig(t)
	r2.agentUp(r.k("bob"))
	r2.Paused = func() string { return "Waiting: new work is paused." }
	r2.task(r.k("bob"), db.Human, "the founder's own")
	if sent, _ := r2.DeliverQueued(r2.DB, r.k(("bob"))); !sent {
		t.Fatal("a task the founder sent was held by the pause")
	}
}

// While paused, only the deliveries that pass the pause are told, alone; and the load's word for the pause, which the real gate gives
// an agent at its prompt, does not hold them.
func TestWhilePausedOnlyAPassedTaskIsToldAndThePausesLoadWordDoesNotHoldIt(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	paused := "Waiting: new work is paused."
	r.Paused = func() string { return paused }
	load := paused
	r.LoadHeld = func(string) string { return load }
	r.task(r.k("bob"), r.k(("alice")), "the first job")
	second := r.task(r.k("bob"), r.k(("alice")), "the second job")
	if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); sent || len(r.term.typed) != 0 {
		t.Fatalf("told while paused: %v", r.term.typed)
	}
	_, err := db.AddEvent(r.DB, r.k(("bob")), db.PausePassed, db.KV{K: "request_id", V: second})
	r.must(err)
	r.must(db.PromoteToFront(r.DB, second))
	if sent, _ := r.DeliverQueued(r.DB, r.k(("bob"))); !sent {
		t.Fatal("the passed task was held")
	}
	typed := strings.Join(r.term.typed, "\n")
	if !strings.Contains(r.named(typed), "the second job") || strings.Contains(r.named(typed), "the first job") {
		t.Fatalf("told: %q", typed)
	}
	r3 := newRig(t)
	r3.agentUp(r.k("bob"))
	r3.Paused = func() string { return paused }
	r3.LoadHeld = func(string) string { return paused }
	r3.task(r.k("bob"), db.Human, "the founder's own")
	if sent, _ := r3.DeliverQueued(r3.DB, r.k(("bob"))); !sent {
		t.Fatal("the founder's task at the prompt was held by the pause")
	}
	load = "Waiting: the computer is busy right now."
	r2 := newRig(t)
	r2.agentUp(r.k("bob"))
	r2.Paused = func() string { return paused }
	r2.LoadHeld = func(string) string { return load }
	r2.task(r.k("bob"), db.Human, "the founder's own")
	if sent, _ := r2.DeliverQueued(r2.DB, r.k(("bob"))); sent {
		t.Fatal("the load's own hold let the founder's task through at the prompt")
	}
}

func TestAnyPersonsMessageIsUrgentNotOnlyTheOwners(t *testing.T) {
	for _, c := range []struct {
		requester string
		want      bool
	}{{db.Human, true}, {"p2", true}, {"a3", false}, {"sys", false}} {
		if got := urgent(&db.Request{Requester: c.requester}); got != c.want {
			t.Errorf("urgent(requester %s) = %v, want %v", c.requester, got, c.want)
		}
	}
}
