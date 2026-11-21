package placement

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/store"
)

const gb = int64(1) << 30

// start is a session's start for agent whose terminal opens where it was placed.
func start(p *Placer, agent string) string {
	node := p.Where(agent)
	p.Placed(agent, true)
	return node
}

// two is a master m0 with two runners on Run, r1 and r2, both holding the shop workspace, and the agent a1 (bob) of shop.
func two(t *testing.T) (*Placer, func(node string, free int64, sessions int, programs ...string)) {
	t.Helper()
	d, err := store.Open(filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	for _, q := range []string{
		"INSERT INTO workspaces(id, name, folder, created_at) VALUES(1, 'shop', '/shop', 1)",
		"INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(1, 1, 'bob', 'p1', 1)",
		"INSERT INTO nodes(id, name, joined_at, lending) VALUES('m0', 'mac', 1, 'run'), ('r1', 'one', 2, 'run'), ('r2', 'two', 3, 'run')",
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	reg := nodes.New(d, t.TempDir())
	reg.Self, reg.Version = "m0", "v1"
	now := time.Now()
	reg.Clock = func() time.Time { return now }
	hear := func(node string, free int64, sessions int, programs ...string) {
		b := runner.Beat{Version: "v1", Load: runner.Load{RAMFree: free}, Has: runner.Has{CLIs: []string{"claude"}, Programs: programs}}
		for i := 0; i < sessions; i++ {
			b.Sessions = append(b.Sessions, runner.BeatSession{Agent: "x", Handle: "%1"})
		}
		reg.Hear(node, b, now)
	}
	hear("r1", 4*gb, 0)
	hear("r2", 8*gb, 0)
	p := &Placer{DB: d, Reg: reg, Home: t.TempDir(),
		Holds:  func(node, ws string) bool { return ws == "w1" },
		Here:   func() (Room, error) { return Room{RAMFree: 1 * gb}, nil },
		Limits: func() Limits { return Limits{Sessions: 2} }}
	return p, hear
}

// A new agent lands on the machine with the most room; its next session stays there; when that one is full or on Pause it moves to the
// other and stays there afterwards.
func TestAnAgentStaysOnItsMachineWhileItHasRoom(t *testing.T) {
	p, hear := two(t)
	if got := start(p, "bob"); got != "r2" {
		t.Fatalf("a new agent went to %q, not r2 with the most room", got)
	}
	hear("r1", 16*gb, 0) // r1 has more room now: bob stays on r2
	if got := start(p, "bob"); got != "r2" {
		t.Fatalf("its next session went to %q", got)
	}
	p.DB.Exec("INSERT INTO node_sessions(agent, node, handle, opened_at) VALUES('x', 'r2', '%1', 1), ('y', 'r2', '%2', 1)")
	if why := p.Lacks("bob", "r2"); why != "full: 2 of 2 sessions" {
		t.Fatalf("r2 with 2 of 2 sessions: %q", why)
	}
	if got := start(p, "bob"); got != "r1" {
		t.Fatalf("with r2 full it went to %q", got)
	}
	p.DB.Exec("DELETE FROM node_sessions")
	if got := start(p, "bob"); got != "r1" {
		t.Fatalf("after the move it went to %q, not stayed on r1", got)
	}
	p.DB.Exec("UPDATE nodes SET lending = 'pause' WHERE id = 'r1'")
	if got := start(p, "bob"); got != "r2" {
		t.Fatalf("with r1 on Pause it went to %q", got)
	}
}

// An agent whose MCP server runs a program a runner lacks never lands there, and the reason reads as what it needs.
func TestAnAgentNeverLandsWhereItsProgramIsMissing(t *testing.T) {
	p, hear := two(t)
	p.DB.Exec(`UPDATE agents SET mcp_config = '{"mcpServers": {"pencil": {"command": "/opt/bin/pencil", "args": ["mcp"]}}}' WHERE id = 1`)
	hear("r1", 4*gb, 0, "pencil")
	if why := p.Lacks("bob", "r2"); why != "needs pencil" {
		t.Fatalf("r2 without pencil: %q", why)
	}
	if got := start(p, "bob"); got != "r1" {
		t.Fatalf("it went to %q, not r1, the one with pencil", got)
	}
	if got := p.Check(); strings.Join(got, ",") != "pencil" {
		t.Fatalf("runners are asked about %v", got)
	}
}

// With no runner able to take it, an agent starts on this machine, as before runners; one that ran before runners stays here.
func TestWithNoRoomElsewhereItStaysHere(t *testing.T) {
	p, _ := two(t)
	p.DB.Exec("UPDATE nodes SET lending = 'off' WHERE id != 'm0'")
	if got := start(p, "bob"); got != "" {
		t.Fatalf("it went to %q", got)
	}
	p.DB.Exec("UPDATE nodes SET lending = 'run'")
	if got := start(p, "bob"); got != "" {
		t.Fatalf("placed here, it moved to %q though here has room", got)
	}
	p.DB.Exec("INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(2, 1, 'ann', 'p1', 1)")
	p.DB.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES('a2', 'IDLE', 1, '/w/ann')")
	if got := start(p, "ann"); got != "" {
		t.Fatalf("an agent that ran here before runners moved to %q", got)
	}
}

// The machine a terminal failed to open on does not become the agent's, and a pick holds its slot until its terminal opens: two starts
// at once never take the same last one.
func TestAPickHoldsItsSlotAndIsKeptOnlyOnceItsTerminalOpens(t *testing.T) {
	p, _ := two(t)
	if got := p.Where("bob"); got != "r2" {
		t.Fatalf("bob went to %q", got)
	}
	p.Placed("bob", false)
	var node sql.NullString
	p.DB.QueryRow("SELECT node FROM agent_state WHERE agent = 'a1'").Scan(&node)
	if node.Valid {
		t.Fatalf("a terminal that did not open made %q bob's machine", node.String)
	}
	p.DB.Exec("INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(2, 1, 'ann', 'p1', 1)")
	p.DB.Exec("INSERT INTO node_sessions(agent, node, handle, opened_at) VALUES('x', 'r2', '%1', 1)")
	if got := p.Where("bob"); got != "r2" { // the last of r2's two slots, held while it opens
		t.Fatalf("bob went to %q", got)
	}
	if got := p.Where("ann"); got != "r1" {
		t.Fatalf("ann took %q while bob's start held r2's last slot", got)
	}
}

func TestARunnersOwnMemoryCapIsWhereItIsFull(t *testing.T) {
	p, _ := two(t)
	p.Reg.Hear("r2", runner.Beat{Version: "v1", Load: runner.Load{RAMFree: 8 * gb, AgentsRSS: 3 * gb},
		Has: runner.Has{CLIs: []string{"claude"}}}, time.Now())
	if why := p.Lacks("bob", "r2"); why != "" {
		t.Fatalf("with no cap anywhere r2 is %q", why)
	}
	p.Limits = func() Limits { return Limits{Sessions: 2, AgentsRSS: 2 * gb} }
	if why := p.Lacks("bob", "r2"); !strings.Contains(why, "3 of 2 GB") {
		t.Fatalf("under the master's 2 GB cap r2 is %q", why)
	}
	if _, err := p.DB.Exec("UPDATE nodes SET memory_max_gb = 4 WHERE id = 'r2'"); err != nil {
		t.Fatal(err)
	}
	if why := p.Lacks("bob", "r2"); why != "" {
		t.Fatalf("its own 4 GB cap still leaves r2 %q", why)
	}
	if _, err := p.DB.Exec("UPDATE nodes SET memory_max_gb = 3 WHERE id = 'r2'"); err != nil {
		t.Fatal(err)
	}
	if why := p.Lacks("bob", "r2"); !strings.Contains(why, "3 of 3 GB") {
		t.Fatalf("at its own 3 GB cap r2 is %q", why)
	}
}
