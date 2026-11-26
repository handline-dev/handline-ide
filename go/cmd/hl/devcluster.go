package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/handline/handline/go/internal/cluster"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
)

const clusterUsage = `usage: hl dev cluster up [--runners N] [--worktree PATH]
       hl dev cluster down
       hl dev cluster status`

// clusterPort is the master's port; runner i listens on clusterPort+i.
const clusterPort = 7461

// dev is `hl dev cluster`; anything else is its usage.
func dev(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 0 || args[0] != "cluster" {
		fmt.Fprintln(os.Stderr, clusterUsage)
		return 2, true
	}
	return runCluster(c.Home, args[1:], os.Stdout, os.Stderr), true
}

// runCluster is `hl dev cluster`: a master and runners on this Mac (internal/cluster), built from a worktree (the checkout here by
// default), with homes under ~/.handline-dev/cluster. up starts them afresh and joins each runner, down stops them and keeps the homes
// for their logs, status says what runs.
func runCluster(liveHome string, args []string, out, errOut io.Writer) int {
	p, ok := parse(args, []string{"--runners", "--worktree"}, nil, nil)
	if !ok || len(p.pos) != 1 {
		fmt.Fprintln(errOut, clusterUsage)
		return 2
	}
	o := cluster.Opts{Root: filepath.Join(paths.UserHome(), ".handline-dev", "cluster"), BasePort: clusterPort, Detach: true}
	o.Bin = filepath.Join(o.Root, "bin")
	_, runners := p.opt("--runners")
	_, wt := p.opt("--worktree")
	if p.pos[0] != "up" && (runners || wt) {
		fmt.Fprintln(errOut, clusterUsage)
		return 2
	}
	var err error
	switch p.pos[0] {
	case "up":
		err = clusterUp(liveHome, o, p, out)
	case "down":
		err = cluster.Open(o).Stop()
		if err == nil {
			fmt.Fprintln(out, "the cluster is stopped; its homes stay under", o.Root)
		}
	case "status":
		clusterStatus(cluster.Open(o), o.Bin, out)
	default:
		fmt.Fprintln(errOut, clusterUsage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	return 0
}

func clusterUp(liveHome string, o cluster.Opts, p parsed, out io.Writer) error {
	if overlaps(o.Root, liveHome) {
		return fmt.Errorf("%s overlaps the live home %s", o.Root, liveHome)
	}
	for _, n := range cluster.Open(o).Nodes() {
		if n != nil && n.Running() {
			return fmt.Errorf("the cluster runs (%s): `hl dev cluster down` first", n.Name)
		}
	}
	if v, ok := p.opt("--runners"); ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 9 {
			return fmt.Errorf("--runners is 1 to 9, not %q", v)
		}
		o.Runners = n
	}
	wt, _ := p.opt("--worktree")
	if wt == "" {
		top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			return fmt.Errorf("not in a checkout: give --worktree")
		}
		wt = strings.TrimSpace(string(top))
	}
	if err := os.RemoveAll(o.Root); err != nil {
		return err
	}
	if err := os.MkdirAll(o.Bin, 0o755); err != nil {
		return err
	}
	build := exec.Command("go", "build", "-o", o.Bin+"/", "./cmd/handlined", "./cmd/hl")
	build.Dir = filepath.Join(wt, "go")
	if b, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("go build in %s: %v\n%s", build.Dir, err, b)
	}
	c, err := cluster.Start(o)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "built from %s; each runner joined and on Run\n", wt)
	clusterStatus(c, o.Bin, out)
	return nil
}

func clusterStatus(c *cluster.Cluster, bin string, out io.Writer) {
	if c.Master == nil {
		fmt.Fprintln(out, "no cluster: `hl dev cluster up`")
		return
	}
	for _, n := range c.Nodes() {
		state := "stopped"
		if n.Running() {
			state = "running"
			if n != c.Master && c.Master.Running() && c.Master.Lists(joined.NodeIDOf(n.Home)) {
				state = "running, listed by the master"
			}
		}
		fmt.Fprintf(out, "%-8s %s  %s  %s\n  env -u HANDLINE_AGENT HANDLINE_HOME=%s HANDLINE_TEST_ROOT=%s %s\n", n.Name, n.Addr, state,
			n.Home, n.Home, n.UserHome(), filepath.Join(bin, "hl"))
	}
}

// resolved is p absolute with links followed, through its nearest ancestor that exists: a home not made yet still
// compares by where it will be.
func resolved(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		p = a
	}
	for rest := ""; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest, p = filepath.Join(filepath.Base(p), rest), parent
	}
}

// within is whether p is dir or inside it.
func within(p, dir string) bool {
	p, dir = resolved(p), resolved(dir)
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// overlaps is whether a and b are one folder or one holds the other.
func overlaps(a, b string) bool { return within(a, b) || within(b, a) }
