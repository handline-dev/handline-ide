package proposalwrites

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// refusal is the message of the error m answers; a call that was not refused fails the test.
func refusal(t *testing.T, m map[string]any) string {
	t.Helper()
	e, _ := m["error"].(map[string]any)
	if e == nil {
		t.Fatalf("not refused: %v", m)
	}
	return e["message"].(string)
}

// A lead is retired like any agent: the oldest other agent of its workspace leads it from then on.
func TestAgentRetireOfALeadMakesTheOldestOtherAgentTheLead(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	for _, n := range []string{"bob/kid", "bob/pup"} {
		if _, err := team.NewRoster(r.home).Register(n, team.RegisterOpts{Role: "r"}); err != nil {
			t.Fatal(err)
		}
	}
	bob, kid, pup := r.who("bob"), r.who("bob/kid"), r.who("bob/pup")
	m := r.call("agent.retire", `{"agent":"`+bob+`","request_id":"r2"}`)
	if res, _ := m["result"].(map[string]any); res == nil || !strings.HasPrefix(res["said"].(string), "bob/lead is retired") {
		t.Fatalf("%v", m)
	}
	if slices.Contains(paths.ListAgents(r.home), bob) || r.who("bob") != kid {
		t.Fatalf("bob's workspace is led by %s, want the oldest other agent %s", r.who("bob"), kid)
	}
	r.db.Exec("INSERT INTO agent_state(agent, state, updated) VALUES(?, 'idle', 1)", pup)
	m = r.call("agent.retire", `{"agent":"`+pup+`","request_id":"r3"}`)
	res, _ := m["result"].(map[string]any)
	archive := filepath.Join(r.home, "agents", "archive", time.Now().Format("2006-01-02")+"-bob-pup")
	if res == nil || !strings.HasPrefix(res["said"].(string), "bob/pup is retired; its folder is at "+archive) {
		t.Fatalf("%v", m)
	}
	if _, err := os.Stat(filepath.Join(archive, "memory")); err != nil {
		t.Fatal("the folder is archived:", err)
	}
	kid = pup // the rest of the test reads the agent retired last
	var rows int
	r.db.QueryRow("SELECT COUNT(*) FROM agent_state WHERE agent = ?", kid).Scan(&rows)
	if rows != 0 || strings.Contains(r.rosterText(), "bob/pup") {
		t.Fatalf("rows %d\n%s", rows, r.rosterText())
	}
	again, _ := r.call("agent.retire", `{"agent":"`+kid+`","request_id":"r3"}`)["result"].(map[string]any)
	if again == nil || again["replayed"] != true {
		t.Fatalf("a retry is the stored reply: %v", again)
	}
	if msg := refusal(t, r.call("agent.retire", `{"agent":"`+kid+`","request_id":"r4"}`)); !strings.Contains(msg, "no such agent") {
		t.Fatal(msg)
	}
}

