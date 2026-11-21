package layers

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The master reaches the terminals agents run in through term.Machines, which sends a runner's handle to that runner. These are the
// calls that reach this machine's terminal host around it, and why each may: file (under go/) and a fragment of the line.
var hostAllowed = []struct{ file, line, why string }{
	{"cmd/handlined/main.go", "agenthost.Current(home).(runner.Terminals)", "a runner's own host, which the master drives"},
	{"cmd/hl/launch.go", "agenthost.Current(c.Home)", "hl launch runs in the terminal it launches in"},
	{"internal/termchan/rpc.go", `[]string{"tmux", "-C"}`, "this machine's control client: a runner's bytes are relayed over its link"},
	{"internal/daemon/daemon.go", "local = agenthost.Current(cfg.Home)", "the Local of the terminal channel's term.Machines"},
	{"internal/daemon/daemon.go", "host = agenthost.Current(d.cfg.Home)", "the pane processes this machine's load is measured by"},
	{"internal/compose/lifecycle.go", "agenthost.Tmux(home).(term.PaneAgenter)", "the pane an hl launch on this machine runs in"},
	{"internal/compose/termport.go", "&term.Machines{Local: agenthost.Dynamic(home)", "the Term the lifecycle drives"},
	{"internal/daemon/daemon.go", "&term.Machines{Local: agenthost.Dynamic(d.cfg.Home)", "the Term the typing methods drive"},
	{"internal/cluster/cluster.go", `exec.Command("tmux", "-L", n.tmux(), "kill-server")`, "the test rig ends its nodes' tmux servers"},
}

var hostCall = regexp.MustCompile(`exec\.Command(Context)?\([^)]*"tmux"|\[\]string\{"tmux"|agenthost\.(Tmux|Current|Dynamic)\(`)

func TestTerminalsAreReachedThroughTheMachineSeam(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == "testdata" || rel == "agentctl/term/tmux" || rel == "data/agenthost") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for n, line := range strings.Split(string(b), "\n") {
			if !hostCall.MatchString(line) || strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			allowed := false
			for _, a := range hostAllowed {
				allowed = allowed || (a.file == rel && strings.Contains(line, a.line))
			}
			if !allowed {
				t.Errorf("%s:%d reaches the terminal host around term.Machines, so a runner's terminal would miss it: go through the "+
					"Term the daemon builds; if it is this machine's alone, add it to hostAllowed with why\n\t%s", rel, n+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
