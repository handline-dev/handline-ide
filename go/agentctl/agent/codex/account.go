// Package codex is the Codex CLI vendor (docs/design/codex-support.md): an account keeps its own CODEX_HOME. It registers no
// launchable Agent yet, so a codex agent is refused at launch until its launch line, screen and hooks are built.
package codex

import (
	"os"
	"path/filepath"

	"github.com/handline/handline/go/agentctl/agent"
)

// Vendor is the vendor name account.add takes.
const Vendor = "codex"

// Account is codex's account facts.
type Account struct{}

func init() {
	agent.Register(Account{})
	agent.RegisterTool(agent.Tool{Vendor: Vendor, Command: "codex", Fix: "npm install -g @openai/codex"})
}

func (Account) Name() string          { return Vendor }
func (Account) ConfigDirName() string { return ".codex" }
func (Account) ConfigDirEnv() string  { return "CODEX_HOME" }
func (Account) LoginArgv() []string   { return []string{"codex", "login"} }
func (Account) LogoutArgv() []string  { return []string{"codex", "logout"} }

// authFile is where codex keeps a login under its home.
func authFile(configDir string) string { return filepath.Join(configDir, "auth.json") }

// CredentialModified is when auth.json last changed, nil when there is none.
func (Account) CredentialModified(configDir string) *float64 {
	fi, err := os.Stat(authFile(configDir))
	if err != nil {
		return nil
	}
	t := float64(fi.ModTime().UnixNano()) / 1e9
	return &t
}
