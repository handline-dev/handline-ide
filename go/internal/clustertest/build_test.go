package clustertest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/cluster"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/joined"
)

// logSays is whether n's log has s.
func logSays(n *cluster.Node, s string) bool {
	b, _ := os.ReadFile(n.Log())
	return strings.Contains(string(b), s)
}

// logSize is how long n's log is now.
func logSize(n *cluster.Node) int64 {
	fi, _ := os.Stat(n.Log())
	if fi == nil {
		return 0
	}
	return fi.Size()
}

// linkedAgain is whether n's log, past from, shows its daemon starting again and then linking. Taking a build starts again by exec,
// which keeps the pid, so the executable changes before the new build writes the hooks.
func linkedAgain(n *cluster.Node, from int64) bool {
	b, _ := os.ReadFile(n.Log())
	if int64(len(b)) < from {
		return false
	}
	s := string(b[from:])
	i := strings.LastIndex(s, "handlined: starting again as what this machine is now")
	return i >= 0 && strings.Contains(s[i:], "runner: linked to the master at")
}

// runsOn is whether n's daemon runs the executable at path.
func runsOn(n *cluster.Node, path string) bool {
	a, errA := os.Stat(n.Exe())
	b, errB := os.Stat(path)
	return errA == nil && errB == nil && os.SameFile(a, b)
}

// A runner on another build than the master's: on Manual it fetches nothing until its Update (device.update), then takes the master's
// build and starts again on it; started again on its old build, it starts on the kept one; on Auto a runner on another build takes it
// by itself.
func TestALentMacRunsTheMastersBuild(t *testing.T) {
	c := Start(t, Opts{})
	r := c.Runners[0]
	kept := filepath.Join(runner.BuildDir(r.Home), "handlined")
	if _, err := c.Master.Call("settings.set", map[string]any{"changes": map[string]any{"node_build_auto": false}, "request_id": "manual"}); err != nil {
		t.Fatal(err)
	}
	r.RunOn(BuildAs(t, "older1"))
	if err := r.Restart(); err != nil {
		t.Fatal(err)
	}
	restarted := time.Now() // a last_seen after it is of the older build's process: it is kept to the second
	node := joined.NodeIDOf(r.Home)
	waitFor(t, "a heartbeat answered on Manual", func() bool { // the answer that would name the build
		rw := row(c.Master, node)
		seen, _ := rw["last_seen"].(string)
		at, err := time.Parse(time.RFC3339, seen)
		return err == nil && at.After(restarted)
	})
	if logSays(r, "taking its build") {
		t.Fatal("Manual fetched the master's build")
	}
	m, _ := r.Joined()
	from := logSize(r)
	if _, err := c.Master.Call("device.update", map[string]any{"device": m.Device, "request_id": "update"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the runner on the master's build", func() bool { return runsOn(r, kept) && linkedAgain(r, from) })
	if b, _ := os.ReadFile(filepath.Join(runner.HooksDir(r.Home), "pre-push")); !strings.Contains(string(b), filepath.Join(runner.BuildDir(r.Home), "hl")) {
		t.Fatalf("its hooks call another hl: %s", b)
	}
	if err := r.Restart(); err != nil { // the app starts it on its own, older build
		t.Fatal(err)
	}
	waitFor(t, "the runner started on the kept build", func() bool { return runsOn(r, kept) })
	if _, err := c.Master.Call("settings.set", map[string]any{"changes": map[string]any{"node_build_auto": true}, "request_id": "auto"}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(runner.BuildDir(r.Home)); err != nil {
		t.Fatal(err)
	}
	if err := r.Restart(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Auto: the runner on the master's build by itself", func() bool { return runsOn(r, kept) })
}
