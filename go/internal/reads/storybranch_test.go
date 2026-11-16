package reads

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/protocol/rpc"
)

// A story's sub-task names its branch in task.get, with whether it exists in its assignee's worktree; worktree.diff with that branch is
// the branch's commits alone, after the agent has checked out other work with changes of its own.
func TestAStorysSubTaskBranchIsReadWhateverTheWorktreeHasCheckedOut(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	prAgent(t, e, "wb", "")
	key := teamtest.Key(t, e.Home, "wb")
	dir, git := agentGit(t, e, "wb")
	commit := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", ".")
		git("commit", "-q", "-m", name)
	}
	commit("a.txt")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(7, 'part', '"+key+"', 'p1', 1)")
	run(t, e, `INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'p1', 'task_new', '{"task_id": 7, "draft": true}')`)
	branch := func() map[string]any {
		t.Helper()
		got, err := e.taskGet(nil, map[string]any{"task": json.Number("7")})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := got.(map[string]any)["branch"].(map[string]any)
		return b
	}
	if b := branch(); b["name"] != "wb-T7" || b["exists"] != false {
		t.Fatalf("before its first commit: %v", b)
	}

	git("checkout", "-q", "-b", "wb-T7")
	commit("story.txt")
	git("checkout", "-q", "-")
	git("checkout", "-q", "-b", "other")
	commit("other.txt")
	if err := os.WriteFile(filepath.Join(dir, "loose.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b := branch(); b["name"] != "wb-T7" || b["exists"] != true {
		t.Fatalf("after it: %v", b)
	}
	got, err := e.worktreeDiff(nil, map[string]any{"path": dir, "branch": "wb-T7"})
	if err != nil {
		t.Fatal(err)
	}
	d := got.(map[string]any)
	if s := d["diff"].(string); !strings.Contains(s, "story.txt") || strings.Contains(s, "other.txt") || strings.Contains(s, "loose.txt") || d["against"] != "base" {
		t.Fatalf("only the branch's commits:\n%v", d)
	}
	if got, err := e.worktreeDiff(nil, map[string]any{"path": dir, "branch": "wb-T7", "file": "other.txt"}); err != nil || got.(map[string]any)["diff"] != "" {
		t.Fatalf("a file the branch did not change: %v %v", got, err)
	}
	for p, code := range map[string]string{`{"branch": "-x"}`: "bad_request", `{"branch": "no..pe"}`: "bad_request", `{"branch": "nope"}`: "not_found",
		`{"branch": "wb-T7", "against": "head"}`: "bad_request"} {
		var params map[string]any
		if err := json.Unmarshal([]byte(p), &params); err != nil {
			t.Fatal(err)
		}
		params["path"] = dir
		var re *rpc.Error
		if _, err := e.worktreeDiff(nil, params); !errors.As(err, &re) || re.Code != code {
			t.Fatalf("%s: %v", p, err)
		}
	}
}
