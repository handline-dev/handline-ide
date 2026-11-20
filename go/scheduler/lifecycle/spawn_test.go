package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/protected"
	"github.com/handline/handline/go/scheduler/worktreepool"
	"github.com/handline/handline/go/scheduler/worktreesetup"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func repoWithOrigin(t *testing.T) (repo, origin string) {
	t.Helper()
	origin = t.TempDir()
	gitIn(t, origin, "init", "-q", "-b", "main", "--bare")
	repo = t.TempDir()
	gitIn(t, repo, "init", "-q", "-b", "main")
	gitIn(t, repo, "remote", "add", "origin", origin)
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "one")
	gitIn(t, repo, "push", "-q", "origin", "main")
	return
}

// Local main may be stale or another history (a rewritten repository): a new worktree takes the fetched origin/main whichever side leads,
// and says when local main holds commits it leaves out.
func TestPickBaseBranchesFromOriginEvenWhenLocalMainLeadsOrDiverged(t *testing.T) {
	repo, origin := repoWithOrigin(t)
	if ref, why, _ := PickBase(repo, "main"); ref != "origin/main" || why != "fetched" {
		t.Fatalf("%q %q", ref, why)
	}
	other := t.TempDir()
	gitIn(t, other, "clone", "-q", origin, ".")
	gitIn(t, other, "commit", "-q", "--allow-empty", "-m", "pushed by someone else")
	gitIn(t, other, "push", "-q", "origin", "main")
	if ref, _, _ := PickBase(repo, "main"); ref != "origin/main" {
		t.Fatalf("origin leads: %q", ref)
	}
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "local only")
	if ref, why, _ := PickBase(repo, "main"); ref != "origin/main" || !strings.Contains(why, "local main has 1 commit(s) not on it") {
		t.Fatalf("diverged: %q %q", ref, why)
	}
}

func TestPickBaseWithNoBaseUsesTheCheckedOutBranch(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q", "-b", "trunk")
	if ref, _, _ := PickBase(repo, "main"); ref != "" {
		t.Fatalf("a repo with no commit has nothing to branch from: %q", ref)
	}
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "one")
	if ref, why, _ := PickBase(repo, "main"); ref != "trunk" || !strings.Contains(why, "using the checked-out trunk") {
		t.Fatalf("%q %q", ref, why)
	}
}

type fakeHost struct {
	Host
	opened []string
	handle string
}

func (*fakeHost) Available() bool { return true }
func (*fakeHost) HasRuns() bool   { return false }
func (h *fakeHost) OpenTerminal(cmd, cwd, title, agent string) (string, string) {
	h.opened = append(h.opened, cmd+"|"+cwd+"|"+title)
	return h.handle, ""
}

type upDelivery struct {
	Delivery
	up bool
}

func (d upDelivery) WaitForSession(string, int, string) bool { return d.up }

func TestStartInOpensTheTerminalInTheKeptWorktreeAndChecksASessionCameUp(t *testing.T) {
	wt := t.TempDir()
	q := openDB(t)
	h := &fakeHost{handle: "t9"}
	s := &Spawns{Q: q, Host: h, Launch: fakeLaunch{}, Delivery: upDelivery{up: true}, Sleep: func(time.Duration) {}}
	ok, said, _ := s.StartIn("a", wt, "hl launch a --why 'wake'")
	if !ok || !strings.Contains(said, "a: started in "+wt+" (terminal t9)") {
		t.Fatalf("%v %q", ok, said)
	}
	if row, _ := db.GetAgentState(q, "a"); row.Terminal.String != "t9" || row.Worktree.String != wt {
		t.Fatalf("%+v", row)
	}
	s.Delivery = upDelivery{up: false}
	if ok, said, _ := s.StartIn("a", wt, "x"); ok || !strings.Contains(said, "no session is running in it after 20s") {
		t.Fatalf("%v %q", ok, said)
	}
	if ok, said, _ := s.StartIn("a", filepath.Join(wt, "gone"), "x"); ok || !strings.Contains(said, "its worktree") {
		t.Fatalf("%v %q", ok, said)
	}
}

// The folder is marked trusted before the terminal opens in it, on both ways a terminal is opened, so the session never shows the dialog.
func TestTheWorktreeIsTrustedBeforeItsTerminalOpens(t *testing.T) {
	s, h, repo := spawnRig(t)
	var trusted []string
	s.TrustFolder = func(agent, folder string) {
		trusted = append(trusted, fmt.Sprintf("%s@%s, %d open", agent, folder, len(h.opened)))
	}
	s.Spawn("a", repo)
	wt := deskOf(t, s, "a")
	s.StartIn("a", wt, "x")
	if want := []string{"a@" + wt + ", 0 open", "a@" + wt + ", 1 open"}; !slices.Equal(trusted, want) || len(h.opened) != 2 {
		t.Fatalf("%q", trusted)
	}
}

