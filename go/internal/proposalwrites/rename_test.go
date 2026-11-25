package proposalwrites

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team"
)

// The founder renames an idle agent at once: its key stays and the new word finds it, within its workspace for an agent and as the
// workspace's name for a lead alone in it. A lead with others, a word that is not a free name, a seat and an agent the daemon does not
// have are refused, and rename nothing; the name it has already answers at once, without touching a running session.
func TestTheFounderRenamesAnIdleAgentAndIsRefusedWhatTheRenameWouldRefuse(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := team.NewRoster(r.home).Register("alice.kid", team.RegisterOpts{Role: "r"}); err != nil {
		t.Fatal(err)
	}
	bob, kid := r.who("bob"), r.who("alice.kid")
	for i, c := range []struct{ params, want string }{
		{`"agent":"alice","to":"ann"`, "a rename would rename the workspace, which has 1 other agent(s)"},
		{`"agent":"` + kid + `","to":"bob"`, `"bob" is`},
		{`"agent":"` + kid + `","to":"bo.b"`, `"bo.b" is not a`},
		{`"agent":"nobody","to":"x"`, "no such agent"},
	} {
		if msg := refusal(t, r.call("agent.rename", fmt.Sprintf(`{%s,"request_id":"r%d"}`, c.params, i))); !strings.Contains(msg, c.want) {
			t.Errorf("%s: %s", c.params, msg)
		}
	}
	r.setLive("alice.kid", true)
	if m := r.call("agent.rename", `{"agent":"`+kid+`","to":"kid","request_id":"same"}`); m["result"] == nil || len(r.e.listing()) != 0 {
		t.Fatalf("its own name held a rename: %v %d", m, len(r.e.listing()))
	}
	r.setLive("alice.kid", false)
	if m := r.call("agent.rename", `{"agent":"`+kid+`","to":"kiddo","request_id":"k1"}`); m["result"] == nil {
		t.Fatalf("an agent in a workspace with others: %v", m)
	}
	if r.who("alice.kiddo") != kid {
		t.Fatalf("alice.kiddo is %q", r.who("alice.kiddo"))
	}
	m := r.call("agent.rename", `{"agent":"bob","to":"robert","request_id":"n5"}`)
	if res, _ := m["result"].(map[string]any); res == nil || res["said"] != "bob is now robert." {
		t.Fatalf("%v", m)
	}
	if r.who("robert") != bob || strings.Contains(r.rosterText(), "  bob: {}") {
		t.Fatalf("a lead alone renamed, same key: %s\n%s", r.who("robert"), r.rosterText())
	}
	seat, err := team.NewSeat(r.db, bob, 1)
	if err != nil {
		t.Fatal(err)
	}
	if msg := refusal(t, r.call("agent.rename", `{"agent":"`+seat+`","to":"lent","request_id":"n3"}`)); !strings.Contains(msg, "is a seat lent from") {
		t.Fatal(msg)
	}
}

// The founder's rename of an agent whose turn runs is applied now: the turn is interrupted, the session closes at its prompt, the rename
// runs and the agent starts under its new name; the call answers only then, with what was done.
func TestTheFoundersRenameOfARunningAgentInterruptsItsTurnAndAnswersOnceRenamed(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob := r.who("bob")
	r.setLive("bob", true)
	got := make(chan map[string]any, 1)
	go func() { got <- r.call("agent.rename", `{"agent":"bob","to":"robert","request_id":"h1"}`) }()
	for deadline := time.Now().Add(10 * time.Second); ; {
		if held := r.e.listing(); len(held) == 1 && held[0].str("state") == "applying" {
			break
		}
		select {
		case m := <-got:
			t.Fatalf("answered before the rename was held: %v", m)
		case <-time.After(10 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("the rename was never held")
		}
	}
	idle, interrupted, closed, started := false, []string{}, []string{}, []string{}
	ports := HeldPorts{Alive: func(a string) bool { return r.live[a] }, Idle: func(string) bool { return idle },
		Interrupt: func(a string) (bool, string) { interrupted = append(interrupted, a); return true, "" },
		Close:     func(a string) (bool, string) { closed = append(closed, a); r.setLive(a, false); return true, "" },
		Start:     func(a string) (bool, string) { started = append(started, a); return true, "" }}
	r.e.SweepHeld(ports)
	if !reflect.DeepEqual(interrupted, []string{bob}) || len(closed) != 0 {
		t.Fatalf("its turn is interrupted, not cut: %v %v", interrupted, closed)
	}
	idle = true
	r.e.SweepHeld(ports) // at its prompt: closed
	r.e.SweepHeld(ports) // applied
	if !reflect.DeepEqual(closed, []string{bob}) || !reflect.DeepEqual(started, []string{bob}) {
		t.Fatalf("closed, then started under its new name: %v %v", closed, started)
	}
	select {
	case m := <-got:
		if res, _ := m["result"].(map[string]any); res == nil || res["said"] != "bob is now robert." {
			t.Fatalf("the outcome, not an acceptance: %v", m)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the call never answered")
	}
	if r.who("robert") != bob {
		t.Fatalf("robert is %q", r.who("robert"))
	}
}
