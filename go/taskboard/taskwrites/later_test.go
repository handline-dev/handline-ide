package taskwrites

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
)

// laterEnv is task 1, worker's and doing, with laters later comments of worker's on it.
func laterEnv(t *testing.T, laters ...string) (*sql.DB, func(agent string, rid, method, params string) map[string]any) {
	t.Helper()
	sdb, mk := discardEnv(t)
	exec(t, sdb, "INSERT INTO tasks(id, title, owner, opened_by, created_at, sent_at) VALUES(1, 'build', 'worker', 'manager', 1, 1)")
	f := &fakeTeller{}
	servers := map[string]*rpc.Server{}
	run := func(agent, rid, method, params string) map[string]any {
		if servers[agent] == nil {
			servers[agent] = mk(agent, f)
		}
		return call(t, servers[agent], fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":{"task":1,"request_id":%q%s}}`, method, rid, params))[0]
	}
	for i, l := range laters {
		if r := run("worker", fmt.Sprint("l", i), "task.comment.add", fmt.Sprintf(`,"text":%q,"later":"human"`, l)); r["error"] != nil {
			t.Fatal(r)
		}
	}
	return sdb, run
}

func openLater(t *testing.T, sdb *sql.DB) []db.LaterComment {
	t.Helper()
	person, err := db.LaterFor(sdb, 1)
	if err != nil {
		t.Fatal(err)
	}
	open, err := db.OpenLaterComments(sdb, 1, person)
	if err != nil {
		t.Fatal(err)
	}
	return open
}

func okSaid(t *testing.T, r map[string]any) string {
	t.Helper()
	if r["error"] != nil {
		t.Fatal(r["error"])
	}
	s, _ := r["result"].(map[string]any)["said"].(string)
	return s
}

// A later comment asks nothing: no turn, no request, nobody told; the agent keeps the task.
func TestALaterCommentAsksNothingNow(t *testing.T) {
	t.Parallel()
	sdb, _ := laterEnv(t, "log in on the mini")
	if n := count(t, sdb, "SELECT COUNT(*) FROM task_turns") + count(t, sdb, "SELECT COUNT(*) FROM requests"); n != 0 {
		t.Fatalf("%d turns and requests", n)
	}
	if open := openLater(t, sdb); len(open) != 1 || open[0].Body != "log in on the mini" {
		t.Fatalf("open later steps %+v", open)
	}
}

// The founder's comment answers the later comments before it, not one after.
func TestTheFoundersCommentAnswersTheEarlierLaterSteps(t *testing.T) {
	t.Parallel()
	sdb, run := laterEnv(t, "first")
	okSaid(t, run("human", "h", "task.comment.add", `,"text":"done that"`))
	okSaid(t, run("worker", "l2", "task.comment.add", `,"text":"second","later":"human"`))
	if open := openLater(t, sdb); len(open) != 1 || open[0].Body != "second" {
		t.Fatalf("open later steps %+v", open)
	}
}

// A done with later steps open waits on one founder turn listing them: the pass closes the task with the agent's words, a comment sends
// it back to the agent.
func TestADoneWithLaterStepsWaitsOnTheFounder(t *testing.T) {
	t.Parallel()
	for _, pass := range []bool{true, false} {
		sdb, run := laterEnv(t, "log in on the mini", "look at the badge")
		said := okSaid(t, run("worker", "d", "task.done", `,"evidence":"shipped it"`))
		if !strings.Contains(said, "waits on") {
			t.Fatalf("the done said %q", said)
		}
		turns, _ := db.Turns(sdb, 1)
		if len(turns) != 1 || turns[0].OnPass != db.TurnOnPassFinish || !strings.Contains(turns[0].Evidence, `"Words":"shipped it"`) ||
			!strings.Contains(turns[0].Ask, "log in on the mini") || !strings.Contains(turns[0].Ask, "look at the badge") {
			t.Fatalf("turns %+v", turns)
		}
		if count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE done_evidence IS NOT NULL") != 0 {
			t.Fatal("the task closed with later steps open")
		}
		if pass {
			okSaid(t, run("human", "p", "task.turn.pop", `,"outcome":"pass"`))
			if count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE done_evidence = 'shipped it'") != 1 {
				t.Fatal("the founder's pass did not close the task with the agent's words")
			}
			continue
		}
		okSaid(t, run("human", "c", "task.comment.add", `,"text":"the badge is wrong"`))
		if count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE done_evidence IS NULL") != 1 || count(t, sdb, "SELECT COUNT(*) FROM task_turns") != 0 {
			t.Fatal("the founder's comment did not send the task back open to its agent")
		}
		if open := openLater(t, sdb); len(open) != 0 {
			t.Fatalf("the founder's comment left %+v open", open)
		}
		if said := okSaid(t, run("worker", "d2", "task.done", `,"evidence":"fixed"`)); said != "task T1 done (fixed)" {
			t.Fatalf("a done with none open said %q", said)
		}
	}
}

