package opencode

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
)

// Account is opencode's account facts; its credential lives under XDG_DATA_HOME.
type Account struct{}

func init() { agent.Register(Account{}) }

func (Account) Name() string          { return "opencode" }
func (Account) ConfigDirName() string { return "xdg-data" }
func (Account) ConfigDirEnv() string  { return "XDG_DATA_HOME" }
func (Account) LoginArgv() []string   { return []string{"opencode", "auth", "login"} }
func (Account) LogoutArgv() []string  { return []string{"opencode", "auth", "logout"} }

// WebLoginArgv: opencode has no login the web page can drive.
func (Account) WebLoginArgv() ([]string, bool) { return nil, false }

// CredentialModified: nothing tells when opencode's credential changed.
func (Account) CredentialModified(string) *float64 { return nil }

// CredentialPresent is a run of `opencode auth list` once its auth file is there.
func (a Account) CredentialPresent(configDir string) (present, known bool) {
	if _, err := os.Stat(filepath.Join(configDir, "opencode", "auth.json")); err != nil {
		return false, true
	}
	exe, err := exec.LookPath("opencode")
	if err != nil {
		return false, true
	}
	_, out := agent.Run(20*time.Second, append(os.Environ(), a.ConfigDirEnv()+"="+configDir), exe, "auth", "list")
	return countsCredentials(out), true
}

var credentialsRe = regexp.MustCompile(`\b(\d+) credentials\b`)

// countsCredentials is whether `opencode auth list` printed a count of credentials above zero.
func countsCredentials(out string) bool {
	m := credentialsRe.FindStringSubmatch(out)
	if m == nil {
		return false
	}
	n, err := strconv.Atoi(m[1])
	return err == nil && n > 0
}
