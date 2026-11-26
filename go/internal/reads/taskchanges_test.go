package reads

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/protocol/rpc"
)

// changesRepo is a repo cloned from a bare origin, worked by agent wb in a worktree of its own; it is wb's workspace folder.
type changesRepo struct {
	t        *testing.T
	repo, wt string
}

func newChangesRepo(t *testing.T, e *Env) *changesRepo {
	t.Helper()
	dir := t.TempDir()
	r := &changesRepo{t: t, repo: filepath.Join(dir, "proj"), wt: filepath.Join(dir, "proj-wb")}
	r.git(dir, "init", "-q", "--bare", "-b", "main", "origin.git")
	r.git(dir, "clone", "-q", filepath.Join(dir, "origin.git"), "proj")
	r.git(r.repo, "commit", "-q", "--allow-empty", "-m", "start")
	r.git(r.repo, "push", "-q", "origin", "HEAD:main")
	r.git(r.repo, "worktree", "add", "-q", "-b", "wb-work", r.wt)
	teamtest.Set(t, e.Home, "wb", "repo", r.repo)
	run(t, e, "INSERT INTO agent_state(agent, state, updated, worktree) VALUES('"+teamtest.Key(t, e.Home, "wb")+"', 'idle', 1, '"+r.wt+"')")
	return r
}

func (r *changesRepo) git(dir string, args ...string) string {
	r.t.Helper()
	return r.gitEnv(nil, dir, args...)
}

