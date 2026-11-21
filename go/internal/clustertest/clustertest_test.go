package clustertest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/protected"
)

func TestMain(m *testing.M) {
	code := m.Run()
	Remove()
	os.Exit(code)
}

// A master and a runner start joined: the master lists the runner on Run under the master's founder; a runner killed and started
// again on its home is listed again, its board set aside once; and nothing is left running when the test ends.
func TestAMasterAndARunnerStartJoined(t *testing.T) {
	c := Start(t, Opts{})
	r := c.Runners[0]
	node := joined.NodeIDOf(r.Home)
	got, err := c.Master.Call("node.status", nil)
	if err != nil {
		t.Fatal(err)
	}
	nodes := got["nodes"].([]any)
	var row map[string]any
	for _, v := range nodes {
		if r := v.(map[string]any); r["id"] == node {
			row = r
		}
	}
	if len(nodes) != 2 || row["lending"] != "run" || row["person"] == nil {
		t.Fatalf("listed: %v", nodes)
	}
	if _, err := os.Stat(protected.RootUnder(filepath.Join(filepath.Dir(r.Home), "user"), r.Home)); err != nil {
		t.Fatalf("the runner's protected store is not under the cluster's root: %v", err)
	}
	if err := r.Kill(); err != nil || r.Running() {
		t.Fatalf("kill: %v", err)
	}
	if err := r.Restart(); err != nil || !c.Master.Lists(node) {
		t.Fatalf("restart: %v", err)
	}
	if aside, _ := filepath.Glob(filepath.Join(r.Home, "db.set-aside-*.sqlite")); len(aside) != 1 {
		t.Fatalf("set aside: %v", aside)
	}
}

// Two runners both join (slice 18 needs two).
func TestTwoRunnersBothJoin(t *testing.T) {
	c := Start(t, Opts{Runners: 2})
	for _, r := range c.Runners {
		if !c.Master.Lists(joined.NodeIDOf(r.Home)) {
			t.Fatalf("%s is not listed", r.Name)
		}
	}
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	for _, n := range c.Nodes() {
		if n.Running() {
			t.Fatalf("%s still runs after Stop", n.Name)
		}
	}
}
