package screenpipe

import (
	"fmt"
	"testing"
)

// screen.status reads who holds and who watches the agent's page, and the machine it runs on, while acting on nothing: a person holding it
// does not refuse it, watchers are sent no action, and the page is neither opened nor in use by it.
func TestStatusReadsTheHolderAndTheWatchersAndActsOnNothing(t *testing.T) {
	w := newWorld(t)
	a := dial(t, w.agent, "")
	got := result(t, a.call("screen.status", `"source":"browser"`))
	if h := got["holder"].(map[string]any); h["kind"] != "agent" || len(got["watchers"].([]any)) != 0 || got["machine"] != nil {
		t.Fatalf("nobody watching: %v", got)
	}
	if s := w.hub.Sources("a1"); len(s) != 0 {
		t.Fatalf("the read opened the page: %v", s)
	}
	c := dial(t, w.founder, "founder")
	result(t, c.call("screen.attach", page))
	name := result(t, c.call("screen.take", page+req()))["holder"].(map[string]any)["name"].(string)
	before := fmt.Sprint(w.hub.Sources("a1"))
	got = result(t, a.call("screen.status", `"source":"browser"`))
	if h := got["holder"].(map[string]any); h["kind"] != "person" || h["name"] != name {
		t.Errorf("held: %v", got)
	}
	second := dial(t, w.founder, "founder") // the same person on a second connection is named once
	result(t, second.call("screen.attach", page))
	got = result(t, a.call("screen.status", `"source":"browser"`))
	if ws := got["watchers"].([]any); len(ws) != 1 || ws[0] != name {
		t.Errorf("watched: %v", got["watchers"])
	}
	if n := c.count("screen_action"); n != 0 {
		t.Errorf("watchers were shown %d actions", n)
	}
	if s := fmt.Sprint(w.hub.Sources("a1")); s != before {
		t.Errorf("the read opened or used the page: %v, before %v", s, before)
	}
	w.hub.mu.Lock()
	_, used := w.hub.used[key{"a1", "browser"}]
	w.hub.mu.Unlock()
	if used {
		t.Error("the read marked the page in use")
	}
	w.hub.Node = func(string) string { return "n1" }
	w.hub.NodeName = func(string) string { return "Mac mini" }
	if got := result(t, a.call("screen.status", `"source":"browser"`)); got["machine"] != "Mac mini" {
		t.Errorf("on a runner: %v", got["machine"])
	}
	if got := refusal(c.call("screen.status", `"source":"browser"`)); got != "this is an agent's own call, on its own source" {
		t.Errorf("a person read the agent's own status: %q", got)
	}
}
