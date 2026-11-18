package worktreepool

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/scheduler/worktreepaths"
)

type rig struct {
	t    *testing.T
	db   *sql.DB
	repo string
	pool *Pool
	at   int64
}

func newRig(t *testing.T) *rig {
	t.Helper()
	d, err := store.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	repo := filepath.Join(t.TempDir(), "proj")
	os.MkdirAll(repo, 0o755)
	r := &rig{t: t, db: d, repo: canonical(repo), at: 1000}
	r.git(repo, "init", "-q", "-b", "main")
	r.git(repo, "config", "user.email", "t@example.com")
	r.git(repo, "config", "user.name", "t")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0o644)
	r.git(repo, "add", "a.txt")
	r.git(repo, "commit", "-q", "-m", "first")
	r.pool = &Pool{DB: d,
		Base:   func(string, string) (string, string, error) { return "main", "local main", nil },
		Branch: func(agent string) string { return agent + "-work" },
		Now:    func() time.Time { r.at++; return time.Unix(r.at, 0) }}
	return r
}

func (r *rig) git(dir string, args ...string) string {
	r.t.Helper()
	res := gitrun.RunTimeout(dir, time.Minute, args...)
	if res.RC != 0 {
		r.t.Fatalf("git %v in %s: %s", args, dir, res.Stderr)
	}
	return strings.TrimSpace(res.Stdout)
}

func (r *rig) take(agent string) Taken {
	r.t.Helper()
	got, why, err := r.pool.Take(agent, r.repo)
	if err != nil || why != "" {
		r.t.Fatalf("take %s: %v %s", agent, err, why)
	}
	return got
}

func (r *rig) give(agent string) Given {
	r.t.Helper()
	g, err := r.pool.GiveBack(agent, 42)
	if err != nil {
		r.t.Fatalf("give back %s: %v", agent, err)
	}
	return g
}

func (r *rig) holder(desk string) string {
	var h sql.NullString
	if err := r.db.QueryRow("SELECT holder FROM worktree_folders WHERE path = ?", desk).Scan(&h); err != nil {
		r.t.Fatal(err)
	}
	return h.String
}

func TestANewDeskIsTheLowestFreeNumberWithTheAgentsBranchAndSetup(t *testing.T) {
	r := newRig(t)
	var setups []string
	r.pool.Setup = func(agent, repo, desk string) string {
		setups = append(setups, agent+" "+filepath.Base(desk))
		return "set up"
	}
	a := r.take("ann")
	if a.How != "new" || filepath.Base(a.Path) != "desk-1" || filepath.Dir(a.Path) != canonical(worktreepaths.Dir(r.repo)) {
		t.Fatalf("first take: %+v", a)
	}
	if a.Branch != "ann-work" || head(a.Path) != "ann-work" || len(a.Said) != 2 || len(setups) != 1 {
		t.Fatalf("branch or setup: %+v %v", a, setups)
	}
	b := r.take("bob")
	if filepath.Base(b.Path) != "desk-2" {
		t.Fatalf("second: %+v", b)
	}
	// desk-1's number is taken again once its folder is gone from the pool
	r.db.Exec("DELETE FROM worktree_folders WHERE path = ?", a.Path)
	r.git(r.repo, "worktree", "remove", "--force", a.Path)
	r.give("ann")
	c := r.take("cat")
	if filepath.Base(c.Path) != "desk-1" {
		t.Fatalf("the lowest free number: %+v", c)
	}
	var last, branch string
	r.db.QueryRow("SELECT worktree, branch FROM agent_state WHERE agent = 'cat'").Scan(&last, &branch)
	if last != c.Path || branch != "cat-work" {
		t.Fatalf("agent_state: %s %s", last, branch)
	}
}

