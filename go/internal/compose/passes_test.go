package compose

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/scheduler/session"
	"github.com/handline/handline/go/scheduler/settings"
	"github.com/handline/handline/go/scheduler/tick"
	"github.com/handline/handline/go/taskboard/supervisor"
)

func TestTheBackgroundCheckNamesWhatTheLastStopSaidRunsAndWhenEachStarted(t *testing.T) {
	p, _ := rig(t)
	tr := &turns.Tracker{Agent: func(string) string { return "alice" }}
	tr.Hook("alice", "UserPromptSubmit", map[string]any{}, 10)
	tr.Hook("alice", "Stop", map[string]any{"background_tasks": []any{
		map[string]any{"id": "a3", "type": "agent", "status": "running", "description": "review the diff"},
		map[string]any{"id": "b7", "type": "shell", "status": "running", "command": "go test ./..."},
	}}, 11)
	// b7's start is its background_started event in this session, not one from before; a3, a subagent, has none: the first Stop that
	// listed it.
	for _, e := range []struct {
		ts   int64
		kind string
		kv   dbpkg.KV
	}{
		{2, session.BackgroundKind, dbpkg.KV{K: "job", V: "b7"}},
		{5, "SessionStart", dbpkg.KV{K: "source", V: "resume"}},
		{8, "Stop", dbpkg.KV{K: "background_tasks", V: []any{map[string]any{"id": "a3"}}}},
		{9, session.BackgroundKind, dbpkg.KV{K: "job", V: "b7"}},
	} {
		if _, err := dbpkg.AddEventAt(p.db, e.ts, "alice", e.kind, e.kv); err != nil {
			t.Fatal(err)
		}
	}
	p.turns = tr
	ports := timeboxPorts{p: p}
	want := []tick.BgJob{{Name: "agent: review the diff", Started: 8}, {Name: "shell: go test ./...", Started: 9}}
	if got := ports.BgJobs(p.db, "alice", 0, 0); !slices.Equal(got, want) {
		t.Fatalf("named %v", got)
	}
	if got := ports.BgJobs(p.db, "alice", 1, 0); !slices.Equal(got, want[1:]) {
		t.Fatalf("with a count of 1: %v", got)
	}
	if got := (timeboxPorts{p: &Parts{}}).BgJobs(p.db, "alice", 0, 0); got != nil {
		t.Fatalf("no tracker named %v", got)
	}
}

// The time box reads blocker_alarm and its two thresholds from config.yaml, each in its place; unset, the alarm is off at 1h and 4h.
func TestTheBlockerAlarmSettingsReachTheTimebox(t *testing.T) {
	home := t.TempDir()
	ports := timeboxPorts{set: &settings.Settings{Home: home}}
	if on, h, l := ports.BlockerAlarm(); on || h != 3600 || l != 14400 {
		t.Fatalf("defaults: %v %d %d", on, h, l)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("blocker_alarm: true\nblocker_alarm_s: 5\nblocker_alarm_opener_s: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if on, h, l := ports.BlockerAlarm(); !on || h != 5 || l != 7 {
		t.Fatalf("set: %v %d %d", on, h, l)
	}
}

// The time box's TellAbout is the supervisor's: an agent gets a notice linked to the task, the founder a warning.
func TestTheTimeboxTellsThroughTheSupervisor(t *testing.T) {
	p, home := rig(t)
	alice, bob := teamtest.Key(t, home, "alice"), teamtest.Key(t, home, "bob")
	env := &supervisor.Env{DB: p.db, Home: home, Log: func(string, ...any) {}}
	t.Cleanup(env.Wait)
	tid, err := dbpkg.NewTask(p.db, "the work", bob, alice, 0, dbpkg.DefaultPriority, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	ports := timeboxPorts{env: env}
	if err := ports.TellAbout(p.db, alice, bob, tid, "past its timeout", "task_timeout", 100); err != nil {
		t.Fatal(err)
	}
	var linked int
	p.db.QueryRow("SELECT COUNT(*) FROM task_requests l JOIN requests r ON r.id = l.request_id WHERE l.task_id = ? AND r.target = ?", tid, alice).Scan(&linked)
	if linked != 1 {
		t.Fatalf("alice gets a notice on the task: %d", linked)
	}
	if err := ports.TellAbout(p.db, actor.OwnerKey, bob, 0, "stale", "stale_background", 100); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := dbpkg.Meta(p.db, dbpkg.FounderNoticePrefix+bob+":stale_background"); !ok {
		t.Fatal("the founder gets a warning")
	}
}
