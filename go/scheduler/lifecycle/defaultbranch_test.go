package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cli "github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/gitrun"
)

// cloneOf is a clone of a bare repository whose default branch is def, holding one commit on it and on each of beside. named is
// whether the clone's origin/HEAD is left as the clone made it; without it, it names no branch, as in a repository made and then pushed.
func cloneOf(t *testing.T, named bool, def string, beside ...string) string {
	t.Helper()
	root := t.TempDir()
	bare, seed, clone := filepath.Join(root, "origin.git"), filepath.Join(root, "seed"), filepath.Join(root, "clone")
	git := func(dir string, args ...string) {
		t.Helper()
		if r := gitrun.Run(dir, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...); r.RC != 0 {
			t.Fatalf("git %v: %+v", args, r)
		}
	}
	git(root, "init", "-q", "--bare", "-b", def, bare)
	git(root, "init", "-q", "-b", def, seed)
	git(seed, "commit", "-q", "--allow-empty", "-m", "base")
	push := []string{"push", "-q", bare, def}
	for _, b := range beside {
		push = append(push, def+":"+b)
	}
	git(seed, push...)
	git(root, "clone", "-q", bare, clone)
	if !named {
		git(clone, "remote", "set-head", "origin", "--delete")
	}
	return clone
}

// A launch reads the branch work merges into from the repository and does not rename it: where the remote's default is develop
// and a main sits beside it, origin/HEAD stays on develop, the session is kept from moving develop, and the workspace's lander may
// push to develop. An origin/HEAD that names no branch is given the repository's own: master where that is the branch, main as before.
func TestALaunchReadsTheRepositorysDefaultBranchAndDoesNotRenameIt(t *testing.T) {
	for _, c := range []struct {
		why, def string
		named    bool
		beside   []string
	}{
		{"develop beside a main", "develop", true, []string{"main"}},
		{"master, origin/HEAD naming nothing", "master", false, nil},
		{"main, origin/HEAD naming nothing", "main", false, nil},
		{"main", "main", true, nil},
	} {
		r := newLaunchRig(t)
		var got cli.LaunchArgs
		r.l.Ports.Vendors = argsVendors{fakeVendors{"claude"}, argsDriver{fakeDriver{vendor: "claude"}, &got}}
		r.l.Ports.Held = func(string) team.Held { return team.Held{LandBy: "ann"} }
		clone := cloneOf(t, c.named, c.def, c.beside...)
		site := &Site{Cwd: clone, Env: map[string]string{}, Handle: "term_1", WorktreeID: "wt_1", PID: os.Getpid(), HlExe: "hl"}
		if _, err := r.l.Prepare(site, "ann", nil, false, "", "main", "", ""); err != nil {
			t.Fatalf("%s: %v", c.why, err)
		}
		if head := strings.TrimSpace(gitrun.Run(clone, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD").Stdout); head != "origin/"+c.def {
			t.Errorf("%s: origin/HEAD after the launch is %q", c.why, head)
		}
		if got.LandPush != c.def {
			t.Errorf("%s: the lander may push to %q", c.why, got.LandPush)
		}
		// the default branch's ref, the packed refs and origin/HEAD, which says which branch that is, each with its lock; main's ref is
		// not among them where main is not the default
		real, _ := filepath.EvalSymlinks(clone)
		var want []string
		for _, p := range []string{"refs/heads/" + c.def, "packed-refs", "refs/remotes/origin/HEAD"} {
			want = append(want, filepath.Join(real, ".git", p), filepath.Join(real, ".git", p+".lock"))
		}
		var noWrite []string
		for _, p := range got.NoWrite {
			noWrite = append(noWrite, realFile(p))
		}
		if !slices.Equal(noWrite, want) {
			t.Errorf("%s: the files a session may not write are %v, want %v", c.why, noWrite, want)
		}
		fetchesWithout(t, c.why, clone, c.def, noWrite)
	}

	// anyone but the lander gets no push rule
	r := newLaunchRig(t)
	var got cli.LaunchArgs
	r.l.Ports.Vendors = argsVendors{fakeVendors{"claude"}, argsDriver{fakeDriver{vendor: "claude"}, &got}}
	r.l.Ports.Held = func(string) team.Held { return team.Held{LandBy: "someone-else"} }
	if _, _, err := r.build("ann", nil, false, "main"); err != nil || got.LandPush != "" {
		t.Fatalf("not the lander: %q %v", got.LandPush, err)
	}

	// the lander on a runner: its folder is the runner's, so no repository of this machine at that path is asked what its default is
	r.l.Ports.Held = func(string) team.Held { return team.Held{LandBy: "ann"} }
	r.l.Ports.Devices = fakeDevices{"on": {"ann": "box"}, "box": {"main": "/r/main"}}
	far := &Site{Cwd: cloneOf(t, true, "develop", "main"), Env: map[string]string{}, Node: "box", Handle: "term_1", WorktreeID: "wt_1", PID: os.Getpid(), HlExe: "hl"}
	if _, _, err := r.l.Build(far, "ann", nil, false, "main"); err != nil || got.LandPush != "main" {
		t.Fatalf("the lander on a runner: %q %v", got.LandPush, err)
	}
}

// realFile is path with the links of its folder resolved, as a sandbox profile names a file.
func realFile(path string) string {
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(dir, filepath.Base(path))
	}
	return path
}

