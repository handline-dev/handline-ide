package daemon

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// joinedMaster is the master home joined.
func joinedMaster(t *testing.T, home string) joined.Master {
	t.Helper()
	m, ok := joined.Of(home)
	if !ok {
		t.Fatal("the runner's home has joined no master")
	}
	return m
}

// runnerTerm is the terminal host of a runner's home.
func runnerTerm(home string) runner.Terminals { return agenthost.Tmux(home).(runner.Terminals) }

// fakeClock is a time a test moves by hand.
type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.at }
func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	c.at = c.at.Add(d)
	c.mu.Unlock()
}

// gitIn runs git in dir, failing the test when it fails.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// pushedWorktree is a clone in dir of a new bare origin, with one commit pushed on branch work.
func pushedWorktree(t *testing.T, dir string) (origin, worktree, pushed string) {
	t.Helper()
	origin, worktree = filepath.Join(dir, "origin.git"), filepath.Join(dir, "wt")
	gitIn(t, dir, "init", "-q", "--bare", origin)
	gitIn(t, dir, "clone", "-q", origin, worktree)
	gitIn(t, worktree, "checkout", "-q", "-b", "work")
	os.WriteFile(filepath.Join(worktree, "a.txt"), []byte("a\n"), 0o644)
	gitIn(t, worktree, "add", "a.txt")
	gitIn(t, worktree, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "a")
	gitIn(t, worktree, "push", "-q", "-u", "origin", "work")
	return origin, worktree, gitIn(t, worktree, "rev-parse", "HEAD")
}

// A runner that stops is offline 60 s after its last heartbeat and takes no session; its session is taken back once it has been
// offline for the reclaim time: bob's task is told the commit last pushed on his branch, and his terminal there reads gone. The runner
// started again still holds that session: the master's answer to its heartbeat stops it, and what bob left in its worktree is pushed to
// a rescue branch, his own branch left as it was.
func TestAStoppedRunnerIsOfflineThenItsSessionIsTakenBackAndItsLeftoversRescued(t *testing.T) {
	master, node, home, _, r := masterWithRunner(t, nil)
	clock := &fakeClock{at: time.Now()}
	master.nodes.SetClock(clock.now)
	bob := teamtest.Key(t, master.cfg.Home, "bob")
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'one', '" + bob + "', 'p1', 1, 'p1')",
		"UPDATE tasks SET sent_at = 1 WHERE id = 1",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('" + bob + "', 'RUNNING', 1, 1)",
	} {
		if _, err := master.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	origin, wt, pushed := pushedWorktree(t, home)
	rn := linkedRunner(t, master, node)
	handle, err := rn.Open("bob", wt, "cd "+wt)
	if err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if b, _, _ := master.nodes.Heard(node); len(b.Sessions) == 1 && b.Sessions[0].Pushed == pushed {
			if b.Sessions[0].Branch != "work" || b.Sessions[0].Handle != handle {
				t.Fatalf("the heartbeat says %+v", b.Sessions[0])
			}
			break
		}
		if time.Now().After(end) {
			t.Fatal("no heartbeat named bob's session and its pushed commit")
		}
	}
	r.Close() // its sessions keep running, as when a lid closes

	clock.add(nodes.OfflineAfter)
	if !master.nodes.Online(node) {
		t.Fatal("offline before 60 s without a heartbeat")
	}
	clock.add(time.Second)
	if master.nodes.Online(node) || !strings.Contains(master.nodes.Takes(node), "offline") {
		t.Fatalf("online 61 s after the last heartbeat: %q", master.nodes.Takes(node))
	}
	if _, err := rn.Open("ann", wt, "true"); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("an offline runner opened a session: %v", err)
	}
	tm := master.nodes.Terminals()
	if hs, err := tm.Terminals(node); err != nil || len(hs) != 1 || hs[0] != handle {
		t.Fatalf("an offline runner's session does not read running until it is taken back: %v %v", hs, err)
	}
	if got, err := master.nodes.Sweep(nodes.ReclaimS); err != nil || len(got) != 0 {
		t.Fatalf("taken back before the reclaim time: %v %v", got, err)
	}
	clock.add(nodes.ReclaimS * time.Second)
	got, err := master.nodes.Sweep(nodes.ReclaimS)
	if err != nil || len(got) != 1 || got[0].Agent != bob || got[0].Task != 1 {
		t.Fatalf("the sweep took back %+v (%v)", got, err)
	}
	var memo string
	master.db.QueryRow("SELECT body FROM task_memos WHERE task_id = 1").Scan(&memo)
	if !strings.Contains(memo, pushed[:9]) || !strings.Contains(memo, "on work") || !strings.Contains(memo, "box") {
		t.Fatalf("the task's memo: %q", memo)
	}
	t.Logf("memo: %s", memo)
	if hs, _ := tm.Terminals(node); len(hs) != 0 {
		t.Fatalf("a taken back session still reads running: %v", hs)
	}

	os.WriteFile(filepath.Join(wt, "left.txt"), []byte("not committed\n"), 0o644)
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "") // a lent machine may have no git identity: the rescue still commits
	}
	r2, err := runner.Start(runner.Config{Home: home, Sha: master.nodes.Version, Master: joinedMaster(t, home), Term: runnerTerm(home),
		HL: "true", Log: t.Logf}) // the same home, started again
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r2.Close)
	var branch string
	for end := time.Now().Add(15 * time.Second); branch == ""; time.Sleep(100 * time.Millisecond) {
		branch = gitIn(t, home, "--git-dir", origin, "for-each-ref", "--format=%(refname:short)", "refs/heads/rescue/")
		if time.Now().After(end) {
			t.Fatal("no rescue branch was pushed")
		}
	}
	if !strings.HasPrefix(branch, "rescue/bob-") {
		t.Fatalf("the rescue branch is %q", branch)
	}
	if files := gitIn(t, home, "--git-dir", origin, "ls-tree", "--name-only", branch); !strings.Contains(files, "left.txt") {
		t.Fatalf("the rescue branch holds %q", files)
	}
	if head := gitIn(t, home, "--git-dir", origin, "rev-parse", "work"); head != pushed {
		t.Fatalf("bob's branch moved to %s", head)
	}
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		b, _ := os.ReadFile(runner.StateFile(home))
		if !strings.Contains(string(b), "bob") {
			break
		}
		if time.Now().After(end) {
			t.Fatalf("the runner still holds bob's session: %s", b)
		}
	}
}

