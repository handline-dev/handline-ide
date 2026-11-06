package paths

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// RealUserHome is the user's home from the user database, not $HOME, so a test that sets HOME cannot move what the
// guard protects (paths._real_user_home). Without cgo, darwin's os/user still asks libc.
var RealUserHome = func() string {
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir
	}
	h, _ := os.UserHomeDir()
	return h
}

// IsLive is whether home is, or is inside, the real user's ~/.handline.
func IsLive(home string) bool {
	real := RealUserHome()
	if real == "" {
		return false
	}
	h, live := Resolved(home), Resolved(filepath.Join(real, ".handline"))
	return h == live || strings.HasPrefix(h, live+string(filepath.Separator))
}

// TmuxSocket is the tmux server (tmux -L) the daemon of home keeps its terminals on: TMUX_SOCKET when set, else "handline" for the
// live home and "handline-" plus a hash of the home for any other. A scratch home never gets "handline", even when TMUX_SOCKET says so:
// that is the live server, and a scratch daemon on it puts windows among the live agents' (backends/tmux._socket).
func TmuxSocket(home string) string {
	sock, _ := Getenv("TMUX_SOCKET")
	live := IsLive(home)
	switch {
	case live && sock == "":
		return liveTmuxSocket
	case live || (sock != "" && sock != liveTmuxSocket):
		return sock
	}
	sum := sha256.Sum256([]byte(Resolved(home)))
	return fmt.Sprintf("%s-%x", liveTmuxSocket, sum[:4])
}

const liveTmuxSocket = "handline"

// Resolved is p absolute, with ~/ expanded and the links in the part that exists followed.
func Resolved(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(h, p[2:])
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	// As Python's resolve(): the part that exists through its links, the rest as written.
	var rest []string
	for dir := filepath.Clean(p); ; dir = filepath.Dir(dir) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(append([]string{real}, rest...)...)
		}
		if dir == filepath.Dir(dir) {
			return p
		}
		rest = append([]string{filepath.Base(dir)}, rest...)
	}
}
