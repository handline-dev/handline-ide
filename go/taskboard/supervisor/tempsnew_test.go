package supervisor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// openFor is task.new by carol for alice with a brief, and the words its reply says.
func (r *rig) openFor(title string) (int64, string) {
	r.t.Helper()
	out := r.turnCall("carol", "task.new", fmt.Sprintf(`{"owner":"alice","title":%q,"body":"the brief of %s","request_id":"n-%s"}`, title, title, title))
	res := out["result"].(map[string]any)
	return int64(res["task"].(float64)), strings.NewReplacer("<@", "", ">", "").Replace(fmt.Sprint(res["said"]))
}

// sent is whether task tid was asked of its owner.
func (r *rig) sent(tid int64) bool {
	r.t.Helper()
	row, err := db.GetTask(r.DB, tid)
	r.must(err)
	return row.SentAt.Valid
}

// A task opened for an agent mid-turn is queued unsent and a temp of it takes it, with its brief.
func TestATaskOpenedForAnAgentMidTurnGoesToATemp(t *testing.T) {
	r, temps, first, _ := tempRig(t, false)
	tid, said := r.openFor("the footer")
	if r.sent(tid) || !strings.Contains(said, "a temp of alice takes it") {
		t.Fatalf("sent %v, said %q", r.sent(tid), said)
	}
	if cur, _, _ := db.CurrentTask(r.DB, r.k("alice")); cur != first {
		t.Fatalf("alice holds %d, not her own T%d", cur, first)
	}
	for i := 0; i < 2; i++ { // the task queued before it takes the first temp
		r.clock += HireRetryS
		r.must(r.SweepTaskQueue(r.clock))
	}
	if len(temps.hired) != 2 || r.acting(tid) != temps.hired[1] {
		t.Fatalf("seats %v; T%d is %s's", temps.hired, tid, r.acting(tid))
	}
	if said := r.says(temps.hired[len(temps.hired)-1]); len(said) == 0 && !r.told(temps.hired[len(temps.hired)-1], tid) {
		t.Fatal("the temp was not given the task")
	}
	var body string
	r.must(r.DB.QueryRow(`SELECT q.body FROM requests q JOIN task_requests l ON l.request_id = q.id WHERE l.task_id = ? AND q.target = ? ORDER BY q.id DESC LIMIT 1`,
		tid, r.acting(tid)).Scan(&body))
	if !strings.Contains(body, "the brief of the footer") {
		t.Fatalf("the temp was given %q", body)
	}
}

// While a temp of it works, a task opened for the idle agent goes to a temp too.
func TestATaskOpenedForAnIdleAgentWhoseTempWorksGoesToATemp(t *testing.T) {
	r, temps, first, _ := tempRig(t, false)
	r.must(r.SweepTaskQueue(r.clock))
	r.turnCall("alice", "task.done", fmt.Sprintf(`{"task":%d,"evidence":"shipped","request_id":"d1"}`, first))
	r.must(db.SetAgentState(r.DB, r.k("alice"), states.Idle, 0, ""))
	tid, said := r.openFor("the header")
	if r.sent(tid) || !strings.Contains(said, "a temp of alice takes it") {
		t.Fatalf("sent %v, said %q", r.sent(tid), said)
	}
	r.clock += HireRetryS
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 2 || r.acting(tid) != temps.hired[1] {
		t.Fatalf("seats %v; T%d is %s's", temps.hired, tid, r.acting(tid))
	}
}

// An idle agent with no temp at work is asked at once, as before; so is a busy one with temps Manual.
func TestATaskOpenedForAFreeAgentOrWithManualTempsIsAskedAtOnce(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	r.agentUp(r.k("alice"))
	r.Temps = &fakeTemps{r: r}
	tid, said := r.openFor("a typo")
	if !r.sent(tid) || strings.Contains(said, "temp") {
		t.Fatalf("idle: sent %v, said %q", r.sent(tid), said)
	}
	r, _, _, _ = tempRig(t, true)
	tid, said = r.openFor("the footer")
	if !r.sent(tid) || strings.Contains(said, "temp") {
		t.Fatalf("Manual: sent %v, said %q", r.sent(tid), said)
	}
}

