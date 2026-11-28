package nodes

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
)

func registry(t *testing.T) *Registry {
	t.Helper()
	d, err := store.Open(filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	for _, q := range []string{
		"INSERT INTO nodes(id, name, joined_at, lending) VALUES('n1', 'box', 1, 'run')",
		"INSERT INTO node_sessions(agent, node, handle, opened_at) VALUES('bob', 'n1', '%1', 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at) VALUES(1, 'one', 'bob', 'p1', 1, 'p1', 1)",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'RUNNING', 1, 1)",
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	r := New(d, t.TempDir())
	r.Version = "v1"
	return r
}

// An agent whose machine its person set to Off is named with that machine, so the tick hands it over; on Run or Pause it is not.
func TestOffMachineIsTheMachineSetToOff(t *testing.T) {
	r := registry(t)
	if got := r.OffMachine("bob"); got != "" {
		t.Fatalf("on run: %q", got)
	}
	r.DB.Exec("UPDATE nodes SET lending = 'pause' WHERE id = 'n1'")
	if got := r.OffMachine("bob"); got != "" {
		t.Fatalf("on pause: %q", got)
	}
	r.DB.Exec("UPDATE nodes SET lending = 'off' WHERE id = 'n1'")
	if got := r.OffMachine("bob"); got != "box" || r.OffMachine("ann") != "" {
		t.Fatalf("on off: %q", got)
	}
}

// A revoked device's sessions are taken back at once, before its node row and node_sessions go.
func TestARevokedRunnersSessionsAreTakenBackAtOnce(t *testing.T) {
	r := registry(t)
	now := time.Now()
	r.SetClock(func() time.Time { return now })
	r.beats["n1"] = heard{at: now, beat: runner.Beat{Version: "v1", Sessions: []runner.BeatSession{{Agent: "bob", Handle: "%1", Branch: "work"}}}}
	if why := r.Takes("n1"); why != "" {
		t.Fatalf("a runner heard now on the master's version on run: %q", why)
	}
	got, err := r.Reclaim("n1", "its device was revoked")
	if err != nil || len(got) != 1 || got[0].Agent != "bob" || got[0].Task != 1 {
		t.Fatalf("taken back %+v (%v)", got, err)
	}
	if !strings.Contains(got[0].Comment, "revoked") || !strings.Contains(got[0].Comment, "Nothing on work was pushed") {
		t.Fatalf("comment %q", got[0].Comment)
	}
	if again, _ := r.Reclaim("n1", "again"); len(again) != 0 {
		t.Fatalf("taken back twice: %+v", again)
	}
}

// What a runner says decides whether it takes a new session: heard within 60 s, on the master's version, and on Run.
func TestTakesNeedsAHeartbeatTheVersionAndRun(t *testing.T) {
	r := registry(t)
	now := time.Now()
	r.SetClock(func() time.Time { return now })
	if why := r.Takes("n1"); !strings.Contains(why, "offline") {
		t.Fatalf("never heard: %q", why)
	}
	r.beats["n1"] = heard{at: now}
	if why := r.Takes("n1"); !strings.Contains(why, "heartbeat") {
		t.Fatalf("linked, no heartbeat yet: %q", why)
	}
	r.beats["n1"] = heard{at: now, beat: runner.Beat{Version: "v0"}}
	if why := r.Takes("n1"); !strings.Contains(why, "update to take work") {
		t.Fatalf("another version: %q", why)
	}
	r.beats["n1"] = heard{at: now, beat: runner.Beat{Version: "v1"}}
	r.DB.Exec("UPDATE nodes SET lending = 'pause' WHERE id = 'n1'")
	if why := r.Takes("n1"); !strings.Contains(why, "pause") {
		t.Fatalf("on pause: %q", why)
	}
}

// An email is one board account on every runner: the account an agent there starts on and its usage counts under. Two runners that
// each call a folder alpha, logged in as two emails, hold two accounts; one email in two folders on one runner is one account.
func TestARunnersAccountsAreTheBoardsByEmail(t *testing.T) {
	r := registry(t)
	r.Self = "n0"
	board := map[string]string{"a@x": "main"}
	r.AccountFor = func(email, name string, add bool) (string, error) {
		if b := board[email]; b != "" || !add {
			return b, nil
		}
		b := name
		for n := 2; slices.Contains(slices.Collect(maps.Values(board)), b); n++ {
			b = fmt.Sprintf("%s-%d", name, n)
		}
		board[email] = b
		return b, nil
	}
	for _, q := range []string{"INSERT OR IGNORE INTO people(id, display_name, role, created_at) VALUES(1, 'Ron', 'owner', 0), (2, 'Kim', 'member', 0)",
		"UPDATE nodes SET person = 1 WHERE id = 'n1'", "INSERT INTO nodes(id, name, joined_at, lending, person) VALUES('n2', 'two', 1, 'run', 1), ('n3', 'kims', 1, 'run', 2)"} {
		if _, err := r.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	r.Hear("n1", runner.Beat{Accounts: []runner.BeatAccount{{Name: "alpha", Email: "b@x", Dir: "accounts/k1/.claude"},
		{Name: "mine", Email: "a@x", Dir: "accounts/.unregistered/mine/.claude"}, {Name: "again", Email: "a@x", Dir: "accounts/k2/.claude"}}}, now)
	r.Hear("n2", runner.Beat{Accounts: []runner.BeatAccount{{Name: "alpha", Email: "c@x", Dir: "accounts/k1/.claude"},
		{Name: "out", Email: "d@x", Dir: "../elsewhere"}}}, now)
	want1 := map[string]string{"alpha": filepath.Join(r.Home, "accounts/k1/.claude"), "main": filepath.Join(r.Home, "accounts/.unregistered/mine/.claude")}
	if got := r.Accounts("n1"); !maps.Equal(got, want1) {
		t.Fatalf("n1: %v", got)
	}
	if got := r.Accounts("n2"); !maps.Equal(got, map[string]string{"alpha-2": filepath.Join(r.Home, "accounts/k1/.claude")}) {
		t.Fatalf("n2, a folder outside its home left out: %v", got)
	}
	// Another person's runner makes no account of the owner's: only an email the board has is one there.
	r.Hear("n3", runner.Beat{Accounts: []runner.BeatAccount{{Name: "x", Email: "a@x", Dir: "accounts/k1/.claude"}, {Name: "y", Email: "k@x", Dir: "accounts/k2/.claude"}}}, now)
	if got := r.Accounts("n3"); !maps.Equal(got, map[string]string{"main": filepath.Join(r.Home, "accounts/k1/.claude")}) || board["k@x"] != "" {
		t.Fatalf("n3: %v", got)
	}
	// A folder heard again with its email keeps its account, asked no more.
	r.AccountFor = func(string, string, bool) (string, error) { t.Fatal("asked again"); return "", nil }
	r.Hear("n2", runner.Beat{Accounts: []runner.BeatAccount{{Name: "alpha", Email: "c@x", Dir: "accounts/k1/.claude"}}}, now)
	here := func(node, folder string) string {
		board, named := r.accountHere(node, folder)
		return fmt.Sprintf("%s %v", board, named)
	}
	if here("n2", "alpha") != "alpha-2 true" || here("n1", "mine") != "main true" || here("n9", "z") != " false" || here("n3", "y") != " true" {
		t.Fatal("a runner's usage counts under its folder's board account")
	}
	r.Placer = nextOn{"bob": "n2", "cat": "n2", "dan": "n0"}
	for agent, want := range map[string]string{"bob": "n1", "cat": "n2", "dan": "", "eve": ""} {
		if got := r.DeviceOf(agent); got != want {
			t.Errorf("%s is on %q, want %q: its session's runner, else where it would open, the master as none", agent, got, want)
		}
	}
}

// nextOn places each agent on its machine.
type nextOn map[string]string

func (n nextOn) Where(agent string) string { return n[agent] }
func (nextOn) Placed(string, bool)         {}
func (n nextOn) Next(agent string) string  { return n[agent] }

// A wake leaves a runner set to Off or Pause, or where none of the agent's accounts can start, at once; one with no link only once it has
// been away past LeaveAfter. Release drops the agent's session there.
func TestAWakeLeavesARunnerForLastingReasonsAndAnUnlinkedOneOnlyPastTheWait(t *testing.T) {
	r := registry(t)
	r.Self = "m0"
	now := time.Now()
	r.SetClock(func() time.Time { return now })
	r.Hear("n1", runner.Beat{Version: "v1"}, now)
	if why, wait := r.LeaveFor("bob", "m0"); why != "" || wait {
		t.Fatalf("this machine: %q %v", why, wait)
	}
	if why, wait := r.LeaveFor("bob", "n1"); why != "" || !wait {
		t.Fatalf("no link a moment: waited for, %q %v", why, wait)
	}
	r.SetClock(func() time.Time { return now.Add(LeaveAfter + time.Second) })
	if why, _ := r.LeaveFor("bob", "n1"); why != "it is offline" {
		t.Fatalf("away past the wait: %q", why)
	}
	r.DB.Exec("UPDATE nodes SET lending = 'off' WHERE id = 'n1'")
	if why, _ := r.LeaveFor("bob", "n1"); why != "it is set to off" {
		t.Fatalf("set to off: %q", why)
	}
	r.DB.Exec("UPDATE nodes SET lending = 'run' WHERE id = 'n1'")
	r.SetClock(func() time.Time { return now })
	r.mu.Lock()
	r.links = map[string]*rpc.Conn{"n1": nil}
	r.mu.Unlock()
	r.StartsOn = func(agent, node string) bool { return false }
	if why, _ := r.LeaveFor("bob", "n1"); why != "none of its accounts can start there" {
		t.Fatalf("no account there: %q", why)
	}
	r.StartsOn = func(agent, node string) bool { return true }
	if why, wait := r.LeaveFor("bob", "n1"); why != "" || wait {
		t.Fatalf("linked, with its account: %q %v", why, wait)
	}
	r.mu.Lock()
	r.links = nil
	r.mu.Unlock()
	if err := r.Release("bob", "n1", "it is set to off"); err != nil {
		t.Fatal(err)
	}
	if node, _ := r.sessionNode("bob"); node != "" {
		t.Fatalf("its session is still placed on %q", node)
	}
	if r.NodeName("n1") != "box" {
		t.Fatal(r.NodeName("n1"))
	}
}

// A machine is silent once it has been offline, or the master has held no link to it, for LeaveAfter, since its last heartbeat; online and
// linked, or this machine, it is not.
func TestAMachineIsSilentWhenOfflineOrNotLinked(t *testing.T) {
	r := registry(t)
	r.Self = "m0"
	now := time.Now()
	r.SetClock(func() time.Time { return now })
	r.Hear("n1", runner.Beat{Version: "v1"}, now)
	if _, silent := r.Silent("m0"); silent {
		t.Fatal("this machine is never silent")
	}
	if _, silent := r.Silent("n1"); silent {
		t.Fatal("not linked a moment: not silent yet")
	}
	r.SetClock(func() time.Time { return now.Add(LeaveAfter) })
	if since, silent := r.Silent("n1"); !silent || !since.Equal(now) {
		t.Fatalf("not linked past the wait: %v %v", since, silent)
	}
	r.SetClock(func() time.Time { return now })
	r.mu.Lock()
	r.links = map[string]*rpc.Conn{"n1": nil}
	r.mu.Unlock()
	if _, silent := r.Silent("n1"); silent {
		t.Fatal("online and linked is not silent")
	}
	r.SetClock(func() time.Time { return now.Add(OfflineAfter + LeaveAfter) })
	if since, silent := r.Silent("n1"); !silent || !since.Equal(now) {
		t.Fatalf("linked but offline: %v %v", since, silent)
	}
}