func TestATakeGivesTheLastDeskWhenFreeElseTheOldestFree(t *testing.T) {
	r := newRig(t)
	d1 := r.take("ann").Path
	d2 := r.take("bob").Path
	r.give("ann") // d1 free, older
	r.give("bob") // d2 free, newer
	if got := r.take("bob"); got.Path != d2 || got.How != "last" || head(d2) != "bob-work" {
		t.Fatalf("bob's last desk: %+v", got)
	}
	r.give("bob")
	if got := r.take("cat"); got.Path != d1 || got.How != "free" {
		t.Fatalf("cat gets the oldest free: %+v", got)
	}
	// ann's last desk (d1) is cat's now: ann gets d2, on her own branch
	got := r.take("ann")
	if got.Path != d2 || got.How != "free" || head(d2) != "ann-work" {
		t.Fatalf("ann after her desk was taken: %+v", got)
	}
	if again := r.take("ann"); again.Path != d2 || again.How != "held" {
		t.Fatalf("a held desk is given again: %+v", again)
	}
}

func TestGiveBackCommitsTrackedChangesAsWIPOnTheAgentsBranch(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("two\n"), 0o644)
	g := r.give("ann")
	if !g.Freed || g.Committed == "" || r.holder(d) != "" {
		t.Fatalf("give back: %+v", g)
	}
	if msg := r.git(d, "log", "-1", "--format=%s", "ann-work"); msg != "WIP at close (T42)" {
		t.Fatalf("commit: %q", msg)
	}
	if !clean(d) || head(d) != "" {
		t.Fatal("the desk is clean after the WIP commit and holds no branch")
	}
	// bob takes it and finds his own branch, not ann's work; ann's branch still has it
	r.take("bob")
	if head(d) != "bob-work" || r.git(r.repo, "show", "ann-work:a.txt") != "two" {
		t.Fatal("ann's WIP stays on her branch")
	}
}

func TestADetachedDeskGetsABranchBeforeItsWIPCommit(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.git(d, "checkout", "-q", "--detach")
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("three\n"), 0o644)
	g := r.give("ann")
	var branch string
	r.db.QueryRow("SELECT branch FROM agent_state WHERE agent = 'ann'").Scan(&branch)
	if !g.Freed || g.Committed == "" || !strings.HasPrefix(branch, "ann-work-wip-") || r.git(r.repo, "log", "-1", "--format=%s", branch) != "WIP at close (T42)" {
		t.Fatalf("detached: %+v on %q", g, branch)
	}
}

func TestUntrackedFilesKeepTheDeskHeldAndAreToldOnce(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	os.WriteFile(filepath.Join(d, "new.txt"), []byte("x"), 0o644)
	g := r.give("ann")
	if g.Freed || g.Tell == "" || !strings.Contains(g.Tell, "new.txt") || r.holder(d) != "ann" {
		t.Fatalf("first: %+v", g)
	}
	if again := r.give("ann"); again.Freed || again.Tell != "" {
		t.Fatalf("told once: %+v", again)
	}
	if other := r.take("bob"); other.Path == d {
		t.Fatal("a held desk went to another agent")
	}
	os.Remove(filepath.Join(d, "new.txt"))
	if g := r.give("ann"); !g.Freed {
		t.Fatalf("cleaned up: %+v", g)
	}
}

func TestAPinnedDeskGoesToNobodyElse(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.give("ann")
	if err := Pin(r.db, d, Founder); err != nil {
		t.Fatal(err)
	}
	if got := r.take("ann"); got.Path == d {
		t.Fatal("a founder's desk went to an agent, even its last user")
	}
	Pin(r.db, d, "cat")
	if got := r.take("bob"); got.Path == d {
		t.Fatal("cat's desk went to bob")
	}
	if got := r.take("cat"); got.Path != d || got.How != "pinned" {
		t.Fatalf("cat's own pin: %+v", got)
	}
	if err := Pin(r.db, filepath.Join(r.repo, "nope"), "cat"); err == nil {
		t.Fatal("pinning a folder that is not a desk")
	}
}

func TestAFreeDeskWithChangesIsPassedOver(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.give("ann")
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("left by hand\n"), 0o644)
	if got := r.take("bob"); got.Path == d {
		t.Fatal("bob was put in a desk with someone's changes")
	}
}

