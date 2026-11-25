package lifecycle

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
)

// A session launched with some rules is told the lines that changed; once that is kept it is not told again, and the rules are no
// drift that would relaunch it. A launch that wrote its record since keeps its own.
func TestARulesChangeIsToldOnceAndIsNoDrift(t *testing.T) {
	r := newLaunchRig(t)
	rules := "# Rules for team core\n- keep it short\n- reply in English"
	r.l.Ports.TeamRules = func(string) []string { return []string{rules} }
	r.l.Ports.Routing = func() *team.Team { return &team.Team{Teams: map[string][]string{"core": {"ann"}}} }
	r.rulesOf(t, "x", "core")
	core := "core"
	rec, err := r.l.LaunchRecord("ann")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetLaunchParts(r.l.Q, "ann", rec); err != nil {
		t.Fatal(err)
	}
	row := func() *db.AgentState {
		s, err := db.GetAgentState(r.l.Q, "ann")
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if note, kept, _ := r.l.RulesNote("ann", row()); note != "" || kept != "" {
		t.Fatalf("unchanged rules told: %q", note)
	}
	rules = "# Rules for team core\n- keep it short\n- reply in Korean"
	was := row()
	note, kept, err := r.l.RulesNote("ann", was)
	want := "Your team rules changed (" + core + "). Your session goes on; follow the new rules from now.\n\nAdded:\n+ - reply in Korean\n\nRemoved:\n- - reply in English"
	if err != nil || note != want {
		t.Fatalf("%q, %v", note, err)
	}
	if again, _, _ := r.l.RulesNote("ann", row()); again != note {
		t.Fatal("nothing kept, and the note was not the same again")
	}
	if err := r.l.KeepRulesTold(r.l.Q, "ann", was.LaunchParts.String, kept); err != nil {
		t.Fatal(err)
	}
	if note, _, _ := r.l.RulesNote("ann", row()); note != "" {
		t.Fatalf("told twice: %q", note)
	}
	if got, _, _ := r.l.whatChanged("ann"); got != "" {
		t.Fatalf("a later launch would still name the told rules: %q", got)
	}
	// A launch that recorded itself between the note and its keeping is not overwritten.
	rules = "# Rules for team core\n- new"
	stale := row()
	_, kept, _ = r.l.RulesNote("ann", stale)
	fresh, _ := r.l.LaunchRecord("ann")
	if err := db.SetLaunchParts(r.l.Q, "ann", fresh); err != nil {
		t.Fatal(err)
	}
	if err := r.l.KeepRulesTold(r.l.Q, "ann", stale.LaunchParts.String, kept); err != nil || row().LaunchParts.String != fresh {
		t.Fatalf("a new launch's record was overwritten: %v", err)
	}
	// A record from before the text was kept is told the whole rules.
	var old map[string]any
	_ = json.Unmarshal([]byte(row().LaunchParts.String), &old)
	delete(old, LaunchedRules)
	old[rulesPart] = "0badc0de"
	b, _ := json.Marshal(old)
	if err := db.SetLaunchParts(r.l.Q, "ann", string(b)); err != nil {
		t.Fatal(err)
	}
	if note, _, _ := r.l.RulesNote("ann", row()); !strings.HasSuffix(note, "They now read:\n\n"+rules) {
		t.Fatalf("%q", note)
	}
}

// ROLE.md moves its part, and the rules still reach the system prompt.
func TestROLEmdMovesItsPartAndTheRulesStayInThePrompt(t *testing.T) {
	r := newLaunchRig(t)
	rules := "RULES-A"
	r.l.Ports.TeamRules = func(string) []string { return []string{rules} }
	rules = "RULES-B"
	before, _ := partsJSON(r.l, "ann")
	r.setPart(t, db.PartRole, "a new role")
	if after, _ := partsJSON(r.l, "ann"); after == before {
		t.Fatal("ROLE.md did not move the fingerprint")
	}
	if ctx, _ := r.l.ContextFor("ann"); !strings.Contains(ctx, "RULES-B") {
		t.Fatal("the rules left the system prompt")
	}
}

func TestLineChangesKeepsOrderAndSkipsBlankLines(t *testing.T) {
	added, removed := lineChanges("a\nb\n\nc", "a\nx\nc\n\ny")
	if strings.Join(added, "|") != "x|y" || strings.Join(removed, "|") != "b" {
		t.Fatalf("%q %q", added, removed)
	}
}

// rulesOf gives each named team a workspace row with text as its rules.
func (r *launchRig) rulesOf(t *testing.T, text string, names ...string) {
	t.Helper()
	for _, n := range names {
		res, err := r.l.Q.Exec("INSERT INTO workspaces(name, folder, created_at) VALUES(?, '', 0)", n)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if _, err := db.SetInstruction(r.l.Q, actor.Key(actor.Workspace, id), db.PartRules, text, "p1", "", 1); err != nil {
			t.Fatal(err)
		}
	}
}

// setPart keeps text as ann's part, by the founder.
func (r *launchRig) setPart(t *testing.T, part, text string) {
	t.Helper()
	key, err := paths.AgentKeyIn(r.l.Q, "ann")
	if err == nil {
		_, err = db.SetInstruction(r.l.Q, key, part, text, actor.OwnerKey, "", time.Now().Unix())
	}
	if err != nil {
		t.Fatal(err)
	}
}

// A resume keeps the rules its conversation began with, so rules changed before it are still told after it.
func TestRulesChangedBeforeAResumeAreStillTold(t *testing.T) {
	r := newLaunchRig(t)
	rules := "# Rules for team core\n- keep it short"
	r.l.Ports.TeamRules = func(string) []string { return []string{rules} }
	if err := r.prepare("ann", nil, false, "", "main", "launch", ""); err != nil {
		t.Fatal(err)
	}
	rules = "# Rules for team core\n- keep it short\n- reply in Korean"
	if err := r.prepare("ann", []string{"--resume", "s1"}, false, "", "main", "wake", ""); err != nil {
		t.Fatal(err)
	}
	row, err := db.GetAgentState(r.l.Q, "ann")
	if err != nil {
		t.Fatal(err)
	}
	if note, _, err := r.l.RulesNote("ann", row); err != nil || !strings.Contains(note, "+ - reply in Korean") {
		t.Fatalf("the changed rules are not told after the resume: %q %v", note, err)
	}
}
