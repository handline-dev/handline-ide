package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

// daemon.status carries missing_tools from the claude binary the daemon was given.
func TestDaemonStatusCarriesTheMissingTools(t *testing.T) {
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth = pidAuth{}
	cfg.Claude = filepath.Join(t.TempDir(), "no-claude")
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	f := readAll(t, pipeTo(t, d, "k"), 2)
	r := f.ask(2, `{"jsonrpc":"2.0","id":2,"method":"daemon.status","params":{}}`)["result"].(map[string]any)
	m, _ := r["missing_tools"].([]any)
	if len(m) == 0 || m[0].(map[string]any)["tool"] != "claude" || m[0].(map[string]any)["fix"] != "curl -fsSL https://claude.ai/install.sh | bash" {
		t.Fatalf("missing_tools: %v", r["missing_tools"])
	}
}

// missing_tools is absent, not null, when the daemon runs no agent side and when nothing is missing.
func TestDaemonStatusLeavesOutMissingToolsWhenNoneIsMissing(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	for _, n := range []string{"claude", "git", "tmux"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, claude := range map[string]string{"no agent side": "", "nothing missing": "claude"} {
		cfg := tlsConfig(t, t.TempDir())
		cfg.Auth = pidAuth{}
		cfg.Claude = claude
		d, err := Start(cfg)
		if err != nil {
			t.Fatal(err)
		}
		f := readAll(t, pipeTo(t, d, "k"), 2)
		r := f.ask(2, `{"jsonrpc":"2.0","id":2,"method":"daemon.status","params":{}}`)["result"].(map[string]any)
		d.Close()
		if v, ok := r["missing_tools"]; ok {
			t.Errorf("%s: missing_tools is there: %v", name, v)
		}
	}
}
