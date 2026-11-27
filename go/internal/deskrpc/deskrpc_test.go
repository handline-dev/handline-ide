package deskrpc

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/rpctest"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/worktreepool"
)

func TestTheFounderPinsADeskByItsNameAndAnyoneReadsTheDesks(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker")
	worker := teamtest.Key(t, home, "gp/worker")
	for _, q := range []string{"INSERT INTO worktree_folders(path, repo, holder, last_agent, last_used) VALUES('/r.worktrees.noindex/desk-1', '/r', NULL, 'x', 5)",
		"INSERT INTO worktree_folders(path, repo, last_used) VALUES('/s.worktrees.noindex/desk-1', '/s', 6)",
		"INSERT INTO worktree_folders(path, repo, last_used) VALUES('/r.worktrees.noindex/desk-2', '/r', 7)"} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	run := runner(t, d)
	pinned := func(path string) string {
		var p string
		d.QueryRow("SELECT COALESCE(pinned_to, '') FROM worktree_folders WHERE path = ?", path).Scan(&p)
		return p
	}
	if errText(run(worker, "desk.pin", `"desk":"desk-2","to":"p1"`)) == "" {
		t.Error("an agent pinned a desk")
	}
	if got := errText(run("", "desk.pin", `"desk":"desk-1","to":"p1"`)); !strings.Contains(got, "2 desks are named desk-1") {
		t.Errorf("an ambiguous name: %q", got)
	}
	if got := errText(run("", "desk.pin", `"desk":"desk-2","to":"p1"`)); got != "" || pinned("/r.worktrees.noindex/desk-2") != worktreepool.Founder {
		t.Fatalf("founder's pin: %q", got)
	}
	if got := errText(run("", "desk.pin", fmt.Sprintf(`"desk":"/r.worktrees.noindex/desk-1","to":%q`, worker))); got != "" || pinned("/r.worktrees.noindex/desk-1") != worker {
		t.Fatalf("an agent's pin by path: %q", got)
	}
	if got := errText(run("", "desk.pin", `"desk":"desk-2"`)); got != "" || pinned("/r.worktrees.noindex/desk-2") != "" {
		t.Fatalf("unpin: %q", got)
	}
	got := run(worker, "desks.list", `"repo":"/r"`)
	res, _ := got["result"].(map[string]any)
	desks, _ := res["desks"].([]any)
	if len(desks) != 2 || desks[0].(map[string]any)["name"] != "desk-1" || desks[0].(map[string]any)["pinned_to"] != worker {
		t.Fatalf("list: %v", got)
	}
}

// runner calls one method on a server of its own as as (an agent key, "" for the founder); a write gets a request id of its own.
func runner(t *testing.T, d *sql.DB) func(as, method, params string) map[string]any {
	n := 0
	return func(as, method, params string) map[string]any {
		n++
		if method != "desks.list" {
			params += fmt.Sprintf(`,"request_id":"r%d"`, n)
		}
		s := rpc.NewServer(auth{as}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		Register(s)
		return call(t, s, `{"jsonrpc":"2.0","id":2,"method":"`+method+`","params":{`+params+`}}`)[0]
	}
}

type auth struct{ agent string }

func (a auth) AgentOf(int) string         { return a.agent }
func (a auth) FounderKeyOK(k string) bool { return a.agent == "" && k == "founder" }
func (a auth) SessionsReadable() error    { return nil }

func call(t *testing.T, srv *rpc.Server, calls ...string) []map[string]any {
	return rpctest.Call(t, srv, "founder", calls...)
}

func errText(r map[string]any) string {
	if e, ok := r["error"].(map[string]any); ok {
		return fmt.Sprint(e["message"])
	}
	return ""
}

// desks.list with a repo gives each desk its size and its holder's task.
func TestDesksListGivesEachDeskItsSizeAndItsHoldersTask(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	home := t.TempDir()
	repo, _ := filepath.EvalSymlinks(t.TempDir()) // the pool keeps repos by their resolved path
	d := teamtest.Seed(t, home, "gp", "gp/worker")
	teamtest.Set(t, home, "gp", "repo", repo)
	worker := teamtest.Key(t, home, "gp/worker")
	for _, q := range []string{
		fmt.Sprintf("INSERT INTO worktree_folders(path, repo, last_used) VALUES('/w/desk-1', '%s', 1), ('/w/desk-2', '%s', 2)", repo, repo),
		fmt.Sprintf("INSERT INTO worktree_folders(path, repo, holder, last_used) VALUES('/w/desk-4', '%s', '%s', 3)", repo, worker),
		fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(77, 'x', '%s', 'p1', 1)", worker),
		fmt.Sprintf("INSERT INTO agent_state(agent, state, updated, current_task) VALUES('%s', 'running', 1, 77)", worker),
		"INSERT INTO meta(name, value, updated) VALUES('worktree_size:/w/desk-1', '2048', 1)",
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	res, _ := runner(t, d)(worker, "desks.list", fmt.Sprintf(`"repo":%q`, repo))["result"].(map[string]any)
	desks := map[string]map[string]any{}
	for _, x := range res["desks"].([]any) {
		desks[x.(map[string]any)["name"].(string)] = x.(map[string]any)
	}
	if desks["desk-1"]["size_bytes"] != float64(2048) || desks["desk-2"]["size_bytes"] != nil || desks["desk-4"]["task"] != float64(77) || desks["desk-1"]["task"] != nil {
		t.Fatalf("size and task: %v %v %v", desks["desk-1"], desks["desk-2"], desks["desk-4"])
	}
}
