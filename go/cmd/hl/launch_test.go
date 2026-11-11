package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/paths"
)

// launchRig is a founder's rig whose home runs tmux, with a.one a Claude agent on account alpha.
func launchRig(t *testing.T) *rig {
	r := hookRig(t, false)
	for p, text := range map[string]string{
		paths.ConfigFile(r.home): "terminal: tmux\n",
		filepath.Join(r.home, "accounts", "alpha", ".claude", "settings.json"): "{}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r.pane = true
	return r
}

func TestLaunchIsRefusedOutsideTheDaemonsPane(t *testing.T) {
	r := launchRig(t)
	calls := r.daemon(map[string]string{"session.launch_prepare": `{"argv": ["/bin/sh", "-c", "echo ran"], "env": []}`})
	r.pane = false
	out, errs, code := r.run(nil, "launch", "a.one")
	want := "hl launch: agents are started by the daemon; start one from the app or with hl say <agent>\n"
	if code != 1 || out != "" || errs != want {
		t.Errorf("exit %d, out %q, err %q", code, out, errs)
	}
	if got := calls(); len(got) != 0 {
		t.Errorf("nothing is asked of the daemon: %v", got)
	}
	if _, errs, _ := r.run(nil); strings.Contains(errs, "launch") {
		t.Errorf("hl lists launch among its commands: %q", errs)
	}
}

func TestLaunchRefusesWhatItDoesNotLaunch(t *testing.T) {
	r := launchRig(t)
	use := launchUsage + "\n"
	for name, c := range map[string]struct {
		env       []string
		args      []string
		out, errs string
		code      int
	}{
		"under Orca":     {[]string{"HANDLINE_TERMINAL=orca"}, []string{"launch", "a.one"}, "", "hl launch: the terminal host orca is not available here\n", 1},
		"no agent named": {nil, []string{"launch"}, "", use, 2},
		"a typo":         {nil, []string{"launch", "a.one", "--acount", "x"}, "", use, 2},
		"help":           {nil, []string{"launch", "--help"}, use, "", 0},
	} {
		out, errs, code := r.run(c.env, c.args...)
		if out != c.out || errs != c.errs || code != c.code {
			t.Errorf("%s: out %q err %q exit %d", name, out, errs, code)
		}
	}
}

// The launch is the daemon's to build and record: hl sends where it runs and what it was given, says the refusal as the daemon words it,
// and becomes what the daemon returns.
func TestLaunchAsksTheDaemonAndSaysItsRefusal(t *testing.T) {
	r := launchRig(t)
	calls := r.daemon(map[string]string{"session.launch_prepare": "error:agent 'a.one' has no recorded session to resume"})
	// What follows -- is claude's: here a resume, with no session on record to resume.
	out, errs, code := r.run(nil, "launch", "a.one", "--account", "alpha", "--why", "test", "--", "--resume", "abc")
	if code != 1 || out != "" || errs != "agent 'a.one' has no recorded session to resume\n" {
		t.Fatalf("exit %d, out %q, err %q", code, out, errs)
	}
	var asked map[string]any
	for _, m := range calls() {
		if m["method"] == "session.launch_prepare" {
			asked = m["params"].(map[string]any)
		}
	}
	extra, _ := asked["extra"].([]any)
	env, _ := asked["env"].(map[string]any)
	if asked["agent"] != "a.one" || asked["account"] != "alpha" || asked["why"] != "test" || !sameFolder(t, asked["cwd"], r.root) ||
		len(extra) != 2 || extra[0] != "--resume" || env["HANDLINE_HOME"] != r.home {
		t.Fatalf("asked %v", asked)
	}
}

func TestLaunchBecomesWhatTheDaemonReturns(t *testing.T) {
	r := launchRig(t)
	r.daemon(map[string]string{"session.launch_prepare": `{"argv": ["/bin/sh", "-c", "echo $MARK"], "env": ["MARK=launched"], "warnings": ["warning: be careful"]}`})
	out, errs, code := r.run(nil, "launch", "a.one")
	if code != 0 || out != "launched\n" || !strings.HasPrefix(errs, "warning: be careful\n") {
		t.Fatalf("exit %d, out %q, err %q", code, out, errs)
	}
}

func TestOwnTerminalIsAPaneOnOurServerOnly(t *testing.T) {
	wd, _ := os.Getwd()
	real, _ := filepath.EvalSymlinks(wd)
	home := t.TempDir()
	t.Setenv("HANDLINE_HOME", home)
	t.Setenv("HANDLINE_TMUX_SOCKET", "")
	own := paths.TmuxSocket(home)
	for _, c := range []struct {
		pane, server, sock, handle string
	}{
		{"%12", "/tmp/tmux-501/" + own + ",123,0", "", "%12"},
		{"%12", "/tmp/tmux-501/handline,123,0", "", ""}, // a scratch home's hl in a pane of the live server
		{"%12", "/tmp/tmux-501/handline,123,0", "handline", ""},
		{"%12", "/tmp/tmux-501/default,123,0", "", ""},
		{"%12", "/tmp/tmux-501/suite,1,0", "suite", "%12"},
		{"12", "/tmp/tmux-501/handline,1,0", "", ""},
		{"", "/tmp/tmux-501/handline,1,0", "", ""},
	} {
		t.Setenv("TMUX_PANE", c.pane)
		t.Setenv("TMUX", c.server)
		t.Setenv("HANDLINE_TMUX_SOCKET", c.sock)
		handle, wt := tmuxOwnTerminal()
		if handle != c.handle || (handle != "" && wt != real) || (handle == "" && wt != "") {
			t.Errorf("%+v: %q %q", c, handle, wt)
		}
	}
}

func sameFolder(t *testing.T, got any, want string) bool {
	t.Helper()
	g, _ := got.(string)
	a, _ := filepath.EvalSymlinks(g)
	b, _ := filepath.EvalSymlinks(want)
	return a != "" && a == b
}
