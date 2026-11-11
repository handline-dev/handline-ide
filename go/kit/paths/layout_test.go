package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListsAreSortedAndNeedTheirMarker(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{"accounts/z", "accounts/.hidden", "accounts/m"} {
		os.MkdirAll(filepath.Join(home, d), 0o755)
	}
	os.WriteFile(filepath.Join(home, "accounts/file"), nil, 0o644)
	seedAgents(t, home)
	if got := ListAgents(home); !reflect.DeepEqual(got, []string{"w", "w.x"}) {
		t.Errorf("%v", got)
	}
	if got := ListAccounts(home); !reflect.DeepEqual(got, []string{"m", "z"}) {
		t.Errorf("%v", got)
	}
	if got := ListAgents(filepath.Join(home, "none")); got == nil || len(got) != 0 {
		t.Errorf("%#v", got)
	}
	os.WriteFile(filepath.Join(home, "accounts/m/.removing"), nil, 0o644)
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
	if StatusFile("/h", "acct") != "/h/accounts/acct/status.json" || AgentStatusFile("/h", "a3") != "/h/agents/a3/status.json" ||
		HandoffNoteFile("/h", "a3") != "/h/agents/a3/handoff.md" || MCPConfig("/h") != "/h/shared/mcp.json" {
		t.Error("layout")
	}
}
