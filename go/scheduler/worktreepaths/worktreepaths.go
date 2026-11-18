// Package worktreepaths reads the worktree directories an agent's row records.
package worktreepaths

import (
	"os"
	"path/filepath"
	"strings"
)

// Dir is where repo's worktrees live: a sibling named <repo>.worktrees.noindex, because macOS Spotlight skips everything under a folder
// with the .noindex suffix and would otherwise index every node_modules and build output the agents churn. Worktrees made before that
// live in <repo>.worktrees and stay there.
func Dir(repo string) string { return repo + ".worktrees.noindex" }

// WorktreePath is the directory a worktree column records, "" when there is none or it is gone.
func WorktreePath(recorded string) string {
	path := RecordedPath(recorded)
	if st, err := os.Stat(path); path != "" && err == nil && st.IsDir() {
		return path
	}
	return ""
}

// RecordedPath is the path a worktree column holds (it may be `<repo>::<path>`); "" for none, or for a relative one, which would be read
// against the daemon's own working directory.
func RecordedPath(recorded string) string {
	path := recorded
	if i := strings.Index(recorded, "::"); i >= 0 {
		path = recorded[i+2:]
	}
	if !filepath.IsAbs(path) {
		return ""
	}
	return path
}
