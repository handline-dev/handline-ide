//go:build !windows

package tmux

import (
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/handline/handline/go/agentctl/term"
	"golang.org/x/sys/unix"
)

const (
	// A detached session has no client to size it, and a TUI drawn at 80x24 hides what the screen readers look for.
	sessionCols, sessionRows = "200", "50"
)

var windowNameRE = regexp.MustCompile(`[.:]`)

func (t Tmux) serverUp() bool {
	code, _, _ := t.tmux(tmuxTimeout, "", "has-session", "-t", tmuxSession)
	return code == 0
}

// AgentOption is the tmux pane option that says which agent the daemon opened the pane for; a pane without it is not ours to close.
const AgentOption = "@hl_agent"

func (t Tmux) CreateTerminal(command, cwd, title, agent string) (string, string) {
	pane, why := t.newPane(cwd, title, "")
	if pane == "" {
		return "", why
	}
	if agent != "" {
		if code, _, err := t.tmux(tmuxTimeout, "", "set-option", "-p", "-t", pane, AgentOption, agent); code != 0 {
			t.CloseTerminal(pane, true)
			return "", "could not stamp the new window with its agent: " + err
		}
	}
	if ok, why := t.Send(pane, command, true); !ok {
		t.CloseTerminal(pane, true)
		return "", "could not type the command into the new window: " + why
	}
	return pane, ""
}

// ShellOption is the tmux pane option that names the agent a plain shell was opened beside; such a pane is the person's, not an agent's.
const ShellOption = "@hl_shell"

// CreateShell opens a window in cwd running the user's login shell, stamped as a shell of agent.
func (t Tmux) CreateShell(cwd, title, agent string) (string, string) {
	pane, why := t.newPane(cwd, title, `exec "${SHELL:-/bin/zsh}" -l`)
	if pane == "" {
		return "", why
	}
	if code, _, err := t.tmux(tmuxTimeout, "", "set-option", "-p", "-t", pane, ShellOption, agent); code != 0 {
		t.CloseTerminal(pane, true)
		return "", "could not stamp the new window as a shell: " + err
	}
	return pane, ""
}

// RunOption is the tmux pane option that tags a window the daemon opened to run a command ("<message id>:<n>@<start>"); a pane id comes
// back after a restart of the tmux server, the tag does not.
const RunOption = "@hl_run"

// CreateRun opens a window in cwd, stamped as a shell of agent and tagged tag, whose own command is `$SHELL -lc command`. The window
// stays after the command ends (remain-on-exit), so tmux keeps its exit status and end time. The option is set on a plain window that
// is then respawned with the command, so a command that ends at once cannot close it first.
func (t Tmux) CreateRun(cwd, title, agent, tag, command string) (string, string) {
	pane, why := t.newPane(cwd, title, "")
	if pane == "" {
		return "", why
	}
	for _, o := range [][]string{{ShellOption, agent}, {RunOption, tag}, {"remain-on-exit", "on"}} {
		if code, _, err := t.tmux(tmuxTimeout, "", "set-option", "-p", "-t", pane, o[0], o[1]); code != 0 {
			t.CloseTerminal(pane, true)
			return "", "could not set up the new window: " + err
		}
	}
	if code, _, err := t.tmux(tmuxTimeout, "", "respawn-pane", "-k", "-t", pane, "/bin/sh", "-c", `exec "${SHELL:-/bin/zsh}" -lc "$1"`, "sh", command); code != 0 {
		t.CloseTerminal(pane, true)
		return "", "could not run the command in the new window: " + err
	}
	return pane, ""
}

// signalNumber is the number of the signal pane_dead_signal names, as a number, a name with or without SIG in any case, or the system's
// words for it ("interrupt"); 0 when it is none of these.
func signalNumber(name string) int {
	if n, err := strconv.Atoi(name); err == nil {
		return n
	}
	if n := int(unix.SignalNum("SIG" + strings.TrimPrefix(strings.ToUpper(name), "SIG"))); n != 0 {
		return n
	}
	for n := 1; n < 32; n++ {
		if strings.EqualFold(syscall.Signal(n).String(), name) {
			return n
		}
	}
	return 0
}

