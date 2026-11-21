package daemon

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/clustertest"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/lifecycle"
)

// A master and a runner, each with its own home: the runner links itself to the master with its device token; the master opens a
// terminal on it and reads its screen; hl run in that terminal reaches the master as the agent the terminal was opened for, and the
// same hl from a process the runner did not open is refused.
func TestTheMasterDrivesARunnerAndItsAgentsCallsReachTheMasterAsThem(t *testing.T) {
	master, node, home, hl := masterAndRunner(t)
	rn, _ := master.nodes.Runner(node)
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

// The machine seam: with bob placed on the runner, the lifecycle's own launch (Spawns.StartIn) opens his terminal there through
// term.Machines; the master reads its screen, types at it and closes it by the handle it recorded, and a terminal of this machine's
// still opens here.
func TestALaunchForAnAgentOnARunnerOpensItsTerminalThere(t *testing.T) {
	master, node, home, _ := masterAndRunner(t)
	t.Cleanup(func() { exec.Command("tmux", "-L", paths.TmuxSocket(master.cfg.Home), "kill-server").Run() })
	tm := term.New(&term.Machines{Local: agenthost.Tmux(master.cfg.Home), Remote: master.nodes.Terminals(),
		Where: func(agent string) string {
			if agent == "bob" {
				return node
			}
			return ""
		}}, claudecode.ScreenFor)
	spawns := &lifecycle.Spawns{Q: master.db, Host: spawnHost{tm}, Delivery: noSession{}}
	ok, said, err := spawns.StartIn("bob", home, "echo launched-$((40+2))")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("StartIn: %v %s", ok, said) // no session confirms here: a runner's session facts come with slice 7
	handle, err := db.Terminal(master.db, "bob")
	if err != nil || !strings.HasPrefix(handle, node+":%") {
		t.Fatalf("bob's terminal is %q (%v), want one on %s", handle, err, node)
	}
	shows := func(want string) []string {
		t.Helper()
		var rows []string
		for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
			if s := tm.Backend.ReadScreen(handle, 40); s != nil {
				if rows = s.Tail; strings.Contains(strings.Join(rows, "\n"), want) {
					return rows
				}
			}
		}
		t.Fatalf("%s never showed %q: %q", handle, want, rows)
		return nil
	}
	t.Logf("%s: %q", handle, shows("launched-42"))
	if !tm.Alive(handle) {
		t.Fatal("the runner's terminal is not listed alive")
	}
	if ok, why := tm.Send(handle, "echo typed-$((1+1))", true); !ok {
		t.Fatal(why)
	}
	t.Logf("%s: %q", handle, shows("typed-2"))
	if ok, why := tm.CloseTerminal(handle); !ok {
		t.Fatal(why)
	}
	var left int
	master.db.QueryRow("SELECT COUNT(*) FROM node_sessions").Scan(&left)
	if left != 0 || tm.Alive(handle) {
		t.Fatalf("the closed terminal is still there (%d kept)", left)
	}

	here, why := tm.CreateTerminal("true", home, "ann", "ann")
	if !strings.HasPrefix(here, "%") {
		t.Fatalf("a terminal of this machine opened at %q: %s", here, why)
	}
}

