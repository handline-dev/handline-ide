package team

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/paths"
)

func TestOnlyAnEmptyJournalFolderIsRemoved(t *testing.T) {
	home := t.TempDir()
	empty := filepath.Join(paths.AgentsDir(home), "a1", "journal")
	full := filepath.Join(paths.AgentsDir(home), "a2", "journal")
	for _, d := range []string{empty, full} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(full, "2026-10-09.md"), []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(paths.AgentsDir(home), "a3", "journal")
	link := filepath.Join(paths.AgentsDir(home), "a4", "journal")
	for _, d := range []string{filepath.Dir(file), filepath.Dir(link)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(file, []byte("a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	var logs []string
	removed := RemoveEmptyJournals(home, func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) })
	if len(removed) != 1 || removed[0] != empty {
		t.Fatalf("removed %v", removed)
	}
	if _, err := os.Stat(full); err != nil || len(logs) != 1 || !strings.Contains(logs[0], full) {
		t.Fatalf("a journal with files in it was not left and told: %v %q", err, logs)
	}
	for _, p := range []string{file, link} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s was removed: %v", p, err)
		}
	}
}
