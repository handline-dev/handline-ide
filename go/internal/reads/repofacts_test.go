package reads

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTestCommandIsReadFromTheFilesAtTheTop(t *testing.T) {
	script := `{"scripts":{"test":"vitest"}}`
	for _, c := range []struct {
		files map[string]string
		want  any
	}{
		{map[string]string{"package.json": script, "pnpm-lock.yaml": ""}, "pnpm test"},
		{map[string]string{"package.json": script, "yarn.lock": ""}, "yarn test"},
		{map[string]string{"package.json": script, "bun.lockb": ""}, "bun test"},
		{map[string]string{"package.json": script, "bun.lock": ""}, "bun test"},
		{map[string]string{"package.json": script, "package-lock.json": "", "go.mod": ""}, "npm test"},
		{map[string]string{"package.json": `{"scripts":{"test":"echo \"Error: no test specified\" && exit 1"}}`, "go.mod": ""}, "go test ./..."},
		{map[string]string{"package.json": `{"scripts":{"build":"tsc"}}`}, nil},
		{map[string]string{"pyproject.toml": "", "Makefile": "test:\n\tpytest\n"}, "pytest"},
		{map[string]string{"Makefile": "build:\n\tcc\ntest: build\n\t./t\n"}, "make test"},
		{map[string]string{"Makefile": "build:\n\tcc\ntests:\n\t./t\n"}, nil},
		{map[string]string{"README.md": "hi"}, nil},
	} {
		dir := t.TempDir()
		writeFiles(t, dir, c.files)
		if got := testCommand(dir); got != c.want {
			t.Errorf("%v: test_command %v, want %v", c.files, got, c.want)
		}
	}
}

func TestRepoFactsCountsTodosAndUncommittedFilesAndAsksForIssues(t *testing.T) {
	e := testEnv(t)
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	git := gitIn(t)
	git(dir, "init", "-q", "-b", "main")
	writeFiles(t, dir, map[string]string{
		"go.mod":   "module x\n",
		"a.go":     "// TODO one\n// FIXME two\n// TODOS is not the word\n",
		"b/c.go":   "x := 1 // TODO three\n",
		"logo.bin": "\x00TODO\x00",
	})
	git(dir, "add", "-A")
	git(dir, "commit", "-q", "-m", "start")
	writeFiles(t, dir, map[string]string{"go.mod": "module y\n", "new.txt": "TODO untracked, not counted\n"})
	asked := ""
	e.Issues = func(_ context.Context, d string) ([]any, error) {
		asked = d
		return []any{map[string]any{"number": 7, "title": "Crash"}}, nil
	}

	res, err := e.repoFacts(nil, map[string]any{"path": dir})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"test_command": "go test ./...", "issues": []any{map[string]any{"number": 7, "title": "Crash"}}, "todos": 3, "uncommitted": 2}
	if !reflect.DeepEqual(res, want) || asked != dir {
		t.Fatalf("repo.facts %v (issues asked in %q), want %v", res, asked, want)
	}
}

func TestRepoFactsGivesWhatIsDoneByItsBudget(t *testing.T) {
	e := testEnv(t)
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	gitIn(t)(dir, "init", "-q", "-b", "main")
	var left time.Duration
	e.Issues = func(ctx context.Context, _ string) ([]any, error) {
		d, _ := ctx.Deadline()
		left = time.Until(d)
		return nil, errors.New("gh: no GitHub remote")
	}
	res, err := e.repoFacts(nil, map[string]any{"path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["issues"] != nil {
		t.Fatalf("issues %v when gh failed, want null", res.(map[string]any)["issues"])
	}
	if left <= 2*time.Second || left > issuesBudget {
		t.Fatalf("gh was given %v, want its 3 s cap", left)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	if n := todoCount(ctx, dir); n != 0 {
		t.Fatalf("todos %d past the budget, want 0", n)
	}
}

func TestRepoFactsRefusesWhatIsNotAGitFolder(t *testing.T) {
	e := testEnv(t)
	plain := t.TempDir()
	for _, p := range []string{plain, filepath.Join(plain, "gone"), "relative/path"} {
		_, err := e.repoFacts(nil, map[string]any{"path": p})
		var re *rpc.Error
		if !errors.As(err, &re) || re.Data["what"] != "path" {
			t.Errorf("%s: %v, want not_found what path", p, err)
		}
	}
	if _, err := e.repoFacts(nil, map[string]any{}); err == nil {
		t.Error("no path was not refused")
	}
}

func TestRepoFactsReturnsAtItsBudgetWhenAPartDoesNotStop(t *testing.T) {
	e := testEnv(t)
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	gitIn(t)(dir, "init", "-q", "-b", "main")
	writeFiles(t, dir, map[string]string{"go.mod": "module x\n"})
	was, wasIssues := factsBudget, issuesBudget
	factsBudget, issuesBudget = 300*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { factsBudget, issuesBudget = was, wasIssues })
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	e.Issues = func(context.Context, string) ([]any, error) { <-release; return nil, nil } // ignores its context

	start := time.Now()
	res, err := e.repoFacts(nil, map[string]any{"path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the read took %v past a 300 ms budget", took)
	}
	m := res.(map[string]any)
	if m["issues"] != nil || m["test_command"] != "go test ./..." {
		t.Fatalf("repo.facts %v, want the parts done and issues null", m)
	}
}
