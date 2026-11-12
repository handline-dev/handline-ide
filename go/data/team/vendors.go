package team

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/handline/handline/go/agentctl/agent"
	_ "github.com/handline/handline/go/data/agenthost" // sets the user home and run dir agent.Run and the credential checks use
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// The vendors a driver exists for (handline/vendors/__init__.py). Only what the account and agent files need is here: the
// name, the folder an account keeps its config in, how a login is told, how it is started.
const DefaultVendor = "claude"

// KnownVendors is the vendors with a driver, sorted: the agent CLIs that registered with package agent.
func KnownVendors() []string { return agent.Names() }

// UnknownVendor is the refusal for a vendor with no driver.
type UnknownVendor struct{ Vendor string }

func (e *UnknownVendor) Error() string {
	return fmt.Sprintf("no driver for vendor %q; known: %s", e.Vendor, strings.Join(KnownVendors(), ", "))
}

// CheckVendor is nil for a vendor with a driver.
func CheckVendor(vendor string) error {
	for _, v := range KnownVendors() {
		if v == vendor {
			return nil
		}
	}
	return &UnknownVendor{vendor}
}

// facts is the account facts of the vendor's agent CLI; a vendor with none is asked as the default is, as it always was.
func facts(vendor string) agent.Account {
	a, ok := agent.Lookup(vendor)
	if !ok {
		a, _ = agent.Lookup(DefaultVendor)
	}
	return a
}

// ConfigDirName is the folder under an account that the vendor's config-dir variable names.
func ConfigDirName(vendor string) string { return facts(vendor).ConfigDirName() }

// ConfigDirEnv is the variable that moves the credential.
func ConfigDirEnv(vendor string) string { return facts(vendor).ConfigDirEnv() }

// LoginArgv is the command a human runs to log in.
func LoginArgv(vendor string) []string { return facts(vendor).LoginArgv() }

// WebLoginArgv is the command the daemon runs for a login the web page drives, with no terminal. ok is false for a vendor with no such login.
func WebLoginArgv(vendor string) (argv []string, ok bool) { return facts(vendor).WebLoginArgv() }

// WebLoginGivesCode is whether the vendor's web login prints a one-time code for the person to type at its URL (agent.DeviceCodeLogin).
func WebLoginGivesCode(vendor string) bool {
	d, ok := facts(vendor).(agent.DeviceCodeLogin)
	return ok && d.DeviceCodeLogin()
}

// AccountVendor is the vendor an account is an account of. An account that records none predates vendors and is the default's;
// a file that is there but empty is not a vendor, so it names none and every driver lookup refuses it.
func AccountVendor(home, acct string) string {
	b, err := os.ReadFile(filepath.Join(paths.AccountDir(home, acct), "vendor"))
	if err != nil {
		return DefaultVendor
	}
	return strings.TrimSpace(string(b))
}

// PinFor is the account a new agent of vendor is pinned to: the vendor's only account, for a vendor other than the default. "" for the
// default vendor, or with no or several accounts of the vendor.
func PinFor(home, vendor string) string {
	if vendor == "" || vendor == DefaultVendor {
		return ""
	}
	pin := ""
	for _, a := range paths.ListAccounts(home) {
		if AccountVendor(home, a) != vendor {
			continue
		}
		if pin != "" {
			return ""
		}
		pin = a
	}
	return pin
}

// AccountConfigDir is the directory the account's config-dir variable names; an error when its vendor has no driver.
func AccountConfigDir(home, acct string) (string, error) {
	v := AccountVendor(home, acct)
	if err := CheckVendor(v); err != nil {
		return "", err
	}
	if real, ok := paths.Getenv(LiveAccounts); ok && real != "" && inTestRoot() {
		key, err := paths.AccountKeyReadOnly(real, acct) // the real home is read, never opened for writing
		if err != nil || key == "" {
			return "", fmt.Errorf("live account %s: not in %s (%v)", acct, real, err)
		}
		return filepath.Join(paths.AccountsDir(real), key, ConfigDirName(v)), nil
	}
	return filepath.Join(paths.AccountDir(home, acct), ConfigDirName(v)), nil
}

// LiveAccounts (HANDLINE_LIVE_ACCOUNTS) is the real home a live run takes its accounts' config dirs from: the keychain keeps a login
// under its config dir's exact path, so a scratch home's own path has none. Read only under a test root.
const LiveAccounts = "LIVE_ACCOUNTS"

func inTestRoot() bool { r, _ := paths.Getenv("TEST_ROOT"); return r != "" }

// CredentialPresent is whether the account's agent CLI finds a credential in configDir; known is false when this process cannot tell.
func CredentialPresent(vendor, configDir string) (present, known bool) {
	return facts(vendor).CredentialPresent(configDir)
}

func isDarwin() bool { return runtime.GOOS == "darwin" }

// yamlFloat is a float as Python's json.dumps writes it.
func yamlFloat(f float64) string { return yamlcompat.FloatRepr(f) }
