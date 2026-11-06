package compose

import (
	"os"
	"path/filepath"
	"testing"

	cli "github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
)

type fakeAgent struct{ claudecode.Driver }

func (fakeAgent) Name() string { return "fakevendor" }

func TestLaunchVendorsChooseTheAgentByVendorName(t *testing.T) {
	cli.RegisterAgent("fakevendor", func(cli.Host) cli.Agent { return fakeAgent{} })
	home := t.TempDir()
	for agent, vendor := range map[string]string{"a.fake": "fakevendor", "a.none": "nosuch", "a.claude": "claude"} {
		dir := paths.AgentDir(home, agent)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte("name: "+agent+"\nvendor: "+vendor+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v := launchVendors{vendors{home: home, roster: team.NewRoster(home)}}
	for agent, want := range map[string]string{"a.fake": "fakevendor", "a.claude": "claude"} {
		if drv, err := v.ForAgent(agent); err != nil || drv.Name() != want {
			t.Errorf("%s: %v %v", agent, drv, err)
		}
	}
	if _, err := v.ForAgent("a.none"); err == nil {
		t.Error("a vendor with no Agent must be refused")
	}
}
