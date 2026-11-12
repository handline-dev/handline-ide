package reads

import (
	"os/exec"
	"runtime"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
)

// MissingTool is DaemonStatus.missing_tools' item: a tool agents are run with that this machine lacks, and the shell command that installs it.
type MissingTool struct {
	Tool string `json:"tool"`
	Fix  string `json:"fix"`
}

// missingTools is DaemonStatus.missing_tools: none when the daemon runs no agent side. Another vendor's CLI is listed only while
// an account or an agent uses it, so a founder who never picked it never sees it missing.
func (e *Env) missingTools(vendors []VendorStatus) []MissingTool {
	if e.Claude == "" {
		return nil
	}
	out := MissingTools(e.Claude)
	var inUse map[string]bool
	for _, v := range vendors {
		if v.Vendor == team.DefaultVendor || v.Installed {
			continue
		}
		if inUse == nil {
			inUse = e.vendorsInUse()
		}
		if inUse[v.Vendor] {
			out = append(out, MissingTool{v.Binary, v.Fix})
		}
	}
	return out
}

// VendorStatus is DaemonStatus.vendors' item: a vendor agents can run on, its command, whether that is on PATH and how to install it.
type VendorStatus struct {
	Vendor    string `json:"vendor"`
	Binary    string `json:"binary"`
	Installed bool   `json:"installed"`
	Fix       string `json:"fix"`
}

// vendors is DaemonStatus.vendors: every registered vendor CLI, read at each call; claude at the path agents are run with.
func (e *Env) vendors() []VendorStatus {
	var out []VendorStatus
	for _, t := range agent.Tools() {
		look := t.Command
		if t.Vendor == team.DefaultVendor && e.Claude != "" {
			look = e.Claude
		}
		_, err := exec.LookPath(look)
		out = append(out, VendorStatus{t.Vendor, t.Command, err == nil, t.Fix})
	}
	return out
}

// vendorsInUse is the vendors an account or an agent runs on.
func (e *Env) vendorsInUse() map[string]bool {
	in := map[string]bool{}
	for _, acct := range paths.ListAccounts(e.Home) {
		in[team.AccountVendor(e.Home, acct)] = true
	}
	roster := team.NewRosterOn(e.DB, e.Home)
	for _, a := range e.listAgents() {
		if cfg, err := roster.Read(a); err == nil {
			v, _, _ := team.Parts(cfg)
			in[v] = true
		}
	}
	return in
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
