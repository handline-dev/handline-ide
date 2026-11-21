package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/projects"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/internal/sessionrpc"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

// A runner that joins gets a clone of every workspace whose repo has an origin, and node.status says so; a workspace with none is
// no_remote. A workspace added while the runner is linked is cloned there, and one added while it was away is cloned when it comes
// back.
func TestARunnerGetsEveryWorkspacesRepoWhenItLinksAndWhenOneIsAdded(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth = fakeAuth{key: "k", agent: "a"}
	cfg.Log = t.Logf
	var env *sessionrpc.Env
	cfg.Session = func() *sessionrpc.Env { env = &sessionrpc.Env{}; return env }
	master, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(master.Close)
	src := t.TempDir()
	workspace := func(name string, remote bool) string {
		t.Helper()
		folder := filepath.Join(src, name)
		git := func(args ...string) {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, out)
			}
		}
		git("init", "-q", "-b", "main", folder)
		if remote {
			bare := folder + ".git"
			git("init", "-q", "--bare", "-b", "main", bare)
			git("-C", folder, "remote", "add", "origin", bare)
			git("-C", folder, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "first")
			git("-C", folder, "push", "-q", "origin", "main")
		}
		res, err := master.db.Exec("INSERT INTO workspaces(name, folder, created_at) VALUES(?, ?, 0)", name, folder)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return actor.Key(actor.Workspace, id)
	}
	shop, notes := workspace("shop", true), workspace("notes", false)

	home, node, m := joinRunner(t, master)
	if _, err := master.db.Exec("UPDATE nodes SET lending = 'run' WHERE id = ?", node); err != nil { // node.status lists a machine on run
		t.Fatal(err)
	}
	start := func() *runner.Runner {
		r, err := runner.Start(runner.Config{Home: home, Master: m, HL: "true", Log: t.Logf})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	states := func() map[string]string {
		got, err := master.local.Dispatch(rpc.Caller{Kind: "person", Key: actor.OwnerKey, Role: "owner"}, "test", "", "node.status", nil)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, n := range got.(map[string]any)["nodes"].([]any) {
			if row := n.(map[string]any); row["id"] == node && row["projects"] != nil {
				for _, p := range row["projects"].([]any) {
					p := p.(map[string]any)
					out[p["workspace"].(string)] = p["state"].(string)
				}
			}
		}
		return out
	}
	until := func(ws, state string) {
		t.Helper()
		for end := time.Now().Add(20 * time.Second); states()[ws] != state; time.Sleep(50 * time.Millisecond) {
			if time.Now().After(end) {
				t.Fatalf("%s never %s on the runner: %v", ws, state, states())
			}
		}
	}
	r := start()
	until(shop, projects.Cloned)
	until(notes, projects.NoRemote)
	clones := func() int {
		ents, _ := os.ReadDir(filepath.Join(home, "runner-repos"))
		return len(ents)
	}
	if clones() != 1 {
		t.Fatalf("clones on the runner: %d", clones())
	}

	web := workspace("web", true)
	env.WorkspaceAdded("web")
	until(web, projects.Cloned)

	r.Close()
	for end := time.Now().Add(10 * time.Second); len(master.nodes.Linked()) != 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("the runner's link never ended")
		}
	}
	api := workspace("api", true)
	env.WorkspaceAdded("api")
	if _, ok := states()[api]; ok {
		t.Fatal("a workspace was sent to a runner that is away")
	}
	r = start()
	defer r.Close()
	until(api, projects.Cloned)
	master.projects.Wait()
	if clones() != 3 {
		t.Fatalf("clones on the runner: %d", clones())
	}
}

// A commit an agent makes in its worktree on a runner is on the remote within the push: the runner's hook calls hl from the agent's
// session, which the runner proves the agent's and pushes the branch for.
func TestACommitInAnAgentsWorktreeOnARunnerIsPushed(t *testing.T) {
	master, node, home, _ := masterAndRunner(t)
	rn, _ := master.nodes.Runner(node)
	dir := t.TempDir()
	bare := filepath.Join(dir, "o.git")
	for _, args := range [][]string{{"init", "-q", "--bare", "-b", "main", bare}, {"clone", "-q", bare, filepath.Join(dir, "src")},
		{"-C", filepath.Join(dir, "src"), "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "first"},
		{"-C", filepath.Join(dir, "src"), "push", "-q", "origin", "HEAD:main"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	wt := filepath.Join(home, "wt")
	if err := rn.MakeWorktree(bare, wt, "bob", "main"); err != nil {
		t.Fatal(err)
	}
	handle, err := rn.Open("bob", wt, "cd "+wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := rn.Keys(handle, "git -c user.name=t -c user.email=t@t commit -q --allow-empty -m work && git rev-parse HEAD > ../made", true); err != nil {
		t.Fatal(err)
	}
	var made, pushed string
	for end := time.Now().Add(20 * time.Second); made == "" || pushed != made; time.Sleep(100 * time.Millisecond) {
		b, _ := os.ReadFile(filepath.Join(home, "made"))
		made = strings.TrimSpace(string(b))
		out, _ := exec.Command("git", "-C", bare, "rev-parse", "--verify", "--quiet", "refs/heads/bob").Output()
		pushed = strings.TrimSpace(string(out))
		if time.Now().After(end) {
			rows, _ := rn.Screen(handle, 40, false)
			t.Fatalf("the commit %q never reached the remote (it has %q): %q", made, pushed, rows)
		}
	}
}
