// Package lifecycle is the Python daemon's session lifecycle: the idle-close settings (session/idle_close.py), and starting, parking,
// waking and recovering sessions (daemon/lifecycle.py, reconciler.py, session/launch.py, team/spawn.py, usage/recover.py). What it
// does not own it reaches through the small interfaces in deps.go; tests use fakes.
package lifecycle

import (
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/kit/paths"
)

func expandUser(p string) string {
	if p == "~" {
		return paths.UserHome()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(paths.UserHome(), p[2:])
	}
	return p
}

// ExpandRepo is a workspace folder as a path git can run in: ~ taken to the home, links resolved; "" stays "".
func ExpandRepo(p string) string {
	if p == "" {
		return ""
	}
	return realpath(expandUser(p))
}

// realpath resolves symlinks as far as the path exists, like os.path.realpath.
func realpath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	rest := ""
	for cur := abs; ; cur = filepath.Dir(cur) {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest)
		}
		if cur == filepath.Dir(cur) {
			return abs
		}
		rest = filepath.Join(filepath.Base(cur), rest)
	}
}
