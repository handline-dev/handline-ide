package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
)

// seedAgents makes home's database with workspace w, led by agent "lead" (a2), and its agent x (a1): listed lead first, a2 then a1.
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
	if list, err := ReadAgents(home); err != nil || !reflect.DeepEqual(list, []string{"a2", "a1"}) {
		t.Fatalf("%v %v", list, err)
	}
	broken := t.TempDir()
	os.WriteFile(DBFile(broken), []byte("not a database"), 0o644)
	t.Cleanup(func() { store.ForgetShared(DBFile(broken)) })
	if _, err := ReadAgents(broken); err == nil {
		t.Fatal("a database that cannot be read is an error")
	}
}

func TestAnAgentsFolderIsNamedByItsKeyWhateverItIsCalled(t *testing.T) {
	home := t.TempDir()
	seedAgents(t, home)
	for who, want := range map[string]string{"w.x": "a1", "w": "a2", "a1": "a1", "w.nobody": ""} {
		if got := AgentKey(home, who); got != want {
			t.Errorf("AgentKey(%s) = %q, want %q", who, got, want)
		}
	}
	if AgentDir(home, "w.x") != filepath.Join(home, "agents", "a1") || CacheDir(home, "w") != filepath.Join(home, "caches", "a2") {
		t.Error("a folder by key")
	}
	if got := AgentDir(home, "w.nobody"); got != filepath.Join(home, "agents", ".unregistered", "w.nobody") {
		t.Errorf("a name no agent has: %s", got)
	}
}

func TestAgentKeyInTakesAKeyOrAName(t *testing.T) {
	home := t.TempDir()
	seedAgents(t, home)
	d, err := store.Shared(DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	for who, want := range map[string]string{"a1": "a1", "a9": "", "w.x": "a1", "w": "a2", "w.nobody": ""} {
		if got, err := AgentKeyIn(d, who); err != nil || got != want {
			t.Errorf("AgentKeyIn(%s) = %q %v, want %q", who, got, err, want)
		}
	}
}
