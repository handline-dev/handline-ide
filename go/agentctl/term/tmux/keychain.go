//go:build !windows

package tmux

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
)

// A tmux server stays in the macOS login session it was started in, and every window it opens runs there. A server started over SSH
// cannot reach the login keychain the desktop unlocked, so a claude started in one of its windows finds no credential and ends its
// first turn "Not logged in", however often the account logs in again: the login lands in the keychain, the agent cannot read it, and
// the account is marked expired once more. So before the daemon opens an agent on a server that is already running, it asks that server
// whether its windows reach the keychain, once per server process.

const keychainTimeout = 10 * time.Second

// keychainMark starts the line the probe prints in the server: the exit status of `security show-keychain-info` there.
const keychainMark = "hl-keychain="

// goos and ownKeychain are variables so a test can stand in for them. ownKeychain is whether this process reaches the login keychain.
var (
	goos        = runtime.GOOS
	ownKeychain = func() bool {
		code, _, _ := run(keychainTimeout, "", "security", "show-keychain-info")
		return code == 0
	}
)

// keychainSeen is each socket's last answer, kept while the same server process runs: a new server is asked again.
var (
	keychainMu   sync.Mutex
	keychainSeen = map[string]keychainAnswer{}
)

type keychainAnswer struct {
	pid string
	ok  bool
}

// serverKeychain is whether windows of the running server reach the login keychain; known is false when no server runs or it cannot say.
func (t Tmux) serverKeychain() (ok, known bool) {
	code, out, _ := t.tmux(tmuxTimeout, "", "display-message", "-p", "#{pid}")
	pid := strings.TrimSpace(out)
	if code != 0 || pid == "" {
		return false, false
	}
	keychainMu.Lock()
	seen, had := keychainSeen[t.Socket]
	keychainMu.Unlock()
	if had && seen.pid == pid {
		return seen.ok, true
	}
	code, out, _ = t.tmux(keychainTimeout, "", "run-shell", "security show-keychain-info >/dev/null 2>&1; echo "+keychainMark+"$?")
	if code != 0 {
		return false, false
	}
	for _, line := range strings.Split(out, "\n") {
		status, found := strings.CutPrefix(strings.TrimSpace(line), keychainMark)
		if !found || status == "" {
			continue
		}
		ok = status == "0"
		keychainMu.Lock()
		keychainSeen[t.Socket] = keychainAnswer{pid, ok}
		keychainMu.Unlock()
		return ok, true
	}
	return false, false
}

// keychainRefusal is why an agent may not be opened on the running server, "" when it may. Only a server known to be cut off from the
// keychain, under a process that reaches it, is refused: when this process cannot reach it either, a new server would be no better.
func (t Tmux) keychainRefusal() string {
	if goos != "darwin" {
		return ""
	}
	if ok, known := t.serverKeychain(); !known || ok {
		return ""
	}
	if !ownKeychain() {
		return ""
	}
	return fmt.Sprintf("the tmux server %q was started in another login session (over SSH?) and cannot reach the macOS keychain, so an "+
		"agent started there is not logged in; `tmux -L %s kill-server` closes it and every window in it, and the next agent starts a new one",
		t.Socket, t.Socket)
}
