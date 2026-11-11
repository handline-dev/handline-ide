package compose

import (
	"testing"

	cli "github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/data/team/teamtest"
)

// namedPort answers its name as every receipt's op, so a test sees which port a call went to.
type namedPort struct{ name string }

func (n namedPort) Structured(string) bool { return n.name != "none" }
func (n namedPort) Live(string) bool       { return true }
func (n namedPort) Send(_, _, _ string, _ delivery.Delivery) (delivery.Receipt, error) {
	return delivery.Receipt{Op: n.name}, nil
}
func (n namedPort) Withdraw(string, string) (delivery.Receipt, bool, error) {
	return delivery.Receipt{Op: n.name}, true, nil
}
func (n namedPort) Promote(string, string) (delivery.Receipt, error) {
	return delivery.Receipt{Op: n.name}, nil
}

type portAgent struct{ fakeAgent }

func (portAgent) Name() string                    { return "portvendor" }
func (portAgent) Deliver(cli.DeliverEnv) cli.Port { return namedPort{"portvendor"} }

func writeVendor(t *testing.T, home, agent, vendor string) {
	t.Helper()
	teamtest.Set(t, home, agent, "vendor", vendor)
}

func TestADeliveryGoesThroughThePortOfTheAgentsVendor(t *testing.T) {
	cli.Register(portAgent{})
	cli.RegisterAgent("portvendor", func(cli.Host) cli.Agent { return portAgent{} })
	home := t.TempDir()
	writeVendor(t, home, "a.other", "portvendor")
	writeVendor(t, home, "a.claude", "claude")
	writeVendor(t, home, "a.unknown", "nosuch")
	p := &Parts{home: home, Peer: func(string) bool { return false }}
	v, ok := p.agentsPort().(vendorAgents)
	if !ok {
		t.Fatalf("%T", p.agentsPort())
	}
	if _, ok := v.others["portvendor"]; !ok || v.others["claude"] != nil {
		t.Fatalf("a port per launchable vendor but Claude Code: %v", v.others)
	}
	v.claude = namedPort{"claude"}
	for agent, want := range map[string]string{"a.other": "portvendor", "a.claude": "claude", "a.unknown": "claude"} {
		r, err := v.Send(agent, "op", "x", delivery.Queue)
		w, _, _ := v.Withdraw(agent, "op")
		pr, _ := v.Promote(agent, "op")
		if err != nil || r.Op != want || w.Op != want || pr.Op != want || !v.Structured(agent) || !v.Live(agent) {
			t.Errorf("%s went to %q %q %q, want %s", agent, r.Op, w.Op, pr.Op, want)
		}
	}
}
