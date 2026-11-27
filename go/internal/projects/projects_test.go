package projects

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
)

// A repo that fails on a runner is said once and not tried again at each link: only once its wait is over, which doubles each time.
func TestARepoThatFailsOnARunnerIsSaidOnceAndTriedAgainAfterAGrowingWait(t *testing.T) {
	d, err := store.Open(filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	folder := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", folder).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	exec.Command("git", "-C", folder, "remote", "add", "origin", "https://example.invalid/shop.git").Run()
	if _, err := d.Exec("INSERT INTO workspaces(id, name, folder, created_at) VALUES(1, 'shop', ?, 1)", folder); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var said []string
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	s := &Syncer{DB: d, Reg: nodes.New(d, t.TempDir()), RetryAfter: 10 * time.Minute, Stop: stop, Log: func(f string, a ...any) {
		mu.Lock()
		said = append(said, f)
		mu.Unlock()
	}, Clone: func(string, string) error { return rpc.Refused("repository not found", "git", "") }}
	tries := func() int {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.fails["n1"]["w1"].n
	}
	s.Node("n1")
	s.Wait()
	if tries() != 1 || s.state["n1"]["w1"].State != Failed {
		t.Fatalf("after the first try: %d tries, %v", tries(), s.state["n1"])
	}
	first := time.Until(s.fails["n1"]["w1"].next)
	for range 3 { // a runner that links again and again
		s.Node("n1")
		s.Wait()
	}
	if tries() != 1 {
		t.Fatalf("a link before the wait was over tried it again: %d tries", tries())
	}
	s.mu.Lock()
	s.fails["n1"]["w1"] = fail{n: 1} // its wait is over
	s.mu.Unlock()
	s.Node("n1")
	s.Wait()
	second := time.Until(s.fails["n1"]["w1"].next)
	if tries() != 2 || second < first+9*time.Minute {
		t.Fatalf("the second try: %d tries, waits %s then %s", tries(), first, second)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(said) != 1 || !strings.Contains(said[0], "tried again after") {
		t.Fatalf("said: %q", said)
	}
}