// retireHeld calls agent.retire for bob, whose session runs, and returns what the call answers. The call must not answer while the
// retire is held: during runs once it is, with sweep carrying the held retire with bob's session ended (ended) or running.
func (r *rig) retireHeld(params string, during func(sweep func(ended bool))) map[string]any {
	r.t.Helper()
	r.setLive("bob", true) // read by the call before it holds the retire, never after
	got := make(chan map[string]any, 1)
	go func() { got <- r.call("agent.retire", params) }()
	for deadline := time.Now().Add(10 * time.Second); ; {
		if held := r.e.listing(); len(held) == 1 && held[0].str("state") == "applying" {
			break
		}
		select {
		case m := <-got:
			r.t.Fatalf("answered before the retire was held: %v", m)
		case <-time.After(10 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			r.t.Fatal("the retire was never held")
		}
	}
	sweep := func(ended bool) {
		r.e.SweepHeld(HeldPorts{Alive: func(string) bool { return !ended }, Idle: func(string) bool { return false },
			Close: func(string) (bool, string) { r.t.Error("a turn is never cut"); return false, "" },
			Start: func(a string) (bool, string) { r.t.Errorf("%s started again", a); return true, "" }})
	}
	during(sweep)
	select {
	case m := <-got:
		return m
	case <-time.After(10 * time.Second):
		r.t.Fatal("the call never answered")
		return nil
	}
}

// The founder's retire of an agent whose turn runs answers only once the turn ended and the retire ran, with what was done.
func TestTheFoundersRetireOfARunningAgentAnswersOnceItIsRetired(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob := r.who("bob")
	m := r.retireHeld(`{"agent":"bob","request_id":"h1"}`, func(sweep func(bool)) {
		sweep(false)
		if !slices.Contains(paths.ListAgents(r.home), bob) {
			t.Error("bob stays while its turn runs")
		}
		sweep(true)
	})
	if res, _ := m["result"].(map[string]any); res == nil || !strings.HasPrefix(res["said"].(string), "bob/lead is retired; its folder is at ") {
		t.Fatalf("the outcome, not an acceptance: %v", m)
	}
	if strings.Contains(r.rosterText(), "bob") || r.state(r.e.listing()[0].id()) != "applied" {
		t.Fatalf("retired: %s\n%s", r.state(r.e.listing()[0].id()), r.rosterText())
	}
}

// A turn that has not ended by the limit: the call is refused, the held retire is given up, and nothing is deleted then or later.
func TestARetireWhoseTurnOutlastsTheLimitIsRefusedAndDropped(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob := r.who("bob")
	r.e.Wait = 50 * time.Millisecond
	m := r.retireHeld(`{"agent":"bob","request_id":"h4"}`, func(sweep func(bool)) { sweep(false) })
	if msg := refusal(t, m); !strings.Contains(msg, bob+" is still in its turn, so nothing was deleted") {
		t.Fatalf("refused at the limit: %v", m)
	}
	pid := r.e.listing()[0].id()
	if r.state(pid) != "withdrawn" {
		t.Fatalf("given up, not left held: %s", r.state(pid))
	}
	r.e.SweepHeld(HeldPorts{Alive: func(string) bool { return false }, Idle: func(string) bool { return true },
		Close: func(string) (bool, string) { return true, "" }, Start: func(string) (bool, string) { return true, "" }})
	if !strings.Contains(r.rosterText(), "  bob: {}") || r.state(pid) != "withdrawn" {
		t.Fatalf("a later sweep retired bob: %s\n%s", r.state(pid), r.rosterText())
	}
}

// bobsWorktrees is a repo with bob's three worktrees: done (merged, clean) the one it records, and two desks pinned to it, ahead (a commit
// not on main) and shared (alice works in it too).
func bobsWorktrees(t *testing.T, r *rig) (main string, wt map[string]string, git func(dir string, args ...string) bool) {
	t.Helper()
	return bobsWorktreesOn(t, r, "main")
}

// bobsWorktreesOn is bobsWorktrees in a repository whose default branch is branch.
func bobsWorktreesOn(t *testing.T, r *rig, branch string) (main string, wt map[string]string, git func(dir string, args ...string) bool) {
	t.Helper()
	bob := r.who("bob")
	root, _ := filepath.EvalSymlinks(t.TempDir())
	main = filepath.Join(root, "main")
	git = func(dir string, args ...string) bool {
		return gitrun.Run(dir, append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).RC == 0
	}
	os.MkdirAll(main, 0o755)
	git(main, "init", "-q", "-b", branch)
	git(main, "commit", "-q", "--allow-empty", "-m", "start")
	wt = map[string]string{}
	for _, name := range []string{"done", "ahead", "shared"} {
		wt[name] = filepath.Join(root, name)
		git(main, "worktree", "add", "-q", "-b", name, wt[name])
	}
	git(wt["ahead"], "commit", "-q", "--allow-empty", "-m", "unmerged work")
	r.db.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES(?, 'idle', 1, ?)", bob, "repo::"+wt["done"])
	pinDesks(r, bob, main, wt["ahead"], wt["shared"])
	r.db.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES('alice', 'idle', 1, ?)", wt["shared"])
	return main, wt, git
}

// pinDesks makes each path a desk of repo pinned to agent: the worktrees a retire judges besides the one the agent records.
func pinDesks(r *rig, agent, repo string, desks ...string) {
	for _, d := range desks {
		r.db.Exec("INSERT INTO worktree_folders(path, repo, pinned_to) VALUES(?,?,?)", d, repo, agent)
	}
}

// The founder's retire asks about a worktree that holds work at once: with no answer it is refused (gate dirty) naming each such
// worktree and what it holds, and nothing is retired.
func TestTheFoundersRetireIsRefusedWhileAWorktreeHoldsWorkWithNoAnswer(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob := r.who("bob")
	_, wt, _ := bobsWorktrees(t, r)
	m := r.call("agent.retire", `{"agent":"bob","request_id":"w0"}`)
	e, _ := m["error"].(map[string]any)
	d, _ := e["data"].(map[string]any)
	list, _ := d["worktrees"].([]any)
	if d["gate"] != "dirty" || len(list) != 1 {
		t.Fatalf("%v", m)
	}
	if h := list[0].(map[string]any); h["path"] != wt["ahead"] || h["commits_ahead"] != float64(1) || h["files_changed"] != float64(0) {
		t.Fatalf("%v", h)
	}
	if msg, _ := e["message"].(string); !strings.Contains(msg, wt["ahead"]+" (0 files changed, 1 commits not on main)") {
		t.Fatalf("%q", msg)
	}
	if !slices.Contains(paths.ListAgents(r.home), bob) {
		t.Fatal("bob was retired")
	}
	if m := r.call("agent.retire", `{"agent":"bob","worktree":"maybe","request_id":"w0b"}`); m["error"] == nil {
		t.Fatalf("an answer that is none of keep, save, force: %v", m)
	}
}

// With an answer the retire goes on: its worktrees go first, a merged, clean one with its branch, one that holds work saved to a rescue
// branch or forced away as answered, and one another agent shares is left alone. keep leaves the worktree, so the agent stays too.
// Nothing is put to the founder as a task.
func TestTheFoundersAnswerIsCarriedOutForAWorktreeThatHoldsWork(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ choice, said string }{
		{"save", " is removed; what it held is on rescue/{bob}-"}, {"force", " is removed with what it held"},
	} {
		t.Run(c.choice, func(t *testing.T) {
			r := newRig(t)
			main, wt, git := bobsWorktrees(t, r)
			r.e.Ask = func(title, body string, tags ...string) (int64, error) {
				t.Fatal("asked the founder: " + title)
				return 0, nil
			}
			bob := r.who("bob") // before the retire, which ends the key's name
			m := r.call("agent.retire", `{"agent":"bob","worktree":"`+c.choice+`","request_id":"w1"}`)
			res, _ := m["result"].(map[string]any)
			if res == nil {
				t.Fatalf("%v", m)
			}
			said, _ := res["said"].(string)
			rescue := strings.ReplaceAll(c.said, "{bob}", bob) // the rescue branch is named by the agent's key
			for _, want := range []string{wt["done"] + " is removed with its branch done", wt["ahead"] + rescue} {
				if !strings.Contains(said, want) {
					t.Errorf("%q lacks %q", said, want)
				}
			}
			for name, gone := range map[string]bool{"done": true, "ahead": true, "shared": false} {
				if _, err := os.Stat(wt[name]); (err != nil) != gone {
					t.Errorf("%s gone %v", name, err != nil)
				}
			}
			if git(main, "rev-parse", "--verify", "-q", "refs/heads/done") || !git(main, "rev-parse", "--verify", "-q", "refs/heads/ahead") {
				t.Fatal("the merged branch goes, the unmerged one stays")
			}
		})
	}
	t.Run("keep", func(t *testing.T) {
		r := newRig(t)
		bob := r.who("bob")
		_, wt, _ := bobsWorktrees(t, r)
		m := r.call("agent.retire", `{"agent":"bob","worktree":"keep","request_id":"w1"}`)
		if e, _ := m["error"].(map[string]any); e == nil || !strings.Contains(e["message"].(string), wt["ahead"]+" holds work (ahead is not merged into main) and is kept, as asked, so "+addr.Ref(bob)+" is not deleted") {
			t.Fatalf("%v", m)
		}
		assertNotRetired(t, r, wt)
	})
}

