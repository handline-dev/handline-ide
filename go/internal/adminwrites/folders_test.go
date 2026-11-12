package adminwrites

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/paths"
)

// noGitConfig runs git without the machine's global config, so the test does not depend on who runs it.
func noGitConfig(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	os.WriteFile(empty, nil, 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	r := gitrun.Run(dir, args...)
	if r.RC != 0 {
		t.Fatalf("git %v: %s", args, r.Stderr)
	}
	return strings.TrimSpace(r.Stdout)
}

func (r *rig) reason(m map[string]any) string {
	r.t.Helper()
	if r.code(m) != "refused" {
		r.t.Fatalf("not refused: %v", m)
	}
	d := m["error"].(map[string]any)["data"].(map[string]any)
	if d["gate"] != "state" {
		r.t.Fatalf("gate: %v", d)
	}
	return d["reason"].(string)
}

func TestFolderInitCommitsWhatTheFolderHoldsButSecrets(t *testing.T) {
	noGitConfig(t)
	r := newRig(t)
	dir := t.TempDir()
	for name, body := range map[string]string{"main.go": "package main\n", ".env": "KEY=1\n", ".env.example": "KEY=\n", "deploy.pem": "x"} {
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
	}
	call := fmt.Sprintf(`{"path":%q,"request_id":"i1"}`, dir)
	res := r.result(r.one("folder.init", call))
	if res["path"] != dir || res["said"] != dir+" is a git repository on main with a first commit" {
		t.Fatalf("%v", res)
	}
	if got := git(t, dir, "ls-files"); got != ".env.example\nmain.go" {
		t.Fatalf("committed %q", got)
	}
	if got := git(t, dir, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("commits %s", got)
	}
	if git(t, dir, "status", "--porcelain") != "" {
		t.Fatal("the secrets stay ignored after the commit")
	}
	if r.result(r.one("folder.init", call))["replayed"] != true {
		t.Fatal("a retry replays")
	}
	if why := r.reason(r.one("folder.init", fmt.Sprintf(`{"path":%q,"request_id":"i2"}`, dir))); why != dir+" is already a git repository" {
		t.Fatal(why)
	}
}

func TestFolderInitOfAnEmptyFolderKeepsTheConfiguredBranch(t *testing.T) {
	noGitConfig(t)
	git(t, "", "config", "--global", "init.defaultBranch", "trunk")
	git(t, "", "config", "--global", "user.name", "Pat")
	git(t, "", "config", "--global", "user.email", "pat@example.com")
	r := newRig(t)
	dir := t.TempDir()
	r.result(r.one("folder.init", fmt.Sprintf(`{"path":%q,"request_id":"i1"}`, dir)))
	if b := git(t, dir, "branch", "--show-current"); b != "trunk" {
		t.Fatalf("branch %s", b)
	}
	if who := git(t, dir, "log", "-1", "--format=%an <%ae>"); who != "Pat <pat@example.com>" {
		t.Fatalf("author %s", who)
	}
}

func TestFolderInitRefusesWhatItMustNotTouch(t *testing.T) {
	noGitConfig(t)
	r := newRig(t)
	outer := t.TempDir()
	git(t, outer, "init", "-q")
	inner := filepath.Join(outer, "sub")
	os.Mkdir(inner, 0o755)
	if why := r.reason(r.one("folder.init", fmt.Sprintf(`{"path":%q,"request_id":"i1"}`, inner))); !strings.Contains(why, "is inside the git repository") {
		t.Fatal(why)
	}
	home := paths.UserHome()
	for i, p := range []string{home, filepath.Dir(home)} {
		if why := r.reason(r.one("folder.init", fmt.Sprintf(`{"path":%q,"request_id":"h%d"}`, p, i))); !strings.Contains(why, "holds the home folder") {
			t.Fatal(why)
		}
	}
	for i, p := range []string{"rel/dir", filepath.Join(outer, "missing")} {
		if c := r.code(r.one("folder.init", fmt.Sprintf(`{"path":%q,"request_id":"n%d"}`, p, i))); c != "not_found" {
			t.Fatalf("%s: %s", p, c)
		}
	}
}

func TestFolderInitLeavesNoGitWhenTheCommitFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any file")
	}
	noGitConfig(t)
	r := newRig(t)
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.txt")
	os.WriteFile(locked, []byte("x"), 0o000)
	t.Cleanup(func() { os.Chmod(locked, 0o644) })
	if why := r.reason(r.one("folder.init", fmt.Sprintf(`{"path":%q,"request_id":"i1"}`, dir))); !strings.Contains(why, "git add failed") {
		t.Fatal(why)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatal("the .git made is removed")
	}
}

// origin is a repository with one commit, to clone from; the test may clone from a local folder.
func origin(t *testing.T) string {
	t.Helper()
	was := cloneProtocols
	cloneProtocols = append(slices.Clone(was), "file")
	t.Cleanup(func() { cloneProtocols = was })
	src := filepath.Join(t.TempDir(), "proj")
	os.Mkdir(src, 0o755)
	git(t, src, "init", "-q")
	os.WriteFile(filepath.Join(src, "README.md"), []byte("hi\n"), 0o644)
	git(t, src, "add", "-A")
	git(t, src, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "one")
	return src
}

