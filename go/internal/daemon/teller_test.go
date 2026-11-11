package daemon

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// typedAt is a structured agent port that keeps what was typed at each agent.
type typedAt struct {
	mu    sync.Mutex
	typed map[string][]string
}

func (a *typedAt) Structured(string) bool { return true }
func (a *typedAt) Live(string) bool       { return true }
func (a *typedAt) Send(agent, op, text string, _ delivery.Delivery) (delivery.Receipt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.typed[agent] = append(a.typed[agent], text)
	return delivery.Receipt{}, nil
}
func (a *typedAt) Withdraw(string, string) (delivery.Receipt, bool, error) {
	return delivery.Receipt{}, true, nil
}
func (a *typedAt) Promote(string, string) (delivery.Receipt, error) {
	return delivery.Receipt{}, nil
}
func (a *typedAt) said(agent string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Join(a.typed[agent], "\n")
}

// The ports a structured-only daemon has no screen for: nothing is on a screen, so every telling goes through the agent port.
type noScreen struct{}

func (noScreen) Available() bool                             { return false }
func (noScreen) Missing() string                             { return "no terminal" }
func (noScreen) Alive(string) bool                           { return false }
func (noScreen) TypeableNow(string) bool                     { return false }
func (noScreen) ScreenBlocker(string) *term.Blocker          { return nil }
func (noScreen) SessionOnScreen(string) (bool, bool)         { return false, false }
func (noScreen) ComposerOnScreen(string) (string, bool)      { return "", false }
func (noScreen) IsTextTyped(string, string) bool             { return false }
func (noScreen) BackgroundWork(string) (string, bool)        { return "", false }
func (noScreen) LastTail(string) []string                    { return nil }
func (noScreen) Send(string, string, bool) (bool, string)    { return false, "no terminal" }
func (noScreen) SendToSession(string, string) (bool, string) { return false, "no terminal" }
func (noScreen) SendCtrlC(string)                            {}
func (noScreen) ClearContext(string) (bool, string)          { return false, "no terminal" }
func (noScreen) ClearCommand(string) string                  { return "/clear" }

// wiredPorts is every other port as an agent with no screen, no limit and nothing in the way: the telling layer can call it. live is
// the agents with a session, by key.
type wiredPorts struct{ live map[string]bool }

func (w wiredPorts) LiveSessions() (map[string]bool, bool)                           { return w.live, true }
func (wiredPorts) PastDialog(db.Q, string, bool) (string, string)                    { return "", "" }
func (wiredPorts) Reason(kind, detail string) string                                 { return kind + ": " + detail }
func (wiredPorts) DecisionAfterS() int64                                             { return 600 }
func (wiredPorts) WaitingOnPrompt(db.Q, *db.AgentState) bool                         { return false }
func (wiredPorts) AtLimit(db.Q, string) bool                                         { return false }
func (wiredPorts) NoticeDigestS(string) int64                                        { return 900 }
func (wiredPorts) CloseTimeoutS(string) int64                                        { return 600 }
func (wiredPorts) Never() int64                                                      { return -1 }
func (wiredPorts) BgGraceS(string) int64                                             { return 300 }
func (wiredPorts) ConfigSeconds(_ string, def int64) int64                           { return def }
func (wiredPorts) FooterCount(string) int                                            { return 1 }
func (wiredPorts) LiveSession(db.Q, string) string                                   { return "s1" }
func (wiredPorts) ContextUse(db.Q, string) (int64, int64)                            { return 5000, 200000 }
func (wiredPorts) KeepsTranscripts(string) bool                                      { return false }
func (wiredPorts) FindTranscript(string, string) string                              { return "" }
func (wiredPorts) MovableTrigger(string) bool                                        { return false }
func (wiredPorts) CacheLeft(string, int64) (int64, bool)                             { return 0, false }
func (wiredPorts) StartForWork(db.Q, string) string                                  { return "" }
func (wiredPorts) WakeAgent(db.Q, string) (bool, string)                             { return false, "" }
func (wiredPorts) AccountWait(db.Q, string) (string, error)                          { return "", nil }
func (wiredPorts) QueuedOrIdle(db.Q, string, string) error                           { return nil }
func (wiredPorts) Marker(db.Q, string, string, map[string]any)                       {}
func (wiredPorts) RecordScreenSample(db.Q, string, *db.AgentState, string, []string) {}
func (wiredPorts) OpenProposalOf(int64) string                                       { return "" }
func (wiredPorts) Answered(db.Q, string, *db.Request, string) (string, string, error) {
	return "", "", nil
}
func (wiredPorts) DeliveryBlock(db.Q, string) (string, bool) { return "", false }