// PaneRuns is every pane that carries a run tag, by handle; nil when tmux cannot say (a pane absent from a non-nil answer is gone).
func (t Tmux) PaneRuns() map[string]term.PaneRun {
	code, out, err := t.tmux(tmuxTimeout, "", "list-panes", "-a", "-F", "#{pane_id}\t#{"+RunOption+"}\t#{pane_dead}\t#{pane_dead_status}\t#{pane_dead_time}\t#{pane_dead_signal}")
	if code != 0 {
		low := strings.ToLower(err)
		for _, m := range noServer {
			if strings.Contains(low, m) {
				return map[string]term.PaneRun{}
			}
		}
		return nil
	}
	runs := map[string]term.PaneRun{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 6 || !paneRE.MatchString(f[0]) || f[1] == "" {
			continue
		}
		r := term.PaneRun{Tag: f[1], Dead: f[2] == "1"}
		r.Status, _ = strconv.Atoi(f[3])
		r.DeadTime, _ = strconv.ParseInt(f[4], 10, 64)
		if f[5] != "" { // killed by a signal: tmux leaves the status empty and names the signal; a shell would say 128 plus its number
			r.Status = 128 + signalNumber(f[5])
		}
		runs[f[0]] = r
	}
	return runs
}

// PaneShell is the agent a shell pane was opened beside; "" for any other pane, known false when tmux cannot say.
func (t Tmux) PaneShell(handle string) (agent string, known bool) {
	return t.paneOption(handle, ShellOption)
}

// paneOption is the pane's value of a tmux pane option, known false when tmux cannot say.
func (t Tmux) paneOption(handle, option string) (string, bool) {
	if !paneRE.MatchString(handle) {
		return "", false
	}
	code, out, _ := t.tmux(tmuxTimeout, "", "display-message", "-p", "-t", handle, "#{"+option+"}")
	return strings.TrimSpace(out), code == 0
}

// newPane opens a window in cwd that runs shellCmd (the default shell when empty) and answers its pane's handle.
func (t Tmux) newPane(cwd, title, shellCmd string) (string, string) {
	var args []string
	if title != "" {
		args = append(args, "-n", windowNameRE.ReplaceAllString(title, "_"))
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	// The place's variables go into the window's environment so what runs there reads them and not whatever the tmux server or the
	// program's own environment was started with.
	for _, kv := range t.Env {
		args = append(args, "-e", kv)
	}
	args = append(args, "-P", "-F", "#{pane_id}")
	if shellCmd != "" {
		args = append(args, shellCmd)
	}
	var code int
	var out, err string
	if t.serverUp() {
		code, out, err = t.tmux(tmuxTimeout, "", append([]string{"new-window", "-t", tmuxSession + ":"}, args...)...)
	} else {
		code, out, err = t.tmux(tmuxTimeout, "", append([]string{"new-session", "-d", "-s", tmuxSession, "-x", sessionCols, "-y", sessionRows}, args...)...)
	}
	pane := strings.TrimSpace(out)
	if code != 0 || !paneRE.MatchString(pane) {
		if err == "" {
			err = "tmux gave no pane id (" + term.PyRepr(term.Cut(pane, 80)) + ")"
		}
		return "", err
	}
	return pane, ""
}

func (t Tmux) CloseTerminal(handle string, tab bool) (bool, string) {
	if !paneRE.MatchString(handle) {
		return false, "not a tmux pane: " + term.PyRepr(handle)
	}
	kill := "kill-pane"
	if tab {
		kill = "kill-window"
	}
	code, _, err := t.tmux(tmuxTimeout, "", kill, "-t", handle)
	return code == 0, err
}

func (t Tmux) RespawnTerminal(handle string) (bool, string) {
	if !paneRE.MatchString(handle) {
		return false, "not a tmux pane: " + term.PyRepr(handle)
	}
	code, _, err := t.tmux(tmuxTimeout, "", "respawn-pane", "-k", "-t", handle)
	return code == 0, err
}

// PaneAgent is the agent the daemon stamped the pane with; "" for a pane it did not open for an agent, known false when tmux cannot say.
func (t Tmux) PaneAgent(handle string) (agent string, known bool) {
	return t.paneOption(handle, AgentOption)
}

// PanePid is the pid of the process the pane runs (its shell); 0 when tmux cannot say.
func (t Tmux) PanePid(handle string) int {
	if !paneRE.MatchString(handle) {
		return 0
	}
	code, out, _ := t.tmux(tmuxTimeout, "", "display-message", "-p", "-t", handle, "#{pane_pid}")
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if code != 0 || err != nil {
		return 0
	}
	return pid
}