// gitEnv runs git with env added, such as a commit date.
func (r *changesRepo) gitEnv(env []string, dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit writes the files (name, text pairs) in dir and commits them with subject; it is the new commit's sha.
func (r *changesRepo) commit(dir, subject string, files ...string) string {
	r.t.Helper()
	for i := 0; i < len(files); i += 2 {
		if err := os.WriteFile(filepath.Join(dir, files[i]), []byte(files[i+1]), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.git(dir, "add", "-A")
	r.git(dir, "commit", "-q", "-m", subject)
	return r.git(dir, "rev-parse", "HEAD")
}

func changesOf(t *testing.T, e *Env, params string) map[string]any {
	t.Helper()
	var p map[string]any
	d := json.NewDecoder(strings.NewReader(params))
	d.UseNumber()
	if err := d.Decode(&p); err != nil {
		t.Fatal(err)
	}
	got, err := e.taskChanges(nil, p)
	if err != nil {
		t.Fatalf("%s: %v", params, err)
	}
	b, _ := json.Marshal(got)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func subjects(got map[string]any) []string {
	var s []string
	for _, c := range got["commits"].([]any) {
		s = append(s, c.(map[string]any)["subject"].(string))
	}
	return s
}

func addTask(t *testing.T, e *Env, id, owner string) {
	t.Helper()
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES("+id+", 'x', '"+owner+"', 'p1', 1)")
}

// A task's commits are found by the task named in their subject's last parenthesis, landed or on an agent's branch; a rebased copy
// shows once (the landed one when there is one), and T7 is never T71 or T70.
func TestATasksCommitsAreFoundByTheirSubjectOnMainAndOnAnyBranch(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	r := newChangesRepo(t, e)
	wb := teamtest.Key(t, e.Home, "wb")
	addTask(t, e, "7", wb)
	addTask(t, e, "8", wb)

	r.git(r.wt, "fetch", "-q", "origin")
	r.git(r.wt, "reset", "-q", "--hard", "origin/main")
	landed := r.commit(r.wt, "Landed part (T7)", "a.txt", "a\n")
	r.git(r.wt, "push", "-q", "origin", "HEAD:main")
	r.commit(r.wt, "Not this one (T71)", "x.txt", "x\n")
	r.commit(r.wt, "Nor this (T70) fixing T7 (see notes)", "y.txt", "y\n")
	both := r.commit(r.wt, "Shared work (T5, T7)", "b.txt", "b\n")
	// the lander's later copy of the landed commit and of the branch's last one: each shows once, the landed one kept
	later := []string{"GIT_COMMITTER_DATE=@3786825600 +0000"}
	r.git(r.repo, "checkout", "-q", "-b", "lander-t7", "origin/main~1")
	r.gitEnv(later, r.repo, "cherry-pick", "-x", landed)
	r.gitEnv(later, r.repo, "cherry-pick", both)
	copyOfBoth := r.git(r.repo, "rev-parse", "HEAD")

	got := changesOf(t, e, `{"task": 7}`)
	if s := subjects(got); len(s) != 2 || s[0] != "Landed part (T7)" || s[1] != "Shared work (T5, T7)" {
		t.Fatalf("the task's commits, once each, oldest first: %q", s)
	}
	cs := got["commits"].([]any)
	first, second := cs[0].(map[string]any), cs[1].(map[string]any)
	if first["sha"] != landed || first["on_main"] != true || first["agent"] != nil || first["repo"] != "proj" {
		t.Fatalf("the landed copy, not the lander's: %v", first)
	}
	if second["on_main"] != false || second["sha"] != copyOfBoth || second["agent"] != nil || second["files"] != float64(1) || second["added"] != float64(1) {
		t.Fatalf("the newest copy of the branch's commit, the lander's, which no agent's worktree has checked out: %v", second)
	}
	r.git(r.repo, "checkout", "-q", "main")
	r.git(r.repo, "branch", "-q", "-D", "lander-t7")
	if c := changesOf(t, e, `{"task": 7}`)["commits"].([]any)[1].(map[string]any); c["sha"] != both || c["agent"] != wb {
		t.Fatalf("wb's branch holds it: %v", c)
	}
	if !strings.Contains(got["diff"].(string), "+++ b/a.txt") || !strings.Contains(got["diff"].(string), "+++ b/b.txt") ||
		strings.Contains(got["diff"].(string), "x.txt") || strings.Contains(got["diff"].(string), "y.txt") {
		t.Fatalf("the whole task's diff is its commits' alone:\n%s", got["diff"])
	}
	if f := got["files"].([]any); len(f) != 2 || f[0].(map[string]any)["path"] != "a.txt" || f[1].(map[string]any)["path"] != "b.txt" {
		t.Fatalf("files in path order: %v", f)
	}

	one := changesOf(t, e, `{"task": 7, "commit": "`+landed[:10]+`"}`)
	if one["commit"] != landed || len(one["files"].([]any)) != 1 || strings.Contains(one["diff"].(string), "b.txt") || len(one["commits"].([]any)) != 2 {
		t.Fatalf("one commit's files, with every commit still listed: %v", one)
	}

	if none := changesOf(t, e, `{"task": 8}`); len(none["commits"].([]any)) != 0 || none["diff"] != "" || len(none["files"].([]any)) != 0 {
		t.Fatalf("a task with no commits: %v", none)
	}

	for p, code := range map[string]string{`{"task": "T7"}`: "bad_request", `{"task": 7, "commit": "zz"}`: "bad_request",
		`{"task": 999}`: "not_found", `{"task": 8, "commit": "` + landed + `"}`: "not_found"} {
		var params map[string]any
		d := json.NewDecoder(strings.NewReader(p))
		d.UseNumber()
		_ = d.Decode(&params)
		var re *rpc.Error
		if _, err := e.taskChanges(nil, params); !errors.As(err, &re) || re.Code != code {
			t.Fatalf("%s: %v", p, err)
		}
	}
}

// Commits that follow each other are one range, a file changed twice shown once with its final change; the diff stops whole files
// short of the cap and names the files left out.
func TestARunOfCommitsIsOneRangeAndTheDiffIsCappedByWholeFiles(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	r := newChangesRepo(t, e)
	wb := teamtest.Key(t, e.Home, "wb")
	addTask(t, e, "9", wb)
	r.commit(r.wt, "First (T9)", "f.txt", "one\n")
	r.commit(r.wt, "Second (T9)", "f.txt", "two\n", "big.txt", strings.Repeat("0123456789abcdef\n", maxChangesBytes/16), "z.txt", "z\n")

	got := changesOf(t, e, `{"task": 9}`)
	files := got["files"].([]any)
	if len(files) != 3 {
		t.Fatalf("three files: %v", files)
	}
	f := files[1].(map[string]any)
	if f["path"] != "f.txt" || f["added"] != float64(1) || f["deleted"] != float64(0) {
		t.Fatalf("f.txt from before the first to after the second: %v", f)
	}
	diff := got["diff"].(string)
	if !strings.Contains(diff, "+two") || strings.Contains(diff, "+one") || !strings.Contains(diff, "+z") {
		t.Fatalf("the range's diff:\n%.400s", diff)
	}
	if left := got["left_out"].([]any); got["truncated"] != true || len(left) != 1 || left[0] != "big.txt" || strings.Contains(diff, "big.txt") {
		t.Fatalf("big.txt left out whole: %v", left)
	}
	// a commit with no parent is diffed against the empty tree
	addTask(t, e, "10", wb)
	r.git(r.wt, "checkout", "-q", "--orphan", "fresh")
	r.git(r.wt, "rm", "-rqf", ".")
	r.commit(r.wt, "Root (T10)", "r.txt", "r\n")
	if root := changesOf(t, e, `{"task": 10}`); !strings.Contains(root["diff"].(string), "+++ b/r.txt") {
		t.Fatalf("a root commit's diff: %v", root)
	}
}

// A path with letters above ASCII reads the same in the files and in the diff's own lines, which name it without quotes.
func TestATasksDiffWritesANonASCIIPathAsItIs(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	r := newChangesRepo(t, e)
	addTask(t, e, "9", teamtest.Key(t, e.Home, "wb"))
	sha := r.commit(r.wt, "Menu (T9)", "café.txt", "one\n")
	for _, params := range []string{`{"task": 9}`, `{"task": 9, "commit": "` + sha + `"}`} {
		got := changesOf(t, e, params)
		files := got["files"].([]any)
		if len(files) != 1 || files[0].(map[string]any)["path"] != "café.txt" {
			t.Fatalf("%s: the file by its name: %v", params, files)
		}
		if diff := got["diff"].(string); !strings.Contains(diff, "diff --git a/café.txt b/café.txt\n") || !strings.Contains(diff, "+++ b/café.txt\n") {
			t.Fatalf("%s: the diff names it without quotes:\n%s", params, diff)
		}
	}
}

func TestTheTaskInASubjectIsItsLastParenthesisToken(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		subject string
		want    bool
	}{
		{"Fix (T7)", true}, {"Fix (T5, T7)", true}, {"Fix (T5,T7 )", true}, {"Fix (T71)", false}, {"Fix (T70)", false},
		{"Fix (T7) and more (notes)", false}, {"Fix T7", false}, {"Fix (T7", false}, {"Fix (XT7)", false},
	} {
		if got := gitrun.Names(c.subject, 7); got != c.want {
			t.Errorf("%q: %v", c.subject, got)
		}
	}
}
