package reads

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team/teamtest"
)

// fakeRepo makes dir a repo as the scan sees one: a .git folder with HEAD and index stamped at head and index.
func fakeRepo(t *testing.T, dir string, head, index time.Time) {
	t.Helper()
	g := filepath.Join(dir, ".git")
	if err := os.MkdirAll(g, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, at := range map[string]time.Time{"HEAD": head, "index": index} {
		p := filepath.Join(g, name)
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
}

func recentNames(fs []recentFolder) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, f.name)
	}
	return out
}

func TestRecentReposFindsGitFoldersNewestFirstAndSkipsWhatItMustNotEnter(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	at := func(h int) time.Time { return time.Date(2026, 11, 7, h, 0, 0, 0, time.UTC) }
	fakeRepo(t, filepath.Join(home, "code", "old"), at(1), at(2))
	fakeRepo(t, filepath.Join(home, "code", "new"), at(3), at(1))                          // HEAD is the later of the two
	fakeRepo(t, filepath.Join(home, "deep", "a", "mid"), at(1), at(2).Add(30*time.Minute)) // three levels down: the deepest looked at
	fakeRepo(t, filepath.Join(home, "deep", "a", "b", "far"), at(9), at(9))
	fakeRepo(t, filepath.Join(home, "code", "old", "vendor", "inner"), at(9), at(9)) // inside a repo: never entered
	fakeRepo(t, filepath.Join(home, "known"), at(9), at(9))
	for _, n := range []string{"Documents", "Desktop", "Downloads", "Library", "Pictures", "Music", "Movies", "Applications",
		".hidden", "node_modules", "Tool.app", "Photos Library.photoslibrary", "Music Library.musiclibrary"} {
		fakeRepo(t, filepath.Join(home, n, "x"), at(9), at(9))
	}
	// A linked worktree's .git is a file: neither listed nor entered.
	wt := filepath.Join(home, "wt")
	os.MkdirAll(wt, 0o755)
	os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: elsewhere"), 0o644)
	fakeRepo(t, filepath.Join(wt, "sub"), at(9), at(9))
	// A symlink to a folder holding a repo is not followed.
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	fakeRepo(t, filepath.Join(outside, "linked"), at(9), at(9))
	if err := os.Symlink(outside, filepath.Join(home, "link")); err != nil {
		t.Fatal(err)
	}

	got := recentRepos(home, map[string]bool{filepath.Join(home, "known"): true}, time.Now().Add(time.Minute))
	if names := recentNames(got); !reflect.DeepEqual(names, []string{"new", "mid", "old"}) {
		t.Fatalf("folders %v, want new, mid, old", names)
	}
	if got[0].path != filepath.Join(home, "code", "new") || !got[0].touched.Equal(at(3)) {
		t.Fatalf("first %+v, want code/new touched at its HEAD", got[0])
	}
}

func TestRecentReposGivesAtMostTwelveAndStopsAtItsDeadline(t *testing.T) {
	home := t.TempDir()
	for i := range 15 {
		at := time.Date(2026, 11, 1, i, 0, 0, 0, time.UTC)
		fakeRepo(t, filepath.Join(home, fmt.Sprintf("r%02d", i)), at, at)
	}
	got := recentRepos(home, nil, time.Now().Add(time.Minute))
	if len(got) != 12 || got[0].name != "r14" || got[11].name != "r03" {
		t.Fatalf("folders %v, want r14 down to r03", recentNames(got))
	}
	if got := recentRepos(home, nil, time.Now().Add(-time.Second)); len(got) != 0 {
		t.Fatalf("a passed deadline found %v", recentNames(got))
	}
}

func TestReposRecentServesTheUsersHomeLessTheKnownRepos(t *testing.T) {
	e := testEnv(t)
	raw := t.TempDir() // on macOS under /var, a symlink to /private/var: the read resolves it
	home, _ := filepath.EvalSymlinks(raw)
	t.Setenv("HANDLINE_TEST_ROOT", raw)
	at := time.Date(2026, 11, 7, 6, 30, 0, 0, time.UTC)
	fakeRepo(t, filepath.Join(home, "mine"), at, at)
	known := filepath.Join(home, "taken")
	fakeRepo(t, known, at, at)
	addAgent(t, e, "alice", "")
	teamtest.Set(t, e.Home, "alice", "repo", filepath.Join(raw, "taken")+"/")

	res, err := e.reposRecent(nil, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"folders": []any{map[string]any{"path": filepath.Join(home, "mine"), "name": "mine", "touched_at": "2026-11-07T06:30:00Z"}}}
	if !reflect.DeepEqual(res, want) {
		t.Fatalf("repos.recent %v, want %v", res, want)
	}
}
