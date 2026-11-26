package lifecycle

import (
	"os"
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
		ref := filepath.Join(".git", "refs", "heads")
		kept := slices.ContainsFunc(got.NoWrite, func(p string) bool { return strings.HasSuffix(p, filepath.Join(ref, c.def)) })
		other := slices.ContainsFunc(got.NoWrite, func(p string) bool { return c.def != "main" && strings.HasSuffix(p, filepath.Join(ref, "main")) })
		if !kept || other || len(got.NoWrite) != 4 {
			t.Errorf("%s: the files a session may not write: %v", c.why, got.NoWrite)
		}
	}

	// anyone but the lander gets no push rule
	r := newLaunchRig(t)
	var got cli.LaunchArgs
	r.l.Ports.Vendors = argsVendors{fakeVendors{"claude"}, argsDriver{fakeDriver{vendor: "claude"}, &got}}
	r.l.Ports.Held = func(string) team.Held { return team.Held{LandBy: "someone-else"} }
	if _, _, err := r.build("ann", nil, false, "main"); err != nil || got.LandPush != "" {
		t.Fatalf("not the lander: %q %v", got.LandPush, err)
	}
}
