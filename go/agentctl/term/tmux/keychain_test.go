//go:build !windows

package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/term"
)

// keychainTmux puts a `tmux` on PATH whose running server has pid 42 and whose windows' keychain probe exits with status; it logs
// every call. It stands in for darwin and for this process's own keychain answer, and forgets what an earlier test saw.
func keychainTmux(t *testing.T, status string, own bool) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls")
	script := `#!/bin/sh
printf '%s\n' "$*" >> ` + log + `
case "$4" in
  display-message) echo 42;;
  run-shell) echo "` + keychainMark + status + `";;
  has-session) exit 0;;
  new-window) echo %7;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	oldGoos, oldOwn := goos, ownKeychain
	goos, ownKeychain = "darwin", func() bool { return own }
	keychainMu.Lock()
	keychainSeen = map[string]keychainAnswer{}
	keychainMu.Unlock()
	t.Cleanup(func() { goos, ownKeychain = oldGoos, oldOwn })
	return log
}

var onT = Tmux{Place: term.Place{Key: "/h", Socket: "t"}}

func TestAnAgentIsNotOpenedOnAServerCutOffFromTheKeychain(t *testing.T) {
	log := keychainTmux(t, "36", true)
	handle, why := onT.CreateTerminal("hl launch alice", "/wt/alice", "alice", "alice")
	if handle != "" || !strings.Contains(why, "cannot reach the macOS keychain") || !strings.Contains(why, "tmux -L t kill-server") {
		t.Fatalf("%q %q", handle, why)
	}
	b, _ := os.ReadFile(log)
	if strings.Contains(string(b), "new-window") {
		t.Fatalf("a window was opened: %s", b)
	}
}

func TestAServerThatReachesTheKeychainIsAskedOncePerServerProcess(t *testing.T) {
	log := keychainTmux(t, "0", true)
	for range 2 {
		if handle, why := onT.CreateTerminal("hl launch alice", "/wt/alice", "alice", "alice"); handle != "%7" || why != "" {
			t.Fatalf("%q %q", handle, why)
		}
	}
	b, _ := os.ReadFile(log)
	if n := strings.Count(string(b), "run-shell"); n != 1 {
		t.Fatalf("probed %d times: %s", n, b)
	}
}

func TestAServerIsNotRefusedWhenThisProcessCannotReachTheKeychainEither(t *testing.T) {
	keychainTmux(t, "36", false)
	if handle, why := onT.CreateTerminal("hl launch alice", "/wt/alice", "alice", "alice"); handle != "%7" || why != "" {
		t.Fatalf("%q %q", handle, why)
	}
}

func TestOffMacOSTheKeychainIsNotAsked(t *testing.T) {
	log := keychainTmux(t, "36", true)
	goos = "linux"
	if handle, why := onT.CreateTerminal("hl launch alice", "/wt/alice", "alice", "alice"); handle != "%7" || why != "" {
		t.Fatalf("%q %q", handle, why)
	}
	b, _ := os.ReadFile(log)
	if strings.Contains(string(b), "run-shell") || strings.Contains(string(b), "display-message") {
		t.Fatalf("%s", b)
	}
}
