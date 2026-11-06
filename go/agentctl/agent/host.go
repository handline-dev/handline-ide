package agent

import (
	"os"
	"os/user"
)

// Host is what the program that runs agents tells agentctl about itself, so agentctl guesses none of it.
type Host struct {
	// Home is the program's own folder; the agent CLI's shared settings live under it.
	Home string
	// Socket is the program's control socket, which a sandboxed agent may connect to.
	Socket string
	// Commands are the program's command names, main first: a hook line runs one of them, and the safe commands are theirs.
	Commands []string
}

// UserHome is the user's home as the agent CLIs read it, and RunDir the folder a CLI run outside any worktree starts in. The program
// sets both once at start.
var (
	UserHome = func() string { h, _ := os.UserHomeDir(); return h }
	RunDir   = os.TempDir
)

// UserName is the user's login name as the agent CLIs read it: $USER, else the OS user's. Claude Code files its keychain credential
// under this name, and a CLI run without USER files it under "unknown" instead, a second item nothing else reads.
func UserName() string {
	if n := os.Getenv("USER"); n != "" {
		return n
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}
