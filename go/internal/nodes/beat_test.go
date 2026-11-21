package nodes

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/store"
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
	r.Clock = func() time.Time { return now }
	r.beats["n1"] = heard{at: now, beat: runner.Beat{Version: "v1", Sessions: []runner.BeatSession{{Agent: "bob", Handle: "%1", Branch: "work"}}}}
	if why := r.Takes("n1"); why != "" {
		t.Fatalf("a runner heard now on the master's version on run: %q", why)
	}
	got, err := r.Reclaim("n1", "its device was revoked")
	if err != nil || len(got) != 1 || got[0].Agent != "bob" || got[0].Task != 1 {
		t.Fatalf("taken back %+v (%v)", got, err)
	}
	if !strings.Contains(got[0].Memo, "revoked") || !strings.Contains(got[0].Memo, "Nothing on work was pushed") {
		t.Fatalf("memo %q", got[0].Memo)
	}
	if again, _ := r.Reclaim("n1", "again"); len(again) != 0 {
		t.Fatalf("taken back twice: %+v", again)
	}
}

// What a runner says decides whether it takes a new session: heard within 60 s, on the master's version, and on Run.
func TestTakesNeedsAHeartbeatTheVersionAndRun(t *testing.T) {
	r := registry(t)
	now := time.Now()
	r.Clock = func() time.Time { return now }
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