func tellDaemon(t *testing.T, ports bool) (*Daemon, *typedAt) {
	t.Helper()
	return wiredDaemon(t, ports, func(*sql.DB, string) supervisor.Proposals { return wiredPorts{} })
}

// wiredDaemon is tellDaemon with the proposals port given, or none of the ports when !ports.
func wiredDaemon(t *testing.T, ports bool, proposals func(*sql.DB, string) supervisor.Proposals) (*Daemon, *typedAt) {
	t.Helper()
	t.Setenv("HANDLINE_TEST_ROOT", t.TempDir())
	home := t.TempDir()
	live := map[string]bool{}
	for _, a := range []string{"alice", "bob"} {
		teamtest.Set(t, home, a, "vendor", "claude")
		live[teamtest.Key(t, home, a)] = true
	}
	agents := &typedAt{typed: map[string][]string{}}
	cfg := Config{Home: home, DBFile: filepath.Join(home, "db.sqlite"), Auth: pidAuth{}, Log: t.Logf}
	if ports {
		cfg.OnOpen = func(d *sql.DB, env *supervisor.Env) {
			env.Ports = supervisor.Ports{Agents: agents, Terminal: noScreen{}, Observer: wiredPorts{}, Dialogs: wiredPorts{}, Waits: wiredPorts{}, Idle: wiredPorts{}, Sessions: wiredPorts{live: live},
				Lifecycle: wiredPorts{}, Evaluate: wiredPorts{}, Proposals: proposals(d, env.Home), Stuck: wiredPorts{}}
		}
	}
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d, agents
}

func TestAMemoOnADoingTaskIsTypedAtItsHolderWhenThePortsAreWired(t *testing.T) {
	d, agents := tellDaemon(t, true)
	doing(t, d, "bob")
	f := pipeTo(t, d, "")
	r := f.ask(`{"jsonrpc":"2.0","id":2,"method":"task.memo.add","params":{"task":1,"text":"@bob mind the cache","request_id":"m1"}}`)
	if r["result"] == nil {
		t.Fatalf("%v", r)
	}
	if got := agents.said(key(t, d, "bob")); !strings.Contains(got, "mind the cache") {
		t.Fatalf("the holder was not told: %q", got)
	}
	result(t, f.ask(`{"jsonrpc":"2.0","id":3,"method":"task.memo.edit","params":{"task":1,"memo":1,"text":"@bob mind the disk","request_id":"m2"}}`))
	if got := agents.said(key(t, d, "bob")); !strings.Contains(got, "comment 1 edited by a: @bob mind the disk") {
		t.Fatalf("the edit was not typed: %q", got)
	}
	// a delete names no reader, so nobody is told
	result(t, f.ask(`{"jsonrpc":"2.0","id":4,"method":"task.memo.delete","params":{"task":1,"memo":1,"request_id":"m3"}}`))
	held, err := db.PendingNotices(d.db, key(t, d, "bob"))
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 0 {
		t.Fatalf("the delete waits in the holder's digest: %v", held)
	}
}

