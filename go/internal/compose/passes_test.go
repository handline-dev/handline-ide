package compose

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	"github.com/handline/handline/go/scheduler/settings"
)

func TestTheBackgroundCheckNamesWhatTheLastStopSaidRuns(t *testing.T) {
	tr := &turns.Tracker{Agent: func(string) string { return "alice" }}
	tr.Hook("alice", "UserPromptSubmit", map[string]any{}, 10)
	tr.Hook("alice", "Stop", map[string]any{"background_tasks": []any{
		map[string]any{"type": "agent", "status": "running", "description": "review the diff"},
		map[string]any{"type": "shell", "status": "running", "command": "go test ./..."},
	}}, 11)
	ports := timeboxPorts{p: &Parts{turns: tr}}
	if got := ports.BgJobs(nil, "alice", 0, 0); !slices.Equal(got, []string{"agent: review the diff", "shell: go test ./..."}) {
		t.Fatalf("named %q", got)
	}
	if got := ports.BgJobs(nil, "alice", 1, 0); !slices.Equal(got, []string{"shell: go test ./..."}) {
		t.Fatalf("with a count of 1: %q", got)
	}
	if got := (timeboxPorts{p: &Parts{}}).BgJobs(nil, "alice", 0, 0); got != nil {
		t.Fatalf("no tracker named %q", got)
	}
}

// The time box reads blocker_alarm and its two thresholds from config.yaml, each in its place; unset, the alarm is off at 1h and 4h.
func TestTheBlockerAlarmSettingsReachTheTimebox(t *testing.T) {
	home := t.TempDir()
	ports := timeboxPorts{set: &settings.Settings{Home: home}}
	if on, h, l := ports.BlockerAlarm(); on || h != 3600 || l != 14400 {
		t.Fatalf("defaults: %v %d %d", on, h, l)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("blocker_alarm: true\nblocker_alarm_s: 5\nblocker_alarm_lead_s: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if on, h, l := ports.BlockerAlarm(); !on || h != 5 || l != 7 {
		t.Fatalf("set: %v %d %d", on, h, l)
	}
}
