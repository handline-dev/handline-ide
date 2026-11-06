package claudecode

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
)

func init() {
	agent.Register(Driver{})
	agent.RegisterAgent("claude", func(h agent.Host) agent.Agent { return Driver{h} })
}

func (Driver) ConfigDirName() string { return ".claude" }
func (Driver) ConfigDirEnv() string  { return "CLAUDE_CONFIG_DIR" }
func (Driver) LoginArgv() []string   { return []string{"claude", "/login"} }

// WebLoginArgv prints the authorize URL and reads the code on stdin.
func (Driver) WebLoginArgv() ([]string, bool) { return []string{"claude", "auth", "login"}, true }

// KeychainService is the macOS Keychain service name Claude Code uses for a config dir: the default ~/.claude uses
// "Claude Code-credentials", any other dir that name plus the first 8 hex of the sha256 of its path.
func KeychainService(configDir string) string {
	if configDir == filepath.Join(agent.UserHome(), ".claude") {
		return "Claude Code-credentials"
	}
	h := sha256.Sum256([]byte(configDir))
	return "Claude Code-credentials-" + hex.EncodeToString(h[:])[:8]
}

// keychainAccountRe is the names Claude Code uses verbatim; others become "claude-code-user".
var keychainAccountRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// keychainFind is the security command that finds the item an agent's claude reads: the account's service under the user's name.
func keychainFind(configDir string) []string {
	name := agent.UserName()
	if !keychainAccountRe.MatchString(name) {
		name = "claude-code-user"
	}
	return []string{"security", "find-generic-password", "-a", name, "-s", KeychainService(configDir)}
}

// CredentialPresent is a file's existence or, on macOS, the keychain item's presence, asked without -w so the secret is never read.
func (Driver) CredentialPresent(configDir string) (present, known bool) {
	if _, err := os.Stat(filepath.Join(configDir, ".credentials.json")); err == nil {
		return true, true // file-backed storage (Linux, and macOS installs that opt out of Keychain)
	}
	if runtime.GOOS != "darwin" {
		return false, true
	}
	rc, _ := agent.Run(10*time.Second, nil, keychainFind(configDir)...)
	if rc == -1 {
		return false, true
	}
	if rc != 0 && os.Getenv("SANDBOX_RUNTIME") != "" {
		return false, false
	}
	return rc == 0, true
}

var mdatRe = regexp.MustCompile(`"mdat"<timedate>=\S+\s+"(\d{14})Z`)

func (Driver) CredentialModified(configDir string) *float64 {
	if st, err := os.Stat(filepath.Join(configDir, ".credentials.json")); err == nil {
		v := float64(st.ModTime().UnixNano()) / 1e9
		return &v
	}
	if runtime.GOOS != "darwin" {
		return nil
	}
	_, out := agent.Run(10*time.Second, nil, keychainFind(configDir)...)
	m := mdatRe.FindStringSubmatch(out)
	if m == nil {
		return nil
	}
	t, err := time.ParseInLocation("20060102150405", m[1], time.UTC)
	if err != nil {
		return nil
	}
	v := float64(t.Unix())
	return &v
}