// A runner on another Handline version than the master's takes no new session until it is updated.
func TestARunnerOnAnotherVersionTakesNoWork(t *testing.T) {
	master, node, home, _, r := masterWithRunner(t, nil)
	r.Close()
	r2, err := runner.Start(runner.Config{Home: home, Sha: "0ld", Master: joinedMaster(t, home), Term: runnerTerm(home), HL: "true",
		Log: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r2.Close)
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if b, _, _ := master.nodes.Heard(node); b.Version == "0ld" {
			break
		}
		if time.Now().After(end) {
			t.Fatal("the old runner's heartbeat never came")
		}
	}
	why := master.nodes.Takes(node)
	if !strings.Contains(why, "update to take work") {
		t.Fatalf("a runner on another version takes work: %q", why)
	}
	rn, _ := master.nodes.Runner(node)
	if _, err := rn.Open("bob", home, "true"); err == nil {
		t.Fatal("a runner on another version opened a session")
	}
}

// With the battery switch on, a runner that goes on battery is set to Off and back to Run once plugged in; one its person set to
// Off stays Off when plugged in.
func TestTheBatterySwitchTurnsARunnerOffOnBatteryAndBackOnPower(t *testing.T) {
	var battery atomic.Bool
	master, node, _, _, r := masterWithRunnerAs(t, nil, func(c *runner.Config) { c.Battery = battery.Load })
	lending := func() string {
		var l string
		master.db.QueryRow("SELECT lending FROM nodes WHERE id = ?", node).Scan(&l)
		return l
	}
	beat := func() {
		t.Helper()
		if err := r.SendBeat(); err != nil {
			t.Fatal(err)
		}
	}
	battery.Store(true)
	beat()
	if lending() != "run" {
		t.Fatalf("the switch is off by default, yet unplugged it is %s", lending())
	}
	battery.Store(false)
	beat()
	os.WriteFile(filepath.Join(master.cfg.Home, "config.yaml"), []byte(nodes.BatteryKey+": true\n"), 0o644)
	battery.Store(true)
	beat()
	if lending() != "off" {
		t.Fatalf("unplugged with the switch on, it is %s", lending())
	}
	battery.Store(false)
	beat()
	if lending() != "run" {
		t.Fatalf("plugged in again, it is %s", lending())
	}
	battery.Store(true)
	beat()
	master.db.Exec("UPDATE nodes SET lending = 'pause' WHERE id = ?", node) // its person's own choice since
	battery.Store(false)
	beat()
	if lending() != "pause" {
		t.Fatalf("plugged in, its person's Pause became %s", lending())
	}
}