func TestTwoTakesAtOnceNeverShareADesk(t *testing.T) {
	r := newRig(t)
	for _, a := range []string{"ann", "bob", "cat"} {
		r.take(a)
		r.give(a)
	}
	var wg sync.WaitGroup
	got := make([]string, 6)
	for i, a := range []string{"ann", "bob", "cat", "dan", "eve", "fay"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tk, why, err := r.pool.Take(a, r.repo)
			if err != nil || why != "" {
				t.Errorf("%s: %v %s", a, err, why)
			}
			got[i] = tk.Path
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, p := range got {
		if seen[p] {
			t.Fatalf("shared desk %s: %v", p, got)
		}
		seen[p] = true
	}
}

func TestAdoptKeepsTheOldFoldersAsTheyAreHeldByTheirAgents(t *testing.T) {
	r := newRig(t)
	old := filepath.Join(worktreepaths.Dir(r.repo), "handline.architect")
	r.git(r.repo, "worktree", "add", "-q", "-b", "story4", old)
	stray := filepath.Join(t.TempDir(), "elsewhere")
	r.git(r.repo, "worktree", "add", "-q", "--detach", stray)
	mine := filepath.Join(worktreepaths.Dir(r.repo), "founders")
	r.git(r.repo, "worktree", "add", "-q", "-b", "mine", mine)
	old, mine = canonical(old), canonical(mine)
	r.db.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES('a2','IDLE',1,?)", r.repo+"::"+old)
	n, err := r.pool.Adopt(r.repo)
	if err != nil || n != 2 {
		t.Fatalf("adopted %d: %v (only the folders under the worktrees dir)", n, err)
	}
	if r.holder(old) != "a2" {
		t.Fatal("the agent working in it holds it")
	}
	pin := func(path string) string {
		var p sql.NullString
		r.db.QueryRow("SELECT pinned_to FROM worktree_folders WHERE path = ?", path).Scan(&p)
		return p.String
	}
	if pin(mine) != Founder || pin(old) != "" {
		t.Fatalf("pins: unrecorded %q, held %q", pin(mine), pin(old))
	}
	if got := r.take("bob"); got.Path == mine {
		t.Fatalf("bob was given someone's folder: %+v", got)
	}
	var branch string
	r.db.QueryRow("SELECT branch FROM agent_state WHERE agent = 'a2'").Scan(&branch)
	if branch != "story4" {
		t.Fatalf("branch: %q", branch)
	}
	if n, _ := r.pool.Adopt(r.repo); n != 0 {
		t.Fatal("adopting twice adds nothing")
	}
	if got := r.take("a2"); got.Path != old || got.How != "held" || head(old) != "story4" {
		t.Fatalf("a2 resumes in its folder: %+v", got)
	}
}

func TestAFolderOutsideThePoolStaysTheAgentsOwn(t *testing.T) {
	r := newRig(t)
	r.db.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES('ann','IDLE',1,?)", r.repo)
	got := r.take("ann")
	if got.Path != r.repo || got.How != "own" || head(r.repo) != "main" {
		t.Fatalf("the repo's own checkout: %+v", got)
	}
	var n int
	r.db.QueryRow("SELECT COUNT(*) FROM worktree_folders").Scan(&n)
	if n != 0 {
		t.Fatal("no desk was made or taken")
	}
}

func TestTheSweepGivesBackAClosedAgentsDeskOnceItsSessionLetGoAndTheGraceIsOver(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	taken := r.at
	running := true
	r.pool.Running = func(string, string) bool { return running }
	sweep := func(now int64) {
		t.Helper()
		if err := r.pool.Sweep(now, nil); err != nil {
			t.Fatal(err)
		}
	}
	sweep(taken + GraceS + 1)
	if r.holder(d) != "ann" {
		t.Fatal("an open session keeps its desk")
	}
	r.db.Exec("UPDATE agent_state SET closed_at = 5 WHERE agent = 'ann'")
	sweep(taken + GraceS + 1)
	if r.holder(d) != "ann" {
		t.Fatal("a process still in the folder keeps it")
	}
	running = false
	sweep(taken + GraceS - 1)
	if r.holder(d) != "ann" {
		t.Fatal("a desk just taken by a wake is not given back before its session starts")
	}
	os.WriteFile(filepath.Join(d, "u.txt"), []byte("x"), 0o644)
	var told []string
	r.pool.Sweep(taken+GraceS+1, func(agent, text string) { told = append(told, agent) })
	r.pool.Sweep(taken+GraceS+2, func(agent, text string) { told = append(told, agent) })
	if r.holder(d) != "ann" || len(told) != 1 {
		t.Fatalf("untracked: held and told once, got %v", told)
	}
	os.Remove(filepath.Join(d, "u.txt"))
	sweep(taken + GraceS + 3)
	if r.holder(d) != "" {
		t.Fatal("given back")
	}
}

func TestAHeldDeskOnADetachedHeadKeepsTheBranchOnRecord(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.git(d, "checkout", "-q", "--detach")
	if got := r.take("ann"); got.How != "held" || got.Branch != "ann-work" {
		t.Fatalf("%+v", got)
	}
	var branch string
	r.db.QueryRow("SELECT branch FROM agent_state WHERE agent = 'ann'").Scan(&branch)
	if branch != "ann-work" {
		t.Fatalf("branch on record: %q", branch)
	}
}

func TestAnAgentWithNoBranchOnRecordGetsTheOneItsKeyAlreadyHas(t *testing.T) {
	r := newRig(t)
	r.git(r.repo, "branch", "a7")
	os.WriteFile(filepath.Join(r.repo, "a.txt"), []byte("on main\n"), 0o644)
	if got := r.take("a7"); got.Branch != "a7" || head(got.Path) != "a7" {
		t.Fatalf("the agent's old branch is left behind: %+v", got)
	}
}

func TestAHalfDoneMergeKeepsTheDeskHeldAndIsToldNotCommitted(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("ann\n"), 0o644)
	r.git(d, "commit", "-q", "-am", "ann's")
	r.git(r.repo, "checkout", "-q", "-b", "other", "main")
	os.WriteFile(filepath.Join(r.repo, "a.txt"), []byte("other\n"), 0o644)
	r.git(r.repo, "commit", "-q", "-am", "other's")
	r.git(r.repo, "checkout", "-q", "main")
	if res := gitrun.Run(d, "merge", "other"); res.RC == 0 {
		t.Fatal("the merge was meant to conflict")
	}
	g := r.give("ann")
	if g.Freed || g.Committed != "" || !strings.Contains(g.Tell, "merge in progress") || r.holder(d) != "ann" {
		t.Fatalf("%+v", g)
	}
}

