package worktreepool

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/protected"
)

// Auto keeps the week's average of each day's most agents at once, rounded up, plus one; another project's agents are not counted, and
// a day the machine ran no agents of this project counts 0.
func TestAutoKeepsTheWeeksAverageDailyPeakPlusOne(t *testing.T) {
	r := newRig(t)
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	at := func(daysAgo, minute int) int64 {
		return now.AddDate(0, 0, -daysAgo).Truncate(time.Hour).Unix() - int64(minute*60)
	}
	row := func(minute int64, agent string) {
		if _, err := r.db.Exec("INSERT INTO load_minutes(minute, agent, cpu_avg, cpu_max, rss_avg, rss_max, procs_max) VALUES(?,?,0,0,0,0,1)", minute, agent); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok, err := AutoKept(r.db, []string{"a1"}, now); ok || err != nil {
		t.Fatalf("nothing recorded: no number (%v)", err)
	}
	row(at(1, 30), "a1")
	if _, ok, _ := AutoKept(r.db, []string{"a1"}, now); ok {
		t.Fatal("a day recorded is not a week")
	}
	// day 1: three at once, then two; day 2: one; day 3: only another project's agent ran
	for _, a := range []string{"a1", "a2", "a3"} {
		row(at(1, 10), a)
	}
	row(at(1, 20), "a1")
	row(at(1, 20), "a2")
	row(at(2, 5), "a1")
	row(at(3, 5), "x9")
	row(at(9, 5), "a1") // older than the week
	n, ok, err := AutoKept(r.db, []string{"a1", "a2", "a3"}, now)
	if err != nil || !ok || n != 3 { // (3 + 1 + 0) / 3 days, rounded up, plus one
		t.Fatalf("auto: %d %v %v", n, ok, err)
	}
}

func TestKeptIsAutoUntilAManualNumberIsSet(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	st := &protected.Store{Root: t.TempDir()}
	if k := GetKept(st, "/r"); k.Mode != Auto {
		t.Fatalf("default: %+v", k)
	}
	if err := PutKept(st, "/r", Kept{Mode: Manual, Value: 4}); err != nil {
		t.Fatal(err)
	}
	if k := GetKept(st, "/r"); k != (Kept{Mode: Manual, Value: 4}) {
		t.Fatalf("manual: %+v", k)
	}
	for _, bad := range []Kept{{Mode: Manual}, {Mode: Manual, Value: ManualMax + 1}, {Mode: "some"}} {
		if PutKept(st, "/r", bad) == nil {
			t.Errorf("%+v kept", bad)
		}
	}
	if err := PutKept(st, "/r", Kept{Mode: Auto}); err != nil || GetKept(st, "/r").Mode != Auto {
		t.Fatalf("back to auto: %v", err)
	}
}

// The prune removes free desks beyond the kept number that went unused since the cutoff, the oldest first: never one held, pinned,
// used since, or holding work; the branch it had stays.
func TestThePruneRemovesTheOldestFreeDesksBeyondTheKeptNumber(t *testing.T) {
	r := newRig(t)
	desks := map[string]string{}
	for _, a := range []string{"ann", "bob", "cat", "dan", "eve", "fay"} {
		desks[a] = r.take(a).Path
	}
	for _, a := range []string{"ann", "bob", "cat", "dan", "eve"} { // given back in this order: ann's is the oldest
		r.give(a)
	}
	if err := Pin(r.db, desks["bob"], Founder); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(desks["cat"], "loose.txt"), []byte("x\n"), 0o644) // untracked work in a free desk
	cutoff := r.at                                                               // eve's desk is used after it
	r.give("eve")
	r.db.Exec("UPDATE worktree_folders SET last_used = ? WHERE path = ?", cutoff+100, desks["eve"])

	// six desks, two kept: up to four go, but bob's is pinned, cat's holds work, eve's is too recent and fay holds hers
	got, err := r.pool.Prune(r.repo, 2, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != desks["ann"] || got[1].Path != desks["dan"] {
		t.Fatalf("pruned: %+v", got)
	}
	for _, a := range []string{"ann", "dan"} {
		if exists(desks[a]) {
			t.Errorf("%s's desk is still there", a)
		}
		if gitBranch := r.git(r.repo, "branch", "--list", a+"-work"); gitBranch == "" {
			t.Errorf("%s's branch went with its desk", a)
		}
	}
	left, _ := List(r.db, r.repo)
	if len(left) != 4 {
		t.Fatalf("left: %+v", left)
	}
	if more, _ := r.pool.Prune(r.repo, 4, cutoff+1000); len(more) != 0 {
		t.Fatalf("at the kept number nothing goes: %+v", more)
	}
}

func TestRemoveKeepsADeskTakenOrPinnedSinceItWasListed(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	if why, err := r.pool.Remove(d, math.MaxInt64); why == "" || err != nil {
		t.Fatalf("a held desk: %q %v", why, err)
	}
	r.give("ann")
	Pin(r.db, d, "a9")
	if why, _ := r.pool.Remove(d, math.MaxInt64); why != "pinned" {
		t.Fatalf("a pinned desk: %q", why)
	}
	Pin(r.db, d, "")
	r.pool.Running = func(string, string) bool { return true }
	if why, _ := r.pool.Remove(d, math.MaxInt64); why == "" {
		t.Fatal("a desk a session runs in")
	}
	r.pool.Running = nil
	if why, _ := r.pool.Remove(d, 0); why != "used since" {
		t.Fatalf("used since the cutoff: %q", why)
	}
	// a commit made on the detached HEAD of a free desk is on no branch: removing the desk would lose it
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("two\n"), 0o644)
	r.git(d, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qam", "loose")
	if why, _ := r.pool.Remove(d, math.MaxInt64); why != "its HEAD is on no branch" || !exists(d) {
		t.Fatalf("a commit on no branch: %q", why)
	}
	r.git(d, "branch", "kept-loose")
	if why, err := r.pool.Remove(d, math.MaxInt64); why != "" || err != nil || exists(d) {
		t.Fatalf("a free desk: %q %v", why, err)
	}
	if left, _ := List(r.db, r.repo); len(left) != 0 {
		t.Fatalf("still listed: %+v", left)
	}
}

// While its folder is being deleted a desk stays listed, pinned to Removing: its last agent's next take neither gets it nor treats its
// folder as one of its own outside the pool.
func TestADeskBeingRemovedIsTakenByNobody(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.give("ann")
	r.db.Exec("UPDATE worktree_folders SET pinned_to = ? WHERE path = ?", Removing, d)
	if got := r.take("ann"); got.Path == d || got.How == "own" {
		t.Fatalf("took the desk being removed: %+v", got)
	}
}
