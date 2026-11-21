// Package clustertest is cluster.Start for a test: handlined and hl built from this tree once per test process, homes in a short
// temp folder, every process stopped and the homes removed when the test ends, the daemons' logs printed when it failed.
package clustertest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/handline/handline/go/internal/cluster"
)

// Opts are Start's choices: how many runners (0 is 1) and more handlined flags for every node.
type Opts struct {
	Runners int
	Flags   []string
	Env     []string
}

var built struct {
	once sync.Once
	dir  string
	err  error
}

// Build is the folder of the handlined and hl built from this tree, once per test process.
func Build(t testing.TB) string {
	t.Helper()
	built.once.Do(func() {
		mod, err := exec.Command("go", "env", "GOMOD").Output() // the daemon's module: internal reaches no other
		if err != nil {
			built.err = fmt.Errorf("go env GOMOD: %w", err)
			return
		}
		if built.dir, built.err = os.MkdirTemp("", "hlbin"); built.err != nil {
			return
		}
		cmd := exec.Command("go", "build", "-o", built.dir+"/", "./cmd/handlined", "./cmd/hl")
		cmd.Dir = filepath.Dir(strings.TrimSpace(string(mod)))
		if out, err := cmd.CombinedOutput(); err != nil {
			built.err = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if built.err != nil {
		t.Fatal(built.err)
	}
	return built.dir
}

// Remove removes what Build made; a package's TestMain calls it after its tests.
func Remove() {
	if built.dir != "" {
		os.RemoveAll(built.dir)
	}
}

// Start is a master and o.Runners runners, each joined and on Run.
func Start(t testing.TB, o Opts) *cluster.Cluster {
	t.Helper()
	bin := Build(t)
	root, err := os.MkdirTemp("", "hlc") // not t.TempDir(): a test's name can take a socket path past its 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	c, err := cluster.Start(cluster.Opts{Bin: bin, Root: root, Runners: o.Runners, Flags: o.Flags, Env: o.Env})
	t.Cleanup(func() {
		if c != nil {
			if err := c.Stop(); err != nil {
				t.Error(err)
			}
		}
		if t.Failed() {
			logs(t, root)
		}
		os.RemoveAll(root)
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func logs(t testing.TB, root string) {
	found, _ := filepath.Glob(filepath.Join(root, "*", "handlined.log"))
	for _, f := range found {
		if b, err := os.ReadFile(f); err == nil {
			t.Logf("--- %s\n%s", f, b)
		}
	}
}
