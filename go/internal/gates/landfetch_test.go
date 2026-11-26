package gates

import (
	"testing"
	"time"
)

// The fetch ends at its limit even when what git started does not: here the origin's upload-pack is a command that sleeps half a
// minute and holds git's output open, as a stuck ssh or https helper does. Killing git alone would wait for it; the whole process
// group is killed, so the gate pass that ran the fetch goes on to its next gate.
func TestAFetchEndsAtItsLimitThoughAHelperOfGitHangs(t *testing.T) {
	r := newLandRig(t, "main")
	gitAt(t, r.repo, "config", "remote.origin.uploadpack", "sleep 30 #")
	began := time.Now()
	err := fetchBase(r.repo, 300*time.Millisecond, "file")
	if err == nil || err.Error() != "git fetch timed out after 300ms" {
		t.Fatalf("the fetch: %v", err)
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Fatalf("the fetch came back after %s, held by the helper it started", took.Round(time.Second))
	}
}