// With no further temp able to start, the task opened for the busy agent waits, says so, and reaches the agent, with its brief, once it is
// free: it is not stranded.
func TestATaskHeldWhenNoTempCanStartReachesTheAgentOnceFree(t *testing.T) {
	r, temps, first, second := tempRig(t, false)
	alice := r.k("alice")
	temps.noRoom = true
	tid, said := r.openFor("the footer")
	if r.sent(tid) || !strings.Contains(said, "it waits for alice") {
		t.Fatalf("sent %v, said %q", r.sent(tid), said)
	}
	for _, done := range []int64{first, second} { // the task queued before it goes first
		r.turnCall("alice", "task.done", fmt.Sprintf(`{"task":%d,"evidence":"shipped","request_id":"d%d"}`, done, done))
		r.must(db.SetAgentState(r.DB, alice, states.Idle, 0, ""))
		r.clock += HireRetryS
		r.must(r.SweepTaskQueue(r.clock))
	}
	if cur, _, _ := db.CurrentTask(r.DB, alice); len(temps.hired) != 0 || cur != tid || !r.sent(tid) {
		t.Fatalf("seats %v; alice holds %d, want T%d sent", temps.hired, cur, tid)
	}
	var body string
	r.must(r.DB.QueryRow(`SELECT q.body FROM requests q JOIN task_requests l ON l.request_id = q.id WHERE l.task_id = ? AND q.target = ? AND q.state != 'withdrawn' ORDER BY q.id DESC LIMIT 1`,
		tid, alice).Scan(&body))
	if !strings.Contains(body, "the brief of the footer") {
		t.Fatalf("alice was given %q", body)
	}
}

// A task the opener may not give its busy owner is refused as before, never held for a temp.
func TestATaskForABusyAgentOutOfReachIsRefusedNotHeld(t *testing.T) {
	r := newRig(t)
	r.founderOnlyTeam()
	bob := r.k("ops/bob")
	r.agentUp(bob)
	r.Temps = &fakeTemps{r: r}
	r.beginCurrent("ops/bob", "human", "the screen")
	r.must(db.SetAgentState(r.DB, bob, states.Running, 0, ""))
	got := r.callAs("eng/alice", fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.new","params":{"owner":%q,"title":"the footer","request_id":"n1"}}`, bob))
	if got["error"] == nil || !strings.Contains(fmt.Sprint(got["error"]), "another workspace") {
		t.Fatalf("want refused: %v", got)
	}
}

// The founder's message that is a task reaches the busy agent as their message, asked at once: chat.send holds nothing.
func TestOpenTaskHoldsOnlyWhenAsked(t *testing.T) {
	r, _, _, _ := tempRig(t, false)
	tx, err := r.DB.Begin()
	r.must(err)
	tid, asked, err := taskwrites.OpenTask(tx, r.Env, "p1", taskwrites.NewIn{Owner: r.k("alice"), Title: "from the chat", Body: "from the chat",
		Prio: db.DefaultPriority, Expects: "report"})
	r.must(err)
	r.must(tx.Commit())
	if !asked.OK || asked.RID == 0 || !r.sent(tid) {
		t.Fatalf("%+v, sent %v", asked, r.sent(tid))
	}
}

// With no further temp able to start, an idle agent whose temp works is asked at once.
func TestAnIdleAgentIsAskedAtOnceWhenNoTempCanStart(t *testing.T) {
	r, temps, first, _ := tempRig(t, false)
	r.must(r.SweepTaskQueue(r.clock))
	r.turnCall("alice", "task.done", fmt.Sprintf(`{"task":%d,"evidence":"shipped","request_id":"d1"}`, first))
	r.must(db.SetAgentState(r.DB, r.k("alice"), states.Idle, 0, ""))
	temps.noRoom = true
	tid, said := r.openFor("the header")
	if !r.sent(tid) || strings.Contains(said, "temp") {
		t.Fatalf("sent %v, said %q", r.sent(tid), said)
	}
}
