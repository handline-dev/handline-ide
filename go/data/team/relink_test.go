package team

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRelinkLinksOnlyWhatGainedATargetAndLeavesWhatIsThere(t *testing.T) {
	home := t.TempDir()
	cdir := filepath.Join(accountDir(t, home, "a1"), ".claude")
	os.MkdirAll(cdir, 0o755)
	os.MkdirAll(filepath.Join(home, "shared", ".claude", "skills"), 0o755)
	os.WriteFile(filepath.Join(home, "shared", ".claude", "CLAUDE.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(home, "shared", ".claude", "settings.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(cdir, "settings.json"), []byte("mine"), 0o644) // a real file: never replaced
	got := RelinkAll(home, func(s string) { t.Log(s) })
	if want := map[string][]string{"a1": {"CLAUDE.md", "skills"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if to, _ := os.Readlink(filepath.Join(cdir, "skills")); to != filepath.Join(home, "shared", ".claude", "skills") {
		t.Fatalf("skills links to %q", to)
	}
	if b, _ := os.ReadFile(filepath.Join(cdir, "settings.json")); string(b) != "mine" {
		t.Fatal("a real file was replaced")
	}
	if again := RelinkAll(home, func(string) {}); len(again) != 0 {
		t.Fatalf("a second pass has nothing to do: %v", again)
	}
	os.MkdirAll(filepath.Join(accountDir(t, home, "bare")), 0o755) // no config dir: register never ran on it
	if got := RelinkAll(home, func(string) {}); len(got) != 0 {
		t.Fatal(got)
	}
}

// A codex account links the shared hooks.json, and only that, into its CODEX_HOME.
func TestRelinkLinksACodexAccountsHooks(t *testing.T) {
	home := t.TempDir()
	dir := accountDir(t, home, "cx")
	cdir := filepath.Join(dir, ".codex")
	os.MkdirAll(cdir, 0o755)
	os.WriteFile(filepath.Join(dir, "vendor"), []byte("codex\n"), 0o644)
	os.MkdirAll(filepath.Join(home, "shared", ".codex"), 0o755)
	os.WriteFile(filepath.Join(home, "shared", ".codex", "hooks.json"), []byte("{}"), 0o644)
	if got := RelinkAll(home, func(s string) { t.Log(s) }); !reflect.DeepEqual(got, map[string][]string{"cx": {"hooks.json"}}) {
		t.Fatalf("%v", got)
	}
	if to, _ := os.Readlink(filepath.Join(cdir, "hooks.json")); to != filepath.Join(home, "shared", ".codex", "hooks.json") {
		t.Fatalf("%q", to)
	}
}