func TestPromptLandedWaitsForTheAgentsOwnHook(t *testing.T) {
	q := openDB(t)
	now := time.Unix(0, 0)
	s := &Spawns{Q: q, Now: func() time.Time { return now }, Sleep: func(d time.Duration) { now = now.Add(d) }}
	if ok, _ := s.PromptLanded("a", 0, 10*time.Second); ok {
		t.Fatal("landed with no event")
	}
	since, _ := db.MaxEventID(q)
	db.AddEvent(q, "a", "UserPromptSubmit")
	if ok, _ := s.PromptLanded("a", since, 10*time.Second); !ok {
		t.Fatal("the hook was not believed")
	}
}

type runHost struct {
	fakeHost
	missing    string
	current    string
	exists     bool
	created    string
	used, runs []string
}

func (h *runHost) Available() bool         { return h.missing == "" }
func (h *runHost) Missing() string         { return h.missing }
func (h *runHost) HasRuns() bool           { return true }
func (h *runHost) RunCurrent() string      { return h.current }
func (h *runHost) RunExists(string) bool   { return h.exists }
func (h *runHost) RunUse(run string)       { h.used = append(h.used, run) }
func (h *runHost) RunCreate(string) string { h.runs = append(h.runs, h.created); return h.created }
func (h *runHost) TaskCreate(_, title, _, run string) string {
	return "task-" + title
}
func (h *runHost) Dispatch(task, handle, run string) string { return task + "@" + handle }

func spawnRig(t *testing.T) (*Spawns, *runHost, string) {
	t.Helper()
	repo, _ := repoWithOrigin(t)
	h := &runHost{fakeHost: fakeHost{handle: "t7"}, created: "run-1"}
	q := teamtest.Seed(t, t.TempDir(), "a")
	s := &Spawns{Q: q, Host: h, Launch: fakeLaunch{}, Delivery: upDelivery{up: true}, Sleep: func(time.Duration) {}}
	s.Desks = &worktreepool.Pool{DB: q, Setup: func(agent, repo, desk string) string { return s.Prepare(agent, repo, desk) },
		Base: func(repo, branch string) (string, string, error) {
			if branch == "" {
				branch = gitrun.DefaultBranch(repo)
			}
			return PickBase(repo, branch)
		},
		Branch: func(agent string) string { return agent }}
	return s, h, repo
}

// deskOf is the desk agent's row records.
func deskOf(t *testing.T, s *Spawns, agent string) string {
	t.Helper()
	row, _ := db.GetAgentState(s.Q, agent)
	if row == nil || row.Worktree.String == "" {
		t.Fatalf("%s has no desk on record", agent)
	}
	return row.Worktree.String
}

func TestSpawnTakesADeskFromThePoolAndOpensItsTerminal(t *testing.T) {
	s, h, repo := spawnRig(t)
	ok, said, _ := s.Spawn("a", repo)
	wt := deskOf(t, s, "a")
	if !ok || filepath.Base(wt) != "desk-1" || !strings.Contains(said, "a: desk "+wt+" (new, branch a)") ||
		!strings.Contains(said, "from origin/main (fetched)") || !strings.Contains(said, "terminal t7") {
		t.Fatalf("%v %q", ok, said)
	}
	if row, _ := db.GetAgentState(s.Q, "a"); row.Terminal.String != "t7" {
		t.Fatalf("%+v", row)
	}
	if len(h.opened) != 1 || !strings.HasPrefix(h.opened[0], "hl launch a") {
		t.Fatalf("%v", h.opened)
	}
	// The desk it holds is kept: no second one is made.
	if ok, said, _ := s.Spawn("a", repo); !ok || !strings.Contains(said, "a: desk "+wt+" (held, branch a)") {
		t.Fatalf("%v %q", ok, said)
	}
}