func TestFolderCloneIntoANewFolder(t *testing.T) {
	noGitConfig(t)
	r := newRig(t)
	src := origin(t)
	dest := filepath.Join(t.TempDir(), "a", "b", "copy")
	res := r.result(r.one("folder.clone", fmt.Sprintf(`{"url":%q,"path":%q,"request_id":"c1"}`, src, dest)))
	if res["path"] != dest {
		t.Fatalf("%v", res)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "README.md")); string(b) != "hi\n" {
		t.Fatal("cloned")
	}
	res = r.result(r.one("folder.clone", fmt.Sprintf(`{"url":%q,"request_id":"c2"}`, src+"/")))
	if want := filepath.Join(paths.UserHome(), "Handline", "proj"); res["path"] != want {
		t.Fatalf("default path %v, want %s", res["path"], want)
	}
	if why := r.reason(r.one("folder.clone", fmt.Sprintf(`{"url":%q,"path":%q,"request_id":"c3"}`, src, dest))); !strings.Contains(why, "is not empty") {
		t.Fatal(why)
	}
	empty := filepath.Join(t.TempDir(), "empty")
	os.Mkdir(empty, 0o755)
	r.result(r.one("folder.clone", fmt.Sprintf(`{"url":%q,"path":%q,"request_id":"c4"}`, src, empty)))
	if ents, _ := os.ReadDir(filepath.Dir(empty)); len(ents) != 1 {
		t.Fatalf("an empty folder takes the clone, and no clone folder is left beside it: %v", ents)
	}
	if _, err := os.Stat(filepath.Join(empty, "README.md")); err != nil {
		t.Fatal(err)
	}
}

func TestFolderCloneThatFailsLeavesNothing(t *testing.T) {
	noGitConfig(t)
	r := newRig(t)
	dest := filepath.Join(t.TempDir(), "copy")
	why := r.reason(r.one("folder.clone", fmt.Sprintf(`{"url":%q,"path":%q,"request_id":"c1"}`, "https://127.0.0.1:1/none.git", dest)))
	if !strings.HasPrefix(why, "git clone failed: ") {
		t.Fatal(why)
	}
	if ents, _ := os.ReadDir(filepath.Dir(dest)); len(ents) != 0 {
		t.Fatalf("nothing is left behind: %v", ents)
	}
	for i, call := range []string{`{"url":"--upload-pack=x"}`, `{"url":" "}`, `{"url":"a\nb"}`, `{"url":"https://h/.git"}`, `{"url":"https://h/x","path":"rel"}`} {
		call = strings.TrimSuffix(call, "}") + fmt.Sprintf(`,"request_id":"b%d"}`, i)
		if c := r.code(r.one("folder.clone", call)); c != "bad_request" {
			t.Fatalf("%s: %s", call, c)
		}
	}
}

func TestRepoNameIsWhatGitCloneWouldCallTheFolder(t *testing.T) {
	cases := [][2]string{{"https://github.com/o/repo.git", "repo"}, {"git@github.com:o/repo.git", "repo"}, {"https://h/o/repo/", "repo"},
		{"host:repo", "repo"}, {"https://h/", "h"}, {"https://h/.git", ""}}
	for _, c := range cases {
		if got := repoName(c[0]); got != c[1] {
			t.Errorf("%s: %q, want %q", c[0], got, c[1])
		}
	}
}

func TestCloneSaysNoPasswordAndAsksNothing(t *testing.T) {
	if got := shown("https://me:tok@github.com/o/r.git"); got != "https://github.com/o/r.git" {
		t.Fatal(got)
	}
	if got := shown("git@github.com:o/r.git"); got != "git@github.com:o/r.git" {
		t.Fatal(got)
	}
	noGitConfig(t)
	t.Setenv("GIT_SSH_COMMAND", "")
	if env := strings.Join(noPrompts(), "\n"); !strings.Contains(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes") || !strings.Contains(env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatal(env)
	}
	t.Setenv("GIT_SSH_COMMAND", "mine")
	if env := strings.Join(noPrompts(), "\n"); strings.Contains(env, "GIT_SSH_COMMAND") {
		t.Fatal("the person's own ssh command stands")
	}
}

func TestFolderCloneRunsNoCommandAUrlNames(t *testing.T) {
	noGitConfig(t)
	r := newRig(t)
	made := filepath.Join(t.TempDir(), "made")
	ext := "ext::sh -c touch% " + made
	for i, src := range []string{ext, "file:///etc", "/etc", "./here", "fd::17"} {
		if c := r.code(r.one("folder.clone", fmt.Sprintf(`{"url":%q,"path":%q,"request_id":"x%d"}`, src, filepath.Join(t.TempDir(), "c"), i))); c != "bad_request" {
			t.Fatalf("%s: %s", src, c)
		}
	}
	// git itself refuses it too, should a url get past the check
	err := runGit(t.Context(), "", noPrompts(), "-c", "protocol.ext.allow=always", "clone", "-q", "--", ext, filepath.Join(t.TempDir(), "c"))
	if err == nil || !strings.Contains(err.Error(), "transport 'ext' not allowed") {
		t.Fatalf("git: %v", err)
	}
	if _, err := os.Stat(made); !os.IsNotExist(err) {
		t.Fatal("the command ran")
	}
	cases := [][2]string{{"https://h/r", "https"}, {"HTTPS://h/r", "https"}, {"git@h:o/r.git", "ssh"}, {"ssh://h/r", "ssh"},
		{"git://h/r", "git"}, {"h:r", "ssh"}, {"/a:b", "file"}, {"ext::x", "ext"}}
	for _, c := range cases {
		if got := transport(c[0]); got != c[1] {
			t.Errorf("%s: %s, want %s", c[0], got, c[1])
		}
	}
}

func TestGitIsStoppedBeforeHlStopsWaiting(t *testing.T) {
	for _, m := range []string{"folder.init", "folder.clone"} {
		if limit := max(initLimit, cloneLimit); hlclient.DeadlineOf(m) <= limit+30*time.Second {
			t.Errorf("%s: hl waits %v, git may run %v", m, hlclient.DeadlineOf(m), limit)
		}
	}
}
