package worktreesetup

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/protected"
)

// write makes path holding text, private, with the folders it needs.
func write(t *testing.T, path, text string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err == nil {
		err = os.WriteFile(path, []byte(text), 0o600)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var at = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

func TestCheckRefusesWhatIsNoPathInsideTheRepo(t *testing.T) {
	ok := []Setup{{}, {Script: "pnpm install\nmake gen", Copy: []string{".env", "config/*.local", "secrets/"}}}
	for _, s := range ok {
		if err := Check(s); err != nil {
			t.Errorf("%+v refused: %v", s, err)
		}
	}
	bad := map[string]Setup{
		"absolute":   {Copy: []string{"/etc/passwd"}},
		"above":      {Copy: []string{"../other/.env"}},
		"above deep": {Copy: []string{"a/../../x"}},
		"dot":        {Copy: []string{"."}},
		"git":        {Copy: []string{".git/config"}},
		"empty":      {Copy: []string{""}},
		"twice":      {Copy: []string{".env", ".env"}},
		"newline":    {Copy: []string{"a\nb"}},
		"bad glob":   {Copy: []string{"[a"}},
		"long path":  {Copy: []string{strings.Repeat("a", PathMax+1)}},
		"too many":   {Copy: make([]string, CopyMax+1)},
		"long":       {Script: strings.Repeat("x", ScriptMax+1)},
		"nul":        {Script: "a\x00b"},
	}
	for name, s := range bad {
		if Check(s) == nil {
			t.Errorf("%s: kept", name)
		}
	}
}

func TestPutGetAndClear(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	st := &protected.Store{Root: t.TempDir()}
	repo := t.TempDir()
	if got := Get(st, repo); !got.Empty() || got.Copy == nil {
		t.Fatalf("none set: %+v", got)
	}
	want := Setup{Script: "pnpm install", Copy: []string{".env"}}
	if err := Put(st, repo, want); err != nil {
		t.Fatal(err)
	}
	if got := Get(st, repo); got.Script != want.Script || !slices.Equal(got.Copy, want.Copy) {
		t.Fatalf("got %+v", got)
	}
	if Get(st, t.TempDir()).Script != "" {
		t.Fatal("another repo got it")
	}
	if err := Put(st, repo, Setup{Copy: []string{"/x"}}); err == nil {
		t.Fatal("a bad setup was kept")
	}
	if err := Put(st, repo, Setup{}); err != nil {
		t.Fatal(err)
	}
	if _, there := st.Get(NS, Key(repo)); there {
		t.Fatal("an empty setup left its entry")
	}
}

func TestRunCopiesWhatTheWorktreeLacksAndKeepsWhatItHas(t *testing.T) {
	repo, wt := t.TempDir(), t.TempDir()
	write(t, filepath.Join(repo, ".env"), "SECRET=1")
	write(t, filepath.Join(repo, "config/a.local"), "a")
	write(t, filepath.Join(repo, "config/b.local"), "b")
	write(t, filepath.Join(repo, "keys/one"), "1")
	write(t, filepath.Join(repo, "keys/two"), "2")
	write(t, filepath.Join(repo, "README.md"), "main's")
	write(t, filepath.Join(wt, "README.md"), "the branch's")
	write(t, filepath.Join(wt, "keys/one"), "the branch's one")
	r := Run(repo, wt, Setup{Copy: []string{".env", "config/*.local", "keys", "README.md", "nothing.txt"}}, at, ScriptLimit)
	if !r.OK || r.Error != "" {
		t.Fatalf("failed: %+v", r)
	}
	if want := []string{".env", "config/a.local", "config/b.local", "keys"}; !slices.Equal(r.Copied, want) {
		t.Errorf("copied %v, want %v", r.Copied, want)
	}
	if !slices.Equal(r.Missing, []string{"nothing.txt"}) {
		t.Errorf("missing %v", r.Missing)
	}
	for path, want := range map[string]string{".env": "SECRET=1", "config/b.local": "b", "keys/two": "2", "keys/one": "the branch's one",
		"README.md": "the branch's"} {
		if got := read(t, filepath.Join(wt, path)); got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
	if fi, _ := os.Stat(filepath.Join(wt, ".env")); fi.Mode().Perm() != 0o600 {
		t.Errorf(".env mode %v", fi.Mode().Perm())
	}
	if r.At != "2026-11-17T09:00:00Z" || r.Worktree != wt {
		t.Errorf("at %q worktree %q", r.At, r.Worktree)
	}
}

func TestRunRunsTheScriptInTheWorktreeAfterTheCopy(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	repo, wt := t.TempDir(), t.TempDir()
	write(t, filepath.Join(repo, ".env"), "X=1")
	r := Run(repo, wt, Setup{Script: `cat .env > seen; echo "$HANDLINE_REPO" > repo; echo "$HANDLINE_WORKTREE" > wt; echo done`,
		Copy: []string{".env"}}, at, ScriptLimit)
	if !r.OK || r.Output != "done\n" {
		t.Fatalf("%+v", r)
	}
	for f, want := range map[string]string{"seen": "X=1", "repo": repo + "\n", "wt": wt + "\n"} {
		if got := read(t, filepath.Join(wt, f)); got != want {
			t.Errorf("%s: %q, want %q", f, got, want)
		}
	}
}

func TestAFailedScriptIsToldWithTheEndOfItsOutput(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	r := Run(t.TempDir(), t.TempDir(), Setup{Script: "head -c 5000 /dev/zero | tr '\\0' a; echo; echo ERR_PNPM_NO_LOCKFILE >&2; exit 3"}, at, ScriptLimit)
	if r.OK || r.Error != "script exited 3" {
		t.Fatalf("%+v", r)
	}
	if len(r.Output) != OutputMax || !strings.HasSuffix(r.Output, "ERR_PNPM_NO_LOCKFILE\n") {
		t.Errorf("output %d bytes, ends %q", len(r.Output), r.Output[len(r.Output)-30:])
	}
	if said := r.Said(); said != "setup FAILED: script exited 3" {
		t.Errorf("said %q", said)
	}
}

func TestAScriptPastItsLimitIsStoppedWithWhatItStarted(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	wt := t.TempDir()
	start := time.Now()
	r := Run(t.TempDir(), wt, Setup{Script: "(sleep 1; touch child-lived) & sleep 30"}, at, 300*time.Millisecond)
	if r.OK || !strings.HasPrefix(r.Error, "script stopped after") {
		t.Fatalf("%+v", r)
	}
	if d := time.Since(start); d > 6*time.Second {
		t.Fatalf("took %s", d)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(wt, "child-lived")); err == nil {
		t.Fatal("what the script started outlived the stop")
	}
}

func TestAFailedCopyRunsNoScript(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	repo, wt := t.TempDir(), t.TempDir()
	write(t, filepath.Join(repo, "locked/f"), "x")
	if err := os.Chmod(filepath.Join(repo, "locked/f"), 0); err != nil {
		t.Fatal(err)
	}
	r := Run(repo, wt, Setup{Script: "touch ran", Copy: []string{"locked"}}, at, ScriptLimit)
	if r.OK || !strings.HasPrefix(r.Error, "copying locked:") {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(wt, "ran")); err == nil {
		t.Fatal("the script ran after a failed copy")
	}
}

func TestSaidNamesWhatWasDone(t *testing.T) {
	r := Result{OK: true, Copied: []string{".env"}, Missing: []string{"x"}}
	if got := r.Said(); got != "setup done: copied .env; not in the main checkout: x" {
		t.Errorf("%q", got)
	}
}

func TestNothingIsCopiedThroughALinkInTheWorktree(t *testing.T) {
	repo, wt, elsewhere := t.TempDir(), t.TempDir(), t.TempDir()
	write(t, filepath.Join(repo, "config/a.local"), "a")
	if err := os.Symlink(elsewhere, filepath.Join(wt, "config")); err != nil {
		t.Fatal(err)
	}
	r := Run(repo, wt, Setup{Copy: []string{"config/*.local"}}, at, ScriptLimit)
	if r.OK || !strings.Contains(r.Error, "is a link") {
		t.Fatalf("%+v", r)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatal("copied through the link")
	}
}

func TestAGlobNeverCopiesDotGitAndAFileInAFoldersPlaceIsKept(t *testing.T) {
	repo, wt := t.TempDir(), t.TempDir()
	write(t, filepath.Join(repo, ".git/config"), "x")
	write(t, filepath.Join(repo, ".tool/a"), "a")
	write(t, filepath.Join(wt, ".git"), "gitdir: elsewhere")
	write(t, filepath.Join(wt, ".tool"), "the branch's file")
	r := Run(repo, wt, Setup{Copy: []string{".*"}}, at, ScriptLimit)
	if !r.OK || len(r.Copied) != 0 {
		t.Fatalf("%+v", r)
	}
	if read(t, filepath.Join(wt, ".git")) != "gitdir: elsewhere" {
		t.Fatal(".git touched")
	}
}

func TestAScriptThatLeavesAProcessBehindStillSucceeds(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	start := time.Now()
	r := Run(t.TempDir(), t.TempDir(), Setup{Script: "sleep 30 & echo up"}, at, ScriptLimit)
	if !r.OK || r.Output != "up\n" {
		t.Fatalf("%+v", r)
	}
	if d := time.Since(start); d > 15*time.Second {
		t.Fatalf("took %s", d)
	}
}

func TestKeysOfDifferentReposDiffer(t *testing.T) {
	if a, b := Key("/a/b-c"), Key("/a/b/c"); a == b || !strings.HasPrefix(a, "b-c-") {
		t.Fatalf("%q %q", a, b)
	}
}
