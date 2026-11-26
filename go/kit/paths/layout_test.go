package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/handline/handline/go/kit/store"
)

func TestListsAreSortedAndNeedTheirMarker(t *testing.T) {
	home := t.TempDir()
	seedAgents(t, home)
	d, err := store.Shared(DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec("INSERT INTO accounts(id, name, owner, created_at) VALUES(1, 'z', 'p1', 0), (2, 'm', 'p1', 0), (3, 'gone', 'p1', 0)" +
		"; INSERT INTO accounts(id, name, owner, created_at, folder) VALUES(4, 'h', 'p1', 0, '9f3e0b')"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"accounts/c1", "accounts/c2", "accounts/.hidden", "accounts/m", "accounts/9f3e0b"} {
		os.MkdirAll(filepath.Join(home, f), 0o755)
	}
	if got := ListAgents(home); !reflect.DeepEqual(got, []string{"a2", "a1"}) {
		t.Errorf("%v", got)
	}
	if got := ListAccounts(home); !reflect.DeepEqual(got, []string{"h", "m", "z"}) {
		t.Errorf("an account is a row whose folder is there: %v", got)
	}
	if got := ListAgents(filepath.Join(home, "none")); got == nil || len(got) != 0 {
		t.Errorf("%#v", got)
	}
	if AccountDir(home, "m") != filepath.Join(home, "accounts", "c2") || AccountDir(home, "c2") != AccountDir(home, "m") ||
		AccountOfFolder(home, "c2") != "m" || AccountOfFolder(home, "c9") != "" || AccountOfFolder(home, "a1") != "" {
		t.Error("a folder named by its key, found from the key and the name, and the name from the folder")
	}
	hex := filepath.Join(home, "accounts", "9f3e0b")
	if AccountDir(home, "h") != hex || AccountDir(home, "c4") != hex || AccountOfFolder(home, "9f3e0b") != "h" ||
		AccountOfFolder(home, "c4") != "" || AccountFolder(home, "h") != "9f3e0b" {
		t.Error("a folder the row records, found from the key and the name, and the name from the folder")
	}
	os.MkdirAll(filepath.Join(home, "accounts", "stale"), 0o755)
	if AccountDir(home, "stale") != filepath.Join(home, "accounts", ".unregistered", "stale") || AccountOfFolder(home, "") != "" {
		t.Error("a home with an accounts table never names a folder by itself")
	}
	bare := t.TempDir()
	os.MkdirAll(filepath.Join(bare, "accounts", "9f3e0b"), 0o755)
	if AccountDir(bare, "9f3e0b") != filepath.Join(bare, "accounts", "9f3e0b") || AccountDir(bare, "c7") != filepath.Join(bare, "accounts", "c7") {
		t.Error("a home with no accounts table (a lent machine) names a folder by itself, and a key is its own folder")
	}
	if got := AccountDir(home, "nobody"); got != filepath.Join(home, "accounts", ".unregistered", "nobody") {
		t.Errorf("an account no row has: %s", got)
	}
	os.WriteFile(filepath.Join(home, "accounts/c2/.removing"), nil, 0o644)
	if !Removing(home, "m") || Removing(home, "z") {
		t.Error("removing")
	}
}

func TestSetenvSetsTheHandlineName(t *testing.T) {
	env := map[string]string{}
	Setenv(env, "HOME", "/h")
	if len(env) != 1 || env["HANDLINE_HOME"] != "/h" {
		t.Errorf("%v", env)
	}
}

func TestFilesUnderTheHome(t *testing.T) {
	if StatusFile("/h", "c3") != "/h/accounts/c3/status.json" || AgentStatusFile("/h", "a3") != "/h/agents/a3/status.json" ||
		HandoffNoteFile("/h", "a3") != "/h/agents/a3/handoff.md" || MCPConfig("/h") != "/h/shared/mcp.json" {
		t.Error("layout")
	}
}
