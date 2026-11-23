package reads

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	_ "github.com/handline/handline/go/agentctl/agent/claudecode" // opencode left out on purpose: see accounts_test
	_ "github.com/handline/handline/go/agentctl/agent/codex"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/termchan"
	"github.com/handline/handline/go/protocol/rpc"
)

const codexFix = "npm install -g @openai/codex"

// daemon.status.vendors lists every launchable vendor, installed or not, and claude at the path agents are run with.
func TestVendorsListsEachLaunchableVendorAndWhetherItIsInstalled(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	e := testEnv(t)
	e.Claude = tool(t, t.TempDir(), "my-claude")
	want := []VendorStatus{
		{Vendor: "claude", Binary: "claude", Installed: true, Fix: "curl -fsSL https://claude.ai/install.sh | bash"},
		{Vendor: "codex", Binary: "codex", Installed: false, Fix: codexFix},
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
	if got, want := e.missingTools(e.vendors()), []MissingTool{{Tool: "codex", Fix: codexFix}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("codex in use, not installed: %v", got)
	}
	tool(t, bin, "codex")
	if got := e.missingTools(e.vendors()); got != nil {
		t.Fatalf("codex in use and installed: %v", got)
	}
}

// daemon.status carries a tool's install window on its missing_tools entry and its vendors row while the tool is not installed.
func TestStatusCarriesEachMissingToolsInstall(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	tool(t, bin, "claude")
	e := testEnv(t)
	e.Claude = "claude"
	codexAccount(t, e)
	e.Installs = func() map[string]termchan.ToolInstall {
		return map[string]termchan.ToolInstall{"codex": {Terminal: "%3", Ended: true}, "claude": {Terminal: "%2"}, "git": {Terminal: "%1"}}
	}
	st, err := e.Status(&rpc.Server{})
	if err != nil {
		t.Fatal(err)
	}
	installs := map[string]any{}
	for _, m := range st["missing_tools"].([]MissingTool) {
		installs["missing "+m.Tool] = m.Install
	}
	for _, v := range st["vendors"].([]VendorStatus) {
		installs["vendor "+v.Vendor] = v.Install
	}
	want := map[string]any{
		"missing git": &termchan.ToolInstall{Terminal: "%1"}, "missing tmux": (*termchan.ToolInstall)(nil),
		"missing codex": &termchan.ToolInstall{Terminal: "%3", Ended: true},
		"vendor claude": (*termchan.ToolInstall)(nil), "vendor codex": &termchan.ToolInstall{Terminal: "%3", Ended: true},
	}
	if !reflect.DeepEqual(installs, want) {
		t.Fatalf("%v", installs)
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
