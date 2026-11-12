package agent

import (
	"context"
	"os/exec"
	"sort"
	"sync"
	"time"
)

// Account is what an agent CLI decides about an account of its own: where its config and credential live and how a login is told.
type Account interface {
	Name() string
	// ConfigDirName is the folder under an account that ConfigDirEnv names.
	ConfigDirName() string
	// ConfigDirEnv is the variable that moves the credential.
	ConfigDirEnv() string
	// LoginArgv is the command a human runs to log in.
	LoginArgv() []string
	// WebLoginArgv is the command the daemon runs for a login the web page drives, with no terminal; ok is false for a CLI with none.
	WebLoginArgv() (argv []string, ok bool)
	// CredentialPresent is whether the config dir holds a login; known is false when this process cannot tell (a sandboxed one
	// cannot see the keychain).
	CredentialPresent(configDir string) (present, known bool)
	// CredentialModified is when the credential last changed, as epoch seconds, never its content; nil when it cannot be told.
	CredentialModified(configDir string) *float64
}

// Sharable is an Account whose login is one file another config dir can link to (codex's auth.json); a CLI whose login is keyed by
// its config dir (claude's keychain entry) is not one.
type Sharable interface {
	CredentialFile() string
}

var (
	accountsMu sync.Mutex
	accounts   = map[string]Account{}
)

// Register makes a CLI's account facts findable by its name; each implementation does so as it loads.
func Register(a Account) {
	accountsMu.Lock()
	accounts[a.Name()] = a
	accountsMu.Unlock()
}

// Lookup is the account facts of the CLI named name.
func Lookup(name string) (Account, bool) {
	accountsMu.Lock()
	defer accountsMu.Unlock()
	a, ok := accounts[name]
	return a, ok
}

// Names is the registered CLIs, sorted.
func Names() []string {
	accountsMu.Lock()
	defer accountsMu.Unlock()
	out := make([]string, 0, len(accounts))
	for n := range accounts {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Run is a subprocess's exit code (-1 when it could not run) and output, with a limit. env nil is this process's.
func Run(limit time.Duration, env []string, argv ...string) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = RunDir()
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.Output()
	if err == nil {
		return 0, string(out)
	}
	if ee, ok := err.(*exec.ExitError); ok && ctx.Err() == nil {
		return ee.ExitCode(), string(out)
	}
	return -1, string(out)
}

var (
	agentsMu sync.Mutex
	agents   = map[string]func(Host) Agent{}
)

// RegisterAgent makes a CLI's launchable Agent findable by its name; each implementation does so as it loads.
func RegisterAgent(name string, make func(Host) Agent) {
	agentsMu.Lock()
	agents[name] = make
	agentsMu.Unlock()
}

// New is the Agent of the CLI named vendor for the program h; ok is false when that CLI cannot be launched.
func New(vendor string, h Host) (Agent, bool) {
	agentsMu.Lock()
	make, ok := agents[vendor]
	agentsMu.Unlock()
	if !ok {
		return nil, false
	}
	return make(h), true
}
