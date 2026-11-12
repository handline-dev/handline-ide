package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheNewPeerPartsAreTheHouseRules(t *testing.T) {
	if !strings.Contains(DefaultRules, newSendLine) || !strings.HasSuffix(DefaultRules, newPeers) {
		t.Errorf("default_rules.md and the migration's new parts differ:\n%s", DefaultRules)
	}
}

func TestMigratePeerRulesRewritesOnlyTheOldWordingAsSeededOnce(t *testing.T) {
	home := t.TempDir()
	old := strings.Replace(strings.Replace(DefaultRules, newSendLine, oldSendLine, 1), newPeers, oldReports, 1) + "\nOur own line.\n"
	if !strings.Contains(old, oldReports) || !strings.Contains(old, oldSendLine) {
		t.Fatal("the fixture is not the old seeded file")
	}
	blockEdited := strings.Replace(old, "- Check main before you assign", "- Check main twice before you assign", 1)
	sendEdited := strings.Replace(old, "it hears from you only when a task is done", "it hears from you when a task is done", 1)
	files := map[string]string{"w1": old, "w2": blockEdited, "w3": sendEdited, "w4": "Our own rules.\n"}
	path := func(ws string) string { return filepath.Join(home, "workspaces", ws, "base.md") }
	for ws, text := range files {
		if err := os.MkdirAll(filepath.Dir(path(ws)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path(ws), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(ws string) string {
		b, err := os.ReadFile(path(ws))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	kept, errs := MigratePeerRules(home)
	if len(errs) > 0 || len(kept) != 2 || filepath.Base(filepath.Dir(kept[0])) != "w2" || filepath.Base(filepath.Dir(kept[1])) != "w3" {
		t.Fatalf("kept %v errs %v, want w2 and w3, whose teams changed a part", kept, errs)
	}
	if got := read("w1"); got != DefaultRules+"\nOur own line.\n" {
		t.Errorf("the old seeded file is not today's rules with its own line kept:\n%s", got)
	}
	// each part moves only where it is word for word: w2's block and w3's send line stay as their teams wrote them
	if got := read("w2"); !strings.Contains(got, strings.Replace(oldReports, "Check main before", "Check main twice before", 1)) ||
		strings.Contains(got, newPeers) || !strings.Contains(got, newSendLine) {
		t.Errorf("w2:\n%s", got)
	}
	if got := read("w3"); !strings.Contains(got, "it hears from you when a task is done") || strings.Contains(got, newSendLine) || !strings.Contains(got, newPeers) {
		t.Errorf("w3:\n%s", got)
	}
	if got := read("w4"); got != "Our own rules.\n" {
		t.Errorf("a file with neither part changed: %q", got)
	}
	// a second start finds each workspace done: a team that puts the old wording back keeps it, and nothing is named again
	if err := os.WriteFile(path("w1"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if kept, errs := MigratePeerRules(home); len(kept) != 0 || len(errs) != 0 || read("w1") != old {
		t.Errorf("second run: kept %v errs %v, w1 rewritten again: %v", kept, errs, read("w1") != old)
	}
}
