package reads

import (
	"os/exec"
	"runtime"
)

// MissingTool is DaemonStatus.missing_tools' item: a tool agents are run with that this machine lacks, and the shell command that installs it.
type MissingTool struct {
	Tool string `json:"tool"`
	Fix  string `json:"fix"`
}

// missingTools is DaemonStatus.missing_tools: none when the daemon runs no agent side.
func (e *Env) missingTools() []MissingTool {
	if e.Claude == "" {
		return nil
	}
	return MissingTools(e.Claude)
}

// MissingTools are the tools agents are run with that this machine lacks: claude, git and tmux.
// The daemon still starts without them, so the app can show what to install.
func MissingTools(claude string) []MissingTool {
	stub := ""
	if runtime.GOOS == "darwin" {
		stub = macGitStub
	}
	return missingTools(claude, stub, hasDevTools)
}

// macGitStub is the git every Mac has, which only forwards to the command line tools: without them it is no git.
const macGitStub = "/usr/bin/git"

// hasDevTools is whether the command line tools are installed. It never runs the stub, which would raise Apple's install dialog.
func hasDevTools() bool { return exec.Command("xcode-select", "-p").Run() == nil }

// missingTools is MissingTools with the git stub's path ("" for none) and the check for what it forwards to.
func missingTools(claude, stub string, devTools func() bool) []MissingTool {
	var out []MissingTool
	for _, m := range []struct{ name, tool, fix string }{
		{claude, "claude", "curl -fsSL https://claude.ai/install.sh | bash"}, // Claude Code's native installer
		{"git", "git", "xcode-select --install"},                             // git comes with the command line tools
		{"tmux", "tmux", "brew install tmux"},
	} {
		p, err := exec.LookPath(m.name)             // a path is checked as it is, a name on PATH
		if err != nil || p == stub && !devTools() { // only git can be the stub
			out = append(out, MissingTool{m.tool, m.fix})
		}
	}
	return out
}