// A handoff with later steps open waits on the founder too; the pass hands it on as asked.
func TestAHandoffWithLaterStepsWaitsOnTheFounder(t *testing.T) {
	t.Parallel()
	sdb, run := laterEnv(t, "log in on the mini")
	okSaid(t, run("worker", "h", "task.handoff", `,"to":"other","note":"please land"`))
	turns, _ := db.Turns(sdb, 1)
	if len(turns) != 1 || turns[0].OnPass != db.TurnOnPassHandoff || turns[0].Evidence != "please land" {
		t.Fatalf("turns %+v", turns)
	}
	okSaid(t, run("human", "p", "task.turn.pop", `,"outcome":"pass"`))
	if count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE id = 1 AND owner = 'other'") != 1 {
		t.Fatal("the founder's pass did not hand the task to other")
	}
	if count(t, sdb, "SELECT COUNT(*) FROM requests WHERE kind = 'task' AND body LIKE 'T1 handed to you by %please land'") != 1 {
		t.Fatal("the handoff's note did not reach its agent")
	}
	if open := openLater(t, sdb); len(open) != 0 {
		t.Fatalf("the founder's pass left %+v open", open)
	}
	if said := okSaid(t, run("other", "d", "task.done", `,"evidence":"landed"`)); said != "task T1 done (landed)" {
		t.Fatalf("the next holder's done said %q", said)
	}
}

// A held done is checked as a done now and run again as its agent on the pass; a closer that does not act on the task is not held.
func TestAHeldDoneKeepsTheDoneGates(t *testing.T) {
	t.Parallel()
	sdb, run := laterEnv(t, "look")
	exec(t, sdb, "INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES(2, 'kid', 'other', 'manager', 1, 1)")
	if r := run("worker", "d", "task.done", `,"evidence":"shipped"`); r["error"] == nil || count(t, sdb, "SELECT COUNT(*) FROM task_turns") != 0 {
		t.Fatalf("a done with an open child was held: %v", r)
	}
	exec(t, sdb, "UPDATE tasks SET discarded_by = 'manager' WHERE id = 2")
	okSaid(t, run("worker", "d2", "task.done", `,"evidence":"shipped"`))
	exec(t, sdb, "INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES(3, 'late kid', 'other', 'manager', 1, 1)")
	okSaid(t, run("human", "p", "task.turn.pop", `,"outcome":"pass"`))
	if count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE id = 1 AND done_evidence IS NULL") != 1 || count(t, sdb, "SELECT COUNT(*) FROM task_comments WHERE body LIKE '%not closed:%'") != 1 {
		t.Fatal("a pass that a gate refuses now did not give the task back with why")
	}
	if open := openLater(t, sdb); len(open) != 0 {
		t.Fatalf("the pass left %+v open", open)
	}
}

// A later step goes with no turn: refused with one, and nothing is left behind.
func TestALaterStepWithATurnIsRefused(t *testing.T) {
	t.Parallel()
	sdb, run := laterEnv(t)
	if r := run("worker", "x", "task.comment.add", `,"text":"a","later":"human","turn":"human"`); r["error"] == nil {
		t.Fatal("later with a turn was accepted")
	}
	if n := count(t, sdb, "SELECT COUNT(*) FROM task_comments"); n != 0 {
		t.Fatalf("%d comments left", n)
	}
}

// An agent acting through a turn holds its done like an owner: the pass closes the task as that agent.
func TestAHeldDoneOfATurnsAgentClosesOnThePass(t *testing.T) {
	t.Parallel()
	sdb, run := laterEnv(t)
	okSaid(t, run("worker", "t", "task.comment.add", `,"text":"take this part","turn":"other"`))
	okSaid(t, run("other", "l", "task.comment.add", `,"text":"look at it","later":"human"`))
	okSaid(t, run("other", "d", "task.done", `,"evidence":"part done"`))
	okSaid(t, run("human", "p", "task.turn.pop", `,"outcome":"pass"`))
	if count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE id = 1 AND done_evidence = 'part done'") != 1 {
		t.Fatal("the pass did not close the task with the turn agent's words")
	}
}

// A review's pass with later steps open waits on the founder too, with the review's evidence.
func TestAReviewPassWaitsOnTheLaterSteps(t *testing.T) {
	t.Parallel()
	sdb, run := laterEnv(t, "look at it")
	okSaid(t, run("worker", "r", "task.comment.add", `,"text":"review it","turn":"other","done":"reviewed"`))
	okSaid(t, run("other", "p", "task.turn.pop", `,"outcome":"pass"`))
	turns, _ := db.Turns(sdb, 1)
	if count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE done_evidence IS NOT NULL") != 0 || len(turns) != 1 || turns[0].OnPass != db.TurnOnPassFinish {
		t.Fatalf("the review's pass closed the task, or holds %+v", turns)
	}
	okSaid(t, run("human", "p2", "task.turn.pop", `,"outcome":"pass"`))
	if count(t, sdb, "SELECT COUNT(*) FROM tasks WHERE id = 1 AND done_evidence = 'reviewed'") != 1 {
		t.Fatal("the founder's pass did not close it with the review's evidence")
	}
}

// The founder's send-back answers the later steps the turn brought.
func TestASendBackAnswersTheLaterSteps(t *testing.T) {
	t.Parallel()
	sdb, run := laterEnv(t, "look at it")
	okSaid(t, run("worker", "d", "task.done", `,"evidence":"shipped"`))
	okSaid(t, run("human", "b", "task.turn.pop", `,"outcome":"send_back","note":"not yet"`))
	if open := openLater(t, sdb); len(open) != 0 {
		t.Fatalf("the send-back left %+v open", open)
	}
	if said := okSaid(t, run("worker", "d2", "task.done", `,"evidence":"shipped"`)); said != "task T1 done (shipped)" {
		t.Fatalf("the next done said %q", said)
	}
}
