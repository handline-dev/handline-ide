// Package codex is the Codex CLI vendor (docs/design/codex-support.md): an account keeps its own CODEX_HOME.
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

// credentialFile is the file codex keeps a login in under its home.
const credentialFile = "auth.json"

// CredentialFile makes the login one file another config dir can link to (account.adopt).
func (Account) CredentialFile() string { return credentialFile }

// authFile is where codex keeps a login under its home.
func authFile(configDir string) string { return filepath.Join(configDir, credentialFile) }

// CredentialModified is when auth.json last changed, nil when there is none.
func (Account) CredentialModified(configDir string) *float64 {
	fi, err := os.Stat(authFile(configDir))
	if err != nil {
		return nil
	}
	t := float64(fi.ModTime().UnixNano()) / 1e9
	return &t
}
