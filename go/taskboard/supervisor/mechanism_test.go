package supervisor

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

// breakDB closes the database under the Env: every statement after it fails, as a busy database does.
func (r *rig) breakDB() { r.DB.Close() }

func TestAFailedStatementComesBackAsAnErrorFromEveryExportedReader(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	row, _ := db.GetAgentState(r.DB, r.k(("bob")))
	req := r.ask(r.k("alice"), r.k(("bob")), "x", "ask")
	task, _ := db.GetTask(r.DB, r.newTask(r.k("bob"), r.k(("alice")), "job"))
	head, _ := db.GetRequest(r.DB, req)
	r.breakDB()
	q := db.Q(r.DB)
	calls := map[string]func() error{
		"AgentReason":           func() error { _, _, err := r.AgentReason(q, r.k(("bob")), 1); return err },
		"BgHolds":               func() error { _, err := r.BgHolds(q, r.k(("bob")), 1); return err },
		"BgLastShown":           func() error { _, _, err := r.BgLastShown(q, r.k(("bob"))); return err },
		"BgMayStop":             func() error { _, err := r.BgMayStop(q, r.k(("bob")), 1); return err },
		"BgOwesNothing":         func() error { _, err := r.BgOwesNothing(q, r.k(("bob"))); return err },
		"BgPromptedAt":          func() error { _, err := r.BgPromptedAt(q, r.k(("bob"))); return err },
		"BgSilentS":             func() error { _, _, err := r.BgSilentS(q, r.k(("bob")), 1); return err },
		"BgSince":               func() error { _, _, err := r.BgSince(q, r.k(("bob"))); return err },
		"BgStale":               func() error { _, err := r.BgStale(q, r.k(("bob")), 1); return err },
		"ClearOwed":             func() error { _, err := r.ClearOwed(q, r.k(("bob"))); return err },
		"EffectiveHead":         func() error { _, err := r.EffectiveHead(q, r.k(("bob"))); return err },
		"EffectiveHeadOrMarked": func() error { _, err := r.EffectiveHeadOrMarked(q, r.k(("bob"))); return err },
		"EffectiveTaskHead":     func() error { _, err := r.EffectiveTaskHead(q, r.k(("bob"))); return err },
		"FoldedNotices":         func() error { _, err := FoldedNotices(q, r.k(("bob"))); return err },
		"ForgottenResolve":      func() error { _, err := r.ForgottenResolve(q, r.k(("bob")), row, 1); return err },
		"HasBackgroundWork":     func() error { _, err := r.HasBackgroundWork(q, r.k(("bob"))); return err },
		"HeadTaskBlocked":       func() error { _, err := r.HeadTaskBlocked(q, r.k(("bob")), head); return err },
		"IdleSince":             func() error { _, _, err := r.IdleSince(q, r.k(("bob"))); return err },
		"InReplyTo": func() error {
			c := *head
			c.ParentID.Valid, c.ParentID.Int64 = true, head.ID
			_, err := InReplyTo(q, &c)
			return err
		},
		"LandedAlready":      func() error { _, err := r.LandedAlready(q, r.k(("bob")), req); return err },
		"LinkDelivery":       func() error { return r.LinkDelivery(q, task.ID, r.k(("bob")), req) },
		"NudgeCount":         func() error { _, err := r.NudgeCount(q, r.k(("bob")), 1); return err },
		"NudgeExhausted":     func() error { _, err := r.NudgeExhausted(q, r.k(("bob")), 1); return err },
		"RaiseDecision":      func() error { _, err := r.RaiseDecision(q, &After{}, r.k(("bob")), "t", "r", "", "", 0, 0); return err },
		"Report":             func() error { _, err := r.Report(q, &After{}, r.k(("bob")), "t", "r"); return err },
		"ScreenDrawsSession": func() error { _, err := r.ScreenDrawsSession(r.k("bob")); return err },
		"SessionChanged":     func() error { _, err := r.SessionChanged(q, r.k(("bob"))); return err },
		"StalledOnDialog":    func() error { _, err := r.StalledOnDialog(q, r.k(("bob")), "x"); return err },
		"TaskReason":         func() error { _, _, _, _, err := r.TaskReason(q, task, 1); return err },
		"TypeSay":            func() error { return r.TypeSay(q, r.k(("bob")), "x", "y") },
		"WallReason":         func() error { _, err := r.WallReason(q, r.k(("bob")), nil); return err },
		"DeliverQueued":      func() error { _, err := r.DeliverQueued(q, r.k(("bob"))); return err },
		"TellNow":            func() error { _, err := r.TellNow(q, req); return err },
		"LandingSweep":       func() error { return r.LandingSweep(q, r.k(("bob"))) },
		"AdvanceCurrentTask": func() error { _, _, err := r.AdvanceCurrentTask(q, r.k(("bob"))); return err },
		"Resolve":            func() error { _, _, err := r.Resolve(q, ResolveIn{RID: req}); return err },
		"SweepHolds":         func() error { return r.SweepHolds(1) },
		"SweepTaskQueue":     func() error { return r.SweepTaskQueue(1) },
		"SweepNotices":       func() error { return r.SweepNotices(1) },
	}
	for name, call := range calls {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("%s raised: %v", name, p)
				}
			}()
			if err := call(); err == nil || !strings.Contains(err.Error(), "database") && !strings.Contains(err.Error(), "closed") {
				t.Errorf("%s: err = %v", name, err)
			}
		}()
	}
}

