package gates

import (
	"strings"
	"testing"
)

// A merge closes a task only when the pull request is in the repository the task's holder works in: the watch names o/r on GitHub, and
// the origin of the holder's folder must be that repository. A commit merged in another repository can be on this one's default branch
// too (a fork, a mirror), and says nothing of the work here: the task is given back with both names. So is one whose origin is no
// remote on a host, which cannot be told to be it, and one on another host (a GitLab project of the same name). Names that differ only in case are one repository. The clone holds the merge in
// every case here, so only the repository decides.
func TestAMergeInAnotherRepositoryDoesNotCloseTheTask(t *testing.T) {
	rig := func(origin string) (*landRig, string) {
		r := newLandRig(t, "main")
		r.exec("UPDATE tasks SET watch_vendor = 'github' WHERE id = 1")
		r.s.Vendors["github"] = r.s.Vendors["test"]
		sha := r.merge("main")
		gitAt(t, r.repo, "fetch", "-q", "origin", "main")
		if origin != "" {
			gitAt(t, r.repo, "remote", "set-url", "origin", origin)
		}
		fetched := 0
		r.s.Fetch = func(string) error { fetched++; return nil }
		r.pass(1000)
		if (fetched != 0) != strings.EqualFold(origin, "https://github.com/o/r.git") {
			t.Fatalf("origin %q: fetched %d times", origin, fetched)
		}
		return r, sha
	}
	for origin, why := range map[string]string{
		"https://github.com/other/r.git": "repository here is other/r: a merge in one is not looked for in the other",
		"git@github.com:o/r-fork.git":    "repository here is o/r-fork: a merge in one is not looked for in the other",
		"":                               "repository here is no remote on a host, so it cannot be told to be that one",
		"git@gitlab.com:o/r.git":         "repository here is gitlab.com/o/r: a merge in one is not looked for in the other",
	} {
		r, sha := rig(origin)
		e, _, _, blockers := r.task(2)
		gate, _, _, _ := r.task(1)
		if e != "" || blockers != 0 || !strings.HasPrefix(gate, "merged "+sha+"; T2 not closed: its pull request is in o/r, and ") || !strings.HasSuffix(gate, why) {
			t.Fatalf("origin %q: T2 %q (%d blockers), the gate %q", origin, e, blockers, gate)
		}
		if !strings.Contains(r.comments(), why) {
			t.Fatalf("origin %q: the comment says why: %s", origin, r.comments())
		}
	}
	for _, origin := range []string{"https://github.com/o/r.git", "https://github.com/O/R.git"} {
		r, sha := rig(origin)
		if e, via, by, _ := r.task(2); e != sha || via != "sha" || by != r.bob {
			t.Fatalf("origin %q: T2 done %q via %q by %q, want the merge commit by its holder", origin, e, via, by)
		}
	}
}
