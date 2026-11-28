package daemon

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/chatport"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/holder"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/paths"
)

// chatFakeEnv makes this test binary a Chat session's CLI (package chatfake).
const chatFakeEnv = "HANDLINE_CHAT_FAKE_CLI"

func waitTrue(t *testing.T, what string, within time.Duration, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(within); !ok(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal(what)
		}
	}
}

// A Chat agent launched on a runner is read and written from the master as one here: hl launch there runs its CLI under a holder, the
// master serves that holder at the agent's own socket, a message is confirmed by its uuid, a tool ask is answered from the master, and
// the session's terminal takes no key. The runner going away in the middle of a turn loses nothing: the holder stays up here, a message
// sent meanwhile is refused unwritten, and once the runner is back the turn goes on with no record read twice.
func TestAChatAgentOnARunnerIsReachedFromTheMasterAcrossAReconnect(t *testing.T) {
	var rc runner.Config
	master, node, rhome, hl, r := masterWithRunnerAs(t, nil, func(c *runner.Config) { rc = *c })
	mhome := master.cfg.Home
	bob := teamtest.Key(t, mhome, "bob")
	d := claudecode.Driver{}
	first, secret := d.Init()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	master.nodes.Launch = func(_, agent string, _ map[string]any) (*nodes.Launched, error) {
		return &nodes.Launched{Argv: []string{exe}, Env: []string{chatFakeEnv + "=1"},
			Hold: &nodes.Hold{Socket: paths.ChatSocket(mhome, agent), Init: []string{string(first)}, Secret: secret}}, nil
	}
	rn, _ := master.nodes.Runner(node)
	handle, err := rn.Open("bob", rhome, hl+" launch bob")
	if err != nil {
		t.Fatal(err)
	}
	waitTrue(t, "no holder ran on the runner", 20*time.Second, func() bool { return holder.Alive(paths.ChatSocket(rhome, "bob")) })
	here := paths.ChatSocket(mhome, bob)
	waitTrue(t, "the master serves no holder for bob", 10*time.Second, func() bool { return holder.Alive(here) })

	var mu sync.Mutex
	var delivered, failed, asked []string
	ends := 0
	p := &chatport.Port{Chat: func(string) agent.Chat { return d }, Socket: func(string) string { return here },
		Ask: func(_ context.Context, _ string, ev agent.Event) (agent.AskAnswer, bool) {
			mu.Lock()
			asked = append(asked, ev.Tool)
			mu.Unlock()
			return agent.AskAnswer{Allow: true}, true
		},
		OnDelivered: func(_, op string) { mu.Lock(); delivered = append(delivered, op); mu.Unlock() },
		OnFailed:    func(_, op, why string, _ bool) { mu.Lock(); failed = append(failed, op+": "+why); mu.Unlock() },
		OnTurnEnd:   func(string, bool) { mu.Lock(); ends++; mu.Unlock() }}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	waitTrue(t, "the master's port never read bob's session", 10*time.Second, func() bool {
		p.Watch(ctx, bob)
		return p.Live(bob) && p.Ready(bob)
	})
	seen := func(what string, want func() bool) {
		t.Helper()
		waitTrue(t, what, 10*time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return want() })
	}
	if _, err := p.Send(bob, "hello", "hello", delivery.Now); err != nil {
		t.Fatal(err)
	}
	seen("hello was not confirmed", func() bool { return slices.Equal(delivered, []string{"hello"}) && ends == 1 })
	if _, err := p.Send(bob, "ask", "ask", delivery.Now); err != nil {
		t.Fatal(err)
	}
	seen("the ask was not answered from the master", func() bool { return len(asked) == 1 && asked[0] == "Bash" && ends == 2 })
	if err := rn.Keys(handle, "typed", true); err == nil || !strings.Contains(err.Error(), "Chat mode") {
		t.Fatalf("a key reached a Chat session's terminal: %v", err)
	}

	if _, err := p.Send(bob, "long", "long", delivery.Now); err != nil {
		t.Fatal(err)
	}
	seen("the long turn was not confirmed", func() bool { return len(delivered) == 3 })
	r.Close() // the link ends in the middle of the turn; the session runs on
	waitTrue(t, "the runner's link did not end", 10*time.Second, func() bool { return !master.nodes.IsLinked(node) })
	if _, err := p.Send(bob, "lost", "joins", delivery.Now); err == nil {
		t.Fatal("a message was taken while the runner was away")
	}
	if !holder.Alive(here) || !p.Live(bob) {
		t.Fatal("the session read as gone while its runner was away")
	}
	r2, err := runner.Start(rc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r2.Close)
	linkedRunner(t, master, node)
	waitTrue(t, "the message was not taken once the runner was back", 15*time.Second, func() bool {
		_, err := p.Send(bob, "joins", "joins", delivery.Now)
		return err == nil
	})
	seen("the turn did not end once the runner was back", func() bool { return ends == 3 && len(delivered) == 4 })
	time.Sleep(time.Second) // anything read twice would come now
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(delivered, []string{"hello", "ask", "long", "joins"}) || ends != 3 || len(failed) != 0 {
		t.Fatalf("delivered %v, %d turns, failed %v", delivered, ends, failed)
	}
}
