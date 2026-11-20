package sessionrpc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/testfiles"
	"github.com/handline/handline/go/kit/wslock"
)

func TestWorkspaceCreateMakesItsLeadOnTheCheckoutTopAndStartsIt(t *testing.T) {
	r, sn, c := createRig(t)
	repo, err := filepath.EvalSymlinks(testfiles.GitCheckout(t)) // git names a checkout's top by its real path
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "web")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := result(t, c.call("workspace.create", map[string]any{"name": "shop", "folder": sub, "request_id": rid("w1")}))
	if got["accepted"] != true || got["operation"] == nil || got["said"] != "done" {
		t.Fatalf("answered once the lead is up: %v", got)
	}
	startedByKey(t, r, "shop", repo) // the lead, on the checkout's top
	if team.NewRoster(r.e.Home).RepoOf("shop") != repo {
		t.Fatalf("the workspace's folder is the top of its checkout")
	}
	if text, err := team.RulesText(r.e.DB, "shop"); err != nil || text == "" {
		t.Fatalf("its rules are seeded: %q %v", text, err)
	}
	if last := sn.got[len(sn.got)-1]; last["kind"] != "workspace.create" || last["agent"] != "shop" || last["state"] != "done" {
		t.Fatalf("the operations topic tells it as workspace.create: %v", last)
	}
}

// What can be told at once is refused before any operation starts.
func TestWorkspaceCreateRefusesBeforeAnyOperation(t *testing.T) {
	r, sn, c := createRig(t)
	plain := t.TempDir()
	cases := []struct {
		name, folder string
		check        func(map[string]any) bool
	}{
		{"shop.x", testfiles.GitCheckout(t), func(m map[string]any) bool { return badField(m) == "name" }},
		{"shop", filepath.Join(plain, "gone"), func(m map[string]any) bool {
			e, _ := m["error"].(map[string]any)
			return e["message"] != nil && refused(m) == "" && badField(m) == ""
		}},
		{"ann", testfiles.GitCheckout(t), func(m map[string]any) bool { return refused(m) == "state" }},
	}
	for i, tc := range cases {
		m := c.call("workspace.create", map[string]any{"name": tc.name, "folder": tc.folder, "request_id": rid("wr" + string(rune('a'+i)))})
		if !tc.check(m) {
			t.Errorf("%s on %s: %v", tc.name, tc.folder, m)
		}
	}
	if len(r.ctl.calls) != 0 || len(sn.got) != 0 {
		t.Fatalf("no operation started: %v %v", r.ctl.calls, sn.got)
	}
}

// While its workspace is being deleted, no agent is started or added there.
func TestNothingStartsInAWorkspaceBeingDeleted(t *testing.T) {
	r, _, c := createRig(t)
	release, ok := wslock.Delete(r.e.Home, paths.WorkspaceKey(r.e.Home, "ann"))
	if !ok {
		t.Fatal("free")
	}
	defer release()
	annIn(t, r)
	if g := refused(c.call("session.start", map[string]any{"agent": "ann", "request_id": rid("x1")})); g != "state" {
		t.Errorf("session.start: gate %q", g)
	}
	if g := refused(c.call("agent.create", map[string]any{"workspace": "ann", "name": "new", "role": "r", "request_id": rid("x2")})); g != "state" {
		t.Errorf("agent.create: gate %q", g)
	}
	if len(r.ctl.calls) != 0 {
		t.Fatalf("%v", r.ctl.calls)
	}
}

// A plain folder is a workspace's folder as it is: its lead starts there.
func TestWorkspaceCreateOnAPlainFolder(t *testing.T) {
	r, _, c := createRig(t)
	plain := t.TempDir()
	result(t, c.call("workspace.create", map[string]any{"name": "notes", "folder": plain, "request_id": rid("wp")}))
	startedByKey(t, r, "notes", filepath.Clean(plain))
}
