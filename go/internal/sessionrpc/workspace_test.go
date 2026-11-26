package sessionrpc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/gitrun"
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
	var added []string
	r.e.WorkspaceAdded = func(name string) { added = append(added, name) }
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
	if len(added) != 1 || added[0] != "shop" {
		t.Fatalf("the runners were told of %v", added)
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

// With no folder the daemon makes ~/Handline/<name>; with no name the name comes from the folder, else "workspace", the first free.
func TestWorkspaceCreateFillsWhatIsLeftOut(t *testing.T) {
	r, _, c := createRig(t)
	result(t, c.call("workspace.create", map[string]any{"request_id": rid("wd1")}))
	made := filepath.Join(paths.Projects(), "workspace")
	startedByKey(t, r, "workspace", made)
	if err := os.MkdirAll(filepath.Join(paths.Projects(), "workspace-2", "kept"), 0o755); err != nil {
		t.Fatal(err)
	}
	result(t, c.call("workspace.create", map[string]any{"name": "", "request_id": rid("wd2")}))
	startedByKey(t, r, "workspace-2", filepath.Join(paths.Projects(), "workspace-2-2")) // a folder there with files is passed over
	plain := filepath.Join(t.TempDir(), "My Notes")
	if err := os.Mkdir(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	result(t, c.call("workspace.create", map[string]any{"folder": plain, "request_id": rid("wd3")}))
	startedByKey(t, r, "my-notes", plain)
}

// A typed name the rules refuse makes no folder; a projects folder inside a git checkout is refused and leaves nothing behind.
func TestWorkspaceCreateMakesNoFolderItCannotUse(t *testing.T) {
	r, sn, c := createRig(t)
	if m := c.call("workspace.create", map[string]any{"name": "Shop", "request_id": rid("wn1")}); badField(m) != "name" {
		t.Fatalf("a name of the wrong shape: %v", m)
	}
	if _, err := os.Stat(filepath.Join(paths.Projects(), "Shop")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a folder was made for a refused name: %v", err)
	}
	if err := os.MkdirAll(paths.Projects(), 0o755); err != nil {
		t.Fatal(err)
	}
	if res := gitrun.Run(filepath.Dir(paths.Projects()), "init", "-q"); res.RC != 0 {
		t.Fatal(res.Stderr)
	}
	if m := c.call("workspace.create", map[string]any{"name": "notes", "request_id": rid("wn2")}); refused(m) != "state" {
		t.Fatalf("projects inside a checkout: %v", m)
	}
	if _, err := os.Stat(filepath.Join(paths.Projects(), "notes")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the folder made was left: %v", err)
	}
	if len(r.ctl.calls) != 0 || len(sn.got) != 0 {
		t.Fatalf("no operation started: %v %v", r.ctl.calls, sn.got)
	}
}
