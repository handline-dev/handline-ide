package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	_ "github.com/handline/handline/go/agentctl/agent/vendors"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/peer"
	"github.com/handline/handline/go/scheduler/worktreelock"
)

const launchUsage = "usage: hl launch [--force] [--account ACCOUNT] [--prompt-file PROMPT_FILE] [--why WHY] [--by BY] agent [extra ...]"

// launch is `hl launch <agent>` (session/launch.exec_agent), which the daemon types into the agent's own pane: build the session's command line and become it. The daemon
// refuses a vendor with no launchable Agent.
func launch(c *hlclient.Client, args []string) (int, bool) {
	before, extra := args, []string(nil)
	for i, a := range args {
		if a == "--" { // what follows is claude's, as argparse reads it
			before, extra = args[:i], args[i+1:]
			break
		}
	}
	p, ok := parse(before, []string{"--account", "--prompt-file", "--why", "--by"}, []string{"--force"}, nil)
	if !ok || len(p.pos) == 0 {
		return badArgs(launchUsage, before), true
	}
	agent := p.pos[0]
	extra = append(p.pos[1:], extra...)
	if b := agenthost.Current(c.Home); !b.Available() {
		fmt.Fprintf(os.Stderr, "hl launch: the terminal host %s is not available here\n", b.Name())
		return 1, true
	}
	handle, worktree := launchTerminal()
	if handle == "" {
		fmt.Fprintln(os.Stderr, "hl launch: agents are started by the daemon; start one from the app or with hl say <agent>")
		return 1, true
	}
	cwd, _ := os.Getwd()
	opt := func(k string) string { v, _ := p.opt(k); return v }
	promptFile := opt("--prompt-file")
	if abs, err := filepath.Abs(promptFile); err == nil && promptFile != "" {
		promptFile = abs
	}
	params := map[string]any{"agent": agent, "extra": extra, "force": p.flags["--force"], "prompt_file": promptFile,
		"account": opt("--account"), "why": opt("--why"), "by": opt("--by"), "cwd": cwd, "env": environ(),
		"hl_exe": paths.HlExe(), "handle": handle, "worktree_id": worktree}
	slow := *c
	slow.Deadline = hlclient.Timeout // it installs hooks, reads git and the usage reading before it answers
	got, err := slow.Call("session.launch_prepare", params)
	var refused *hlclient.RPCError
	if errors.As(err, &refused) && refused.Code == "refused" {
		fmt.Fprintln(os.Stderr, refused.Message) // Python's SystemExit(message)
		return 1, true
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "hl launch: %v\n", err)
		return 1, true
	}
	warnings, _ := got["warnings"].([]any)
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, w)
	}
	argv, env := textList(got["argv"]), textList(got["env"])
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "hl launch: the daemon gave no command to run")
		return 1, true
	}
	// The worktree is locked for as long as this process, which becomes the session, runs: nothing prunes or removes it under the agent.
	if err := holdWorktree(cwd, agent, os.Getpid()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: the worktree is not locked for %s: %v\n", agent, err)
	}
	fmt.Fprintf(os.Stderr, "hl launch: %v\n", syscall.Exec(argv[0], argv, env))
	return 1, true
}

func holdWorktree(dir, agent string, pid int) error {
	started, ok := peer.Started(pid)
	if !ok {
		return fmt.Errorf("process %d's start time cannot be read", pid)
	}
	return worktreelock.Hold(dir, agent, pid, started.Unix(), worktreelock.Running)
}

func environ() map[string]string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	return env
}

// textList is a JSON list of text as a Go one.
func textList(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, it := range list {
		if t, ok := it.(string); ok {
			out = append(out, t)
		}
	}
	return out
}

var paneRE = regexp.MustCompile(`^%\d+$`)

// launchTerminal is the pane the session will run in; with no terminal on stdin the pane in the environment is another program's.
func launchTerminal() (string, string) {
	if !hlclient.StdinIsTerminal() {
		return "", ""
	}
	return tmuxOwnTerminal()
}

// tmuxOwnTerminal is the pane this process runs in and its working folder, only when the pane is on our tmux server: a
// pane id of someone else's tmux means nothing on ours (backends/tmux.own_terminal).
func tmuxOwnTerminal() (string, string) {
	pane, server := os.Getenv("TMUX_PANE"), os.Getenv("TMUX")
	sock := paths.TmuxSocket(paths.Home())
	if first, _, _ := strings.Cut(server, ","); !paneRE.MatchString(pane) || filepath.Base(first) != sock {
		return "", ""
	}
	wd, _ := os.Getwd()
	if real, err := filepath.EvalSymlinks(wd); err == nil {
		wd = real // os.getcwd is the folder itself, not $PWD's spelling of it
	}
	return pane, wd
}
