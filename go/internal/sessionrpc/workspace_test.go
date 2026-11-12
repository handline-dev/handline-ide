package sessionrpc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/wslock"
)

func TestWorkspaceCreateMakesItsLeadOnTheCheckoutTopAndStartsIt(t *testing.T) {
	r, sn, c := createRig(t)
	repo, err := filepath.EvalSymlinks(checkout(t)) // git names a checkout's top by its real path
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
	if r.ctl.calls[len(r.ctl.calls)-1] != "start shop "+repo+"  true" {
		t.Fatalf("the lead starts on the checkout's top: %v", r.ctl.calls)
	}
	if team.NewRoster(r.e.Home).RepoOf("shop") != repo {
		t.Fatalf("the workspace's folder is the top of its checkout")
	}
	if _, err := os.Stat(team.RulesFile(r.e.Home, paths.WorkspaceKey(r.e.Home, "shop"))); err != nil {
		t.Fatalf("its rules are seeded: %v", err)
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
		{"shop.x", checkout(t), func(m map[string]any) bool { return badField(m) == "name" }},
		{"shop", plain, func(m map[string]any) bool { return refused(m) == "not_git" }},
		{"shop", filepath.Join(plain, "gone"), func(m map[string]any) bool {
			e, _ := m["error"].(map[string]any)
			return e["message"] != nil && refused(m) == "" && badField(m) == ""
		}},
		{"ann", checkout(t), func(m map[string]any) bool { return refused(m) == "state" }},
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
	if g := refused(c.call("session.start", map[string]any{"agent": "ann", "request_id": rid("x1")})); g != "state" {
		t.Errorf("session.start: gate %q", g)
	}
	if g := refused(c.call("agent.create", map[string]any{"name": "ann.new", "role": "r", "repo": checkout(t), "parent": "ann", "request_id": rid("x2")})); g != "state" {
		t.Errorf("agent.create: gate %q", g)
	}
	if len(r.ctl.calls) != 0 {
		t.Fatalf("%v", r.ctl.calls)
	}
}
