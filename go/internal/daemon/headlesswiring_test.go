package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The reads every server serves tell how an open session runs now (AgentView.session_mode_now): an agent with no Chat holder here is not
// headless, whatever its terminal.
func TestTheReadsAreGivenTheHeadlessCheck(t *testing.T) {
	dir, err := os.MkdirTemp("", "hl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), SocketFile: filepath.Join(dir, "d.sock"),
		Auth: fakeAuth{key: "k"}, Log: t.Logf})
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("this environment will not bind a socket: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	h := d.readsEnv().Headless
	if h == nil {
		t.Fatal("agent rows would leave session_mode_now out")
	}
	if h("bob", "w1") {
		t.Fatal("headless with no holder")
	}
}
