package worktreepool

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
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

// sleeping turns RemoveOnSleep on and marks ann's session closed.
func (r *rig) sleeping(agent string) {
	r.pool.RemoveOnSleep = func() bool { return true }
	r.db.Exec("UPDATE agent_state SET closed_at = 5 WHERE agent = ?", agent)
}

func TestANewFolderIsTheAgentsNameWithAHexEndOnItsBranchWithSetup(t *testing.T) {
	r := newRig(t)
	var setups []string
	r.pool.Setup = func(agent, repo, desk string) string {
		setups = append(setups, agent)
		return "set up"
	}
	r.pool.Name = func(agent string) string { return agent + "-name" }
	a := r.take("ann")
	base := filepath.Base(a.Path)
	if a.How != "new" || !regexp.MustCompile(`^ann-name-[0-9a-f]{6}$`).MatchString(base) || filepath.Dir(a.Path) != canonical(worktreepaths.Dir(r.repo)) {
		t.Fatalf("first take: %+v", a)
	}
	if a.Branch != "ann-work" || head(a.Path) != "ann-work" || len(a.Said) != 2 || len(setups) != 1 {
		t.Fatalf("branch or setup: %+v %v", a, setups)
	}
	var last, branch string
	r.db.QueryRow("SELECT worktree, branch FROM agent_state WHERE agent = 'ann'").Scan(&last, &branch)
	if last != a.Path || branch != "ann-work" || r.holder(a.Path) != "ann" {
		t.Fatalf("agent_state: %s %s", last, branch)
	}
	if again := r.take("ann"); again.Path != a.Path || again.How != "held" || len(setups) != 1 {
		t.Fatalf("the same folder again: %+v", again)
	}
}

// The founder's case: a closed agent keeps its folder however many others wake in between, so its conversation resumes there.
func TestTwoAgentsNeverShareAFolderAndAClosedOnesStaysItsOwn(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.give("ann")
	r.db.Exec("UPDATE agent_state SET closed_at = 5 WHERE agent = 'ann'")
	for _, a := range []string{"bob", "cat", "dan"} {
		if got := r.take(a); got.Path == d {
			t.Fatalf("%s took ann's folder", a)
		}
	}
	if got := r.take("ann"); got.Path != d || got.How != "held" || r.holder(d) != "ann" {
		t.Fatalf("ann's own folder: %+v", got)
	}
}

func TestTwoTakesAtOnceNeverShareAFolder(t *testing.T) {
	r := newRig(t)
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
			t.Fatalf("shared folder %s: %v", p, got)
		}
		seen[p] = true
	}
}

// A free desk from the shared pool goes to the agent that used it last, and to no other that records it.
func TestAFreeFolderGoesOnlyToItsLastAgent(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.db.Exec("UPDATE worktree_folders SET holder = NULL, last_agent = 'ann' WHERE path = ?", d)
	r.db.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES('bob','IDLE',1,?)", d)
	if got := r.take("bob"); got.Path == d {
		t.Fatal("bob took ann's last folder")
	}
	if got := r.take("ann"); got.Path != d || r.holder(d) != "ann" {
		t.Fatalf("ann's: %+v", got)
	}
}

func TestAFounderPinnedFolderGoesToNoAgent(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.free("ann")
	if err := Pin(r.db, d, Founder); err != nil {
		t.Fatal(err)
	}
	if got := r.take("ann"); got.Path == d {
		t.Fatal("a founder's folder went to an agent, even its last user")
	}
	if err := Pin(r.db, filepath.Join(r.repo, "nope"), "cat"); err == nil {
		t.Fatal("pinning a folder that is not a desk")
	}
}

func TestWithRemoveOnSleepOffAGiveBackLeavesTheFolderAsItIs(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("two\n"), 0o644)
	if g := r.give("ann"); g.Removed || g.Committed != "" || !exists(d) || head(d) != "ann-work" || r.holder(d) != "ann" {
		t.Fatalf("%+v", g)
	}
}

func TestRemoveOnSleepCommitsWIPDeletesTheFolderAndTheWakeMakesItAgainAtTheSamePath(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("two\n"), 0o644)
	r.sleeping("ann")
	g := r.give("ann")
	if !g.Removed || g.Committed == "" || exists(d) || r.holder(d) != "ann" {
		t.Fatalf("give back: %+v", g)
	}
	if msg := r.git(r.repo, "log", "-1", "--format=%s", "ann-work"); msg != "WIP at close (T42)" {
		t.Fatalf("commit: %q", msg)
	}
	r.take("bob") // another agent's wake in between neither takes the path nor drops its row
	got := r.take("ann")
	if got.Path != d || got.How != "made again" || head(d) != "ann-work" || r.git(d, "show", "HEAD:a.txt") != "two" {
		t.Fatalf("made again: %+v", got)
	}
}

