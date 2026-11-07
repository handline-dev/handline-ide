package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The test apps of an agent are ended when its turn ends: its state file names the worktree whose tools/app.mjs stops it; another
// agent's instance is left, and a worktree that is gone leaves only the file to remove.
func TestReapAppsEndsOnlyTheAgentsInstances(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("no node")
	}
	home := t.TempDir()
	state := filepath.Join(home, "app-instances")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	put := func(name, owner, top string) string {
		b, _ := json.Marshal(map[string]string{"Owner": owner, "Top": top})
		f := filepath.Join(state, name+".json")
		if err := os.WriteFile(f, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	worktree := func(name string) string {
		top := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(filepath.Join(top, "tools"), 0o755); err != nil {
			t.Fatal(err)
		}
		script := "import fs from 'node:fs'; fs.writeFileSync(process.argv[1] + '.ran', process.argv[2] + ' ' + process.env.HANDLINE_AGENT)"
		if err := os.WriteFile(filepath.Join(top, "tools", "app.mjs"), []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
		return top
	}
	mine, theirs := worktree("mine"), worktree("theirs")
	put("mine", "ann", mine)
	other := put("theirs", "bob", theirs)
	gone := put("gone", "ann", filepath.Join(home, "no-such-worktree"))

	reapApps(home, "ann")

	if b, err := os.ReadFile(filepath.Join(mine, "tools", "app.mjs.ran")); err != nil || string(b) != "stop ann" {
		t.Errorf("ann's instance was not stopped as ann: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(theirs, "tools", "app.mjs.ran")); err == nil {
		t.Error("bob's instance was stopped by ann's turn end")
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("bob's state file went: %v", err)
	}
	if _, err := os.Stat(gone); err == nil {
		t.Error("the state file of a worktree that is gone stayed")
	}
}

func TestAppRefusesWhatIsNotItsVerb(t *testing.T) {
	for _, args := range [][]string{nil, {"launch"}, {"Start"}} {
		if _, ok := appCmd(nil, args); ok {
			t.Errorf("hl app %v was taken", args)
		}
	}
}
