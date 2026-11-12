// Package agent is what the daemon asks of an agent CLI (Claude Code, later opencode): the interface and its shared types. The
// implementations are in the packages below it; the packages that start sessions import this one and never an implementation.
package agent

import "github.com/handline/handline/go/agentctl/term"

// Agent is one agent CLI as the daemon starts it: its binary, the line that launches a session and the hooks that report its state. The
// terminal it runs in is term.Backend's.
type Agent interface {
	Account
	IsResume(extra []string) bool
	Binary() string
	// Screen is how this agent CLI's screen is read and answered: its wording, for the terminal code that knows how to read and press.
	Screen() term.Driver
	// Hooks is the vendor's state hooks, nil when it has none.
	Hooks() Hooks
	AutoModeArgv() []string
	PermissionModeArgv(mode string) []string
	LaunchArgv(exe string, o LaunchArgs) []string
	LaunchEnv(preamble, disallowed, agentDir string) map[string]string
	PromptArgv(prompt string) []string
	// TranscriptDir is where a session in worktree keeps its transcripts under a config directory.
	TranscriptDir(configDir, worktree string) (string, bool)
	// Deliver is the port that gets messages into this CLI's sessions. A CLI with no inbox may type a message into its terminal, but it
	// settles that as unknown (written, the sender told it may have arrived), never as delivered: typing cannot be confirmed.
	Deliver(env DeliverEnv) Port
}

// LaunchArgs are what a vendor's launch line is built from.
type LaunchArgs struct {
	// Cwd is the worktree the session starts in.
	Cwd                       string
	Model, Effort, Disallowed string
	MCPArgs                   []string
	Preamble                  string
	Hide, NoWrite             []string
	MemoryDir                 string
	Trusted                   []string // nil: not asked for
	Chrome                    bool
	SandboxOff                bool
	MCPApproved               []string
	ExtraDirs                 []string
	// ReadAllow are files and folders the session may read with its file tools without asking: the attachments folder and the founder's grants.
	ReadAllow []string
	// AllowWrite and AllowDomains are what a sandboxed session may write beyond its worktrees and reach on the network.
	AllowWrite, AllowDomains []string
	// CacheDir is a sandboxed session's own build caches (claude.CacheEnv), which it may write too.
	CacheDir string
	// WeakerNetwork and Excluded are what the founder allowed the session's repo (team.RepoAllowances).
	WeakerNetwork bool
	Excluded      []string
}

// Hooks is a vendor's state hooks, which launch keeps current.
type Hooks interface {
	EnsureShared() error
	// Missing is whether a wanted event is not installed.
	Missing() bool
	Install(exe string) error
	AllowHomeSocket() error
}
