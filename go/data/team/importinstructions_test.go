package team

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
)

// The files of an older Handline become each part's version, by "imported" at the file's time, and go: a file that differs from the
// version in force is kept after it, so no text is lost; a seeded workspace whose rules file is gone keeps empty rules, so seeding does not bring the house rules back.
func TestImportMovesInstructionFilesIntoTheDatabaseOnce(t *testing.T) {
	home := t.TempDir()
	workspaces(t, home, "shop", "lab")
	d := shared(t, home)
	write := func(rel, text string) string {
		p := filepath.Join(home, rel)
		must(t, os.MkdirAll(filepath.Dir(p), 0o755))
		must(t, os.WriteFile(p, []byte(text), 0o644))
		return p
	}
	role := write("agents/a3/ROLE.md", "# ui\nbuilds the app\n")
	write("agents/a3/base.md", "facts")
	write("agents/a4/base.md", "old")
	write("workspaces/w1/base.md", "be kind")
	write("workspaces/w2/.seeded", "")
	at := int64(1_700_000_000)
	must(t, os.Chtimes(role, time0(at), time0(at)))
	_, err := db.SetInstruction(d, "a4", db.PartBase, "newer", "p1", "", 9)
	must(t, err)

	moved, errs := ImportInstructions(home, d)
	if len(errs) != 0 || len(moved) != 4 {
		t.Fatalf("moved %v, errs %v", moved, errs)
	}
	in, _ := db.CurrentInstruction(d, "a3", db.PartRole)
	if in == nil || in.Text != "# ui\nbuilds the app" || in.Actor != ImportActor || in.At != at {
		t.Fatalf("role: %+v", in)
	}
	if hist, _ := db.InstructionsAfter(d, "a4", db.PartBase, 0); len(hist) != 2 || hist[1].Text != "old" || hist[1].Actor != ImportActor {
		t.Errorf("a file that differs is kept as a version: %+v", hist)
	}
	if got, _ := RulesText(d, "shop"); got != "be kind" {
		t.Errorf("shop's rules: %q", got)
	}
	if in, _ := db.CurrentInstruction(d, "w2", db.PartRules); in == nil || in.Text != "" {
		t.Errorf("lab emptied its rules: %+v", in)
	}
	for _, rel := range []string{"agents/a3/ROLE.md", "agents/a3/base.md", "agents/a4/base.md", "workspaces/w1/base.md"} {
		if _, err := os.Stat(filepath.Join(home, rel)); !os.IsNotExist(err) {
			t.Errorf("%s is still there: %v", rel, err)
		}
	}
	if moved, errs := ImportInstructions(home, d); len(moved)+len(errs) != 0 {
		t.Fatalf("a second run moved %v, %v", moved, errs)
	}
	if hist, _ := db.InstructionsAfter(d, "w2", db.PartRules, 0); len(hist) != 1 {
		t.Errorf("a second run added versions: %+v", hist)
	}
}

func time0(s int64) time.Time { return time.Unix(s, 0) }
