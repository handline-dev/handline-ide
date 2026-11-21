package stream

import "testing"

// A personal topic's event of a person (its Agent a person key) goes to that person's subscriptions only, whatever agent they filter
// on; an event of no one goes to all, and a tasks event of a person to everyone.
func TestAPersonsEventGoesToThatPersonOnly(t *testing.T) {
	kim, owner, bob := "p2", "p1", "a3"
	ev := func(agent *string) *Event { return &Event{Topic: "for_you", Agent: agent} }
	subOf := func(person, agent string) *sub {
		return &sub{topics: map[string]bool{"for_you": true, "tasks": true}, person: person, agent: agent}
	}
	for _, c := range []struct {
		s    *sub
		e    *Event
		want bool
	}{
		{subOf("p2", ""), ev(&kim), true},
		{subOf("p1", ""), ev(&kim), false},
		{subOf("", ""), ev(&kim), false},
		{subOf("p2", "a3"), ev(&kim), true},
		{subOf("p1", ""), ev(&owner), true},
		{subOf("p1", ""), ev(nil), true},
		{subOf("p1", "a4"), ev(&bob), false},
		{subOf("p1", ""), &Event{Topic: "tasks", Agent: &kim}, true}, // a task waiting on Kim is still on everyone's board
	} {
		if got := c.s.wants(c.e); got != c.want {
			t.Errorf("person %q agent %q, event of %v: %v", c.s.person, c.s.agent, c.e.Agent, got)
		}
	}
}