// A runner whose only account is logged in counts its usage on the master with no account list given: a runner keeps no accounts table,
// so its logged-in account folders are its accounts.
func TestARunnersLoggedInAccountFolderIsItsAccount(t *testing.T) {
	master, _, home, _, r := masterWithRunnerAs(t, nil, func(c *runner.Config) { c.Accounts = nil })
	dir := filepath.Join(paths.AccountsDir(home), paths.Unregistered, "alpha", ".claude")
	os.MkdirAll(filepath.Join(dir, "projects", "p"), 0o755)
	os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"a@example.com"}}`), 0o644)
	line := `{"type":"assistant","requestId":"r1","sessionId":"s1","cwd":"/w","timestamp":"` + time.Now().UTC().Format(time.RFC3339) +
		`","message":{"model":"claude-opus-5-5","usage":{"input_tokens":10,"output_tokens":20}}}` + "\n"
	os.WriteFile(filepath.Join(dir, "projects", "p", "s1.jsonl"), []byte(line), 0o644)
	if err := r.SendBeat(); err != nil { // the folder is named to the master before its calls are counted
		t.Fatal(err)
	}
	if err := r.SendTokens(&usage.Env{Home: home}); err != nil {
		t.Fatal(err)
	}
	var n int
	master.db.QueryRow("SELECT COUNT(*) FROM token_calls WHERE account = 'alpha'").Scan(&n)
	if n != 1 {
		t.Fatalf("the master counts %d of alpha's calls", n)
	}
	if err := r.SendBeat(); err != nil {
		t.Fatal(err)
	}
}

// A runner's account folders are the board's accounts by email: one logged in as the master's account's email is that account, its
// config dir the runner's own folder; one logged in as an email the board has not seen becomes an account of its own, named after its
// folder, logged in nowhere here.
func TestARunnersAccountFoldersAreTheBoardsAccountsByEmail(t *testing.T) {
	master, node, home, _, r := masterWithRunnerAs(t, nil, func(c *runner.Config) { c.Accounts = nil })
	mdir, err := (team.Accounts{Home: master.cfg.Home}).Add("main", "")
	if err != nil {
		t.Fatal(err)
	}
	loginAs(t, mdir, "a@example.com")
	loginAs(t, filepath.Join(paths.AccountsDir(home), paths.Unregistered, "alpha", ".claude"), "A@example.com")
	loginAs(t, filepath.Join(paths.AccountsDir(home), paths.Unregistered, "beta", ".claude"), "b@example.com")
	if err := r.SendBeat(); err != nil {
		t.Fatal(err)
	}
	got := master.nodes.Accounts(node)
	want := map[string]string{"main": filepath.Join(master.cfg.Home, "accounts", paths.Unregistered, "alpha", ".claude"),
		"beta": filepath.Join(master.cfg.Home, "accounts", paths.Unregistered, "beta", ".claude")}
	if !maps.Equal(got, want) {
		t.Fatalf("the runner's accounts are %v", got)
	}
	if a := (team.Accounts{Home: master.cfg.Home}); a.RecordedEmail("beta") != "b@example.com" || a.LoggedIn("beta") {
		t.Fatal("beta is the board's, with its email, logged in on the runner only")
	}
}

// fakeMeter reads each account as its readings say; one it has none for is refused.
type fakeMeter struct {
	mu       sync.Mutex
	readings map[string]*usage.Reading
	turns    int
}

func (m *fakeMeter) Read(_ context.Context, acct string) (*usage.Reading, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.readings[acct]
	return r, r == nil
}

func (m *fakeMeter) RunModel(context.Context, string, string, string) (bool, string, string) {
	m.mu.Lock()
	m.turns++
	m.mu.Unlock()
	return true, "", ""
}

