package gates

import (
	"errors"
	"strings"
	"testing"
)

// The daemon fetches over https, ssh and git only, so an origin that is a local path is never fetched: git refuses the transport, and
// no later fetch goes better. The gate does not wait out its deadline on it. It judges the clone as it stands, and after its looks
// gives the task back saying the origin cannot be fetched. A clone that already holds the merge, fetched by someone else, still closes.
func TestAnOriginTheDaemonMayNotFetchGivesTheTaskBackAfterItsLooks(t *testing.T) {
	r := newLandRig(t, "main")
	r.s.Fetch = nil // the daemon's own fetch, with no transport added for the test's local origin
	if err := FetchBase(r.repo); !errors.Is(err, ErrTransport) || err.Error() != "git fetch: transport 'file' not allowed" {
		t.Fatalf("a local origin: %v", err)
	}
	sha := r.merge("main")
	for i := range UnlandedReads - 1 {
		r.pass(int64(1000 + 60*i))
		if e, _, _, b := r.task(2); e != "" || b != 1 {
			t.Fatalf("look %d: T2 %q, %d blockers", i+1, e, b)
		}
		if e, _, _, _ := r.task(1); e != "" || r.watchError() != "passed, not closing T2 yet: "+sha[:7]+" is not on origin/main here" {
			t.Fatalf("look %d: the gate %q saying %q", i+1, e, r.watchError())
		}
	}
	r.pass(2000)
	e, _, _, blockers := r.task(2)
	gate, _, _, _ := r.task(1)
	if e != "" || blockers != 0 || !strings.HasPrefix(gate, "merged "+sha+"; T2 not closed: its merge commit "+sha[:7]+" is not on origin/main of ") ||
		!strings.HasSuffix(gate, "and the daemon cannot fetch that repository's origin to look again (git fetch: transport 'file' not allowed: it fetches over https, ssh, git only)") {
		t.Fatalf("T2 %q (%d blockers), the gate %q", e, blockers, gate)
	}
	if !strings.Contains(r.memos(), "transport 'file' not allowed") {
		t.Fatalf("the comment says why: %s", r.memos())
	}

	r = newLandRig(t, "main")
	r.s.Fetch = nil
	sha = r.merge("main")
	gitAt(t, r.repo, "fetch", "-q", "origin", "main") // someone at the clone fetched it
	r.pass(1000)
	if e, via, by, _ := r.task(2); e != sha || via != "sha" || by != r.bob {
		t.Fatalf("a clone that holds the merge: T2 done %q via %q by %q", e, via, by)
	}
}
