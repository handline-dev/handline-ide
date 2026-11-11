// Package paths is the layout of the handline home, as handline/infra/paths.py has it.
package paths

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/store"
)

// Getenv reads HANDLINE_<suffix>.
func Getenv(suffix string) (string, bool) {
	return os.LookupEnv("HANDLINE_" + suffix)
}

// UserHome is the user's home, or the test suite's stand-in for it. A test binary never gets the real one: with no TEST_ROOT set it gets
// a folder beside itself, in the build folder `go test` removes after the run, so a test that forgets to set one leaves nothing behind.
func UserHome() string {
	if t, ok := Getenv("TEST_ROOT"); ok && t != "" {
		return t
	}
	if testing.Testing() {
		return testRoot()
	}
	h, _ := os.UserHomeDir()
	return h
}

var testRoot = sync.OnceValue(func() string {
	exe, err := os.Executable()
	if err != nil {
		panic(fmt.Sprintf("a test binary needs a stand-in user home: %v", err))
	}
	d := filepath.Join(filepath.Dir(exe), "user-home")
	if err := os.MkdirAll(d, 0o700); err != nil {
		panic(fmt.Sprintf("a test binary needs a stand-in user home: %v", err))
	}
	return d
})

// Home is the handline home: the environment's, else ~/.handline.
func Home() string {
	if h, ok := Getenv("HOME"); ok && h != "" {
		return h
	}
	return filepath.Join(UserHome(), ".handline")
}

func DBFile(home string) string     { return filepath.Join(home, "db.sqlite") }
func SocketFile(home string) string { return filepath.Join(home, "daemon.sock") }

// Commands is the command, main name first (handline/infra/names.py).
var Commands = []string{"hl", "handline"}

// Setenv sets HANDLINE_<suffix> in env.
func Setenv(env map[string]string, suffix, value string) {
	env["HANDLINE_"+suffix] = value
}

func ConfigFile(home string) string { return filepath.Join(home, "config.yaml") }
func MCPConfig(home string) string  { return filepath.Join(home, "shared", "mcp.json") }

// QuietDir is the folder a claude the daemon starts outside any worktree runs in: empty and its own, under the daemon's home, so a claude
// that walks its working folder finds nothing, and never inherits the daemon's (which may be / or the person's home).
func QuietDir() string {
	dir := filepath.Join(Home(), "run", "claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		dir = filepath.Join(os.TempDir(), "handline-claude")
		os.MkdirAll(dir, 0o755)
	}
	return dir
}

func AccountsDir(home string) string { return filepath.Join(home, "accounts") }
func StatusFile(home, acct string) string {
	return filepath.Join(AccountDir(home, acct), "status.json")
}
func AgentsDir(home string) string { return filepath.Join(home, "agents") }

// RemoveCacheDir removes an agent's build caches. Go keeps its module cache read-only, so folders are made writable first.
func RemoveCacheDir(home, name string) error {
	d := CacheDir(home, name)
	_ = filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
		if err == nil && e.IsDir() {
			_ = os.Chmod(p, 0o755)
		}
		return nil
	})
	return os.RemoveAll(d)
}
func AgentStatusFile(home, name string) string {
	return filepath.Join(AgentDir(home, name), "status.json")
}
func HandoffNoteFile(home, agent string) string {
	return filepath.Join(AgentDir(home, agent), "handoff.md")
}

// HandoffArchiveDir keeps each handoff record an agent was handed with its note in it, one timestamped file per record.
func HandoffArchiveDir(home, agent string) string {
	return filepath.Join(AgentDir(home, agent), "handoffs")
}

// StrayHandoffFile is where a session in a dangling terminal of the agent writes its handoff before the terminal is closed.
func StrayHandoffFile(home, agent string, pid int) string {
	return filepath.Join(AgentDir(home, agent), fmt.Sprintf("handoff-%d.md", pid))
}

// ListAccounts is the accounts, by name: each row whose folder is there. A table that cannot be read lists none.
func ListAccounts(home string) []string {
	d, err := store.SharedIfThere(DBFile(home))
	if d == nil || err != nil {
		return []string{}
	}
	rows, err := d.Query("SELECT name, id FROM accounts ORDER BY name")
	if err != nil {
		return []string{}
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var name string
		var id int64
		if rows.Scan(&name, &id) != nil {
			return []string{}
		}
		if st, err := os.Stat(filepath.Join(home, "accounts", actor.Key(actor.Account, id))); err == nil && st.IsDir() {
			out = append(out, name)
		}
	}
	if rows.Err() != nil {
		return []string{}
	}
	return out
}

// Removing is whether the account is mid-removal: nothing is started on an account about to go.
func Removing(home, acct string) bool {
	_, err := os.Stat(filepath.Join(AccountDir(home, acct), ".removing"))
	return err == nil
}

// ListAgents is the registered agents: a row of the agents table, by name. A listing that cannot be read is an empty one; a caller
// that must tell the two apart uses ReadAgents.
func ListAgents(home string) []string {
	out, _ := ReadAgents(home)
	if out == nil {
		return []string{}
	}
	return out
}

// LegacyNameSQL is team.LegacyName in SQL, over agents a joined to workspaces w.
const LegacyNameSQL = "CASE WHEN w.lead = a.id THEN w.name ELSE w.name || '.' || a.name END"

// ReadAgents is ListAgents with the read errors. A home with no database yet has no agents. Each is named by its old dotted name, as
// team.LegacyName writes it, until the who columns hold keys (base-v2 1b).
func ReadAgents(home string) ([]string, error) {
	d, err := store.SharedIfThere(DBFile(home))
	if d == nil {
		return []string{}, err
	}
	rows, err := d.Query(`SELECT ` + LegacyNameSQL + ` AS n
		FROM agents a JOIN workspaces w ON w.id = a.workspace ORDER BY n`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// HlExe is the command a hook or launch line should call hl with: HANDLINE_BIN, else the first of Commands on PATH.
func HlExe() string {
	if v, ok := Getenv("BIN"); ok && v != "" {
		return v
	}
	for _, c := range Commands {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}
