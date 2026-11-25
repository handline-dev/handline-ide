package daemon

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/internal/boardcopy"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/devices"
)

// The owner's runner mirrors the board's copy and restores it to a board a second or two behind, which node.status shows; another
// person's runner is refused and keeps no copy.
func TestTheOwnersRunnerKeepsACopyOfTheBoardAndAnotherPersonsNone(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	t.Setenv("HANDLINE_AGENT", "")
	t.Setenv("HANDLINE_TMUX_SOCKET", "")
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth, cfg.BoardCopy = fakeAuth{key: "k", agent: "a"}, true
	master, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(master.Close)
	start := func(home string, m joined.Master) *runner.Runner {
		t.Helper()
		tm := agenthost.Tmux(home).(runner.Terminals)
		r, err := runner.Start(runner.Config{Home: home, Socket: paths.SocketFile(home), Sha: master.nodes.Version, Master: m, Term: tm, Log: t.Logf})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(r.Close)
		return r
	}
	home, node, m := joinRunner(t, master)
	owners := start(home, m)
	linkedRunner(t, master, node)

	// Several MB, so the snapshot is more than one frame of the link (rpc.MaxLineBytes): it has to go in pieces.
	for i := 0; i < 2000; i++ {
		if _, err := master.db.Exec("INSERT INTO meta(name, value, updated) VALUES(?, randomblob(3000), 0)", fmt.Sprintf("copied-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	var want int
	master.db.QueryRow("SELECT COUNT(*) FROM meta").Scan(&want)
	restored := func() int {
		if err := owners.Pull(); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(t.TempDir(), "board.sqlite")
		if boardcopy.Restore(boardcopy.Dir(home), dest) != nil {
			return -1
		}
		b, err := sql.Open("sqlite", dest)
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()
		n := -1
		b.QueryRow("SELECT COUNT(*) FROM meta").Scan(&n)
		return n
	}
	began := time.Now()
	for n := restored(); n != want; n = restored() {
		if time.Since(began) > 10*time.Second {
			t.Fatalf("the runner's copy restores %d meta rows, the master has %d", n, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("the copy caught up in %s", time.Since(began).Round(100*time.Millisecond))
	var got map[string]any
	var ok bool
	if err := owners.Pull(); err != nil { // the pull that finds it holds everything is what says it is current
		t.Fatal(err)
	}
	got, ok = master.nodes.CopyOf(node).(map[string]any)
	if !ok || got["behind_s"].(int64) > 2 {
		t.Fatalf("node.status copy: %v", master.nodes.CopyOf(node))
	}

	if _, err := master.db.Exec("INSERT INTO people(id, display_name, role, created_at) VALUES(2, 'Kim', 'member', 0)"); err != nil {
		t.Fatal(err)
	}
	kimHome, err := os.MkdirTemp("", "hlr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(kimHome) })
	kimNode, _ := joined.NodeID(kimHome, nil)
	door, _ := master.dev.Doors()
	code, _ := master.dev.PairAt(time.Minute, door, "p2")
	uri := devices.WebLink(door, code)
	km, err := hlclient.Join(kimHome, uri, "kim-box", "server", kimNode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { joined.Forget(kimHome) })
	os.MkdirAll(boardcopy.Dir(kimHome), 0o700) // as if it had held one
	kims := start(kimHome, km)
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) { // on Off it takes no work: only its link counts
		if rn, _ := master.nodes.Runner(kimNode); rn != nil {
			break
		}
		if time.Now().After(end) {
			t.Fatal("Kim's runner never linked")
		}
	}
	if err := kims.Pull(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(boardcopy.Dir(kimHome)); !os.IsNotExist(err) {
		t.Fatalf("Kim's runner keeps a copy: %v", err)
	}
	if c := master.nodes.CopyOf(kimNode); c != nil {
		t.Fatalf("node.status shows a copy on Kim's runner: %v", c)
	}
}
