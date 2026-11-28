package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/sessionrpc"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// With restart_reclaimed on, an agent the reclaim took back that holds a task, or has requests waiting, is started again; off, or with
// nothing to do, it stays closed as before.
func TestAReclaimedAgentWithWorkStartsAgainOnlyWithItsSwitchOn(t *testing.T) {
	p, lc, _, home := wired(t)
	alice := teamtest.Key(t, home, "alice")
	reg := nodes.New(p.db, home)
	var started []string
	start := func(agent, why string) (bool, string, error) {
		started = append(started, agent+": "+why)
		return true, "started on here", nil
	}
	got := []nodes.Reclaimed{{Agent: alice, Node: "n1", Task: 7}}
	p.reclaimed(reg, got, start, t.Logf)
	if len(started) != 0 {
		t.Fatalf("off: started %v", started)
	}
	if err := lc.roster.SetKeys(alice, []yamlcompat.Pair{{Key: "restart_reclaimed", Val: true}}); err != nil {
		t.Fatal(err)
	}
	p.reclaimed(reg, []nodes.Reclaimed{{Agent: alice, Node: "n1"}}, start, t.Logf)
	if len(started) != 0 {
		t.Fatalf("on, with no task and nothing waiting: started %v", started)
	}
	p.reclaimed(reg, got, start, t.Logf)
	if len(started) != 1 || started[0] != alice+": it is offline" {
		t.Fatalf("on, with a task: started %v", started)
	}
}

// agent.restart_here is refused for an agent whose session is on no other machine, and while that machine is not silent.
func TestRestartHereIsRefusedUnlessItsMachineIsSilent(t *testing.T) {
	p, lc, _, home := wired(t)
	alice := teamtest.Key(t, home, "alice")
	reg := nodes.New(p.db, home)
	reg.Self = "m0"
	c := controls{p: p, lc: lc, nodes: reg}
	if ok, said, _ := c.restartHere(sessionrpc.Call{By: "p1"}, alice); ok || said != "its session is not on another machine" {
		t.Fatalf("on this machine: %v %q", ok, said)
	}
	db.SetTerminal(p.db, alice, "m0:%3", "")
	if ok, said, _ := c.restartHere(sessionrpc.Call{By: "p1"}, alice); ok || !strings.Contains(said, "online and linked") {
		t.Fatalf("not silent: %v %q", ok, said)
	}
	if v := p.heldOn(reg, alice).(map[string]any); v["restart_here"] != false || v["silent_since"] != nil {
		t.Fatalf("the view of a machine not silent: %v", v)
	}
	db.SetTerminal(p.db, alice, "n1:%3", "")
	if v := p.heldOn(reg, alice).(map[string]any); v["restart_here"] != false {
		t.Fatalf("silent a moment: not open yet: %v", v)
	}
	reg.SetClock(func() time.Time { return time.Now().Add(nodes.LeaveAfter) })
	if v := p.heldOn(reg, alice).(map[string]any); v["restart_here"] != true || v["silent_since"] == nil || v["node"] != "n1" {
		t.Fatalf("the view of a silent machine: %v", v)
	}
}