func TestSpawnWithSaysWhyItCannotAndWhatItWouldDo(t *testing.T) {
	s, h, repo := spawnRig(t)
	h.missing = "ide_a is not running"
	if ok, said, err := s.SpawnWith("a", repo, SpawnOpts{}); ok || err != nil || said != "ide_a is not running" {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	h.missing = ""
	plain, _ := filepath.EvalSymlinks(t.TempDir()) // the pool keeps a folder by its real path
	if ok, said, err := s.Spawn("a", plain); !ok || err != nil || !strings.HasPrefix(said, "a: folder "+plain+" (plain folder)") ||
		len(h.opened) != 1 || !strings.Contains(h.opened[0], plain) {
		t.Fatalf("a plain folder is no desk: the agent starts in it, on no branch: %v %q %v %v", ok, said, err, h.opened)
	}
	h.opened = nil
	ok, said, err := s.SpawnWith("a", repo, SpawnOpts{DryRun: true})
	if err != nil || !ok || !strings.HasPrefix(said, "would take a desk of "+repo) || len(h.opened) != 0 {
		t.Fatalf("%v %q %v %v", ok, said, err, h.opened)
	}
	s.Desks = nil
	if ok, said, _ := s.Spawn("a", repo); ok || said != "no worktree pool to take a desk from" {
		t.Fatalf("%v %q", ok, said)
	}
}

// A repository whose default branch is master starts from it, even checked out on another branch: no base given means the repository's own.
func TestSpawnWithNoBaseBranchesFromTheRepositorysDefault(t *testing.T) {
	s, _, _ := spawnRig(t)
	origin := t.TempDir()
	gitIn(t, origin, "init", "-q", "-b", "master")
	gitIn(t, origin, "commit", "-q", "--allow-empty", "-m", "one")
	repo := t.TempDir()
	gitIn(t, repo, "clone", "-q", origin, ".")
	gitIn(t, repo, "checkout", "-q", "-b", "feature")
	ok, said, err := s.SpawnWith("a", repo, SpawnOpts{})
	if err != nil || !ok || !strings.Contains(said, "from origin/master (fetched)") {
		t.Fatalf("%v %q %v", ok, said, err)
	}
}

func TestEnsureRunRemembersTheRunAndPrefersTheOneTheTerminalIsBoundTo(t *testing.T) {
	s, h, _ := spawnRig(t)
	run, err := s.EnsureRun()
	if err != nil || run != "run-1" || len(h.runs) != 1 {
		t.Fatalf("%q %v", run, err)
	}
	h.exists = true
	if run, _ := s.EnsureRun(); run != "run-1" || len(h.runs) != 1 || len(h.used) != 1 {
		t.Fatalf("a remembered run is reused, not recreated: %q %v %v", run, h.runs, h.used)
	}
	h.current = "bound"
	if run, _ := s.EnsureRun(); run != "bound" {
		t.Fatalf("%q", run)
	}
	if v, _, _ := db.Meta(s.Q, RunKey); v != "bound" {
		t.Fatalf("%q", v)
	}
	if task, err := s.RegisterWorker("a", "t1"); err != nil || task != "task-a@t1" {
		t.Fatalf("%q %v", task, err)
	}
}

func TestGitThatCannotRunIsAnErrorNotAMissingBranch(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if ref, why, err := PickBase(gone, "main"); err == nil || ref != "" || strings.Contains(why, "make a first commit") {
		t.Fatalf("%q %q %v", ref, why, err)
	}
	s, _, _ := spawnRig(t)
	if ok, said, err := s.SpawnWith("a", gone, SpawnOpts{}); ok || err == nil {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if ok, said, err := s.Spawn("a", gone); ok || err == nil {
		t.Fatalf("Spawn returns the error too: %v %q %v", ok, said, err)
	}
}

type failingLaunch struct{ fakeLaunch }

func (failingLaunch) CommandFor(string, string, string, string) (string, error) {
	return "", errors.New("config unreadable")
}

func TestStartInReturnsWhatItCouldNotTry(t *testing.T) {
	s, _, _ := spawnRig(t)
	s.Launch = failingLaunch{}
	if ok, said, err := s.StartIn("a", t.TempDir(), ""); ok || err == nil {
		t.Fatalf("%v %q %v", ok, said, err)
	}
}

func TestGitThatTimesOutIsAnError(t *testing.T) {
	saved := gitTimeout
	gitTimeout = time.Nanosecond
	defer func() { gitTimeout = saved }()
	if _, _, err := git(t.TempDir(), "status"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("a killed git is an error: %v", err)
	}
}

// A worktree a spawn makes gets its repo's setup before the agent starts in it, and a failed setup is told without stopping the start.
func TestASpawnedWorktreeGetsItsReposSetup(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	t.Setenv("SHELL", "/bin/sh")
	s, _, repo := spawnRig(t)
	home := t.TempDir()
	s.Q = teamtest.Seed(t, home, "a", "b")
	s.Roster = team.NewRosterOn(s.Q, home)
	s.Roster.Store = &protected.Store{Root: t.TempDir()}
	os.WriteFile(filepath.Join(repo, ".env"), []byte("K=1"), 0o600)
	if err := worktreesetup.Put(s.Roster.Store, repo, worktreesetup.Setup{Script: "test -f .env && touch ready", Copy: []string{".env"}}); err != nil {
		t.Fatal(err)
	}
	ok, said, _ := s.Spawn("a", repo)
	if !ok || !strings.Contains(said, "setup done: copied .env") {
		t.Fatalf("%v %q", ok, said)
	}
	wt := deskOf(t, s, "a")
	if _, err := os.Stat(filepath.Join(wt, "ready")); err != nil {
		t.Fatal("the script did not run after the copy")
	}
	if last, _ := worktreesetup.Last(s.Q, repo); last == nil || !last.OK || last.Worktree != wt {
		t.Fatalf("last %+v", last)
	}

	worktreesetup.Put(s.Roster.Store, repo, worktreesetup.Setup{Script: "exit 4"})
	ok, said, _ = s.Spawn("b", repo)
	if !ok || !strings.Contains(said, "setup FAILED: script exited 4") {
		t.Fatalf("a failed setup still starts the agent, and says so: %v %q", ok, said)
	}
	if last, _ := worktreesetup.Last(s.Q, repo); last == nil || last.OK {
		t.Fatalf("last %+v", last)
	}
}
