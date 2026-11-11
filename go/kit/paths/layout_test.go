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
	if _, err := d.Exec("INSERT INTO accounts(id, name, owner, created_at) VALUES(1, 'z', 'p1', 0), (2, 'm', 'p1', 0), (3, 'gone', 'p1', 0)"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"accounts/c1", "accounts/c2", "accounts/.hidden", "accounts/m"} {
		os.MkdirAll(filepath.Join(home, f), 0o755)
	}
	if got := ListAgents(home); !reflect.DeepEqual(got, []string{"a2", "a1"}) {
		t.Errorf("%v", got)
	}
	if got := ListAccounts(home); !reflect.DeepEqual(got, []string{"m", "z"}) {
		t.Errorf("an account is a row whose key folder is there: %v", got)
	}
	if got := ListAgents(filepath.Join(home, "none")); got == nil || len(got) != 0 {
		t.Errorf("%#v", got)
	}
	if AccountDir(home, "m") != filepath.Join(home, "accounts", "c2") || AccountDir(home, "c2") != AccountDir(home, "m") ||
		AccountName(home, "c2") != "m" || AccountName(home, "c9") != "" || AccountName(home, "a1") != "" {
		t.Error("an account's folder by key, and its name from the key")
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