// A runner reads its own accounts' usage and its heartbeat carries it: an account logged in only there is held to its 5-hour wall, one
// logged in on both machines takes the newer reading, and an older one leaves the master's as it is. A login refused even after a real
// turn is no account to start on there.
func TestARunnersReadingsHoldItsAccountsToTheirWalls(t *testing.T) {
	meter := &fakeMeter{}
	master, node, home, _, r := masterWithRunnerAs(t, nil, func(c *runner.Config) { c.Accounts, c.Meter = nil, meter })
	mhome := master.cfg.Home
	mdir, err := (team.Accounts{Home: mhome}).Add("main", "")
	if err != nil {
		t.Fatal(err)
	}
	loginAs(t, mdir, "a@example.com")
	for name, email := range map[string]string{"alpha": "a@example.com", "beta": "b@example.com", "gone": "g@example.com"} {
		loginAs(t, filepath.Join(paths.AccountsDir(home), paths.Unregistered, name, ".claude"), email)
	}
	now := time.Now().Unix()
	at := func(acct string, when int64, short int) *usage.Reading {
		week := 10
		return &usage.Reading{Account: acct, At: when, Windows: []usage.ReadingWindow{
			{Minutes: 300, Pct: &short, Reset: now + 3600}, {Minutes: 10080, Pct: &week, Reset: now + 86400}}}
	}
	usage.Record(mhome, *at("main", now-60, 10))
	meter.readings = map[string]*usage.Reading{"alpha": at("alpha", now-10, 50), "beta": at("beta", now-10, 99)}
	spent, limit := 3.5, 50.0
	meter.readings["beta"].Extra = &usage.Extra{On: true, Cap: &limit, Spent: &spent, Currency: "USD"}
	for range 3 {
		r.MeterOne(t.Context())
	}
	if err := r.SendBeat(); err != nil {
		t.Fatal(err)
	}
	env := &usage.Env{Home: mhome}
	if got := *env.Read("main").ShortTrusted(); got != 50 {
		t.Fatalf("main, logged in on both, reads %d%%, not the runner's newer 50%%", got)
	}
	if got := env.Read("beta").ShortTrusted(); got == nil || *got != 99 {
		t.Fatalf("beta, logged in on the runner only, reads %v", got)
	}
	if x := extraInOutlook(t, master, "beta"); x == nil || x["on"] != true || x["spent"] != 3.5 || x["cap"] != 50.0 || x["currency"] != "USD" {
		t.Fatalf("beta's extra usage, read on the runner, is not in the budget: %v", x)
	}
	accts := master.nodes.Accounts(node)
	if _, ok := accts["gone"]; ok || len(accts) != 2 || meter.turns != 1 {
		t.Fatalf("a login refused after one real turn is no account there: %v (%d turns)", accts, meter.turns)
	}
	picked, reasons, err := env.Pick(master.db, usage.PickOpts{Candidates: []string{"beta", "main"}, Bootstrap: true})
	if err != nil || picked != "main" {
		t.Fatalf("picked %q (%v): %v", picked, err, reasons)
	}
	usage.Record(mhome, *at("main", now, 20)) // the master reads it after the runner did
	if err := r.SendBeat(); err != nil {
		t.Fatal(err)
	}
	if got := *env.Read("main").ShortTrusted(); got != 20 {
		t.Fatalf("an older reading from the runner replaced the master's: %d%%", got)
	}
}

// extraInOutlook is the account's extra usage in stats.outlook's budget; nil when unknown.
func extraInOutlook(t *testing.T, d *Daemon, acct string) map[string]any {
	t.Helper()
	got, err := d.local.Dispatch(rpc.Caller{Kind: "person", Key: "p1", Role: "owner"}, "t", "", "stats.outlook", map[string]any{"range": "24h"})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got.(map[string]any)["budget"].(map[string]any)["accounts"].([]any) {
		if m := a.(map[string]any); m["account"] == acct {
			x, _ := m["extra"].(map[string]any)
			return x
		}
	}
	t.Fatalf("no budget row for %s", acct)
	return nil
}

// loginAs logs the claude config dir in as email, as .claude.json says it.
func loginAs(t *testing.T, dir, email string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"`+email+`"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
}
