package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCarriedPathPutsTheFolderLastOnlyWhereATmuxIs(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "handlined")
	if got := carriedPath(exe, "/usr/bin:/bin"); got != "/usr/bin:/bin" {
		t.Fatalf("no tmux beside the program: PATH changed to %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmux"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := carriedPath(exe, "/usr/bin:/bin"), "/usr/bin:/bin:"+dir; got != want {
		t.Fatalf("PATH = %q, want %q: the system's tmux must come first", got, want)
	}
	if got := carriedPath(exe, "/usr/bin:"+dir); got != "/usr/bin:"+dir {
		t.Fatalf("the folder is on PATH already, got %q", got)
	}
	if got := carriedPath(exe, ""); got != dir {
		t.Fatalf("empty PATH = %q, want the folder", got)
	}
}
