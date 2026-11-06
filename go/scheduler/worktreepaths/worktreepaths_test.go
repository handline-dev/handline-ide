package worktreepaths

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/db"
)

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

func TestAWorktreeIsFoundThroughAnOrcaSelectorOrAPlainPath(t *testing.T) {
	dir := t.TempDir()
	if WorktreePath("repo-id::"+dir) != dir || WorktreePath(dir) != dir || WorktreePath(filepath.Join(dir, "gone")) != "" || WorktreePath("") != "" {
		t.Fatal("selector or path not resolved")
	}
	row := &db.AgentState{Worktree: nullStr(dir), ExtraWorktrees: nullStr(`["` + dir + `", "/nowhere", 3]`)}
	if got := AllPaths(row); len(got) != 2 || got[0] != dir || got[1] != dir {
		t.Fatalf("%v", got)
	}
	if ExtraPaths(&db.AgentState{ExtraWorktrees: nullStr("not json")}) != nil {
		t.Fatal("garbage should list nothing")
	}
}
