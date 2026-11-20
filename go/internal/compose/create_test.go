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

// agent.create through the real start: a new agent given by its dotted name is launched and given its desk by its key, the only
// names `hl launch` and the desk pool resolve.
func TestACreatedAgentIsLaunchedAndGivenItsDeskByItsKey(t *testing.T) {
	ft := newFakeTerminals()
	p, _, _, _ := wiredWith(t, ft)
	dir, _ := team.AccountConfigDir(p.home, teamtest.Account(t, p.home, "a1"))
	ft.started = sessionRecordIn(dir)
	repo, _ := filepath.EvalSymlinks(gitRepo(t))
	srv := rpc.NewServer(founder{}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	srv.DB, srv.Names = p.db, teamtest.DB(t, p.home)
	sessionrpc.Register(srv, p.SessionRPC(cfgOf(p)))
	params, _ := json.Marshal(map[string]any{"name": "alice.town", "role": "worker", "repo": repo, "request_id": "11111111-1111-4111-8111-111111111111"})
	r := call(t, srv, `{"jsonrpc":"2.0","id":2,"method":"agent.create","params":`+string(params)+`}`)[0]
	key := paths.AgentKey(p.home, "alice.town")
	if r["error"] != nil || key == "" {
		t.Fatalf("created and started: %v (key %q)", r, key)
	}
	if len(ft.opened) != 1 || !strings.HasSuffix(ft.opened[0], "|"+key+"|"+lifecycle.CommandFor("", "", "", key)) {
		t.Fatalf("launched by its key: %q", ft.opened)
	}
	var holder, last string
	if err := p.db.QueryRow("SELECT holder, last_agent FROM worktree_folders WHERE repo = ?", repo).Scan(&holder, &last); err != nil || holder != key || last != key {
		t.Fatalf("the desk is held by its key: %q %q %v", holder, last, err)
	}
}
