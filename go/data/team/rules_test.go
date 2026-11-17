package team

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
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

// shared is home's database, as the daemon has it open.
func shared(t *testing.T, home string) *sql.DB {
	t.Helper()
	d, err := store.SharedIfThere(paths.DBFile(home))
	if err != nil || d == nil {
		t.Fatalf("no database: %v", err)
	}
	return d
}

// setRules keeps text as the rules of workspace name, by its team.
func setRules(t *testing.T, d *sql.DB, name, text string) {
	t.Helper()
	k, err := paths.WorkspaceKeyIn(d, name)
	if err == nil {
		_, err = db.SetInstruction(d, k, db.PartRules, text, "a1", "", 1)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestTeamRulesAreTheTeamsTheAgentIsInWithText(t *testing.T) {
	home := t.TempDir()
	workspaces(t, home, "zeta", "alpha", "empty", "other", "nofile")
	d := shared(t, home)
	for name, text := range map[string]string{"zeta": "  last\n", "alpha": "\nfirst rule\n", "empty": " \n", "other": "not mine"} {
		setRules(t, d, name, text)
	}
	tm := &Team{Teams: map[string][]string{"zeta": {"a"}, "alpha": {"b", "a"}, "empty": {"a"}, "other": {"b"}, "nofile": {"a"}}}
	want := []string{"# Rules for team alpha\nfirst rule", "# Rules for team zeta\nlast"}
	if got := Rules(d, "a", tm, noKept); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if got := Rules(d, "a", nil, noKept); got != nil {
		t.Errorf("no routing: %q", got)
	}
	if got := RuleTeams(d, "a", tm); !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Errorf("rule teams: %q", got)
	}
}

func TestSeedRulesGivesEachNewTeamTheDefaultOnce(t *testing.T) {
	home := t.TempDir()
	workspaces(t, home, "a", "own", "b")
	d := shared(t, home)
	read := func(name string) (string, bool) {
		k, _ := paths.WorkspaceKeyIn(d, name)
		in, err := db.CurrentInstruction(d, k, db.PartRules)
		if err != nil || in == nil {
			return "", false
		}
		return in.Text, true
	}
	setRules(t, d, "own", "our rule")
	tm := &Team{Teams: map[string][]string{"a": {"x"}, "own": {"y"}, "nobody": {"q"}}}
	if err := SeedRules(d, tm, 5); err != nil {
		t.Fatal(err)
	}
	if got, _ := read("a"); got != strings.TrimSpace(DefaultRules) {
		t.Errorf("a new team is not seeded: %q", got)
	}
	if got, _ := read("own"); got != "our rule" {
		t.Errorf("a team's own rules were overwritten: %q", got)
	}
	setRules(t, d, "a", "")
	tm.Teams["b"] = []string{"z"}
	if err := SeedRules(d, tm, 6); err != nil {
		t.Fatal(err)
	}
	if got, _ := read("a"); got != "" {
		t.Errorf("a team that emptied its rules was seeded again: %q", got)
	}
	if got, _ := read("b"); got != strings.TrimSpace(DefaultRules) {
		t.Errorf("a team added later is not seeded: %q", got)
	}
}

func noKept(string) string { return "" }
