package homeinit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryShippedSkillIsNamedAsHandlines(t *testing.T) {
	entries, err := skillFiles.ReadDir("skills")
	if err != nil || len(entries) == 0 {
		t.Fatalf("no skills shipped: %v", err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), SkillPrefix) {
			t.Errorf("skill %s does not start with %s", e.Name(), SkillPrefix)
		}
		if _, err := skillFiles.ReadFile("skills/" + e.Name() + "/SKILL.md"); err != nil {
			t.Errorf("skill %s has no SKILL.md", e.Name())
		}
	}
}

func TestAFreshSkillsFolderGetsHandlinesSkillsAndASecondStartChangesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	changed, err := InstallSkills(dir)
	if err != nil || len(changed) == 0 {
		t.Fatalf("first start wrote %v, %v", changed, err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "handline-hl", "SKILL.md")); err != nil || !strings.Contains(string(b), "hl inbox") {
		t.Fatalf("handline-hl/SKILL.md = %q, %v", b, err)
	}
	if again, err := InstallSkills(dir); err != nil || len(again) != 0 {
		t.Fatalf("second start wrote %v, %v", again, err)
	}
}

func TestANewVersionReplacesTheSkillAndALeftoverFileGoes(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallSkills(dir); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(dir, "handline-hl")
	// A skill of an older build: its text and mark differ, and it has a file this build lacks.
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "extra.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, SkillMark), []byte("an older hash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := InstallSkills(dir); err != nil || len(changed) != 1 {
		t.Fatalf("replace wrote %v, %v", changed, err)
	}
	if b, _ := os.ReadFile(filepath.Join(skill, "SKILL.md")); string(b) == "old" {
		t.Error("SKILL.md was not replaced")
	}
	if _, err := os.Stat(filepath.Join(skill, "extra.md")); err == nil {
		t.Error("a file the build no longer has stayed")
	}
}

func TestASkillTheBuildDroppedIsRemovedAndAUsersOwnIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	mk := func(name, mark string) {
		d := filepath.Join(dir, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
		if mark != "" {
			if err := os.WriteFile(filepath.Join(d, SkillMark), []byte(mark), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("handline-gone", "abc\n") // ours from an older build
	mk("handline-hl", "")        // the user's, under a name Handline would use
	mk("handline-theirs", "")    // the user's, with our prefix
	mk("pixel-sprite", "abc\n")  // not our prefix: never ours, whatever it holds
	mk("not-ours", "")
	changed, err := InstallSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "handline-gone")); err == nil {
		t.Error("a dropped skill stayed")
	}
	for _, keep := range []string{"handline-theirs", "pixel-sprite", "not-ours"} {
		if _, err := os.Stat(filepath.Join(dir, keep, "SKILL.md")); err != nil {
			t.Errorf("%s was touched: %v", keep, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "handline-hl", "SKILL.md")); string(b) != "mine" {
		t.Errorf("the user's handline-hl was overwritten: %q (changed %v)", b, changed)
	}
}

func TestSetupInstallsTheSkillsWhereEveryAccountSeesThem(t *testing.T) {
	home := t.TempDir()
	if err := Setup(home, "", func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "shared", ".claude", "skills", "handline-hl", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
