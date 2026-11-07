package reads

import "os/exec"

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
	var out []MissingTool
	for _, m := range []struct{ name, tool, fix string }{
		{claude, "claude", "curl -fsSL https://claude.ai/install.sh | bash"}, // Claude Code's native installer
		{"git", "git", "xcode-select --install"},                             // git comes with the command line tools
		{"tmux", "tmux", "brew install tmux"},
	} {
		if _, err := exec.LookPath(m.name); err != nil { // a path is checked as it is, a name on PATH
			out = append(out, MissingTool{m.tool, m.fix})
		}
	}
	return out
}
