package agenthost_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/agenthost"
)

// fakeTmux puts a `tmux` on PATH that logs its arguments, one call a line, and answers like a server that is not up yet.
func fakeTmux(t *testing.T) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\ncase \"$3\" in has-session) exit 1;; new-session|new-window) echo %7;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return log
}

func calls(t *testing.T, log string) []string {
	b, _ := os.ReadFile(log)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func TestOnlyTmuxIsAHostAndAnyOtherNameIsNeverAvailable(t *testing.T) {
	t.Setenv("HANDLINE_TERMINAL", "")
	home := t.TempDir()
	if b := agenthost.Current(home); b.Name() != "tmux" {
		t.Fatalf("with nothing said: %q", b.Name())
	}
	os.WriteFile(filepath.Join(home, "config.yaml"), []byte("terminal: orca\n"), 0o644)
	b := agenthost.Current(home)
	if ok, why := b.Send("h", "x", true); b.Name() != "orca" || b.Available() || b.ListTerminals() != nil || ok || !strings.Contains(why, "orca") {
		t.Fatalf("a host the daemon does not drive: %q %v %q", b.Name(), b.Available(), why)
	}
}

// A daemon on a scratch home never reaches the live tmux server (-L handline), whether the socket setting is missing or names handline: a
// scratch daemon that did put a window among the live agents'.
func TestAScratchDaemonNeverReachesTheLiveTmuxServer(t *testing.T) {
	log := fakeTmux(t)
	home := t.TempDir()
	for _, sock := range []string{"", "handline"} {
		t.Setenv("HANDLINE_TMUX_SOCKET", sock)
		tm := agenthost.Current(home)
		tm.ListTerminals()
		tm.(term.Host).CreateTerminal("hl launch alice", home, "alice", "alice")
		for _, argv := range [][]string{tm.(term.ControlSpawner).ControlCommand(), tm.(term.Attacher).AttachCommand()} {
			if argv[1] != "-L" || argv[2] == "handline" {
				t.Errorf("socket %q: %q", sock, argv)
			}
		}
	}
	for _, line := range calls(t, log) {
		if !strings.HasPrefix(line, "-L handline-") {
			t.Errorf("not on the scratch home's own server: %q", line)
		}
	}
}

// A terminal names the daemon's own home and tmux server, not the live home a daemon's environment may still carry: what runs in a
// scratch daemon's window must not reach the live daemon.
func TestATerminalNamesTheDaemonsHomeNotTheEnvironments(t *testing.T) {
	log := fakeTmux(t)
	live, home := filepath.Join(t.TempDir(), ".handline"), t.TempDir()
	t.Setenv("HANDLINE_HOME", live)
	t.Setenv("HANDLINE_TMUX_SOCKET", "")
	if _, why := agenthost.Current(home).(term.Host).CreateTerminal("hl launch alice", home, "alice", "alice"); why != "" {
		t.Fatal(why)
	}
	made := calls(t, log)[1]
	sock := strings.Fields(made)[1]
	for _, want := range []string{"HANDLINE_HOME=" + home, "HANDLINE_TMUX_SOCKET=" + sock} {
		if !strings.Contains(made, " -e "+want+" ") {
			t.Errorf("no %s in %q", want, made)
		}
	}
	if strings.Contains(made, live) {
		t.Errorf("the environment's home reached the window: %q", made)
	}
}
