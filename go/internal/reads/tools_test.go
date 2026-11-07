package reads

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// tool puts an executable named name in dir.
func tool(t *testing.T, dir, name string) string {
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// daemon.status names each tool agents are run with that the machine lacks, with its fix, and nothing on a daemon with no agent side.
func TestDaemonStatusNamesTheMissingToolsOfTheAgentSide(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	e := testEnv(t)
	all := []MissingTool{{"claude", "Install Claude Code"}, {"git", "brew install git"}, {"tmux", "brew install tmux"}}
	if got := e.missingTools(); got != nil {
		t.Fatalf("no agent side: %v", got)
	}
	e.Claude = "claude"
	if got := e.missingTools(); !reflect.DeepEqual(got, all) {
		t.Fatalf("an empty PATH: %v", got)
	}
	tool(t, bin, "git")
	tool(t, bin, "claude")
	if got := e.missingTools(); !reflect.DeepEqual(got, all[2:]) {
		t.Fatalf("claude and git on PATH: %v", got)
	}
	tool(t, bin, "tmux")
	e.Claude = filepath.Join(bin, "gone", "claude")
	if got := e.missingTools(); !reflect.DeepEqual(got, all[:1]) {
		t.Fatalf("a claude path that is not there: %v", got)
	}
	e.Claude = tool(t, t.TempDir(), "my-claude")
	if got := e.missingTools(); got != nil {
		t.Fatalf("a claude path off PATH: %v", got)
	}
}
