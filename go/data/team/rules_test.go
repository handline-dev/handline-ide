package team

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
)

// workspaces gives home's database a workspace row for each name.
func workspaces(t *testing.T, home string, names ...string) {
	t.Helper()
	d, err := storetest.OpenAt(t, paths.DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, n := range names {
		if _, err := d.Exec("INSERT INTO workspaces(name, folder, created_at) VALUES(?, '', 0)", n); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { store.ForgetShared(paths.DBFile(home)) })
}

func TestTeamRulesAreTheTeamsTheAgentIsInWithText(t *testing.T) {
	home := t.TempDir()
	workspaces(t, home, "zeta", "alpha", "empty", "other", "nofile")
	for name, text := range map[string]string{"zeta": "  last\n", "alpha": "\nfirst rule\n", "empty": " \n", "other": "not mine"} {
		if err := os.MkdirAll(filepath.Dir(RulesFile(home, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(RulesFile(home, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tm := &Team{Teams: map[string][]string{"zeta": {"a"}, "alpha": {"b", "a"}, "empty": {"a"}, "other": {"b"}, "nofile": {"a"}}}
	want := []string{"# Rules for team alpha\nfirst rule", "# Rules for team zeta\nlast"}
	if got := Rules(home, "a", tm, noKept); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if got := Rules(home, "a", nil, noKept); got != nil {
		t.Errorf("no routing: %q", got)
	}
	if RulesFile(home, "alpha") != filepath.Join(home, "workspaces", paths.WorkspaceKey(home, "alpha"), "base.md") {
		t.Error("a team's rules are its workspace's base.md, by key")
	}
}

func TestSeedRulesWritesEachNewTeamTheDefaultOnce(t *testing.T) {
	home := t.TempDir()
	workspaces(t, home, "a", "own", "b")
	read := func(name string) (string, bool) {
		b, err := os.ReadFile(RulesFile(home, name))
		return string(b), err == nil
	}
	if err := os.MkdirAll(filepath.Dir(RulesFile(home, "own")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(RulesFile(home, "own"), []byte("our rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tm := &Team{Teams: map[string][]string{"a": {"x"}, "own": {"y"}, "nobody": {"q"}}}
	if err := SeedRules(home, tm); err != nil {
		t.Fatal(err)
	}
	if got, _ := read("a"); got != DefaultRules {
		t.Errorf("a new team is not seeded: %q", got)
	}
	if got, _ := read("own"); got != "our rule\n" {
		t.Errorf("a team's own rules were overwritten: %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, "workspaces", paths.Unregistered)); err == nil {
		t.Error("a team with no workspace row was seeded under .unregistered")
	}
	os.Remove(RulesFile(home, "a"))
	tm.Teams["b"] = []string{"z"}
	if err := SeedRules(home, tm); err != nil {
		t.Fatal(err)
	}
	if _, ok := read("a"); ok {
		t.Error("a team whose file was deleted was seeded again")
	}
	if got, _ := read("b"); got != DefaultRules {
		t.Errorf("a team added later is not seeded: %q", got)
	}
}

func noKept(string) string { return "" }