// In a repository whose default branch is master the retire judges the worktrees against master: only the one ahead of it holds work,
// said as commits not on master, and with an answer the merged one goes with its branch while master stays.
func TestARetireInAMasterRepositoryJudgesItsWorktreesAgainstMaster(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	main, wt, git := bobsWorktreesOn(t, r, "master")
	m := r.call("agent.retire", `{"agent":"bob","request_id":"m0"}`)
	e, _ := m["error"].(map[string]any)
	d, _ := e["data"].(map[string]any)
	if list, _ := d["worktrees"].([]any); len(list) != 1 || list[0].(map[string]any)["path"] != wt["ahead"] {
		t.Fatalf("the worktrees that hold work: %v", m)
	}
	if msg, _ := e["message"].(string); !strings.Contains(msg, wt["ahead"]+" (0 files changed, 1 commits not on master)") {
		t.Fatalf("%q", msg)
	}
	m = r.call("agent.retire", `{"agent":"bob","worktree":"keep","request_id":"m1"}`)
	if e, _ := m["error"].(map[string]any); e == nil || !strings.Contains(e["message"].(string), "holds work (ahead is not merged into master)") {
		t.Fatalf("%v", m)
	}
	m = r.call("agent.retire", `{"agent":"bob","worktree":"force","request_id":"m2"}`)
	res, _ := m["result"].(map[string]any)
	if said, _ := res["said"].(string); !strings.Contains(said, wt["done"]+" is removed with its branch done") {
		t.Fatalf("%v", m)
	}
	if !git(main, "rev-parse", "--verify", "-q", "refs/heads/master") || git(main, "rev-parse", "--verify", "-q", "refs/heads/done") {
		t.Fatal("master stays and the merged branch goes")
	}
}