func TestAFailedStatementAfterTheCommitIsLoggedAndSaidNotRaised(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	rid := r.ask(r.k("alice"), r.k(("bob")), "x", "ask")
	r.breakDB()
	for name, said := range map[string]string{
		"Push":     r.Push(r.DB, r.k(("bob")), rid),
		"PushHere": r.PushHere(r.DB, r.k(("bob")), 0),
	} {
		if !strings.Contains(said, "failed") {
			t.Errorf("%s said %q", name, said)
		}
	}
	if !r.logged("telling bob failed") {
		t.Fatalf("%v", r.logs)
	}
}

func TestWaitingPromptsToldStepReturnsItsFailure(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.ask(r.k("alice"), r.k(("bob")), "wake", "ask")
	_, told, ok, err := r.WaitingPrompt(r.DB, r.k(("bob")))
	r.must(err)
	if !ok {
		t.Fatal("nothing waits")
	}
	r.breakDB()
	if err := told("s1"); err == nil {
		t.Fatal("a failed record was swallowed")
	}
}

func TestWithoutTheProposalsPortNothingIsResolved(t *testing.T) {
	r := newRig(t)
	rid := r.ask(r.k("alice"), r.k(("bob")), "q", "ask")
	if ok, _, err := r.Resolve(r.DB, ResolveIn{RID: rid, Verdict: "yes"}); err != nil || !ok {
		t.Fatalf("with the port: %v %v", ok, err)
	}
	r.Ports.Proposals = nil
	rid2 := r.ask(r.k("alice"), r.k(("bob")), "q2", "ask")
	if _, ok, _, err := r.ResolveWrite(r.DB, ResolveIn{RID: rid2, Verdict: "yes"}); err == nil || ok || !strings.Contains(err.Error(), "proposals port is not wired") {
		t.Fatalf("%v %v", ok, err)
	}
	if row, _ := db.GetRequest(r.DB, rid2); row.State != "queued" {
		t.Fatalf("%+v", row)
	}
}

func TestFinishSayUsesTheCallersTransaction(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	// FinishSay settles on the transaction it is given, so a rollback leaves the say queued
	say := r.ask(r.k("alice"), r.k(("bob")), "psst", "say")
	row, _ := db.GetRequest(r.DB, say)
	tx, err := r.DB.Begin()
	r.must(err)
	r.FinishSay(tx, row)
	r.must(tx.Rollback())
	if row, _ := db.GetRequest(r.DB, say); row.State != "queued" {
		t.Fatalf("settled behind the transaction's back: %+v", row)
	}
	r.FinishSay(r.DB, row)
	if row, _ := db.GetRequest(r.DB, say); row.State != "landed" {
		t.Fatalf("%+v", row)
	}
}

func TestNotifyCancelledNamesTheTaskStateAndReportsAFailedSend(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	task := r.task(r.k("bob"), r.k(("alice")), "the job")
	r.DeliverQueued(r.DB, r.k(("bob")))
	row, _ := db.GetRequest(r.DB, task)
	r.term.typed = nil
	typed, why, err := r.NotifyCancelled(r.DB, row, "no longer needed")
	r.must(err)
	if !typed || why != "" || r.term.last() != "#1 from alice was cancelled: no longer needed. Stop work on it; it is closed and needs no resolve, and T1 is doing - `hl task list` shows what you hold. `hl read #1` shows it." {
		t.Fatalf("%v %q %q", typed, why, r.term.last())
	}
	r.Agents.(*sessionAgents).live[r.k(("bob"))] = false
	other := *row // the ledger answers the same operation from its first send
	other.ID = 99
	if typed, why, _ := r.NotifyCancelled(r.DB, &other, "x"); typed || why != "bob has no live session" {
		t.Fatalf("%v %q", typed, why)
	}
}

func TestTheExportedReadersAnswerTheirValueWhenNothingFails(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	rid := r.ask(r.k("alice"), r.k(("bob")), "q", "ask")
	head, _ := db.GetRequest(r.DB, rid)
	if yes, err := r.SessionPresent(r.k("bob")); err != nil || !yes {
		t.Fatal(yes, err)
	}
	if v, err := states.AnswerVerb(r.DB, head); err != nil || r.ids(v) != "answer with `hl say alice --parent #1 \"<answer>\"`" {
		t.Fatalf("%q %v", v, err)
	}
	if v, err := TellText(r.DB, head, 1, nil, false, nil, ""); err != nil || !strings.HasPrefix(r.ids(v), "#1 from alice, waiting for you:\nq\n") {
		t.Fatalf("%q %v", v, err)
	}
	if v, err := r.EffectiveHead(r.DB, r.k(("bob"))); err != nil || v == nil || v.ID != rid {
		t.Fatal(v, err)
	}
	if why, able, err := r.AgentReason(r.DB, r.k(("bob")), r.clock); err != nil || why != "" || !able {
		t.Fatal(why, able, err)
	}
}
