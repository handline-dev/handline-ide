package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rollout writes testdata/rollout.jsonl (codex 0.157.1's own lines, its session_meta cut to what is read) as a session of cwd on day.
func rollout(t *testing.T, config, cwd string, day time.Time, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "rollout.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(config, "sessions", day.Format("2006/01/02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "rollout-"+name+".jsonl")
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), `"CWD"`, `"`+cwd+`"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSessionFileIsTheNewestOfTheCwd(t *testing.T) {
	config := t.TempDir()
	now := time.Date(2026, 11, 12, 22, 0, 0, 0, time.Local)
	if _, ok := SessionFile(config, "/w", now); ok {
		t.Fatal("no sessions")
	}
	old := rollout(t, config, "/w", now.AddDate(0, 0, -5), "old")
	if p, ok := SessionFile(config, "/w", now); !ok || p != old {
		t.Fatalf("only an old one: the whole tree is read: %q", p)
	}
	yesterday := rollout(t, config, "/w", now.AddDate(0, 0, -1), "y")
	rollout(t, config, "/other", now, "o")
	if p, _ := SessionFile(config, "/w", now); p != yesterday {
		t.Fatalf("yesterday's beats an older one, and another cwd's is not it: %q", p)
	}
	today := rollout(t, config, "/w", now, "t")
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(today, later, later); err != nil {
		t.Fatal(err)
	}
	if p, _ := SessionFile(config, "/w", now); p != today {
		t.Fatalf("%q", p)
	}
}

func TestUserTextIsThePersonsMessageOnly(t *testing.T) {
	b, _ := os.ReadFile(filepath.Join("testdata", "rollout.jsonl"))
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if got, ok := userText([]byte(lines[1])); !ok || got != "List three prime numbers, then count slowly from 1 to 30, one per line." {
		t.Fatalf("%q %v", got, ok)
	}
	for _, l := range []string{lines[0], lines[2], "not json"} {
		if _, ok := userText([]byte(l)); ok {
			t.Fatalf("not a message: %s", l)
		}
	}
}
