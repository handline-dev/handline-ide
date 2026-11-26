package compose

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/sessionrpc"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/lifecycle"
)

// createdThroughTheRealStart runs agent.create with params through the real controls, and gives the new agent's key and where it was
// opened (folder|title|command).
func createdThroughTheRealStart(t *testing.T, p *Parts, ft *fakeTerminals, params map[string]any, name string) (string, string) {
	t.Helper()
	dir, _ := team.AccountConfigDir(p.home, teamtest.Account(t, p.home, "a1"))
	ft.started = sessionRecordIn(dir)
	srv := rpc.NewServer(founder{}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	srv.DB, srv.Names = p.db, teamtest.DB(t, p.home)
	sessionrpc.Register(srv, p.SessionRPC(cfgOf(p)))
	params["role"], params["request_id"] = "worker", "11111111-1111-4111-8111-111111111111"
	b, _ := json.Marshal(params)
	r := call(t, srv, `{"jsonrpc":"2.0","id":2,"method":"agent.create","params":`+string(b)+`}`)[0]
	key := paths.AgentKey(p.home, name)
	if r["error"] != nil || key == "" || len(ft.opened) != 1 {
		t.Fatalf("created and started: %v (key %q, opened %q)", r, key, ft.opened)
	}
	return key, ft.opened[0]
}

// agent.create through the real start: a new agent, registered as workspace/name, is launched and given its desk by its key, the only
// names `hl launch` and the desk pool resolve.
func TestACreatedAgentIsLaunchedAndGivenItsDeskByItsKey(t *testing.T) {
	ft := newFakeTerminals()
	p, _, _, _ := wiredWith(t, ft)
	repo, _ := filepath.EvalSymlinks(gitRepo(t))
	if _, err := p.db.Exec("UPDATE workspaces SET folder = ? WHERE name = 'alice'", repo); err != nil {
		t.Fatal(err)
	}
	key, opened := createdThroughTheRealStart(t, p, ft, map[string]any{"workspace": "alice", "name": "town"}, "alice/town")
	if !strings.HasSuffix(opened, "|"+key+"|"+lifecycle.CommandFor("", "", "", key)) {
		t.Fatalf("launched by its key: %q", opened)
	}
	var holder, last string
	if err := p.db.QueryRow("SELECT holder, last_agent FROM worktree_folders WHERE repo = ?", repo).Scan(&holder, &last); err != nil || holder != key || last != key {
		t.Fatalf("the desk is held by its key: %q %q %v", holder, last, err)
	}
}

// In a workspace of two repos, an agent of the second gets its desk from that repo's pool.
func TestAnAgentOfAWorkspacesSecondRepoGetsItsDeskThere(t *testing.T) {
	ft := newFakeTerminals()
	p, _, _, _ := wiredWith(t, ft)
	web, _ := filepath.EvalSymlinks(gitRepo(t))
	api, _ := filepath.EvalSymlinks(gitRepo(t))
	ws := paths.WorkspaceKey(p.home, "alice")
	p.db.Exec("UPDATE workspaces SET folder = ? WHERE name = 'alice'", web)
	if err := team.NewRoster(p.home).AddFolder(ws, api); err != nil {
		t.Fatal(err)
	}
	_, opened := createdThroughTheRealStart(t, p, ft, map[string]any{"workspace": ws, "name": "server", "folder": api}, "alice/server")
	if !strings.HasPrefix(opened, api+".worktrees.noindex/desk-1|") {
		t.Fatalf("a desk of its own repo: %q", opened)
	}
}
