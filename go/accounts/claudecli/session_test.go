package claudecli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/testkill"
)

// TestMain makes this test binary the fake Claude CLI when FAKE_CLAUDE is set: it answers initialize and get_usage, runs a turn that
// ends at once for a user message, and refuses nothing else.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_CLAUDE") == "1" {
		fakeClaude()
		return
	}
	os.Exit(m.Run())
}

func fakeClaude() {
	testkill.ExitWithParent()
	out := json.NewEncoder(os.Stdout)
	var mu sync.Mutex
	emit := func(v any) { mu.Lock(); out.Encode(v); mu.Unlock() }
	if os.Getenv("FAKE_DEAF") == "1" { // a CLI that ignores the end of stdin and leaves a child behind
		exec.Command("sleep", "60").Start()
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m map[string]any
		json.Unmarshal(sc.Bytes(), &m)
		switch m["type"] {
		case "control_request":
			body := map[string]any{}
			if m["request"].(map[string]any)["subtype"] == "get_usage" {
				body = map[string]any{"rate_limits_available": true}
			}
			emit(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": m["request_id"], "response": body}})
		case "user":
			emit(map[string]any{"type": "control_request", "request_id": "perm1", "request": map[string]any{"subtype": "can_use_tool"}})
			text := m["message"].(map[string]any)["content"].(string)
			emit(map[string]any{"type": "result", "result": "heard " + text})
		}
	}
	if os.Getenv("FAKE_DEAF") == "1" {
		select {}
	}
}

func launch(t *testing.T) Launch {
	t.Helper()
	exe, _ := os.Executable()
	return Launch{Path: exe, Cwd: t.TempDir(), ConfigDir: t.TempDir(), Env: []string{"FAKE_CLAUDE=1"}}
}

func open(t *testing.T, l Launch, h Handlers) *Session {
	t.Helper()
	s, err := Open(context.Background(), l, h, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestGetUsageIsAnsweredByTheCLI(t *testing.T) {
	r, err := open(t, launch(t), Handlers{}).GetUsage(context.Background())
	if err != nil || r["rate_limits_available"] != true {
		t.Fatalf("%v %v", r, err)
	}
}

func TestATurnReportsItsFramesAndTheCLIsRequestDoesNotStopIt(t *testing.T) {
	got := make(chan map[string]any, 16)
	s := open(t, launch(t), Handlers{OnEvent: func(m map[string]any) { got <- m }})
	if err := s.Send("hi"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m := <-got:
			if m["type"] == "result" {
				if m["result"] != "heard hi" {
					t.Fatalf("%v", m)
				}
				return
			}
		case <-deadline:
			t.Fatal("no result frame")
		}
	}
}

func TestOpenFailsWhenTheCLIDoesNotComeUp(t *testing.T) {
	l := launch(t)
	l.Path = "/bin/false"
	if s, err := Open(context.Background(), l, Handlers{}, 5*time.Second); err == nil {
		s.Close()
		t.Fatal("opened a CLI that exited")
	}
}

func TestCloseProvesTheProcessGroupIsGone(t *testing.T) {
	old := CloseGrace
	CloseGrace = 200 * time.Millisecond
	t.Cleanup(func() { CloseGrace = old })
	l := launch(t)
	l.Env = append(l.Env, "FAKE_DEAF=1")
	s := open(t, l, Handlers{})
	pgid := s.cmd.Process.Pid
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(-pgid, 0); err != syscall.ESRCH {
		t.Fatalf("the group %d is still there: %v", pgid, err)
	}
}

func TestAFailedCloseIsNotReportedAsDoneByTheNextCall(t *testing.T) {
	s := open(t, launch(t), Handlers{})
	proveGone = func(int) error { return errors.New("processes of the claude group are still running") }
	t.Cleanup(func() { proveGone = groupGone })
	if s.Close() == nil || s.Close() == nil {
		t.Fatal("a group that is not gone was reported gone")
	}
	proveGone = groupGone
	if err := s.Close(); err != nil {
		t.Fatalf("once the group is gone Close says so: %v", err)
	}
}

func TestTheChildSeesWhatTheCLINeedsToFindTheLoginAndNothingElse(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret")
	t.Setenv("USER", "someone")
	env := strings.Join(Launch{ConfigDir: "/c"}.env(), "\n")
	if strings.Contains(env, "ANTHROPIC") || !strings.Contains(env, "USER=someone") || !strings.Contains(env, "CLAUDE_CONFIG_DIR=/c") {
		t.Fatal(env)
	}
}
