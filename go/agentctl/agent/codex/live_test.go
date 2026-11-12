package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/term"
	tmuxhost "github.com/handline/handline/go/agentctl/term/tmux"
)

// TestLiveCodexSession runs a real codex on its launch line in a tmux server of its own, spending the login HANDLINE_LIVE_CODEX_AUTH
// names (default ~/.codex/auth.json), linked into a scratch CODEX_HOME as account.adopt links it.
// HANDLINE_LIVE=1 go test -run TestLiveCodexSession -v -timeout 15m ./agentctl/agent/codex
func TestLiveCodexSession(t *testing.T) {
	if os.Getenv("HANDLINE_LIVE") != "1" { // as accounts/livetest.Require, which this module cannot import
		t.Skip("a live test: HANDLINE_LIVE=1 runs it")
	}
	exe := Driver{}.Binary()
	if exe == "" {
		t.Fatal("codex is not on PATH")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatal(err)
	}
	auth := os.Getenv("HANDLINE_LIVE_CODEX_AUTH")
	if auth == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		auth = filepath.Join(home, ".codex", credentialFile)
	}
	if _, err := os.Stat(auth); err != nil {
		t.Fatalf("no codex login to spend: %v", err)
	}

	// Log in: the account's home holds the login as account.adopt leaves it, a link to the file.
	root := t.TempDir()
	codexHome, wd := filepath.Join(root, "home"), filepath.Join(root, "wd")
	for _, d := range []string{codexHome, wd} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(auth, authFile(codexHome)); err != nil {
		t.Fatal(err)
	}
	if present, known := (Account{}).CredentialPresent(codexHome); !present || !known {
		t.Fatalf("codex login status: logged in %v, known %v", present, known)
	}
	t.Cleanup(func() { // codex saving a refreshed login over the link would leave the real one stale
		if fi, err := os.Lstat(authFile(codexHome)); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is no longer a link to %s: log in there again", authFile(codexHome), auth)
		}
	})
	if out, err := exec.Command("git", "-C", wd, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}

	// Handline's hooks, with a recorder in place of hl: each call keeps its stdin in a file named for its event.
	hooks := Hooks{agent.Host{Home: filepath.Join(root, "handline")}}
	hookDir := filepath.Join(root, "hooks")
	if err := os.Mkdir(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recorder := filepath.Join(root, "hl")
	script := fmt.Sprintf("#!/bin/sh\ncat > \"$(mktemp %q/\"$2\".XXXXXX)\"\n", hookDir)
	if err := os.WriteFile(recorder, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := hooks.Install(recorder); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(hooks.Target(), filepath.Join(codexHome, "hooks.json")); err != nil {
		t.Fatal(err)
	}

	// Start: the launch line in a tmux server of the test's own.
	sock := fmt.Sprintf("hl-codex-live-%d", os.Getpid())
	tmux := func(args ...string) (string, error) {
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", "/dev/null"}, args...)...).CombinedOutput()
		return string(out), err
	}
	t.Cleanup(func() { tmux("kill-server") })
	argv := Driver{}.LaunchArgv(exe, agent.LaunchArgs{Cwd: wd, Effort: "low", Preamble: "You are a test. Answer in as few words as you can."})
	start := append([]string{"new-session", "-d", "-P", "-F", "#{pane_id}", "-x", "200", "-y", "50", "-e", "CODEX_HOME=" + codexHome, "-c", wd}, argv...)
	out, err := tmux(append(start, ";", "set-option", "-g", "remain-on-exit", "on")...)
	if err != nil {
		t.Fatalf("start: %v %s", err, out)
	}
	pane := strings.TrimSpace(out)
	host := tmuxhost.Tmux{Place: term.Place{Socket: sock}}
	screen := func() []string {
		if s := host.ReadScreen(pane, 50); s != nil {
			return s.Tail
		}
		return nil
	}
	waitFor := func(what string, d time.Duration, ok func() bool) {
		t.Helper()
		for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(500 * time.Millisecond) {
			if ok() {
				return
			}
		}
		t.Fatalf("no %s within %s; the screen:\n%s", what, d, strings.Join(screen(), "\n"))
	}
	waitFor("composer ready to type into", 90*time.Second, func() bool {
		s := screen()
		working, known := Screen{}.WorkingOnScreen(s)
		return Screen{}.ScreenBlocker(s) == nil && known && !working
	})

	// Message: typed through the delivery port and confirmed from the session file.
	delivered, failed := make(chan string, 1), make(chan string, 1)
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	port := Driver{}.Deliver(agent.DeliverEnv{
		Where: func(string) (agent.Place, error) {
			select {
			case <-done: // the test is over: the port stops looking
				return agent.Place{}, agent.ErrNoSession
			default:
				return agent.Place{Config: codexHome, Cwd: wd}, nil
			}
		},
		TypeIn: func(_, text string) error {
			if ok, why := host.Send(pane, text, true); !ok {
				return errors.New(why)
			}
			return nil
		},
		OnDelivered: func(_, op string) { delivered <- op },
		OnFailed:    func(_, _, why string, _ bool) { failed <- why },
	})
	const ask = "Reply with exactly the word PONG and nothing else."
	if _, err := port.Send("live", "op-1", ask, ""); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case <-delivered:
	case why := <-failed:
		t.Fatalf("not delivered: %s", why)
	case <-time.After(2 * time.Minute):
		t.Fatalf("no confirmation within 2m; the screen:\n%s", strings.Join(screen(), "\n"))
	}

	// Reply: the turn's Stop hook carries the last answer.
	var reply string
	events := map[string]bool{}
	waitFor("Stop hook with the reply", 3*time.Minute, func() bool {
		calls, _ := os.ReadDir(hookDir)
		for _, c := range calls {
			event, _, _ := strings.Cut(c.Name(), ".")
			events[event] = true
			var in struct {
				Last string `json:"last_assistant_message"`
			}
			if b, err := os.ReadFile(filepath.Join(hookDir, c.Name())); err == nil && event == "Stop" && json.Unmarshal(b, &in) == nil {
				reply = in.Last
			}
		}
		return reply != ""
	})
	t.Logf("reply %q; hooks seen %v", reply, events)
	if !strings.Contains(strings.ToUpper(reply), "PONG") {
		t.Errorf("reply %q has no PONG", reply)
	}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit"} {
		if !events[ev] {
			t.Errorf("no %s hook", ev)
		}
	}
	waitFor("the turn ended on screen", 30*time.Second, func() bool {
		working, known := Screen{}.WorkingOnScreen(screen())
		return known && !working
	})

	// Stop: Ctrl-C twice on an empty composer ends codex.
	for range 2 {
		if ok, why := host.Send(pane, "\x03", false); !ok {
			t.Fatal(why)
		}
		time.Sleep(300 * time.Millisecond)
	}
	waitFor("codex to exit", 20*time.Second, func() bool {
		out, _ := tmux("display-message", "-p", "-t", pane, "#{pane_dead}")
		return strings.TrimSpace(out) == "1"
	})
}
