package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/handline/handline/go/internal/hlclient"
)

// appVerbs are the forms of hl app; tools/app.mjs of the worktree runs them.
var appVerbs = map[string]bool{"start": true, "shot": true, "click": true, "type": true, "text": true, "eval": true,
	"drag-check": true, "status": true, "stop": true}

// appCmd is hl app <verb> ...: a separate test instance of this worktree's desktop app that an agent starts, looks at and presses
// (tools/app.mjs, which the app-smoke gate shares its launch code with). It never touches the user's own window.
func appCmd(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 0 || !appVerbs[args[0]] {
		return 0, false
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	top := strings.TrimSpace(string(out))
	if err != nil || top == "" {
		fmt.Fprintln(os.Stderr, "hl app: run it inside the worktree whose app you test")
		return 1, true
	}
	script := filepath.Join(top, "tools", "app.mjs")
	if _, err := os.Stat(script); err != nil {
		fmt.Fprintf(os.Stderr, "hl app: %s is not there: this worktree has no desktop app to run\n", script)
		return 1, true
	}
	cmd := exec.Command("node", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "HANDLINE_HOME="+c.Home)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), true
		}
		fmt.Fprintf(os.Stderr, "hl app: %v\n", err)
		return 1, true
	}
	return 0, true
}

// reapApps ends the test apps agent started, when its turn ends or its session does: each instance's state file in the home's
// app-instances names its owner and its worktree, whose tools/app.mjs stops it. A worktree that is gone leaves only the file to remove.
func reapApps(home, agent string) {
	files, _ := filepath.Glob(filepath.Join(home, "app-instances", "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		var s struct{ Owner, Top string }
		if err != nil || json.Unmarshal(b, &s) != nil || s.Owner != agent {
			continue
		}
		script := filepath.Join(s.Top, "tools", "app.mjs")
		if _, err := os.Stat(script); err != nil {
			_ = os.Remove(f)
			continue
		}
		cmd := exec.Command("node", script, "stop")
		cmd.Env = append(os.Environ(), "HANDLINE_HOME="+home, "HANDLINE_AGENT="+agent)
		cmd.Dir = s.Top
		done := make(chan struct{})
		go func() { _ = cmd.Run(); close(done) }()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			_ = cmd.Process.Kill()
		}
	}
}
