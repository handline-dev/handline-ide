package chatport

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/chatport/chatfake"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/holder"
)

func TestMain(m *testing.M) {
	if os.Getenv("CHATPORT_FAKE_CLI") == "1" {
		chatfake.Run()
		return
	}
	os.Exit(m.Run())
}

type events struct {
	mu        sync.Mutex
	delivered []string
	failed    []string
	ends      int
	session   string
}

func (e *events) has(f func() bool) bool {
	for i := 0; i < 300; i++ {
		e.mu.Lock()
		ok := f()
		e.mu.Unlock()
		if ok {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func setup(t *testing.T) (*Port, *events, string) { return setupAsking(t, nil) }

// setupAsking starts a holder over the fake CLI and a port reading it, with ask answering its tool asks.
func setupAsking(t *testing.T, ask func(context.Context, string, agent.Event) (agent.AskAnswer, bool)) (*Port, *events, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "cp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	d := claudecode.Driver{}
	init, secret := d.Init()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go holder.Run(ctx, holder.Config{Argv: []string{os.Args[0]}, Env: append(os.Environ(), "CHATPORT_FAKE_CLI=1"), Socket: sock,
		Init: [][]byte{init}, Secret: secret, Linger: time.Second})
	p, ev := reading(t, ctx, sock, ask)
	if !ev.has(func() bool { return p.Ready("a") }) {
		t.Fatal("not ready")
	}
	return p, ev, sock
}

// reading is a port that reads the holder at sock as agent a, as a daemon does.
func reading(t *testing.T, ctx context.Context, sock string, ask func(context.Context, string, agent.Event) (agent.AskAnswer, bool)) (*Port, *events) {
	t.Helper()
	d := claudecode.Driver{}
	ev := &events{}
	p := &Port{Chat: func(string) agent.Chat { return d }, Socket: func(string) string { return sock }, Ask: ask,
		OnDelivered: func(_, op string) { ev.mu.Lock(); ev.delivered = append(ev.delivered, op); ev.mu.Unlock() },
		OnFailed: func(_, op, why string, _ bool) {
			ev.mu.Lock()
			ev.failed = append(ev.failed, op+": "+why)
			ev.mu.Unlock()
		},
		OnTurnEnd: func(string, bool) { ev.mu.Lock(); ev.ends++; ev.mu.Unlock() },
		OnSession: func(_, id string) { ev.mu.Lock(); ev.session = id; ev.mu.Unlock() }}
	for i := 0; i < 300 && !p.Live("a"); i++ {
		p.Watch(ctx, "a")
		time.Sleep(10 * time.Millisecond)
	}
	return p, ev
}

func TestASentMessageIsConfirmedByItsUUIDAndTheSessionIsNamed(t *testing.T) {
	p, ev, _ := setup(t)
	r, err := p.Send("a", "op1", "hello", delivery.Now)
	if err != nil || r.State != delivery.Sent || r.UUID != delivery.UUIDFor("op1") {
		t.Fatal(r, err)
	}
	if !ev.has(func() bool { return len(ev.delivered) == 1 && ev.ends == 1 && ev.session == "sess-1" }) {
		t.Fatalf("%+v", ev)
	}
	if again, _ := p.Send("a", "op1", "hello", delivery.Now); again.UUID != r.UUID {
		t.Fatal("a repeat wrote again", again)
	}
	time.Sleep(50 * time.Millisecond)
	if len(ev.delivered) != 1 {
		t.Fatal("confirmed twice", ev.delivered)
	}
}

func TestQueueWaitsForTheTurnEndAndNowJoinsIt(t *testing.T) {
	p, ev, _ := setup(t)
	if _, err := p.Send("a", "long", "long", delivery.Now); err != nil {
		t.Fatal(err)
	}
	if !ev.has(func() bool { return len(ev.delivered) == 1 }) {
		t.Fatal("long not taken")
	}
	q, _ := p.Send("a", "q", "queued words", delivery.Queue)
	if q.State != delivery.Queued {
		t.Fatal(q)
	}
	w, _ := p.Send("a", "w", "withdrawn words", delivery.Queue)
	if r, ok, _ := p.Withdraw("a", "w"); !ok || r.State != delivery.Withdrawn || w.State != delivery.Queued {
		t.Fatal("not withdrawn", r)
	}
	if _, err := p.Send("a", "now", "steer", delivery.Now); err != nil { // joins the turn, which then ends
		t.Fatal(err)
	}
	if !ev.has(func() bool { return strings.Join(ev.delivered, ",") == "long,now,q" }) {
		t.Fatalf("%+v", ev.delivered)
	}
	if ev.has(func() bool { return len(ev.delivered) > 3 }) || len(ev.failed) > 0 {
		t.Fatalf("the withdrawn one went: %+v %v", ev.delivered, ev.failed)
	}
}

func TestPromoteWritesAHeldMessageNow(t *testing.T) {
	p, ev, _ := setup(t)
	_, _ = p.Send("a", "long", "long", delivery.Now)
	_ = ev.has(func() bool { return len(ev.delivered) == 1 })
	_, _ = p.Send("a", "q", "x", delivery.Queue)
	if r, err := p.Promote("a", "q"); err != nil || r.State != delivery.Sent {
		t.Fatal(r, err)
	}
	if !ev.has(func() bool { return len(ev.delivered) == 2 && ev.delivered[1] == "q" }) {
		t.Fatal(ev.delivered)
	}
}

func TestAToolAskWaitsForAPersonAndTheAnswerGoesOnTheStream(t *testing.T) {
	answer := make(chan agent.AskAnswer)
	asked := make(chan agent.Event, 1)
	p, ev, _ := setupAsking(t, func(ctx context.Context, _ string, ask agent.Event) (agent.AskAnswer, bool) {
		asked <- ask
		select {
		case a := <-answer:
			return a, true
		case <-ctx.Done():
			return agent.AskAnswer{}, false
		}
	})
	_, _ = p.Send("a", "ask", "ask", delivery.Now)
	select {
	case ask := <-asked:
		if ask.Request != "ask1" || ask.Tool != "Bash" || ask.Input["command"] != "ls" {
			t.Fatalf("%+v", ask)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("not asked")
	}
	if ev.has(func() bool { return ev.ends > 0 }) {
		t.Fatal("the turn went on before anyone answered")
	}
	answer <- agent.AskAnswer{Allow: true}
	if !ev.has(func() bool { return ev.ends == 1 }) {
		t.Fatal("the answer did not reach the session")
	}
}

func TestAnAskNobodyAnsweredIsReadAgainByTheNextReader(t *testing.T) {
	first := make(chan context.Context, 1)
	p, ev, sock := setupAsking(t, func(ctx context.Context, _ string, _ agent.Event) (agent.AskAnswer, bool) {
		first <- ctx
		<-ctx.Done()
		return agent.AskAnswer{}, false
	})
	_, _ = p.Send("a", "ask", "ask", delivery.Now)
	_ = ev.has(func() bool { return len(ev.delivered) == 1 })
	var gone context.Context
	select {
	case gone = <-first:
	case <-time.After(3 * time.Second):
		t.Fatal("not asked")
	}
	// The records after the ask are read, and acknowledged up to it, before the next reader comes.
	if !ev.has(func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		s := p.state("a")
		ask := s.asks["ask1"]
		return ask != nil && s.ackedFor > ask.seq
	}) {
		t.Fatal("nothing after the ask was read")
	}
	again := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	reading(t, ctx, sock, func(_ context.Context, _ string, ask agent.Event) (agent.AskAnswer, bool) {
		again <- ask.Request
		return agent.AskAnswer{Message: "no"}, true
	})
	select {
	case id := <-again:
		if id != "ask1" {
			t.Fatal(id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the next reader was not asked")
	}
	select {
	case <-gone.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the reader that went still waits on its ask")
	}
}

func TestAnAskTheCLITakesBackIsNotAnswered(t *testing.T) {
	d := claudecode.Driver{}
	var mu sync.Mutex
	var wrote []string
	waiting := make(chan context.Context, 1)
	p := &Port{Chat: func(string) agent.Chat { return d }, Socket: func(string) string { return "" },
		Write: func(_ string, line []byte) error {
			mu.Lock()
			wrote = append(wrote, string(line))
			mu.Unlock()
			return nil
		},
		Ask: func(ctx context.Context, _ string, _ agent.Event) (agent.AskAnswer, bool) {
			waiting <- ctx
			<-ctx.Done()
			return agent.AskAnswer{Allow: true}, true
		}}
	p.handle("a", d, holder.Record{Seq: 4, Line: []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash"}}`)})
	p.handle("a", d, holder.Record{Seq: 5, Line: []byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash"}}`)})
	ctx := <-waiting
	if got := p.ackable("a", 9); got != 3 {
		t.Fatal("acknowledged past the waiting ask:", got)
	}
	p.handle("a", d, holder.Record{Seq: 6, Line: []byte(`{"type":"control_cancel_request","request_id":"r1"}`)})
	<-ctx.Done()
	for i := 0; i < 100 && p.ackable("a", 9) != 9; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(wrote) != 0 || len(waiting) != 0 || p.ackable("a", 9) != 9 {
		t.Fatal("answered a taken back ask, or asked twice:", wrote, len(waiting))
	}
}

func TestInterruptEndsTheTurnAndAClosedSessionFailsWhatItDidNotTake(t *testing.T) {
	p, ev, _ := setup(t)
	_, _ = p.Send("a", "long", "long", delivery.Now)
	_ = ev.has(func() bool { return len(ev.delivered) == 1 })
	if err := p.Interrupt("a"); err != nil {
		t.Fatal(err)
	}
	if !ev.has(func() bool { return ev.ends == 1 }) {
		t.Fatal("not interrupted")
	}
	p.mu.Lock()
	p.state("a").ops["dangling"] = "lost-op"
	p.mu.Unlock()
	if err := p.Close("a"); err != nil {
		t.Fatal(err)
	}
	if !ev.has(func() bool { return !p.Live("a") && len(ev.failed) == 1 }) {
		t.Fatalf("%v", ev.failed)
	}
	if _, err := p.Send("a", "late", "x", delivery.Now); err == nil {
		t.Fatal("sent to a closed session")
	}
}

// The turn, the background work and the usage limit are what the stream last said, and the background work stops on it.
func TestTheStreamSaysTheTurnTheBackgroundWorkAndTheLimit(t *testing.T) {
	p, ev, _ := setup(t)
	if _, known := p.Idle("a"); known {
		t.Fatal("known before the stream said")
	}
	if n, _ := p.StopBackground("a", []string{"b9"}); n != 1 {
		t.Fatal("the hooks' pieces are asked while the stream has listed none")
	}
	_, known := p.Background("nobody")
	p.mu.Lock()
	kept := p.agents["nobody"] != nil
	p.mu.Unlock()
	if known || kept {
		t.Fatal("a read kept a state for an agent never seen")
	}
	_, _ = p.Send("a", "bg", "bg", delivery.Now)
	if !ev.has(func() bool { idle, known := p.Idle("a"); return idle && known }) {
		t.Fatal("not idle")
	}
	tasks, known := p.Background("a")
	if !known || len(tasks) != 2 || tasks[1].Description != "sleep 300" {
		t.Fatal(tasks, known)
	}
	if l := p.Limit("a"); !l.Blocked || l.Window != "five_hour" || l.ResetsAt != 99 {
		t.Fatal(l)
	}
	if n, err := p.StopBackground("a", []string{"from-hooks"}); n != 2 || err != nil {
		t.Fatal(n, err)
	}
	if !ev.has(func() bool { tasks, _ := p.Background("a"); return len(tasks) == 0 }) {
		t.Fatal("still running")
	}
}

func TestANewerAskOrATurnsEndEndsAStaleAskAndAReplacedWaitLeavesTheNewEntry(t *testing.T) {
	d := claudecode.Driver{}
	waits := make(chan context.Context, 4)
	p := &Port{Chat: func(string) agent.Chat { return d }, Socket: func(string) string { return "" },
		Write: func(string, []byte) error { return nil },
		Ask: func(ctx context.Context, _ string, _ agent.Event) (agent.AskAnswer, bool) {
			waits <- ctx
			<-ctx.Done()
			return agent.AskAnswer{}, false
		}}
	ended := make(chan string, 8)
	p.waited = func(id string) { ended <- id }
	askLine := func(id string) holder.Record {
		return holder.Record{Seq: 1, Line: []byte(`{"type":"control_request","request_id":"` + id + `","request":{"subtype":"can_use_tool","tool_name":"Bash"}}`)}
	}
	done := func(ctx context.Context, what string) {
		select {
		case <-ctx.Done():
		case <-time.After(3 * time.Second):
			t.Fatal(what)
		}
	}
	p.handle("a", d, askLine("r1"))
	first := <-waits
	p.handle("a", d, askLine("r2"))
	second := <-waits
	done(first, "a newer ask left the older one waiting")
	p.handle("a", d, holder.Record{Seq: 2, Line: []byte(`{"type":"result","subtype":"success"}`)})
	done(second, "a turn's end left an ask waiting")

	// A reader that went: its wait ends while the next reader's wait for the same request has begun.
	p.handle("a", d, askLine("r3"))
	old := <-waits
	p.mu.Lock()
	s := p.state("a")
	gone := s.asks
	s.asks = map[string]*pendingAsk{}
	p.mu.Unlock()
	p.handle("a", d, askLine("r3"))
	<-waits
	gone["r3"].cancel()
	done(old, "not cancelled")
	for r3 := false; !r3; {
		select {
		case id := <-ended:
			r3 = id == "r3"
		case <-time.After(3 * time.Second):
			t.Fatal("the old wait never ended")
		}
	}
	p.mu.Lock()
	_, kept := s.asks["r3"]
	p.mu.Unlock()
	if !kept {
		t.Fatal("the old wait took the new reader's entry")
	}
}
