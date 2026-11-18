package deskrpc

import (
	"fmt"
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
	d := teamtest.Seed(t, home, "gp", "gp.worker")
	worker := teamtest.Key(t, home, "gp.worker")
	for _, q := range []string{"INSERT INTO worktree_folders(path, repo, holder, last_agent, last_used) VALUES('/r.worktrees.noindex/desk-1', '/r', NULL, 'x', 5)",
		"INSERT INTO worktree_folders(path, repo, last_used) VALUES('/s.worktrees.noindex/desk-1', '/s', 6)",
		"INSERT INTO worktree_folders(path, repo, last_used) VALUES('/r.worktrees.noindex/desk-2', '/r', 7)"} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	run := func(as, method, params string) map[string]any {
		n++
		if method == "desk.pin" {
			params += fmt.Sprintf(`,"request_id":"r%d"`, n)
		}
		s := rpc.NewServer(auth{as}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		Register(s)
		return call(t, s, `{"jsonrpc":"2.0","id":2,"method":"`+method+`","params":{`+params+`}}`)[0]
	}
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