// A branch that landed by a push is on origin's copy of the default branch while the clone's own branch stays behind, since nothing
// moves it: a retire takes that worktree with no question, where one whose commit no push has landed is still asked about.
func TestARetireTakesAWorktreeWhoseBranchLandedOnOriginsCopy(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	main, wt, git := bobsWorktrees(t, r)
	if m := r.call("agent.retire", `{"agent":"bob","request_id":"p0"}`); m["error"] == nil {
		t.Fatalf("a commit that has not landed anywhere: %v", m)
	}
	git(main, "update-ref", "refs/remotes/origin/main", "refs/heads/ahead") // as a push of it to origin's main leaves the clone
	if git(main, "merge-base", "--is-ancestor", "ahead", "main") {
		t.Fatal("the local main has the work: the test proves nothing")
	}
	m := r.call("agent.retire", `{"agent":"bob","request_id":"p1"}`)
	res, _ := m["result"].(map[string]any)
	if said, _ := res["said"].(string); !strings.Contains(said, wt["ahead"]+" is removed") || !strings.Contains(said, wt["done"]+" is removed with its branch done") {
		t.Fatalf("%v", m)
	}
	for name, gone := range map[string]bool{"done": true, "ahead": true, "shared": false} {
		if _, err := os.Stat(wt[name]); (err != nil) != gone {
			t.Errorf("%s gone %v", name, err != nil)
		}
	}
	if !git(main, "rev-parse", "--verify", "-q", "refs/heads/ahead") {
		t.Fatal("the branch whose commits the local main lacks went with its worktree")
	}
}

// assertNotRetired is bob still on the team with each of its worktrees where it was.
func assertNotRetired(t *testing.T, r *rig, wt map[string]string) {
	t.Helper()
	bob := r.who("bob")
	if !slices.Contains(paths.ListAgents(r.home), bob) {
		t.Fatal("bob was retired")
	}
	for name, path := range wt {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s was removed: %v", name, err)
		}
	}
}

// A worktree that cannot be removed keeps the agent: refused before anything changes when it is locked, force or not, and put back as it
// was when git fails to remove it.
func TestAnAgentWhoseWorktreeCannotBeRemovedIsNotDeleted(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob := r.who("bob")
	main, wt, git := bobsWorktrees(t, r)
	r.e.Ask = func(title, body string, tags ...string) (int64, error) {
		t.Fatal("asked the founder: " + title)
		return 0, nil
	}
	git(main, "worktree", "lock", wt["done"])
	m := r.call("agent.retire", `{"agent":"bob","worktree":"save","request_id":"l1"}`)
	e, _ := m["error"].(map[string]any)
	if d, _ := e["data"].(map[string]any); d["gate"] != "worktree" || !strings.Contains(e["message"].(string), wt["done"]+" cannot be removed (locked)") {
		t.Fatalf("a locked worktree: %v", m)
	}
	assertNotRetired(t, r, wt)
	m = r.call("agent.retire", `{"agent":"bob","worktree":"force","request_id":"l1f"}`)
	e, _ = m["error"].(map[string]any)
	if d, _ := e["data"].(map[string]any); d["gate"] != "worktree" || !strings.Contains(e["message"].(string), wt["done"]+" cannot be removed (locked)") {
		t.Fatalf("force on a locked worktree: %v", m)
	}
	assertNotRetired(t, r, wt)
	git(main, "worktree", "unlock", wt["done"])

	// done's folder cannot be deleted: git refuses the removal once the retire is under way.
	root := filepath.Dir(wt["done"])
	os.Chmod(root, 0o555)
	t.Cleanup(func() { os.Chmod(root, 0o755) })
	m = r.call("agent.retire", `{"agent":"bob","worktree":"save","request_id":"l2"}`)
	e, _ = m["error"].(map[string]any)
	if d, _ := e["data"].(map[string]any); d["gate"] != "state" || !strings.Contains(e["message"].(string), wt["done"]+" could not be removed (") ||
		!strings.Contains(e["message"].(string), "so "+addr.Ref(bob)+" is not deleted") {
		t.Fatalf("git failing: %v", m)
	}
	os.Chmod(root, 0o755)
	assertNotRetired(t, r, wt)
}

