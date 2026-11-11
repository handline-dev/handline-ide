package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
)

// A session launched with some rules is told the lines that changed; once that is kept it is not told again, and the rules are no
// drift that would relaunch it. A launch that wrote its record since keeps its own.
func TestARulesChangeIsToldOnceAndIsNoDrift(t *testing.T) {
	r := newLaunchRig(t)
	rules := "# Rules for team core\n- keep it short\n- reply in English"
	r.l.Ports.TeamRules = func(string) []string { return []string{rules} }
	r.l.Ports.Routing = func() *team.Team { return &team.Team{Teams: map[string][]string{"core": {"ann"}}} }
	dir := filepath.Join(r.l.Home, "shared", "team")
	if err := os.MkdirAll(dir, 0o755); err != nil || os.WriteFile(filepath.Join(dir, "core.md"), []byte("x"), 0o644) != nil {
		t.Fatal(err)
	}
	rec, err := r.l.LaunchRecord("ann")
	if err != nil {
		t.Fatal(err)
	}
	fp, _ := r.l.Fingerprint("ann")
	if err := db.SetLaunchFingerprint(r.l.Q, "ann", fp, rec); err != nil {
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
	if r.l.Drifted("ann", row()) {
		t.Fatal("a rules change relaunches the session")
	}
	was := row()
	note, kept, err := r.l.RulesNote("ann", was)
	want := "Your team rules changed (shared/team/core.md). Your session goes on; follow the new rules from now.\n\nAdded:\n+ - reply in Korean\n\nRemoved:\n- - reply in English"
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
	if got, _ := r.l.whatChanged("ann"); got != "" {
		t.Fatalf("a later launch would still name the told rules: %q", got)
	}
	// A launch that recorded itself between the note and its keeping is not overwritten.
	rules = "# Rules for team core\n- new"
	stale := row()
	_, kept, _ = r.l.RulesNote("ann", stale)
	fresh, _ := r.l.LaunchRecord("ann")
	if err := db.SetLaunchFingerprint(r.l.Q, "ann", fp, fresh); err != nil {
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
	if err := db.SetLaunchFingerprint(r.l.Q, "ann", fp, string(b)); err != nil {
		t.Fatal(err)
	}
	if note, _, _ := r.l.RulesNote("ann", row()); !strings.HasSuffix(note, "They now read:\n\n"+rules) {
		t.Fatalf("%q", note)
	}
}

// The rules are not in the fingerprint; anything else in the preamble still is.
func TestTheFingerprintLeavesOutTheRulesAlone(t *testing.T) {
	r := newLaunchRig(t)
	rules := "RULES-A"
	r.l.Ports.TeamRules = func(string) []string { return []string{rules} }
	before, _ := r.l.Fingerprint("ann")
	rules = "RULES-B"
	if after, _ := r.l.Fingerprint("ann"); after != before {
		t.Fatal("the rules moved the fingerprint")
	}
	if err := os.WriteFile(filepath.Join(r.l.Home, "agents", "ann", "ROLE.md"), []byte("a new role"), 0o644); err != nil {
		t.Fatal(err)
	}
	if after, _ := r.l.Fingerprint("ann"); after == before {
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
