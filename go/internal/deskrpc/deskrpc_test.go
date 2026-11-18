package deskrpc

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/rpctest"
	"github.com/handline/handline/go/kit/gitrun"
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
	run := runner(t, d, home)
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
func runner(t *testing.T, d *sql.DB, home string) func(as, method, params string) map[string]any {
	n := 0
	return func(as, method, params string) map[string]any {
		n++
		if method != "desks.list" {
			params += fmt.Sprintf(`,"request_id":"r%d"`, n)
		}
		s := rpc.NewServer(auth{as}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		Register(s, home)
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

// desks.list with a repo says how many desks it keeps and which the next prune removes, each desk with its size and its holder's task;
// desks.keep sets the number, the founder only.
func TestDesksKeepSetsTheNumberAndTheListSaysWhatGoesTonight(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	home := t.TempDir()
	repo, _ := filepath.EvalSymlinks(t.TempDir()) // the pool keeps repos by their resolved path
	d := teamtest.Seed(t, home, "gp", "gp.worker")
	teamtest.Set(t, home, "gp", "repo", repo)
	worker := teamtest.Key(t, home, "gp.worker")
	old := time.Now().Add(-5 * 24 * time.Hour).Unix()
	busy := t.TempDir() // the oldest desk holds an untracked file: the prune passes it over
	if r := gitrun.Run(busy, "init", "-q"); r.RC != 0 {
		t.Fatal(r.Stderr)
	}
	os.WriteFile(filepath.Join(busy, "notes.txt"), []byte("x"), 0o644)
	for _, q := range []string{
		fmt.Sprintf("INSERT INTO worktree_folders(path, repo, last_used) VALUES('/w/desk-1', '%s', %d), ('/w/desk-2', '%s', %d), ('/w/desk-3', '%s', %d)", repo, old, repo, old+60, repo, time.Now().Unix()),
		fmt.Sprintf("INSERT INTO worktree_folders(path, repo, holder, last_used) VALUES('/w/desk-4', '%s', '%s', %d)", repo, worker, old),
		fmt.Sprintf("INSERT INTO worktree_folders(path, repo, pinned_to, last_used) VALUES('/w/desk-5', '%s', 'founder', %d)", repo, old-60),
		fmt.Sprintf("INSERT INTO worktree_folders(path, repo, last_used) VALUES('%s', '%s', %d)", busy, repo, old-120),
		fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(77, 'x', '%s', 'p1', 1)", worker),
		fmt.Sprintf("INSERT INTO agent_state(agent, state, updated, current_task) VALUES('%s', 'running', 1, 77)", worker),
		"INSERT INTO meta(name, value, updated) VALUES('worktree_size:/w/desk-1', '2048', 1)",
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	run := runner(t, d, home)
	list := func() (map[string]any, map[string]map[string]any) {
		res, _ := run(worker, "desks.list", fmt.Sprintf(`"repo":%q`, repo))["result"].(map[string]any)
		byName := map[string]map[string]any{}
		for _, x := range res["desks"].([]any) {
			byName[x.(map[string]any)["name"].(string)] = x.(map[string]any)
		}
		kept, _ := res["kept"].(map[string]any)
		return kept, byName
	}
	if kept, _ := list(); kept["mode"] != "auto" || kept["value"] != nil || kept["auto_value"] != nil {
		t.Fatalf("auto with no week of load: %v", kept)
	}
	if errText(run(worker, "desks.keep", fmt.Sprintf(`"repo":%q,"mode":"manual","value":2`, repo))) == "" {
		t.Error("an agent set the number")
	}
	for p, want := range map[string]string{`"repo":"/nope","mode":"auto"`: "no repo", fmt.Sprintf(`"repo":%q,"mode":"manual","value":0`, repo): "value is 1 to"} {
		if got := errText(run("", "desks.keep", p)); !strings.Contains(got, want) {
			t.Errorf("%s: %q", p, got)
		}
	}
	if got := errText(run("", "desks.keep", fmt.Sprintf(`"repo":%q,"mode":"manual","value":2`, repo))); got != "" {
		t.Fatal(got)
	}
	kept, desks := list()
	if kept["mode"] != "manual" || kept["value"] != float64(2) {
		t.Fatalf("manual: %v", kept)
	}
	// six desks, two kept: desk-1 and desk-2 are free and old; desk-3 was just used, desk-4 is held, desk-5 pinned, the oldest holds work
	for name, gone := range map[string]bool{"desk-1": true, "desk-2": true, "desk-3": false, "desk-4": false, "desk-5": false, filepath.Base(busy): false} {
		if desks[name]["removed_tonight"] != gone {
			t.Errorf("%s removed tonight: %v", name, desks[name]["removed_tonight"])
		}
	}
	if desks["desk-1"]["size_bytes"] != float64(2048) || desks["desk-2"]["size_bytes"] != nil || desks["desk-4"]["task"] != float64(77) || desks["desk-1"]["task"] != nil {
		t.Fatalf("size and task: %v %v %v", desks["desk-1"], desks["desk-2"], desks["desk-4"])
	}
}
