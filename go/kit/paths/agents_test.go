package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
)

// seedAgents makes home's database with workspace w, led by agent "lead", and its agent x: listed as w and w.x.
func seedAgents(t *testing.T, home string) {
	t.Helper()
	d, err := storetest.OpenAt(t, DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, q := range []string{"INSERT INTO workspaces(id, name, folder, created_at) VALUES(1, 'w', '', 0)",
		"INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(1, 1, 'x', 'p1', 0), (2, 1, 'lead', 'p1', 0)",
		"UPDATE workspaces SET lead = 2"} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { store.ForgetShared(DBFile(home)) })
}

func TestReadAgentsTellsAnUnreadableListingFromAnEmptyOne(t *testing.T) {
	home := t.TempDir()
	if list, err := ReadAgents(home); err != nil || len(list) != 0 {
		t.Fatalf("no database yet: %v %v", list, err)
	}
	if _, err := os.Stat(DBFile(home)); err == nil {
		t.Fatal("a read made the database")
	}
	os.MkdirAll(filepath.Join(AgentsDir(home), "a-folder-only"), 0o755)
	seedAgents(t, home)
	if list, err := ReadAgents(home); err != nil || !reflect.DeepEqual(list, []string{"w", "w.x"}) {
		t.Fatalf("%v %v", list, err)
	}
	broken := t.TempDir()
	os.WriteFile(DBFile(broken), []byte("not a database"), 0o644)
	t.Cleanup(func() { store.ForgetShared(DBFile(broken)) })
	if _, err := ReadAgents(broken); err == nil {
		t.Fatal("a database that cannot be read is an error")
	}
}