// An approved retire carries no answer for a worktree that holds work: it is refused, and the agent and every worktree stay. Nobody is
// asked: the proposal's apply fails and says which worktree holds what.
func TestAnApprovedRetireIsRefusedWhileAWorktreeHoldsWork(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob := r.who("bob")
	_, wt, _ := bobsWorktrees(t, r)
	r.e.Ask = func(title, body string, tags ...string) (int64, error) {
		t.Fatal("asked the founder: " + title)
		return 0, nil
	}
	prop := &Prop{pairs: []yamlcompat.Pair{{Key: "kind", Val: "retire"}, {Key: "agent", Val: bob}, {Key: "id", Val: "hr1"}}}
	if ok, said, err := r.e.run(prop); err != nil || ok || !strings.Contains(said, wt["ahead"]+" holds work (") || !strings.Contains(said, "so "+addr.Ref(bob)+" is not deleted") {
		t.Fatalf("an approved retire with held work: %v %q %v", ok, said, err)
	}
	assertNotRetired(t, r, wt)
}

// A running agent's retire, held with no answer, finds a worktree that holds work when its turn ends: the waiting call is refused with
// which worktree holds what, the apply is recorded as failed, and the agent and its worktrees stay.
func TestAHeldRetireThatFindsHeldWorkAtItsTurnsEndIsRefused(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob := r.who("bob")
	var wt map[string]string
	m := r.retireHeld(`{"agent":"bob","request_id":"h3"}`, func(sweep func(bool)) {
		_, wt, _ = bobsWorktrees(t, r) // the work appears while the turn runs
		sweep(true)
	})
	if msg := refusal(t, m); !strings.Contains(msg, wt["ahead"]+" holds work (") || !strings.Contains(msg, "so "+addr.Ref(bob)+" is not deleted") {
		t.Fatalf("refused through the waiting call: %v", m)
	}
	if held := r.e.listing(); len(held) != 1 || held[0].str("state") != "approved" || !strings.Contains(held[0].str("error"), wt["ahead"]+" holds work (") {
		t.Fatalf("recorded as failed, not left pending: %v", held)
	}
	assertNotRetired(t, r, wt)
}

// A worktree on no branch is judged by its HEAD: on main and clean it goes as a merged one does, holding a commit it is saved to a rescue
// branch. One git no longer sees as a checkout (its folder's .git is gone) is left as it is.
func TestAWorktreeOnNoBranchIsJudgedByItsHeadAndOneThatIsNoCheckoutIsLeft(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob := r.who("bob")
	main, _, git := bobsWorktrees(t, r)
	detached, held, nested := filepath.Join(filepath.Dir(main), "detached"), filepath.Join(filepath.Dir(main), "held"), filepath.Join(main, "nested")
	git(main, "worktree", "add", "-q", "--detach", detached)
	git(main, "worktree", "add", "-q", "--detach", held)
	git(held, "commit", "-q", "--allow-empty", "-m", "detached work")
	git(main, "worktree", "add", "-q", "-b", "nested", nested)
	os.Remove(filepath.Join(nested, ".git"))
	pinDesks(r, bob, main, detached, held, nested)
	m := r.call("agent.retire", `{"agent":"bob","worktree":"save","request_id":"d1"}`)
	res, _ := m["result"].(map[string]any)
	said, _ := res["said"].(string)
	if !strings.Contains(said, detached+" is removed;") && !strings.HasSuffix(said, detached+" is removed.") {
		t.Fatalf("a clean worktree on no branch: %v", m)
	}
	if !strings.Contains(said, held+" is removed; what it held is on rescue/"+bob+"-") {
		t.Fatalf("a worktree on no branch holding a commit: %v", m)
	}
	for _, p := range []string{detached, held} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s is still there", p)
		}
	}
	if out := gitrun.Run(main, "log", "--format=%s", "--branches=rescue/*").Stdout; !strings.Contains(out, "detached work") {
		t.Fatalf("the detached commit is on no rescue branch: %q", out)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("the folder that is no checkout was removed: %v", err)
	}
}

