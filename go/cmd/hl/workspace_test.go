package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkspaceNewRenameDeleteCallTheOwnersMethods(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"workspace.create": `{"accepted":true,"operation":"op1"}`,
		"workspace.set": `{"accepted":true,"said":"workspace shop is store"}`, "workspace.delete": `{"accepted":true,"said":"workspace store deleted"}`})
	root, _ := filepath.EvalSymlinks(r.root)
	for _, c := range []struct {
		args []string
		out  string
	}{
		{[]string{"workspace", "new", "shop", "--folder", "sub"}, "workspace shop is being made in " + filepath.Join(root, "sub") + " and its lead started (operation op1)"},
		{[]string{"workspace", "new", "here"}, "workspace here is being made in " + root},
		{[]string{"workspace", "rename", "shop", "--to", "store"}, "workspace shop is store"},
		{[]string{"workspace", "delete", "store", "--worktree", "save"}, "workspace store deleted"},
	} {
		if out, errs, code := r.run(nil, c.args...); code != 0 || errs != "" || !strings.HasPrefix(out, c.out) {
			t.Errorf("%v: %d %q %q", c.args, code, out, errs)
		}
	}
	for _, args := range [][]string{{"workspace"}, {"workspace", "new"}, {"workspace", "rename", "shop"}, {"workspace", "delete", "a", "--worktree", "keep"}, {"workspace", "move", "a"}} {
		if _, errs, code := r.run(nil, args...); code != 2 || !strings.Contains(errs, "arguments not understood") {
			t.Errorf("%v: %d %q", args, code, errs)
		}
	}
	if out, _, code := r.run(asAgent, "workspace", "delete", "store"); code != 1 || !strings.Contains(out, "hl propose team") {
		t.Errorf("an agent is not pointed to a proposal: %d %q", code, out)
	}
	want := []string{
		`workspace.create {"folder":"` + filepath.Join(root, "sub") + `","name":"shop"}`,
		`workspace.create {"folder":"` + root + `","name":"here"}`,
		`workspace.set {"name":"store","workspace":"shop"}`,
		`workspace.delete {"workspace":"store","worktree":"save"}`,
	}
	if got := sent(calls()); !reflect.DeepEqual(got, want) {
		t.Errorf("calls\n got  %v\n want %v", got, want)
	}
}
