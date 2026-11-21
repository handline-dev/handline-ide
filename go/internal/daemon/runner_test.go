package daemon

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/clustertest"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/version"
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
			if rows, err = rn.Screen(handle, 40, false); err == nil && strings.Contains(strings.Join(rows, "\n"), want) {
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

// An app connected to the master attaches the terminal of an agent that runs on a runner: the snapshot and the output come from the
// runner's tmux over its link, what the app types reaches it, a resize and a scrollback page reach it, and the styled format reads it.
func TestAnAppAttachesAnAgentsTerminalOnARunnerThroughTheMaster(t *testing.T) {
	master, node, home, _ := masterAndRunner(t)
	rn, _ := master.nodes.Runner(node)
	pane, err := rn.Open("bob", home, "cd "+home)
	if err != nil {
		t.Fatal(err)
	}
	handle := term.RunnerHandle(node, pane)
	bob := teamtest.Key(t, master.cfg.Home, "bob")
	if _, err := master.db.Exec(`INSERT INTO agent_state(agent, state, updated, terminal) VALUES(?, 'IDLE', 0, ?)
		ON CONFLICT(agent) DO UPDATE SET terminal = excluded.terminal`, bob, handle); err != nil {
		t.Fatal(err)
	}
	token, _ := pair(t, master, "app")
	app := helloAs(t, master, token)
	r := app.ask(`{"jsonrpc":"2.0","id":2,"method":"terminal.attach","params":{"terminal":"` + handle + `","format":"bytes","history":10}}`)
	snap, ok := r["result"].(map[string]any)
	if !ok || snap["terminal"] != handle || snap["format"] != "bytes" || snap["cols"] == nil {
		t.Fatalf("attach: %v", r)
	}
	if r := app.askFor(`{"jsonrpc":"2.0","id":3,"method":"input.take","params":{"agent":"bob","request_id":"take-1"}}`, 3); r["result"] == nil {
		t.Fatalf("input.take: %v", r)
	}
	typed := base64.StdEncoding.EncodeToString([]byte("echo relayed-$((40+2))\r"))
	if r := app.askFor(`{"jsonrpc":"2.0","id":4,"method":"terminal.input","params":{"terminal":"`+handle+`","data":"`+typed+`","request_id":"in-1"}}`, 4); r["result"] == nil {
		t.Fatalf("terminal.input: %v", r)
	}
	var seen strings.Builder
	for end := time.Now().Add(15 * time.Second); !strings.Contains(seen.String(), "relayed-42"); {
		if time.Now().After(end) {
			t.Fatalf("the output never came: %q", seen.String())
		}
		m, _ := app.next(time.Until(end))
		if p, _ := m["params"].(map[string]any); m["method"] == "output" && p["terminal"] == handle {
			b, _ := base64.StdEncoding.DecodeString(p["data"].(string))
			seen.Write(b)
		}
	}
	if r := app.askFor(`{"jsonrpc":"2.0","id":5,"method":"terminal.resize","params":{"terminal":"`+handle+`","cols":101,"rows":31,"request_id":"rs-1"}}`, 5); r["result"] == nil {
		t.Fatalf("terminal.resize: %v", r)
	}
	if c, rows, ok := agenthost.Tmux(home).(term.Sizer).Size(pane); !ok || c != 101 || rows != 31 {
		t.Fatalf("the runner's pane is %dx%d", c, rows)
	}
	if r := app.askFor(`{"jsonrpc":"2.0","id":6,"method":"terminal.history","params":{"terminal":"`+handle+`","format":"bytes","before":1000,"limit":10}}`, 6); r["result"] == nil {
		t.Fatalf("terminal.history: %v", r)
	}
	r = app.askFor(`{"jsonrpc":"2.0","id":7,"method":"terminal.attach","params":{"terminal":"`+handle+`","format":"styled"}}`, 7)
	if res, _ := r["result"].(map[string]any); res == nil || !strings.Contains(fmt.Sprint(res["lines"]), "relayed-42") {
		t.Fatalf("styled attach: %v", r)
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
		if rows, _ = rn.Screen(handle, 40, false); strings.Contains(strings.Join(rows, "\n"), "launched-5") {
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
	master, node, home, hl, _ = masterWithRunner(t, nil)
	return
}

// masterWithRunner is masterAndRunner with the master's turn tracker (nil for none), and the runner.
func masterWithRunner(t *testing.T, tracker *turns.Tracker) (master *Daemon, node, home, hl string, r *runner.Runner) {
	t.Helper()
	return masterWithRunnerAs(t, tracker, func(*runner.Config) {})
}

// masterWithRunnerAs is masterWithRunner with the runner's config as edit leaves it.
func masterWithRunnerAs(t *testing.T, tracker *turns.Tracker, edit func(*runner.Config)) (master *Daemon, node, home, hl string, r *runner.Runner) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	t.Setenv("HANDLINE_AGENT", "")
	t.Setenv("HANDLINE_TMUX_SOCKET", "")
	hl = filepath.Join(clustertest.Build(t), "hl")
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth, cfg.Turns = fakeAuth{key: "k", agent: "a"}, tracker
	master, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(master.Close)
	teamtest.Seed(t, master.cfg.Home, "bob")

	home, node, m := joinRunner(t, master)
	tm := agenthost.Tmux(home).(runner.Terminals)
	t.Cleanup(func() { exec.Command("tmux", "-L", paths.TmuxSocket(home), "kill-server").Run() })
	rc := runner.Config{Home: home, Socket: paths.SocketFile(home), Sha: version.Sha, Master: m, Term: tm, HL: hl, Log: t.Logf,
		Accounts: func() []string { return []string{"alpha"} }}
	edit(&rc)
	r, err = runner.Start(rc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	linkedRunner(t, master, node)
	return master, node, home, hl, r
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

// joinRunner is a runner's home joined to master: its node id and the master it reaches.
func joinRunner(t *testing.T, master *Daemon) (home, node string, m joined.Master) {
	t.Helper()
	var err error
	home, err = os.MkdirTemp("", "hlr") // short: a socket path holds 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	node, err = joined.NodeID(home, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, uri := master.dev.Pair(time.Minute)
	m, err = hlclient.Join(home, uri, "box", "server", node)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { joined.Forget(home) })
	if _, err := master.db.Exec("UPDATE nodes SET lending = 'run' WHERE id = ?", node); err != nil { // lent: it takes sessions
		t.Fatal(err)
	}
	return home, node, m
}

// linkedRunner is master's hand on node once its runner has linked and its heartbeat came.
func linkedRunner(t *testing.T, master *Daemon, node string) *nodes.Runner {
	t.Helper()
	var rn *nodes.Runner
	for end := time.Now().Add(10 * time.Second); rn == nil || master.nodes.Takes(node) != ""; time.Sleep(50 * time.Millisecond) {
		if rn, _ = master.nodes.Runner(node); time.Now().After(end) {
			t.Fatalf("the runner never linked to the master and took work: %s", master.nodes.Takes(node))
		}
	}
	return rn
}

func TestMain(m *testing.M) {
	code := m.Run()
	clustertest.Remove() // the handlined and hl the runner test built
	os.Exit(code)
}

// The hooks of a session on a runner reach the master's board, and the runner's own tracker tells the master the turn: bob works on
// the master's tracker after his UserPromptSubmit on the runner and is idle after his Stop, and is unknown once his session there is
// stopped or the runner's link ends;
// what his session writes to its transcript there reaches the master's chat rows.
func TestARunnersSessionShowsWorkingAndIdleOnTheMaster(t *testing.T) {
	tracker := &turns.Tracker{}
	master, node, home, hl, r := masterWithRunner(t, tracker)
	rn, _ := master.nodes.Runner(node)
	handle, err := rn.Open("bob", home, "cd "+home)
	if err != nil {
		t.Fatal(err)
	}
	bob := teamtest.Key(t, master.cfg.Home, "bob") // the master's tracker holds an agent by its key, as its hooks reach it
	becomes := func(want turns.State, what string) {
		t.Helper()
		for end := time.Now().Add(15 * time.Second); tracker.State(bob) != want; time.Sleep(50 * time.Millisecond) {
			if time.Now().After(end) {
				rows, _ := rn.Screen(handle, 20, false)
				t.Fatalf("%s: bob is %v on the master, want %v; the runner shows %q", what, tracker.State(bob), want, rows)
			}
		}
	}
	os.WriteFile(filepath.Join(master.cfg.Home, "config.yaml"), []byte("chat_transcript_rows: true\n"), 0o644)
	path := filepath.Join(home, "s1.jsonl")
	os.WriteFile(path, nil, 0o644)
	hook := func(event string) {
		t.Helper()
		payload := `{"session_id":"s1","transcript_path":"` + path + `"}`
		if err := rn.Keys(handle, "echo '"+payload+"' | HANDLINE_AGENT=bob "+hl+" hook "+event, true); err != nil {
			t.Fatal(err)
		}
	}
	hook("UserPromptSubmit")
	becomes(turns.Working, "after UserPromptSubmit")
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"assistant","message":{"content":[{"type":"text","text":"Reading it on the runner."}]}}` + "\n")
	f.Close()
	for end := time.Now().Add(15 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		var text string
		master.db.QueryRow("SELECT json_extract(body, '$.text') FROM transcript_events WHERE agent = ? AND kind = 'text'", bob).Scan(&text)
		if text == "Reading it on the runner." {
			break
		}
		if time.Now().After(end) {
			t.Fatalf("the runner's transcript line never reached the master's chat rows: %q", text)
		}
	}
	hook("Stop")
	becomes(turns.Idle, "after Stop")
	var n int
	if err := master.db.QueryRow("SELECT COUNT(*) FROM hook_applied WHERE agent = ?", bob).Scan(&n); err != nil || n != 2 {
		t.Fatalf("the master's board has %d of bob's hook events (%v), want 2", n, err)
	}
	if !master.nodes.Elsewhere(bob) {
		t.Fatal("the master's own tracker would hear bob's hooks too")
	}
	if err := rn.Stop("bob"); err != nil {
		t.Fatal(err)
	}
	becomes(turns.Unknown, "after his session there was stopped")
	if handle, err = rn.Open("bob", home, "cd "+home); err != nil {
		t.Fatal(err)
	}
	hook("UserPromptSubmit")
	becomes(turns.Working, "in his next session there")
	r.Close()
	becomes(turns.Unknown, "after his runner's link ended")
}

// What a runner's account's transcripts gain reaches the master's token log under that account, charged to nobody here; the same
// calls sent again count once, and a pass with nothing new sends nothing.
func TestARunnersAccountUsageCountsOnTheMaster(t *testing.T) {
	master, _, home, _, r := masterWithRunner(t, nil)
	dir, err := team.AccountConfigDir(home, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	projects := filepath.Join(dir, "projects", "p")
	os.MkdirAll(projects, 0o755)
	line := `{"type":"assistant","requestId":"r1","sessionId":"s1","cwd":"/w","timestamp":"` + time.Now().UTC().Format(time.RFC3339) +
		`","message":{"model":"claude-opus-5-5","usage":{"input_tokens":10,"output_tokens":20}}}` + "\n"
	os.WriteFile(filepath.Join(projects, "s1.jsonl"), []byte(line), 0o644)
	u := &usage.Env{Home: home}
	sum := func() (n, out int) {
		master.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(out), 0) FROM token_calls WHERE account = 'alpha'").Scan(&n, &out)
		return
	}
	if err := r.SendTokens(u); err != nil {
		t.Fatal(err)
	}
	if n, out := sum(); n != 1 || out != 20 {
		t.Fatalf("the master counts %d calls, %d out", n, out)
	}
	os.Remove(runner.TokensFile(home)) // the positions lost: the same calls go again
	if err := r.SendTokens(u); err != nil {
		t.Fatal(err)
	}
	if n, out := sum(); n != 1 || out != 20 {
		t.Fatalf("sent twice, the master counts %d calls, %d out", n, out)
	}
}
