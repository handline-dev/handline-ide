package codex

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
)

// WebLoginArgv is the device login: it prints a page and a one-time code to type there, and ends when the person has.
func (Account) WebLoginArgv() ([]string, bool) {
	return []string{"codex", "login", "--device-auth"}, true
}

// DeviceCodeLogin: the web login's code is typed at its page, never brought back.
func (Account) DeviceCodeLogin() bool { return true }

// CredentialPresent is a run of `codex login status` under the account's CODEX_HOME once its auth.json is there: exit 0 is logged in,
// "Not logged in" is not, anything else (no codex, a run that fails) cannot tell.
func (a Account) CredentialPresent(configDir string) (present, known bool) {
	if _, err := os.Stat(authFile(configDir)); err != nil {
		return false, true
	}
	exe, err := exec.LookPath("codex")
	if err != nil {
		return false, false
	}
	code, out := agent.Run(20*time.Second, append(os.Environ(), a.ConfigDirEnv()+"="+configDir), exe, "login", "status")
	switch {
	case code == 0:
		return true, true
	case strings.Contains(out, "Not logged in"):
		return false, true
	}
	return false, false
}
