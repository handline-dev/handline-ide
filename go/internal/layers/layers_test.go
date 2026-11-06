// Package layers holds the layering of the internal packages as a test: a package imports only from its own layer or a lower one, and every
// package has a place in the table. A new package goes in the lowest layer its imports allow.
package layers

import (
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePrefix = "github.com/handline/handline/go/internal/"

// layered are the modules beside internal/ whose packages keep their place in the layers, as <module>/<name>: the daemon's records,
// accounts, task board and scheduler sit among its own packages.
var layered = []string{"data/", "accounts/", "taskboard/", "scheduler/"}

const goPrefix = "github.com/handline/handline/go/"

// layer is each internal package's layer. Within a layer Go already forbids a cycle; across layers an import goes down only.
var layer = map[string]int{
	// L0 leaf: no internal imports
	"claudetrust":            0,
	"contracttest":           0,
	"data/rowstate":          0,
	"devinstance":            0,
	"fixtureplay":            0,
	"layers":                 0,
	"outcome":                0,
	"status":                 0,
	"scheduler/staymove":     0,
	"transcript":             0,
	"scheduler/worktreelock": 0,
	// L1 wire and infra
	"data/agenthost":     1,
	"accounts/claudecli": 1,
	"debugterm":          1,
	"hooks":              1,
	// L2 db
	"data/db":           2,
	"cmdruns":           2,
	"filerw":            2,
	"hlclient":          2,
	"inputhold":         2,
	"taskboard/msgtest": 2,
	// L3 model
	"data/states":             3,
	"data/stream":             3,
	"data/team":               3,
	"taskboard/attachments":   3,
	"scheduler/cronspec":      3,
	"scheduler/dangling":      3,
	"fold":                    3,
	"taskboard/handoff":       3,
	"heal":                    3,
	"housekeeping":            3,
	"scheduler/keepawake":     3,
	"accounts/livetest":       3,
	"scheduler/leases":        3,
	"scheduler/loadgate":      3,
	"scheduler/pause":         3,
	"scheduler/procload":      3,
	"prefs":                   3,
	"push":                    3,
	"taskboard/requestref":    3,
	"scheduler/session":       3,
	"scheduler/settings":      3,
	"accounts/subscriptions":  3,
	"scheduler/sysres":        3,
	"taskboard/taskresult":    3,
	"termchan":                3,
	"accounts/usage":          3,
	"accounts/usagepoll":      3,
	"taskboard/watch":         3,
	"accounts/weblogin":       3,
	"scheduler/worktreepaths": 3,
	"scheduler/access":        3,
	"worktreeprune":           3,
	// L4 domain writes and reads
	"adminwrites":          4,
	"chatwrites":           4,
	"pausewrites":          4,
	"proposalwrites":       4,
	"reads":                4,
	"taskboard/supervisor": 4,
	"taskboard/taskwrites": 4,
	// L6 runners
	"agentview":           6,
	"gates":               6,
	"scheduler/lifecycle": 6,
	"prompts":             6,
	"scheduler/schedules": 6,
	"sessionrpc":          6,
	"scheduler/tick":      6,
	"watcher":             6,
	// L7 composition
	"compose": 7,
	"daemon":  7,
}

// packages is every package with Go files under internal/, by its path there, and in each layered module, as <module>/<path>.
func packages(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, under := range append([]string{""}, layered...) {
		walkPackages(t, under, out)
	}
	return out
}

func walkPackages(t *testing.T, under string, out map[string][]string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if under != "" {
		root, err = filepath.Abs(filepath.Join("..", "..", under))
	}
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if d.Name() == "testdata" {
			return filepath.SkipDir
		}
		pkg, err := build.ImportDir(path, 0)
		if err != nil {
			if _, none := err.(*build.NoGoError); none {
				return nil
			}
			return err
		}
		rel, _ := filepath.Rel(root, path)
		var imports []string
		for _, imp := range pkg.Imports { // the package's own files, not its tests
			if p, ok := strings.CutPrefix(imp, modulePrefix); ok {
				imports = append(imports, p)
			} else if p, ok := strings.CutPrefix(imp, goPrefix); ok && slices.ContainsFunc(layered, func(m string) bool { return strings.HasPrefix(p, m) }) {
				imports = append(imports, p)
			}
		}
		out[under+filepath.ToSlash(rel)] = imports
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPackagesImportOnlyDownTheLayers(t *testing.T) {
	t.Parallel()
	pkgs := packages(t)
	var bad []string
	for name, imports := range pkgs {
		own, ok := layer[name]
		if !ok {
			bad = append(bad, name+" is not in the layer table: add it to the lowest layer its imports allow")
			continue
		}
		for _, imp := range imports {
			up, ok := layer[imp]
			if ok && up > own {
				bad = append(bad, fmt.Sprintf("%s (L%d) imports %s (L%d): an import goes down the layers only", name, own, imp, up))
			}
		}
	}
	for name := range layer {
		if _, ok := pkgs[name]; !ok {
			bad = append(bad, name+" is in the layer table but is not a package")
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("\n%s", strings.Join(bad, "\n"))
	}
}

func TestTheWalkSeesThePackages(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat(filepath.Join("..", "reads")); err != nil {
		t.Fatal(err)
	}
	if len(packages(t)) < 50 {
		t.Fatal("the walk found too few packages")
	}
}

// hlMayNotImport are what cmd/hl, the CLI, reaches the daemon without: it talks to it through RPC and opens no database.
var hlMayNotImport = []string{"github.com/handline/handline/go/data/db", "github.com/handline/handline/go/kit/store", "github.com/handline/handline/go/scheduler/lifecycle", modulePrefix + "compose",
	"database/sql", "modernc.org/sqlite"}

func TestHLTalksToTheDaemonOnlyThroughRPC(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", "cmd", "hl")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no files in cmd/hl: %v", err)
	}
	var bad []string
	for _, f := range files {
		name := filepath.Base(f)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			if !slices.Contains(hlMayNotImport, path) {
				continue
			}
			bad = append(bad, fmt.Sprintf("cmd/hl/%s imports %s: the CLI reaches the daemon through RPC only", name, path))
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("\n%s", strings.Join(bad, "\n"))
	}
}

// A package that starts sessions reaches the agent CLI through agent.Agent: none of the code that is one agent CLI's own, or that talks to
// one's session, is among what lifecycle imports, however far down, in any module.
var lifecycleMayNotReach = []string{"accounts/claudecli", "agentctl/peerinbox", "agentctl/peerport", "agentctl/agent/claudecode/turns",
	"agentctl/agent/claudecode"}

func TestLifecycleReachesNoAgentImplementation(t *testing.T) {
	t.Parallel()
	out, err := exec.Command("go", "list", "-deps", "github.com/handline/handline/go/scheduler/lifecycle").Output()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range strings.Fields(string(out)) {
		seen[strings.TrimPrefix(p, "github.com/handline/handline/go/")] = true
	}
	if !seen["agentctl/agent"] {
		t.Fatal("lifecycle does not reach agentctl/agent: the walk saw nothing")
	}
	for _, p := range lifecycleMayNotReach {
		if seen[p] {
			t.Errorf("lifecycle reaches %s: ask for it through agent.Agent", p)
		}
	}
}

// detailTested are the places that still decide on the words of an agent's state detail (path under go/), each a row of its own to move
// to a column. None is left.
var detailTested = map[string]string{}

// detailTest is a string test on a state's detail in one line: strings.Contains(x.Detail..., strings.HasPrefix(x.Detail...
var detailTest = regexp.MustCompile(`strings\.(Contains|HasPrefix|HasSuffix|Index)\([^)]*Detail`)

func TestNoDecisionReadsTheWordsOfAStateDetail(t *testing.T) {
	t.Parallel()
	var bad []string
	root := filepath.Join("..", "..") // go/: every module
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if _, ok := detailTested[rel]; ok {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, ln := range strings.Split(string(src), "\n") {
			if detailTest.MatchString(ln) {
				bad = append(bad, fmt.Sprintf("%s:%d tests the words of a detail: keep the fact in a column (stall_kind) and read that", rel, i+1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("\n%s", strings.Join(bad, "\n"))
	}
}
