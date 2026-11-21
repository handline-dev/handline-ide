package homefiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Files collected under one home land at the same paths under another, with their modes; a path outside the home is refused before
// anything is written, and a change or a removal is seen from the stamps.
func TestFilesLandAtTheSamePathsUnderAnotherHomeAndNeverOutside(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(from, "shared", "skills", "s"), 0o755)
	os.WriteFile(filepath.Join(from, "shared", "skills", "s", "SKILL.md"), []byte("v1"), 0o644)
	os.WriteFile(filepath.Join(from, "shared", "run.sh"), []byte("#!/bin/sh"), 0o755)
	was := Stamps(from, filepath.Join(from, "shared"))
	files, err := Collect(from, filepath.Join(from, "shared"), filepath.Join(from, "missing"))
	if err != nil || len(files) != 2 {
		t.Fatalf("%v %v", files, err)
	}
	back, err := FromWire(anyList(Wire(files)))
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(to, back, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(to, "shared", "skills", "s", "SKILL.md")); string(b) != "v1" {
		t.Fatalf("SKILL.md: %q", b)
	}
	if st, _ := os.Stat(filepath.Join(to, "shared", "run.sh")); st.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", st.Mode())
	}
	for _, bad := range []string{"../out", "/etc/x", "a/../../out", ""} {
		if err := Apply(to, []File{{Path: "shared/ok", Data: []byte("x")}, {Path: bad, Data: []byte("x")}}, nil); err == nil {
			t.Fatalf("%q was written", bad)
		}
		if err := Apply(to, nil, []string{bad}); err == nil {
			t.Fatalf("%q was removed", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(to, "shared", "ok")); err == nil {
		t.Fatal("a refused call wrote part of its files")
	}
	if _, err := Collect(from, "/etc"); err == nil {
		t.Fatal("a path outside the home was collected")
	}

	os.WriteFile(filepath.Join(from, "shared", "skills", "s", "SKILL.md"), []byte("v2!"), 0o644)
	os.Remove(filepath.Join(from, "shared", "run.sh"))
	changed, gone := Changed(was, Stamps(from, filepath.Join(from, "shared")))
	if len(changed) != 1 || changed[0] != "shared/skills/s/SKILL.md" || len(gone) != 1 || gone[0] != "shared/run.sh" {
		t.Fatalf("changed %v gone %v", changed, gone)
	}
}

// anyList is a list as a call decodes it.
func anyList(v []any) any {
	out := make([]any, len(v))
	for i, e := range v {
		m := e.(map[string]any)
		out[i] = map[string]any{"path": m["path"], "data": m["data"], "mode": json.Number(fmt.Sprint(m["mode"]))}
	}
	return out
}

// A link never carries a file out of a home or into one from elsewhere: a folder link on the way and a link in a file's place are
// refused, and a link is not sent.
func TestALinkLeadsNowhereOutsideTheHome(t *testing.T) {
	home, outside := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("key"), 0o600)
	os.MkdirAll(filepath.Join(home, "agents", "k"), 0o755)
	os.Symlink(outside, filepath.Join(home, "agents", "k", "memory"))
	os.Symlink(filepath.Join(outside, "secret"), filepath.Join(home, "agents", "k", "handoff.md"))
	if err := Apply(home, []File{{Path: "agents/k/memory/x", Data: []byte("x")}}, nil); err == nil {
		t.Fatal("a file was written through a folder link")
	}
	if err := Apply(home, []File{{Path: "agents/k/handoff.md", Data: []byte("x")}}, nil); err == nil {
		t.Fatal("a file was written over a link")
	}
	if err := Apply(home, nil, []string{"agents/k/memory/secret"}); err == nil {
		t.Fatal("a file was removed through a folder link")
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "secret")); string(b) != "key" {
		t.Fatal("the file outside changed")
	}
	if files, err := Collect(home, filepath.Join(home, "agents")); err != nil || len(files) != 0 {
		t.Fatalf("collected %v %v", files, err)
	}
	if _, err := Collect(home, filepath.Join(home, "attachments")+"/../accounts"); err == nil {
		t.Fatal("a path with .. was collected")
	}
}
