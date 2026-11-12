package reads

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	_ "github.com/handline/handline/go/agentctl/agent/claudecode" // opencode left out on purpose: see accounts_test
	_ "github.com/handline/handline/go/agentctl/agent/codex"
	"github.com/handline/handline/go/data/team/teamtest"
)

const codexFix = "npm install -g @openai/codex"

// daemon.status.vendors lists every launchable vendor, installed or not, and claude at the path agents are run with.
func TestVendorsListsEachLaunchableVendorAndWhetherItIsInstalled(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	e := testEnv(t)
	e.Claude = tool(t, t.TempDir(), "my-claude")
	want := []VendorStatus{
		{"claude", "claude", true, "curl -fsSL https://claude.ai/install.sh | bash"},
		{"codex", "codex", false, codexFix},
	}
	if got := e.vendors(); !reflect.DeepEqual(got, want) {
		t.Fatalf("codex not on PATH: %+v", got)
	}
	tool(t, bin, "codex")
	want[1].Installed = true
	e.Claude = filepath.Join(bin, "gone")
	want[0].Installed = false
	if got := e.vendors(); !reflect.DeepEqual(got, want) {
		t.Fatalf("codex on PATH, claude's path gone: %+v", got)
	}
}

// missing_tools names codex only while it is not installed and an account or an agent uses it.
func TestMissingToolsNamesCodexOnlyWhileInUse(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	for _, n := range []string{"claude", "git", "tmux"} {
		tool(t, bin, n)
	}
	e := testEnv(t)
	e.Claude = "claude"
	if got := e.missingTools(e.vendors()); got != nil {
		t.Fatalf("no codex account or agent: %v", got)
	}
	codexAccount(t, e)
	if got, want := e.missingTools(e.vendors()), []MissingTool{{"codex", codexFix}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("codex in use, not installed: %v", got)
	}
	tool(t, bin, "codex")
	if got := e.missingTools(e.vendors()); got != nil {
		t.Fatalf("codex in use and installed: %v", got)
	}
}

// The vendors in use are read from the accounts' vendor files and the agents' rows.
func TestVendorsInUseReadsAccountsAndAgents(t *testing.T) {
	e := testEnv(t)
	if got := e.vendorsInUse(); len(got) != 0 {
		t.Fatalf("an empty home: %v", got)
	}
	dir := codexAccount(t, e)
	if got := e.vendorsInUse(); !reflect.DeepEqual(got, map[string]bool{"codex": true}) {
		t.Fatalf("a codex account: %v", got)
	}
	if err := os.Remove(filepath.Join(dir, "vendor")); err != nil {
		t.Fatal(err)
	}
	teamtest.Set(t, e.Home, "ws.dev", "vendor", "opencode")
	if got := e.vendorsInUse(); !reflect.DeepEqual(got, map[string]bool{"claude": true, "opencode": true}) {
		t.Fatalf("a claude account and an opencode agent: %v", got)
	}
}

// codexAccount gives e's home an account "cx" of vendor codex and is its folder.
func codexAccount(t *testing.T, e *Env) string {
	t.Helper()
	dir := teamtest.AccountDir(t, e.Home, "cx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vendor"), []byte("codex\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
