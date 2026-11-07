package reads

import (
	"bufio"
	"context"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/protocol/rpc"
)

const todosMax = 999

// The read's budgets; a test shortens them.
var factsBudget, issuesBudget = 4 * time.Second, 3 * time.Second

func (e *Env) repoFacts(_ *rpc.Conn, params map[string]any) (any, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return nil, badParam("path is a git folder", "path")
	}
	if _, err := os.Lstat(filepath.Join(path, ".git")); err != nil || !filepath.IsAbs(path) {
		nf := rpc.NotFoundError(path + " is not a git folder")
		nf.Data = map[string]any{"what": "path", "path": path}
		return nil, nf
	}
	ctx, cancel := context.WithTimeout(context.Background(), factsBudget)
	defer cancel()
	issues := e.Issues
	if issues == nil {
		issues = ghIssues
	}
	out := map[string]any{"test_command": nil, "issues": nil, "todos": 0, "uncommitted": 0}
	var mu sync.Mutex
	set := func(k string, v any) { mu.Lock(); out[k] = v; mu.Unlock() }
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		set("test_command", testCommand(path))
	}()
	go func() {
		defer wg.Done()
		ictx, icancel := context.WithTimeout(ctx, issuesBudget)
		defer icancel()
		if list, err := issues(ictx, path); err == nil {
			set("issues", list)
		}
	}()
	go func() {
		defer wg.Done()
		set("todos", todoCount(ctx, path))
	}()
	go func() {
		defer wg.Done()
		set("uncommitted", uncommittedCount(ctx, path))
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done(): // a part that does not stop when told is left behind
	}
	mu.Lock()
	defer mu.Unlock()
	return maps.Clone(out), nil
}

// testCommand is the command that runs the folder's tests, by the files at its top: package.json with a test script (by lockfile),
// go.mod, a pytest or Python project file, a Makefile with a test target. nil when none.
func testCommand(dir string) any {
	has := func(name string) bool { _, err := os.Stat(filepath.Join(dir, name)); return err == nil }
	if npmTestScript(filepath.Join(dir, "package.json")) {
		switch {
		case has("pnpm-lock.yaml"):
			return "pnpm test"
		case has("yarn.lock"):
			return "yarn test"
		case has("bun.lock"), has("bun.lockb"):
			return "bun test"
		}
		return "npm test"
	}
	if has("go.mod") {
		return "go test ./..."
	}
	for _, n := range []string{"pytest.ini", "conftest.py", "pyproject.toml", "setup.py", "setup.cfg", "tox.ini"} {
		if has(n) {
			return "pytest"
		}
	}
	for _, n := range []string{"GNUmakefile", "makefile", "Makefile"} {
		if makeTestTarget(filepath.Join(dir, n)) {
			return "make test"
		}
	}
	return nil
}

// npmInitTest is the test script npm init writes, which only fails.
const npmInitTest = `echo "Error: no test specified" && exit 1`

func npmTestScript(file string) bool {
	b, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return false
	}
	t := strings.TrimSpace(pkg.Scripts["test"])
	return t != "" && t != npmInitTest
}

var makeTest = regexp.MustCompile(`^test\s*::?(\s|$)`)

func makeTestTarget(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if makeTest.MatchString(sc.Text()) {
			return true
		}
	}
	return false
}

// todoCount is how many lines of tracked text files hold the word TODO or FIXME, at most todosMax; 0 when git does not answer in time.
func todoCount(ctx context.Context, dir string) int {
	r := gitUntil(ctx, dir, "grep", "-I", "-c", "-w", "-e", "TODO", "-e", "FIXME")
	n := 0
	for _, line := range strings.Split(r.Stdout, "\n") {
		if i := strings.LastIndexByte(line, ':'); i >= 0 {
			c, _ := strconv.Atoi(line[i+1:])
			n += c
		}
	}
	return min(n, todosMax)
}

// uncommittedCount is how many files git status lists, untracked ones included; 0 when git does not answer in time.
func uncommittedCount(ctx context.Context, dir string) int {
	r := gitUntil(ctx, dir, "status", "--porcelain")
	if r.RC != 0 {
		return 0
	}
	return strings.Count(r.Stdout, "\n")
}

func gitUntil(ctx context.Context, dir string, args ...string) gitrun.Result {
	d, _ := ctx.Deadline()
	left := time.Until(d)
	if left <= 0 {
		return gitrun.Result{RC: -1}
	}
	return gitrun.RunTimeout(dir, left, append([]string{"--no-optional-locks"}, args...)...)
}

// ghIssues is the two newest open issues of the folder's GitHub repo, as gh finds it from the folder's remote.
func ghIssues(ctx context.Context, dir string) ([]any, error) {
	cmd := exec.CommandContext(ctx, "gh", "issue", "list", "--state", "open", "--limit", "2", "--json", "number,title")
	cmd.Dir = dir
	cmd.WaitDelay = 100 * time.Millisecond // a helper gh started may hold its output open after the kill
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	out := []any{}
	for _, r := range rows {
		out = append(out, map[string]any{"number": r.Number, "title": r.Title})
	}
	return out, nil
}
