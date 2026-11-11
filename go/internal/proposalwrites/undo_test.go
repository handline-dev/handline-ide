package proposalwrites

import (
	"os"
	"testing"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/storetest"
)

// An agent another path committed while the proposal ran keeps its folder; the one the proposal's rolled-back add made does not.
func TestUndoRemovesOnlyTheFoldersOfTheAgentsItsOwnAddsMade(t *testing.T) {
	home := t.TempDir()
	d, err := storetest.OpenAt(t, paths.DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	snap, err := takeSnapshot(home, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.drop()
	if _, err := d.Exec("INSERT INTO workspaces(id, name, folder, created_at) VALUES(1, 'w', '', 0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec("INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(6, 1, 'theirs', 'p1', 0)"); err != nil {
		t.Fatal(err)
	}
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(7, 1, 'mine', 'p1', 0)"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"a6", "a7"} {
		os.MkdirAll(paths.AgentDir(home, k), 0o755)
		os.MkdirAll(paths.CacheDir(home, k), 0o755)
	}
	snap.undo(tx, d)
	if _, err := os.Stat(paths.AgentDir(home, "a6")); err != nil {
		t.Fatal("the agent committed meanwhile lost its folder")
	}
	for _, f := range []string{paths.AgentDir(home, "a7"), paths.CacheDir(home, "a7")} {
		if _, err := os.Stat(f); err == nil {
			t.Fatalf("%s stays, for the next agent to be given", f)
		}
	}
}
