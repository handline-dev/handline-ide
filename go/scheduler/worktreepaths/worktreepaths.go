// Package worktreepaths reads the worktree directories an agent's row records.
package worktreepaths

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/data/db"
)

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

// ExtraPaths is the agent's further worktrees that are still there, out of what its row holds. A path gone is not listed (and
// EnsureExtras makes it again).
func ExtraPaths(row *db.AgentState) []string {
	if row == nil || row.ExtraWorktrees.String == "" {
		return nil
	}
	var listed []any
	if json.Unmarshal([]byte(row.ExtraWorktrees.String), &listed) != nil {
		return nil
	}
	var out []string
	for _, x := range listed {
		if p, ok := x.(string); ok {
			if wt := WorktreePath(p); wt != "" {
				out = append(out, wt)
			}
		}
	}
	return out
}

// AllPaths is every worktree the agent has: its first, then the others.
func AllPaths(row *db.AgentState) []string {
	var out []string
	if row != nil {
		if first := WorktreePath(row.Worktree.String); first != "" {
			out = append(out, first)
		}
	}
	return append(out, ExtraPaths(row)...)
}
