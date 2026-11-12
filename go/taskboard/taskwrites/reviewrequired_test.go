package taskwrites

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
)

// review_required holds a done with commits until an agent that did not work the task comments on it; the founder, a done without
// commits and another workspace's agents are not held.
func TestReviewRequiredHoldsADoneWithCommitsUntilAnotherAgentComments(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "shop", "shop.bob", "shop.rev", "lab")
	k := func(n string) string { return teamtest.Key(t, home, n) }
	bob, rev, lead, lab := k("shop.bob"), k("shop.rev"), k("shop"), k("lab")
	repo := t.TempDir()
	git := gitIn(t, repo)
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "on main")
	sha := git("rev-parse", "HEAD")
	for id, owner := range map[int]string{1: bob, 2: bob, 3: lab, 4: bob, 5: bob, 6: bob, 7: bob} {
		if _, err := d.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at, expects) VALUES(?, 't', ?, ?, 1, 'report')", id, owner, lead); err != nil {
			t.Fatal(err)
		}
	}
	shop, _, _ := team.WorkspaceID(d, "shop")
	if err := team.SetRule(d, shop, team.RuleReviewRequired, true, "p1", 1); err != nil {
		t.Fatal(err)
	}
	n := 0
	done := func(agent string, task int, params string) map[string]any {
		n++
		s := rpc.NewServer(auth{agent}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		Register(s, home, &fakeTeller{}, nil)
		return call(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.done","params":{"task":%d,"request_id":"r%d"%s}}`, task, n, params))[0]
	}
	withSha := fmt.Sprintf(`,"evidence":%q,"repo":%q`, sha, repo)
	memo := func(task int, author string) {
		if _, err := d.Exec("INSERT INTO task_memos(task_id, author, body, created_at) VALUES(?, ?, 'looks right', 1)", task, author); err != nil {
			t.Fatal(err)
		}
	}
	want := "task T1 has commits, and its workspace's rule review_required true (set by "
	if got := errText(done(bob, 1, withSha)); !strings.HasPrefix(got, want) {
		t.Fatalf("no review: %q", got)
	}
	memo(1, bob)
	memo(1, "p1")
	if _, err := d.Exec(`INSERT INTO events(ts, agent, kind, payload) VALUES(1, ?, 'task_handoff', json_object('task_id', 1, 'from', ?, 'to', ?))`,
		rev, rev, bob); err != nil {
		t.Fatal(err)
	}
	memo(1, rev)
	if got := errText(done(bob, 1, withSha)); !strings.HasPrefix(got, want) {
		t.Fatalf("its own, the founder's and one by an agent that handed it on are no review: %q", got)
	}
	memo(5, lead)
	if got := errText(done(lead, 5, withSha)); !strings.HasPrefix(got, "task T5 has commits") {
		t.Fatalf("nor is one by the agent closing it: %q", got)
	}
	// A commit with words is commits too, and the task's holder, or the agent whose turn it is, did not review it.
	if _, err := d.Exec("INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(6, 1, ?, ?, 1), (7, 1, ?, ?, 1)", rev, bob, rev, bob); err != nil {
		t.Fatal(err)
	}
	memo(6, bob)
	if got := errText(done(lead, 6, fmt.Sprintf(`,"evidence":"built","commit":%q,"repo":%q`, sha, repo))); !strings.HasPrefix(got, "task T6 has commits") {
		t.Fatalf("its holder's: %q", got)
	}
	memo(7, rev)
	if got := errText(done(bob, 7, withSha)); !strings.HasPrefix(got, "task T7 has commits") {
		t.Fatalf("the turn's agent's: %q", got)
	}
	memo(1, lab)
	if r := done(bob, 1, withSha); r["result"] == nil {
		t.Fatalf("another agent's comment is a review: %v", r)
	}
	if r := done(bob, 2, `,"evidence":"answered"`); r["result"] == nil {
		t.Fatalf("a done without commits is not held: %v", r)
	}
	if r := done(lab, 3, withSha); r["result"] == nil {
		t.Fatalf("another workspace is not held: %v", r)
	}
	if r := done("", 4, withSha); r["result"] == nil {
		t.Fatalf("the founder is not held: %v", r)
	}
}