func TestAGoneDeskLeavesThePoolAndFreesItsBranch(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	os.RemoveAll(d)
	got := r.take("ann")
	if got.Path != d || got.How != "new" || head(got.Path) != "ann-work" { // desk-1 again: the lowest free number
		t.Fatalf("%+v", got)
	}
	var n int
	r.db.QueryRow("SELECT COUNT(*) FROM worktree_folders WHERE holder = 'ann'").Scan(&n)
	if n != 1 {
		t.Fatalf("ann holds %d desks", n)
	}
}

func TestADeskASessionStillRunsInIsNeitherTakenNorGivenBack(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.pool.Running = func(_, desk string) bool { return desk == d }
	r.db.Exec("UPDATE agent_state SET closed_at = 5 WHERE agent = 'ann'")
	if g := r.give("ann"); g.Freed {
		t.Fatal("given back under a running session")
	}
	r.db.Exec("UPDATE worktree_folders SET holder = NULL WHERE path = ?", d)
	if got := r.take("bob"); got.Path == d {
		t.Fatal("taken under a running session")
	}
}

func TestAllDesksAreListedWhenNoRepoIsNamed(t *testing.T) {
	r := newRig(t)
	r.take("ann")
	all, err := List(r.db, "")
	if err != nil || len(all) != 1 {
		t.Fatalf("%v %v", all, err)
	}
}
