package reads

import "os/exec"

// MissingTool is DaemonStatus.missing_tools' item: a tool agents are run with that this machine lacks, and how to get it.
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
	if _, err := exec.LookPath(claude); err != nil { // a path is checked as it is, a name on PATH
		out = append(out, MissingTool{"claude", "Install Claude Code"})
	}
	for _, t := range []string{"git", "tmux"} {
		if _, err := exec.LookPath(t); err != nil {
			out = append(out, MissingTool{t, "brew install " + t})
		}
	}
	return out
}