// fetchesWithout is a session's git in clone with the files of noWrite denied to it, as the sandbox denies them (macOS's own
// sandbox-exec; skipped where it cannot be run, as inside another sandbox): the remote's new commit is fetched, a commit is made and
// pushed, and origin/HEAD cannot be pointed at another branch or removed.
func fetchesWithout(t *testing.T, why, clone, def string, noWrite []string) {
	t.Helper()
	if err := exec.Command("sandbox-exec", "-p", "(version 1)(allow default)", "true").Run(); err != nil {
		t.Logf("%s: no sandbox-exec here, so git is not run with the files denied: %v", why, err)
		return
	}
	profile := "(version 1)(allow default)(deny file-write*"
	for _, p := range noWrite {
		profile += fmt.Sprintf(" (literal %q)", p)
	}
	profile += ")"
	denied := func(args ...string) (string, error) {
		cmd := exec.Command("sandbox-exec", append([]string{"-p", profile, "git", "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = clone
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out := func(dir string, args ...string) string {
		t.Helper()
		r := gitrun.Run(dir, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if r.RC != 0 {
			t.Fatalf("%s: git %v: %+v", why, args, r)
		}
		return strings.TrimSpace(r.Stdout)
	}
	seed, bare := filepath.Join(filepath.Dir(clone), "seed"), filepath.Join(filepath.Dir(clone), "origin.git")
	out(seed, "commit", "-q", "--allow-empty", "-m", "landed by someone else")
	out(seed, "push", "-q", bare, def, def+":other")
	if said, err := denied("fetch", "origin"); err != nil {
		t.Fatalf("%s: a fetch with origin/HEAD denied failed: %v: %s", why, err, said)
	}
	if got, want := out(clone, "rev-parse", "origin/"+def), out(seed, "rev-parse", "HEAD"); got != want {
		t.Fatalf("%s: the fetch left origin/%s at %s, the remote has %s", why, def, got, want)
	}
	if said, err := denied("-c", "remote.origin.followRemoteHEAD=never", "fetch", "origin"); err != nil || strings.Contains(said, "error") {
		t.Fatalf("%s: a fetch told to leave origin/HEAD alone: %v: %s", why, err, said)
	}
	for _, args := range [][]string{{"checkout", "-q", "-b", "mine"}, {"commit", "-q", "--allow-empty", "-m", "mine"}, {"push", "-q", "origin", "HEAD:refs/heads/mine"}} {
		if said, err := denied(args...); err != nil {
			t.Fatalf("%s: git %v with the files denied: %v: %s", why, args, err, said)
		}
	}
	for _, args := range [][]string{
		{"symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/other"},
		{"remote", "set-head", "origin", "other"},
		{"remote", "set-head", "origin", "-d"},
		{"update-ref", "refs/heads/" + def, "refs/remotes/origin/other"},
	} {
		if said, err := denied(args...); err == nil {
			t.Fatalf("%s: git %v went through: %s", why, args, said)
		}
	}
	if head := out(clone, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); head != "origin/"+def || gitrun.DefaultBranch(clone) != def {
		t.Fatalf("%s: origin/HEAD is %q and the default branch %q", why, head, gitrun.DefaultBranch(clone))
	}
}