// A running agent retired with an answer is held with it: when its turn ends, SweepHeld carries the answer out, and nobody is asked again.
func TestARunningAgentsRetireKeepsTheFoundersAnswerForItsTurnsEnd(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	_, wt, _ := bobsWorktrees(t, r)
	r.e.Ask = func(title, body string, tags ...string) (int64, error) {
		t.Error("asked the founder again: " + title)
		return 0, nil
	}
	m := r.retireHeld(`{"agent":"bob","worktree":"force","request_id":"h2"}`, func(sweep func(bool)) { sweep(true) })
	if res, _ := m["result"].(map[string]any); res == nil || !strings.Contains(res["said"].(string), wt["ahead"]+" is removed with what it held") {
		t.Fatalf("%v", m)
	}
	if _, err := os.Stat(wt["ahead"]); err == nil || strings.Contains(r.rosterText(), "  bob: {}") {
		t.Fatalf("bob retired and its worktree forced away (stat err %v)\n%s", err, r.rosterText())
	}
}

// An agent that works in the repository's own checkout is deleted and the checkout stays: it is no worktree to remove.
func TestAnAgentInTheRepositorysOwnCheckoutIsDeletedAndTheCheckoutStays(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob := r.who("bob")
	main, _, _ := bobsWorktrees(t, r)
	r.db.Exec("UPDATE agent_state SET worktree = ? WHERE agent = ?", main, bob)
	r.db.Exec("DELETE FROM worktree_folders")
	if m := r.call("agent.retire", `{"agent":"bob","request_id":"c1"}`); m["result"] == nil {
		t.Fatalf("%v", m)
	}
	if _, err := os.Stat(filepath.Join(main, ".git")); err != nil || strings.Contains(r.rosterText(), "  bob: {}") {
		t.Fatalf("bob retired and its checkout kept (stat err %v)", err)
	}
}

func TestRetiringAnAgentIsRefusedWhileASeatIsAtWorkAndTakesItsIdleSeats(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := team.NewRoster(r.home).Register("bob/kid", team.RegisterOpts{Role: "r"}); err != nil {
		t.Fatal(err)
	}
	kid := r.who("bob/kid")
	var seats []string
	for range 2 {
		s, err := team.NewSeat(r.db, kid, 1)
		if err != nil {
			t.Fatal(err)
		}
		seats = append(seats, s)
	}
	if err := team.SetHiredFor(r.db, seats[1], 7); err != nil {
		t.Fatal(err)
	}
	if msg := refusal(t, r.call("agent.retire", `{"agent":"`+kid+`","request_id":"r2"}`)); !strings.Contains(msg, "has seats at work ("+seats[1]+" on T7)") {
		t.Fatal(msg)
	}
	if got, _ := team.TempsOf(r.db, kid); len(got) != 2 || !slices.Contains(paths.ListAgents(r.home), kid) {
		t.Fatalf("a refusal removed nothing: seats %v", got)
	}

	if err := team.SetHiredFor(r.db, seats[1], 0); err != nil {
		t.Fatal(err)
	}
	if res, _ := r.call("agent.retire", `{"agent":"`+kid+`","request_id":"r3"}`)["result"].(map[string]any); res == nil {
		t.Fatal("an agent whose seats are idle is retired")
	}
	var rows int
	r.db.QueryRow("SELECT COUNT(*) FROM agents WHERE hired_from = ?", kid).Scan(&rows)
	if rows != 0 {
		t.Fatalf("its idle seats go with it: %d left", rows)
	}
}