// doing opens T1 for the agent, queues it and makes it the one in hand.
func doing(t *testing.T, d *Daemon, agent string) {
	t.Helper()
	agent = key(t, d, agent)
	tid, err := db.NewTask(d.db, "job", agent, key(t, d, "alice"), 0, db.DefaultPriority, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetTaskQueued(d.db, tid, actor.OwnerKey); err != nil {
		t.Fatal(err)
	}
	if !d.env().Ports.Wired() { // nothing to tell it with: only make the task the one in hand
		if err := db.SetCurrentTask(d.db, agent, tid, true); err != nil {
			t.Fatal(err)
		}
		return
	}
	d.env().AdvanceCurrentTask(d.db, agent)
}

// key is the key of the agent named name by its old name in d's home; human is the owner, p1.
func key(t *testing.T, d *Daemon, name string) string {
	t.Helper()
	return teamtest.Key(t, d.cfg.Home, name)
}

func result(t *testing.T, r map[string]any) map[string]any {
	t.Helper()
	m, _ := r["result"].(map[string]any)
	if m == nil {
		t.Fatalf("%v", r)
	}
	return m
}

func TestAQueuedTaskGoesToAFreeOwnerAndTheReplySaysSo(t *testing.T) {
	d, agents := tellDaemon(t, true)
	tid, _ := db.NewTask(d.db, "job", key(t, d, "bob"), key(t, d, "alice"), 0, db.DefaultPriority, "", 0, "")
	f := pipeTo(t, d, "k")
	said := result(t, f.ask(`{"jsonrpc":"2.0","id":2,"method":"task.queue","params":{"tasks":[1],"request_id":"q1"}}`))["said"]
	if want := "task T1 is queued (" + actor.OwnerKey + ") - " + key(t, d, "bob") + " was free, so it went out now"; said != want {
		t.Fatalf("%v", said)
	}
	if got := agents.said(key(t, d, "bob")); !strings.Contains(got, "job") {
		t.Fatalf("the task was not typed at its owner: %q", got)
	}
	_ = tid
	// the owner is busy now: the second is queued behind the first and the reply says only that
	db.NewTask(d.db, "second", key(t, d, "bob"), key(t, d, "alice"), 0, db.DefaultPriority, "", 0, "")
	said = result(t, f.ask(`{"jsonrpc":"2.0","id":3,"method":"task.queue","params":{"tasks":[2],"request_id":"q2"}}`))["said"]
	if !strings.HasPrefix(said.(string), "task T2 is queued ("+actor.OwnerKey+")") || strings.Contains(said.(string), "went out") {
		t.Fatalf("%v", said)
	}
}

func TestADiscardTypesItsSaysAndMovesTheOwnerOn(t *testing.T) {
	d, agents := tellDaemon(t, true)
	doing(t, d, "bob")
	second, _ := db.NewTask(d.db, "the next one", key(t, d, "bob"), key(t, d, "alice"), 0, db.DefaultPriority, "", 0, "")
	db.SetTaskQueued(d.db, second, actor.OwnerKey)
	reqs, _ := db.TaskRequests(d.db, 1)
	for _, q := range reqs {
		db.CloseIfQueued(d.db, q.ID, "dismissed", "")
	}
	agents.typed = map[string][]string{}
	f := pipeTo(t, d, "k")
	result(t, f.ask(`{"jsonrpc":"2.0","id":2,"method":"task.discard","params":{"task":1,"reason":"not needed","request_id":"d1"}}`))
	if got := agents.said(key(t, d, "bob")); !strings.Contains(got, "T1 discarded by the founder: not needed") {
		t.Fatalf("the discard was not told: %q", got)
	}
	if cur, _, _ := db.CurrentTask(d.db, key(t, d, "bob")); cur != second {
		t.Fatalf("the owner was not moved on to T%d: %d", second, cur)
	}
}

func TestNoTellingWhenThePortsAreNotAllSet(t *testing.T) {
	d, agents := tellDaemon(t, false)
	if d.teller() != nil {
		t.Fatal("a Teller without every port is a nil call")
	}
	doing(t, d, "bob")
	agents.typed = map[string][]string{}
	f := pipeTo(t, d, "k")
	result(t, f.ask(`{"jsonrpc":"2.0","id":2,"method":"task.memo.add","params":{"task":1,"text":"@bob mind the cache","request_id":"m1"}}`))
	if got := agents.said(key(t, d, "bob")); got != "" {
		t.Fatalf("nothing is told: %q", got)
	}
}

// The founder's comment passes their turn before it is told: recipients names the holder it goes back to, not who the task answers to, as add does.
func TestCommentRecipientsOnTheFoundersTurnNameTheHolderBelowIt(t *testing.T) {
	d, agents := tellDaemon(t, true)
	tid, err := db.NewTask(d.db, "job", key(t, d, "bob"), key(t, d, "alice"), 0, db.DefaultPriority, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PushTurn(d.db, tid, db.Turn{Agent: actor.OwnerKey, By: key(t, d, "bob"), Since: 1}); err != nil {
		t.Fatal(err)
	}
	founder := pipeTo(t, d, "k")
	list, _ := result(t, founder.ask(fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"task.comment.recipients","params":{"task":%d,"text":"mind the cache"}}`, tid)))["recipients"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["agent"] != key(t, d, "bob") {
		t.Fatalf("recipients on the founder's turn: %v", list)
	}
	result(t, founder.ask(fmt.Sprintf(`{"jsonrpc":"2.0","id":6,"method":"task.comment.add","params":{"task":%d,"text":"mind the cache","request_id":"rt1"}}`, tid)))
	if !strings.Contains(agents.said(key(t, d, "bob")), "mind the cache") || strings.Contains(agents.said(key(t, d, "alice")), "mind the cache") {
		t.Fatalf("add told other than recipients said: alice %q bob %q", agents.said(key(t, d, "alice")), agents.said(key(t, d, "bob")))
	}
}

// recipients answers who add then tells, by one rule: with and without skip, for the founder's comment and an agent's.
func TestCommentRecipientsAreExactlyWhoAddThenTells(t *testing.T) {
	d, agents := tellDaemon(t, true)
	tid, err := db.NewTask(d.db, "job", key(t, d, "bob"), key(t, d, "alice"), 0, db.DefaultPriority, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	founder, agent := pipeTo(t, d, "k"), pipeTo(t, d, "")
	alice, bob := key(t, d, "alice"), key(t, d, "bob")
	recipients := func(f *pipeConn, text string) string {
		var out []string
		list, _ := result(t, f.ask(fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"task.comment.recipients","params":{"task":%d,"text":%q}}`, tid, text)))["recipients"].([]any)
		for _, r := range list {
			row := r.(map[string]any)
			out = append(out, fmt.Sprint(row["agent"], ":", row["why"]))
		}
		return strings.Join(out, ",")
	}
	told := func(who string) bool { return strings.Contains(agents.said(who), "mind the cache") }
	n := 0
	add := func(f *pipeConn, skip string) {
		n++
		result(t, f.ask(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"task.comment.add","params":{"task":%d,"text":"@alice mind the cache","skip":%s,"request_id":"rc%d"}}`, 10+n, tid, skip, n)))
	}
	if got := recipients(founder, "@alice mind the cache"); got != alice+":asked,"+bob+":assignee" {
		t.Fatalf("the founder's comment is for who it names and the assignee: %q", got)
	}
	if got := recipients(founder, "no one named"); got != bob+":assignee" {
		t.Fatalf("the founder's comment on a task an agent holds is that holder's alone: %q", got)
	}
	if got := recipients(agent, "@alice mind the cache"); got != alice+":asked,"+bob+":assignee" {
		t.Fatalf("an agent's comment is for who it names and the holder: %q", got)
	}
	if got := recipients(agent, "no one named"); got != bob+":assignee" {
		t.Fatalf("a comment naming no one is told to the holder only: %q", got)
	}
	add(founder, `[]`)
	if !told(alice) || !told(bob) {
		t.Fatalf("add told other than recipients said: alice %v bob %v", told(alice), told(bob))
	}
	d2, agents2 := tellDaemon(t, true)
	alice2, bob2 := key(t, d2, "alice"), key(t, d2, "bob")
	tid2, _ := db.NewTask(d2.db, "job", bob2, alice2, 0, db.DefaultPriority, "", 0, "")
	result(t, pipeTo(t, d2, "k").ask(fmt.Sprintf(`{"jsonrpc":"2.0","id":9,"method":"task.comment.add","params":{"task":%d,"text":"@alice mind the cache","skip":["bob"],"request_id":"rs1"}}`, tid2)))
	if !strings.Contains(agents2.said(alice2), "mind the cache") || strings.Contains(agents2.said(bob2), "mind the cache") {
		t.Fatalf("a skipped assignee was told, or the one not skipped was not: alice %q bob %q", agents2.said(alice2), agents2.said(bob2))
	}
	var kept int
	d2.db.QueryRow("SELECT COUNT(*) FROM task_memos WHERE task_id = ?", tid2).Scan(&kept)
	if kept != 1 {
		t.Fatalf("the comment stays on the task: %d", kept)
	}
}
