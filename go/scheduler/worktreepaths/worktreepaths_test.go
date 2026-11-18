package worktreepaths

import (
	"path/filepath"
	"testing"
)

func TestAWorktreeIsFoundThroughAnIdeASelectorOrAPlainPath(t *testing.T) {
	dir := t.TempDir()
	if WorktreePath("repo-id::"+dir) != dir || WorktreePath(dir) != dir || WorktreePath(filepath.Join(dir, "gone")) != "" || WorktreePath("") != "" {
		t.Fatal("selector or path not resolved")
	}
}