// An agent launched on a runner by hl launch there starts with the master's files at the same paths under the runner's home: its
// memory, the shared skills, the MCP servers and the attachment its launch names; the paths it is given are the runner's. What it
// writes to its memory and its handoff note comes back to the master, and a skill changed on the master reaches the runner with no
// relaunch.
func TestAnAgentOnARunnerHasTheMastersFilesAndItsOwnComeBack(t *testing.T) {
	master, node, rhome, hl := masterAndRunner(t)
	mhome := master.cfg.Home
	agentDir := paths.AgentDir(mhome, "bob")
	write := func(path, text string) {
		t.Helper()
		if os.MkdirAll(filepath.Dir(path), 0o755) != nil || os.WriteFile(path, []byte(text), 0o644) != nil {
			t.Fatal("could not write " + path)
		}
	}
	skill := filepath.Join(mhome, "shared", ".claude", "skills", "s", "SKILL.md")
	attached := filepath.Join(mhome, "attachments", "pic.txt")
	write(filepath.Join(agentDir, "memory", "MEMORY.md"), "remember-42")
	write(skill, "skill-v1")
	write(paths.MCPConfig(mhome), `{"mcpServers":{}}`)
	write(attached, "attached-7")
	var asked map[string]any
	master.nodes.Launch = func(n, agent string, params map[string]any) (*nodes.Launched, error) {
		if n != node || agent != "bob" {
			t.Errorf("launch for %s on %s", agent, n)
		}
		asked = params
		script := "cat $MEM/MEMORY.md " + attached + " " + skill + "; echo; echo up-9 > $MEM/new.md; echo note-5 > " +
			paths.HandoffNoteFile(mhome, "bob") + "; echo at=$MEM; echo launched-$((2+3)); sleep 60"
		return &nodes.Launched{Argv: []string{"/bin/sh", "-c", script}, Env: []string{"MEM=" + filepath.Join(agentDir, "memory")}}, nil
	}
	rn, _ := master.nodes.Runner(node)
	handle, err := rn.Open("bob", rhome, hl+" launch bob")
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if rows, _ = rn.Screen(handle, 40); strings.Contains(strings.Join(rows, "\n"), "launched-5") {
			break
		}
	}
	screen := strings.Join(rows, "\n")
	t.Logf("runner screen: %q", rows)
	for _, want := range []string{"remember-42", "attached-7", "skill-v1", "launched-5"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the launched session did not show %s", want)
		}
	}
	if cwd, _ := asked["cwd"].(string); cwd != rhome && !strings.HasSuffix(cwd, rhome) {
		t.Fatalf("the launch was asked for %q, not the runner's folder", cwd)
	}
	rdir := strings.Replace(agentDir, mhome, rhome, 1)
	if !strings.Contains(screen, "at="+filepath.Join(rdir, "memory")) { // both homes are on this Mac: only the path tells them apart
		t.Fatal("the session was not given the runner's paths")
	}
	if b, _ := os.ReadFile(filepath.Join(rdir, "memory", "MEMORY.md")); string(b) != "remember-42" {
		t.Fatalf("the runner's copy of bob's memory: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(rdir, "memory", "new.md")); strings.TrimSpace(string(b)) != "up-9" {
		t.Fatalf("the session did not write its memory on the runner: %q", b)
	}

	came := func(path, want string) {
		t.Helper()
		var b []byte
		for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
			if b, _ = os.ReadFile(path); strings.TrimSpace(string(b)) == want {
				return
			}
		}
		t.Fatalf("%s holds %q, want %q", path, b, want)
	}
	came(filepath.Join(agentDir, "memory", "new.md"), "up-9") // sent up as it changed
	os.Remove(paths.HandoffNoteFile(mhome, "bob"))
	write(filepath.Join(rdir, "handoff.md"), "note-6")
	if err := master.nodes.Flush("bob"); err != nil { // what a close does before it reads the note
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(paths.HandoffNoteFile(mhome, "bob")); string(b) != "note-6" {
		t.Fatalf("the handoff note on the master after a flush: %q", b)
	}
	write(skill, "skill-v2")
	came(strings.Replace(skill, mhome, rhome, 1), "skill-v2")
}

// spawnHost is the lifecycle's host over a Term, as compose's termHost is.
type spawnHost struct{ t *term.Term }

func (h spawnHost) Available() bool { return h.t.Backend.Available() }
func (h spawnHost) Missing() string { return h.t.Missing() }
func (h spawnHost) OpenTerminal(cmd, cwd, title, agent string) (string, string) {
	return h.t.CreateTerminal(cmd, cwd, title, agent)
}
func (h spawnHost) WaitIdle(handle string, ms int) bool { return h.t.WaitIdle(handle, ms) }
func (spawnHost) Submit(string, string, int)            {}
func (spawnHost) HasRuns() bool                         { return false }
func (spawnHost) RunCurrent() string                    { return "" }
func (spawnHost) RunExists(string) bool                 { return false }
func (spawnHost) RunUse(string)                         {}
func (spawnHost) RunCreate(string) string               { return "" }
func (spawnHost) TaskCreate(_, _, _, _ string) string   { return "" }
func (spawnHost) Dispatch(_, _, _ string) string        { return "" }

// noSession is a delivery that sees no session start: StartIn asks only that.
type noSession struct{ lifecycle.Delivery }

func (noSession) WaitForSession(string, int, string) bool { return false }

// masterAndRunner is a master with bob seeded and a runner on its own home and tmux server, linked to it: the runner's node id and
// home, and the hl binary built for them.
func masterAndRunner(t *testing.T) (master *Daemon, node, home, hl string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	t.Setenv("HANDLINE_AGENT", "")
	t.Setenv("HANDLINE_TMUX_SOCKET", "")
	hl = filepath.Join(clustertest.Build(t), "hl")
	master = startNet(t)
	teamtest.Seed(t, master.cfg.Home, "bob")

	home, err := os.MkdirTemp("", "hlr") // short: a socket path holds 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	node, err = joined.NodeID(home, nil)
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
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if rn, _ := master.nodes.Runner(node); rn != nil {
			return master, node, home, hl
		}
		if time.Now().After(end) {
			t.Fatal("the runner never linked to the master")
		}
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
	if r["method"] == "runner.home" { // the master asks a link's home once, before the first forwarded call runs
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": r["id"], "result": map[string]any{"home": "/runner/home"}})
		r = link.ask(string(b))
	}
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
