package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/clustertest"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
)

// A master and a runner, each with its own home: the runner links itself to the master with its device token; the master opens a
// terminal on it and reads its screen; hl run in that terminal reaches the master as the agent the terminal was opened for, and the
// same hl from a process the runner did not open is refused.
func TestTheMasterDrivesARunnerAndItsAgentsCallsReachTheMasterAsThem(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	t.Setenv("HANDLINE_AGENT", "")
	t.Setenv("HANDLINE_TMUX_SOCKET", "")
	hl := filepath.Join(clustertest.Build(t), "hl")
	master := startNet(t)
	teamtest.Seed(t, master.cfg.Home, "bob")

	home, err := os.MkdirTemp("", "hlr") // short: a socket path holds 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	node, err := joined.NodeID(home, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, uri := master.dev.Pair(time.Minute)
	m, err := hlclient.Join(home, uri, "box", "server", node)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { joined.Forget(home) })
	tm := agenthost.Tmux(home).(runner.Terminals)
	t.Cleanup(func() { exec.Command("tmux", "-L", paths.TmuxSocket(home), "kill-server").Run() })
	r, err := runner.Start(runner.Config{Home: home, Socket: paths.SocketFile(home), Master: m, Term: tm, Log: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)

	var rn *nodes.Runner
	for end := time.Now().Add(10 * time.Second); rn == nil; time.Sleep(50 * time.Millisecond) {
		if rn, _ = master.nodes.Runner(node); rn == nil && time.Now().After(end) {
			t.Fatal("the runner never linked to the master")
		}
	}
	handle, err := rn.Open("bob", home, "cd "+home)
	if err != nil {
		t.Fatal(err)
	}
	screen := func(want string) []string {
		t.Helper()
		var rows []string
		for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
			if rows, err = rn.Screen(handle, 40); err == nil && strings.Contains(strings.Join(rows, "\n"), want) {
				return rows
			}
		}
		t.Fatalf("the screen never showed %q: %q %v", want, rows, err)
		return nil
	}
	if err := rn.Keys(handle, "echo hi-from-$((40+2))", true); err != nil {
		t.Fatal(err)
	}
	t.Logf("runner screen: %q", screen("hi-from-42"))

	if err := rn.Keys(handle, hl+" say human ACK-RUNNER; echo said-$?-$((1+1))", true); err != nil {
		t.Fatal(err)
	}
	t.Logf("runner screen: %q", screen("said-0-2"))
	bob := teamtest.Key(t, master.cfg.Home, "bob")
	var who string
	if err := master.db.QueryRow("SELECT requester FROM requests WHERE body = 'ACK-RUNNER'").Scan(&who); err != nil || who != bob {
		t.Fatalf("the say reached the master as %q (%v), want %s", who, err, bob)
	}

	cmd := exec.Command(hl, "say", "human", "NOT-AN-AGENT")
	cmd.Env = append(os.Environ(), "HANDLINE_HOME="+home, "HANDLINE_AGENT=bob") // it claims to be bob; the runner did not open it
	out, err := cmd.CombinedOutput()
	var n int
	master.db.QueryRow("SELECT COUNT(*) FROM requests WHERE body = 'NOT-AN-AGENT'").Scan(&n)
	if err == nil || n != 0 || !strings.Contains(string(out), "runner") {
		t.Fatalf("a process the runner did not open was forwarded: %v %d %s", err, n, out)
	}
	t.Logf("hl from an unlaunched process: %s", strings.TrimSpace(string(out)))

	if err := rn.Stop("bob"); err != nil {
		t.Fatal(err)
	}
	var left int
	master.db.QueryRow("SELECT COUNT(*) FROM node_sessions").Scan(&left)
	if left != 0 {
		t.Fatal("the stopped session is still kept")
	}
}

// node.forward is a runner's link's call only, and only for an agent whose session the master opened on that runner.
func TestNodeForwardIsOnlyARunnersLinksForItsOwnAgents(t *testing.T) {
	d := startNet(t)
	token, _ := pair(t, d, "phone")
	phone := helloAs(t, d, token)
	forward := `{"jsonrpc":"2.0","id":7,"method":"node.forward","params":{"agent":"bob","method":"whoami","params":{}}}`
	if r := phone.ask(forward); gate(r) != "node" {
		t.Fatalf("a phone forwarded: %v", r)
	}
	code, _, _, _ := d.dev.Pair(time.Minute)
	box := joinAs(t, d, code, "box", "server", "n-box")["result"].(map[string]any)
	if r := helloAs(t, d, box["token"].(string)).ask(forward); gate(r) != "node" {
		t.Fatalf("the runner's token on a connection that is not its link forwarded: %v", r)
	}
	link := dialNet(t, d)
	link.ask(`{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"` + runner.Client + `","protocol":1,"token":"` + box["token"].(string) + `"}}`)
	if r := link.ask(forward); gate(r) != "not_on_node" {
		t.Fatalf("a runner forwarded for an agent it does not run: %v", r)
	}
	d.db.Exec("INSERT INTO node_sessions(agent, node, handle, opened_at) VALUES('bob', 'n-box', '%1', 0)")
	r := link.ask(forward)
	res, _ := r["result"].(map[string]any)
	if caller, _ := res["caller"].(map[string]any); caller["kind"] != "agent" || caller["key"] != "bob" {
		t.Fatalf("the forwarded whoami: %v", r)
	}
	if r := link.ask(`{"jsonrpc":"2.0","id":8,"method":"node.forward","params":{"agent":"bob","method":"hello","params":{}}}`); gate(r) != "method" {
		t.Fatalf("hello was forwarded: %v", r)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	clustertest.Remove() // the handlined and hl the runner test built
	os.Exit(code)
}
