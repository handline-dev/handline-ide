package proposalwrites

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
)

// bobWithKid is the rig with bob.kid in bob's workspace, and the keys of both.
func bobWithKid(t *testing.T, r *rig) (bob, kid string) {
	t.Helper()
	if _, err := team.NewRoster(r.home).Register("bob.kid", team.RegisterOpts{Role: "r"}); err != nil {
		t.Fatal(err)
	}
	return r.who("bob"), r.who("bob.kid")
}

func workspaceCount(r *rig, name string) int {
	var n int
	r.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE name = ?", name).Scan(&n)
	return n
}

func TestWorkspaceSetRenamesOnlyAndItsAgentsFollow(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob, kid := bobWithKid(t, r)
	ws := paths.WorkspaceKey(r.home, "bob")
	if m := r.call("workspace.set", `{"workspace":"`+ws+`","name":"shop","request_id":"s1"}`); m["result"] == nil {
		t.Fatalf("%v", m)
	}
	if r.who("shop") != bob || r.who("shop.kid") != kid || workspaceCount(r, "bob") != 0 {
		t.Fatalf("the agents keep their keys under the new name: %s %s", r.who("shop"), r.who("shop.kid"))
	}
	if m := r.call("workspace.set", `{"workspace":"shop","name":"shop","request_id":"s2"}`); m["result"] == nil {
		t.Fatalf("the same name is no change: %v", m)
	}
	for _, c := range []struct{ params, gate string }{
		{`{"workspace":"shop","name":"alice","request_id":"s3"}`, "state"}, // another workspace's name
		{`{"workspace":"shop","name":"Not ok","request_id":"s4"}`, "state"},
	} {
		if g := gate(r.call("workspace.set", c.params)); g != c.gate {
			t.Errorf("%s: gate %q", c.params, g)
		}
	}
	if m := r.call("workspace.set", `{"workspace":"nope","name":"x","request_id":"s5"}`); !strings.Contains(refusalOf(m), "no workspace nope") {
		t.Fatalf("%v", m)
	}
	if m := r.call("workspace.set", `{"workspace":"shop","request_id":"s6"}`); m["error"] == nil {
		t.Fatalf("a name is needed: %v", m)
	}
}

func TestWorkspaceDeleteRetiresEveryAgentLeadLastAndLeavesTheProjectFolder(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob, kid := bobWithKid(t, r)
	project := t.TempDir()
	r.db.Exec("UPDATE workspaces SET folder = ? WHERE name = 'bob'", project)
	m := r.call("workspace.delete", `{"workspace":"bob","request_id":"d1"}`)
	res, _ := m["result"].(map[string]any)
	if res == nil || !strings.HasPrefix(res["said"].(string), "workspace bob is deleted") {
		t.Fatalf("%v", m)
	}
	everyone := paths.ListAgents(r.home)
	if slices.Contains(everyone, bob) || slices.Contains(everyone, kid) || workspaceCount(r, "bob") != 0 || !slices.Contains(everyone, r.who("alice")) {
		t.Fatalf("bob's agents and workspace are gone, alice stays: %v", everyone)
	}
	if _, err := os.Stat(project); err != nil {
		t.Fatal("the project folder is never touched:", err)
	}
	if again, _ := r.call("workspace.delete", `{"workspace":"bob","request_id":"d1"}`)["result"].(map[string]any); again == nil || again["replayed"] != true {
		t.Fatalf("a retry is the stored reply: %v", again)
	}
}

// Nothing is deleted while one of its agents runs, or holds work with no answer for it.
func TestWorkspaceDeleteRefusesWithNothingDone(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob, kid := bobWithKid(t, r)
	r.setLive("bob.kid", true)
	m := r.call("workspace.delete", `{"workspace":"bob","request_id":"d1"}`)
	if gate(m) != "state" || !strings.Contains(refusalOf(m), "stop them first") {
		t.Fatalf("a running session: %v", m)
	}
	r.setLive("bob.kid", false)
	bobsWorktrees(t, r)
	if g := gate(r.call("workspace.delete", `{"workspace":"bob","request_id":"d2"}`)); g != "dirty" {
		t.Fatalf("work with no answer: gate %q", g)
	}
	if m := r.call("workspace.delete", `{"workspace":"bob","worktree":"keep","request_id":"d3"}`); m["error"] == nil || gate(m) != "" {
		t.Fatalf("keep would keep an agent: %v", m)
	}
	if everyone := paths.ListAgents(r.home); !slices.Contains(everyone, bob) || !slices.Contains(everyone, kid) {
		t.Fatalf("nothing was deleted: %v", everyone)
	}
}

// A retire that fails part way puts back every one before it: the lead's locked worktree keeps bob, and so bob.kid too.
func TestWorkspaceDeleteIsAllOrNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bob, kid := bobWithKid(t, r)
	main, wt, git := bobsWorktrees(t, r)
	git(main, "worktree", "lock", wt["done"])
	m := r.call("workspace.delete", `{"workspace":"bob","worktree":"save","request_id":"d1"}`)
	if msg := refusalOf(m); gate(m) != "state" || !strings.Contains(msg, "step 2 of 2") || !strings.Contains(msg, "Nothing was changed") {
		t.Fatalf("%v", m)
	}
	if everyone := paths.ListAgents(r.home); !slices.Contains(everyone, bob) || !slices.Contains(everyone, kid) || workspaceCount(r, "bob") != 1 {
		t.Fatalf("the kid's retire was put back: %v", everyone)
	}
	if _, err := os.Stat(paths.AgentDir(r.home, kid)); err != nil {
		t.Fatal("and its folder:", err)
	}
}