func TestADetachedFolderGetsABranchBeforeItsWIPCommit(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.git(d, "checkout", "-q", "--detach")
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("three\n"), 0o644)
	r.sleeping("ann")
	g := r.give("ann")
	var branch string
	r.db.QueryRow("SELECT branch FROM agent_state WHERE agent = 'ann'").Scan(&branch)
	if !g.Removed || !strings.HasPrefix(branch, "ann-work-wip-") || r.git(r.repo, "log", "-1", "--format=%s", branch) != "WIP at close (T42)" {
		t.Fatalf("detached: %+v on %q", g, branch)
	}
}

func TestUntrackedFilesOrAHalfDoneMergeKeepTheFolder(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	r.sleeping("ann")
	os.WriteFile(filepath.Join(d, "new.txt"), []byte("x"), 0o644)
	if g := r.give("ann"); g.Removed || !exists(d) || len(g.Untracked) != 1 {
		t.Fatalf("untracked: %+v", g)
	}
	os.Remove(filepath.Join(d, "new.txt"))
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("ann\n"), 0o644)
	r.git(d, "commit", "-q", "-am", "ann's")
	r.git(r.repo, "checkout", "-q", "-b", "other", "main")
	os.WriteFile(filepath.Join(r.repo, "a.txt"), []byte("other\n"), 0o644)
	r.git(r.repo, "commit", "-q", "-am", "other's")
	r.git(r.repo, "checkout", "-q", "main")
	if res := gitrun.Run(d, "merge", "other"); res.RC == 0 {
		t.Fatal("the merge was meant to conflict")
	}
	if g := r.give("ann"); g.Removed || g.Committed != "" || !exists(d) || !strings.Contains(strings.Join(g.Untracked, " "), "merge in progress") {
		t.Fatalf("merge: %+v", g)
	}
}

func TestTheSweepDeletesAClosedAgentsFolderOnlyWithTheSettingOnceItsSessionLetGoAndTheGraceIsOver(t *testing.T) {
	r := newRig(t)
	d := r.take("ann").Path
	taken := r.at
	running := true
	r.pool.Running = func(string, string) bool { return running }
	sweep := func(now int64) {
		t.Helper()
		if err := r.pool.Sweep(now); err != nil {
			t.Fatal(err)
		}
	}
	r.db.Exec("UPDATE agent_state SET closed_at = 5 WHERE agent = 'ann'")
	running = false
	sweep(taken + GraceS + 1)
	if !exists(d) {
		t.Fatal("deleted with the setting off")
	}
	r.sleeping("ann")
	running = true
	sweep(taken + GraceS + 1)
	if !exists(d) {
		t.Fatal("a process still in the folder keeps it")
	}
	running = false
	sweep(taken + GraceS - 1)
	if !exists(d) {
		t.Fatal("a folder just taken by a wake is not deleted before its session starts")
	}
	sweep(taken + GraceS + 1)
	if exists(d) || r.holder(d) != "ann" {
		t.Fatal("deleted, and still ann's")
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

func TestAllDesksAreListedWhenNoRepoIsNamed(t *testing.T) {
	r := newRig(t)
	r.take("ann")
	all, err := List(r.db, "")
	if err != nil || len(all) != 1 {
		t.Fatalf("%v %v", all, err)
	}
}

func TestABranchNamedLikeTheWorkspaceGivesTheAgentANameGitCanMake(t *testing.T) {
	r := newRig(t)
	r.pool.Branch = func(agent string) string { return "team/" + agent }
	r.git(r.repo, "branch", "team")
	got := r.take("ann")
	if !strings.HasPrefix(got.Branch, "team-ann-") || len(got.Branch) != len("team-ann-")+6 || head(got.Path) != got.Branch {
		t.Fatalf("a branch named like the workspace is in the way: %+v", got)
	}
	var branch string
	r.db.QueryRow("SELECT branch FROM agent_state WHERE agent = 'ann'").Scan(&branch)
	if branch != got.Branch {
		t.Fatalf("on record: %q, took %q", branch, got.Branch)
	}
	// a folder of refs at the name blocks it too; a free name stays as it is
	r.git(r.repo, "branch", "team2/bob/old")
	r.pool.Branch = func(agent string) string { return "team2/" + agent }
	if b := r.take("bob"); !strings.HasPrefix(b.Branch, "team2-bob-") {
		t.Fatalf("refs under the name: %+v", b)
	}
	if c := r.take("cat"); c.Branch != "team2/cat" {
		t.Fatalf("a free name: %+v", c)
	}
}

func TestTheHashedBranchIsFoundAgainWithNoneOnRecord(t *testing.T) {
	r := newRig(t)
	r.pool.Branch = func(agent string) string { return "team/" + agent }
	r.git(r.repo, "branch", "team")
	first := r.take("ann")
	r.give("ann")
	r.db.Exec("UPDATE agent_state SET branch = NULL WHERE agent = 'ann'")
	if again := r.take("ann"); again.Branch != first.Branch {
		t.Fatalf("took %q, then %q", first.Branch, again.Branch)
	}
}
