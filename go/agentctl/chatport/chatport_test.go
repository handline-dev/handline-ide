package chatport

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/holder"
)

// fakeCLI speaks Claude's stream: it answers initialize with an account, and runs each user line as a turn: queued, started, the echo,
// then (unless the text says "ask") a result. "ask" makes it ask for a tool first and wait for the answer; "long" keeps the turn open
// until the next line comes; "bg" starts two pieces of background work and meets the usage limit, and stop_task stops one. A line in the
// middle of a turn joins it.
func fakeCLI() {
	out := bufio.NewWriter(os.Stdout)
	emit := func(v map[string]any) {
		b, _ := json.Marshal(v)
		out.Write(append(b, '\n'))
		out.Flush()
	}
	sc := bufio.NewScanner(os.Stdin)
	open := false
	var cmds []string
	tasks := []map[string]any{}
	// end closes the turn as the CLI can: its result, then each of its commands completed.
	end := func(v map[string]any) {
		emit(v)
		for _, u := range cmds {
			emit(map[string]any{"type": "command_lifecycle", "command_uuid": u, "state": "completed"})
		}
		cmds = nil
	}
	for sc.Scan() {
		var m map[string]any
		_ = json.Unmarshal(sc.Bytes(), &m)
		switch m["type"] {
		case "control_request":
			req, _ := m["request"].(map[string]any)
			if req["subtype"] == "initialize" {
				emit(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": m["request_id"],
					"response": map[string]any{"account": map[string]any{"email": "someone@example.com"}}}})
				emit(map[string]any{"type": "system", "subtype": "init", "session_id": "sess-1"})
			}
			if req["subtype"] == "stop_task" {
				kept := []map[string]any{}
				for _, t := range tasks {
					if t["task_id"] != req["task_id"] {
						kept = append(kept, t)
					}
				}
				tasks = kept
				emit(map[string]any{"type": "system", "subtype": "background_tasks_changed", "tasks": tasks})
			}
			if req["subtype"] == "interrupt" && open {
				open = false
				end(map[string]any{"type": "result", "subtype": "error_during_execution", "is_error": true})
			}
		case "control_response":
			r, _ := m["response"].(map[string]any)
			inner, _ := r["response"].(map[string]any)
			emit(map[string]any{"type": "assistant", "said": inner["behavior"]})
			open = false
			end(map[string]any{"type": "result", "subtype": "success"})
		case "user":
			u, _ := m["uuid"].(string)
			text, _ := m["message"].(map[string]any)["content"].(string)
			emit(map[string]any{"type": "command_lifecycle", "command_uuid": u, "state": "queued"})
			emit(map[string]any{"type": "command_lifecycle", "command_uuid": u, "state": "started"})
			emit(map[string]any{"type": "user", "uuid": u, "isReplay": true})
			cmds = append(cmds, u)
			switch {
			case text == "ask":
				open = true
				emit(map[string]any{"type": "control_request", "request_id": "ask1", "request": map[string]any{"subtype": "can_use_tool", "tool_name": "Bash"}})
			case text == "long":
				open = true
			case text == "bg":
				emit(map[string]any{"type": "system", "subtype": "session_state_changed", "state": "running"})
				tasks = []map[string]any{{"task_id": "b1", "description": "sleep 8"}, {"task_id": "b2", "description": "sleep 300"}}
				emit(map[string]any{"type": "system", "subtype": "background_tasks_changed", "tasks": tasks})
				emit(map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{"status": "rejected", "rateLimitType": "five_hour", "resetsAt": 99}})
				end(map[string]any{"type": "result", "subtype": "success", "is_error": true, "api_error_status": 429})
				emit(map[string]any{"type": "system", "subtype": "session_state_changed", "state": "idle"})
			case open: // joined the running turn, which ends now
				open = false
				end(map[string]any{"type": "result", "subtype": "success"})
			default:
				end(map[string]any{"type": "result", "subtype": "success"})
			}
		}
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("CHATPORT_FAKE_CLI") == "1" {
		fakeCLI()
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

func setup(t *testing.T) (*Port, *events, string) {
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
	ev := &events{}
	p := &Port{Chat: func(string) agent.Chat { return d }, Socket: func(string) string { return sock },
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
	if !ev.has(func() bool { return p.Ready("a") }) {
		t.Fatal("not ready")
	}
	return p, ev, sock
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

func TestAToolAskIsRefusedAtOnceSoTheTurnGoesOn(t *testing.T) {
	p, ev, _ := setup(t)
	_, _ = p.Send("a", "ask", "ask", delivery.Now)
	if !ev.has(func() bool { return ev.ends == 1 }) {
		t.Fatal("the turn waited on the ask")
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
