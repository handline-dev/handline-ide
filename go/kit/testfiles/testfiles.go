// Package testfiles writes a tree of files for a test.
package testfiles

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Write writes each file under dir, its folders made as needed: name is a slash path relative to dir, text its whole content.
func Write(t testing.TB, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// GitCheckout is a new empty git checkout in a test's temporary folder.
func GitCheckout(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return dir
}
