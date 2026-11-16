package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummaryPicksTheFirstFieldThatSaysSomething(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{map[string]any{"command": "ls -l", "description": "list"}, "ls -l"},
		{map[string]any{"command": "  ", "file_path": "/a/b"}, "/a/b"},
		{map[string]any{"zeta": 1, "alpha": "x"}, `{"alpha": "x", "zeta": 1}`},
		{map[string]any{"command": "a\x1b[31mb\nc"}, "a [31mb c"},
		{nil, ""},
		{"plain", "plain"},
		{strings.Repeat("é", 200), strings.Repeat("é", 120)},
	}
	for _, c := range cases {
		if got := Summary(c.in); got != c.want {
			t.Errorf("Summary(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAWaitIsReadBackAndClearedOnlyByItsOwnCall(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Dir(f.waitFile("worker")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.waitFile("worker"), []byte(`{"tool":"Bash","command":"make","reason":"build","since":5,"after_event_id":7}`), 0o644); err != nil {
		t.Fatal(err)
	}
	w := f.ReadWait("worker")
	if w == nil || w.Tool != "Bash" || w.Command != "make" || w.Reason != "build" || w.Since != 5 || w.AfterEventID == nil || *w.AfterEventID != 7 {
		t.Fatalf("read back %+v", w)
	}
	in := map[string]any{"command": "make"}
	f.ClearWaitFor("worker", "Bash", map[string]any{"command": "other"})
	f.ClearWaitFor("worker", "Edit", in)
	if f.ReadWait("worker") == nil {
		t.Fatal("another call's finish cleared the wait")
	}
	f.ClearWaitFor("worker", "Bash", in)
	if f.ReadWait("worker") != nil {
		t.Fatal("its own call did not clear it")
	}
	f.ClearWaitFor("worker", "Bash", in) // nothing there: nothing happens
	os.WriteFile(f.waitFile("worker"), []byte(`{"tool":"x","since":0}`), 0o644)
	if f.ReadWait("worker") != nil {
		t.Fatal("a wait with no since was read")
	}
	os.WriteFile(f.waitFile("worker"), []byte(`[1]`), 0o644)
	if f.ReadWait("worker") != nil {
		t.Fatal("a wait that is not an object was read")
	}
}
