//go:build !windows

package tmux_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/agentctl/term/tmux"
)

// fakeTmux puts a `tmux` on PATH that logs its arguments, one call a line, and answers like a server: has-session fails until
// new-session has run, and new-window and new-session answer pane %7.
func fakeTmux(t *testing.T) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls")
	script := `#!/bin/sh
printf '%s\n' "$*" >> ` + log + `
case "$3" in
  has-session) [ -f ` + dir + `/up ] && exit 0; echo "no server running on x" >&2; exit 1;;
  new-session) touch ` + dir + `/up; echo %7;;
  new-window) echo %7;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return log
}

// atH is a tmux host as a program at /h sets it up: server t, its home and server in each window's environment.
var atH = tmux.Tmux{Place: term.Place{Key: "/h", Socket: "t", Env: []string{"HANDLINE_HOME=/h", "HANDLINE_TMUX_SOCKET=t"}}}

func calls(t *testing.T, log string) []string {
	b, _ := os.ReadFile(log)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func TestATerminalIsOpenedInTheWorktreeAndTheLaunchLineTypedAtItsShell(t *testing.T) {
	log := fakeTmux(t)
	handle, why := atH.CreateTerminal("hl launch alice", "/wt/alice", "alice.x", "alice")
	if handle != "%7" || why != "" {
		t.Fatalf("%q %q", handle, why)
	}
	got := calls(t, log)
	// no server yet: a session is made with a size; the window name has no dots or colons; the home goes in the environment
	want := "-L t new-session -d -s handline -x 200 -y 50 -n alice_x -c /wt/alice -e HANDLINE_HOME=/h -e HANDLINE_TMUX_SOCKET=t -P -F #{pane_id}"
	if got[1] != want {
		t.Fatalf("%q", got)
	}
	if got[len(got)-2] != "-L t send-keys -t %7 -l -- hl launch alice" || got[len(got)-1] != "-L t send-keys -t %7 Enter" {
		t.Fatalf("%q", got)
	}
	if !strings.Contains(strings.Join(got, "\n"), "-L t set-option -p -t %7 @hl_agent alice") {
		t.Fatalf("the pane is stamped with its agent: %q", got)
	}
	// a second terminal is a window of the session that is up
	atH.CreateTerminal("hl launch bob", "/wt/bob", "bob", "bob")
	got = calls(t, log)
	if !strings.HasPrefix(got[len(got)-4], "-L t new-window -t handline: -n bob -c /wt/bob") {
		t.Fatalf("%q", got)
	}
}

func TestAWindowWhoseLaunchLineCouldNotBeTypedIsClosedAgain(t *testing.T) {
	log := fakeTmux(t)
	dir := filepath.Dir(log)
	os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+log+"\ncase \"$3\" in has-session) exit 1;; new-session) echo %7;; send-keys) echo no pane >&2; exit 1;; esac\n"), 0o755)
	handle, why := atH.CreateTerminal("hl launch alice", "", "", "")
	if handle != "" || !strings.Contains(why, "could not type the command") {
		t.Fatalf("%q %q", handle, why)
	}
	if got := calls(t, log); !strings.HasSuffix(got[len(got)-1], "kill-window -t %7") {
		t.Fatalf("%q", got)
	}
	if ok, _ := (tmux.Tmux{}).CloseTerminal("not a pane", true); ok {
		t.Fatal("only a pane id is closed")
	}
}

// An unstamped pane is not the daemon's to close later, so a window whose stamp failed is closed now and nothing is typed in it.
func TestAWindowWhoseAgentStampFailedIsClosedAgain(t *testing.T) {
	log := fakeTmux(t)
	dir := filepath.Dir(log)
	os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+log+"\ncase \"$3\" in has-session) exit 1;; new-session) echo %7;; set-option) echo no pane >&2; exit 1;; esac\n"), 0o755)
	handle, why := atH.CreateTerminal("hl launch alice", "", "", "alice")
	if handle != "" || !strings.Contains(why, "could not stamp the new window with its agent") {
		t.Fatalf("%q %q", handle, why)
	}
	got := calls(t, log)
	if !strings.HasSuffix(got[len(got)-1], "kill-window -t %7") || strings.Contains(strings.Join(got, "\n"), "send-keys") {
		t.Fatalf("the window is closed and nothing typed in it: %q", got)
	}
}
