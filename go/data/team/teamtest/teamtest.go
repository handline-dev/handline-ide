// Package teamtest seeds a test home's workspaces and agents as rows, named by their old dotted names (team.LegacyName): a bare name is
// a workspace's lead, ws.agent an agent of workspace ws.
package teamtest

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
)

// DB is home's database, made from the test template (with the owner, p1) when it has none yet. Its shared handle is closed when the
// test ends.
func DB(t testing.TB, home string) *sql.DB {
	t.Helper()
	path := paths.DBFile(home)
	if _, err := os.Stat(path); err != nil {
		d, err := storetest.OpenAt(t, path)
		if err != nil {
			t.Fatal(err)
		}
		d.Close()
	}
	d, err := store.Shared(path)
	if err != nil {
		t.Fatal(err)
	}
	// Closed once, by the first caller's cleanup, so after the cleanups a rig registered after it (its waits for work still
	// running) have run.
	if _, again := closing.LoadOrStore(path, true); !again {
		t.Cleanup(func() { store.ForgetShared(path); closing.Delete(path) })
	}
	return d
}

// closing is the homes whose database a test closes at its end.
var closing sync.Map

// Seed registers each agent in home, leads first, with the defaults a new agent gets.
func Seed(t testing.TB, home string, agents ...string) *sql.DB {
	t.Helper()
	d := DB(t, home)
	sorted := append([]string(nil), agents...)
	sort.SliceStable(sorted, func(i, j int) bool { return !strings.Contains(sorted[i], ".") && strings.Contains(sorted[j], ".") })
	for _, n := range sorted {
		if err := add(d, n); err != nil {
			t.Fatalf("seed %s: %v", n, err)
		}
		if err := os.MkdirAll(paths.AgentDir(home, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func add(d *sql.DB, legacy string) error {
	ws, agent := team.SplitLegacy(legacy)
	if _, err := d.Exec("INSERT OR IGNORE INTO workspaces(name, folder, created_at) VALUES(?, '', 0)", ws); err != nil {
		return err
	}
	var wsID int64
	var lead sql.NullInt64
	if err := d.QueryRow("SELECT id, lead FROM workspaces WHERE name = ?", ws).Scan(&wsID, &lead); err != nil {
		return err
	}
	isLead := agent == ""
	if isLead {
		if lead.Valid {
			return nil
		}
		agent = "lead"
	}
	reportsTo := "p1"
	if !isLead && lead.Valid {
		reportsTo = fmt.Sprintf("a%d", lead.Int64)
	}
	res, err := d.Exec("INSERT OR IGNORE INTO agents(workspace, name, owner, reports_to, model, effort, weight, created_at) VALUES(?, ?, 'p1', ?, 'sonnet', 'medium', 'light', 0)",
		wsID, agent, reportsTo)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 && isLead {
		id, _ := res.LastInsertId()
		_, err = d.Exec("UPDATE workspaces SET lead = ? WHERE id = ?", id, wsID)
	}
	return err
}

// Set seeds agent (when it is not there yet) and sets its settings, given as key, value pairs of agents columns: repo is its
// workspace's folder, role its task role, reports_to the old name of the agent it reports to; a nil value is the default.
func Set(t testing.TB, home, agent string, kv ...any) {
	t.Helper()
	d := Seed(t, home, agent)
	if len(kv)%2 != 0 {
		t.Fatalf("teamtest.Set %s: an odd number of key, value arguments", agent)
	}
	ws, name := team.SplitLegacy(agent)
	where := "workspace = (SELECT id FROM workspaces WHERE name = ?) AND name = ?"
	args := []any{ws, name}
	if name == "" {
		where = "id = (SELECT lead FROM workspaces WHERE name = ?)"
		args = []any{ws}
	}
	for i := 0; i < len(kv); i += 2 {
		k, v := kv[i].(string), kv[i+1]
		if b, ok := v.(bool); ok {
			v = map[bool]int{true: 1, false: 0}[b]
		}
		var err error
		switch k {
		case "repo":
			_, err = d.Exec("UPDATE workspaces SET folder = ? WHERE name = ?", v, ws)
		case "reports_to":
			if v == nil {
				_, err = d.Exec("UPDATE agents SET reports_to = NULL WHERE "+where, args...)
				break
			}
			boss, bossAgent := team.SplitLegacy(v.(string))
			q := "(SELECT a.id FROM agents a JOIN workspaces w ON w.id = a.workspace WHERE w.name = ? AND a.name = ?)"
			bossArgs := []any{boss, bossAgent}
			if bossAgent == "" {
				q, bossArgs = "(SELECT lead FROM workspaces WHERE name = ?)", []any{boss}
			}
			_, err = d.Exec("UPDATE agents SET reports_to = 'a' || "+q+" WHERE "+where, append(bossArgs, args...)...)
		default:
			_, err = d.Exec("UPDATE agents SET "+k+" = ? WHERE "+where, append([]any{v}, args...)...)
		}
		if err != nil {
			t.Fatalf("teamtest.Set %s %s: %v", agent, k, err)
		}
	}
}

// AccountDir gives the account named name a row in home's database, when it has none, and is its folder (paths.AccountDir), which
// the caller makes.
func AccountDir(t testing.TB, home, name string) string {
	t.Helper()
	if _, err := DB(t, home).Exec("INSERT INTO accounts(name, owner, created_at) VALUES(?, 'p1', 0) ON CONFLICT(name) DO NOTHING", name); err != nil {
		t.Fatal(err)
	}
	return paths.AccountDir(home, name)
}

// Account gives the account named name a row in home's database, when it has none, and is name: its folder is then by key.
func Account(t testing.TB, home, name string) string {
	t.Helper()
	AccountDir(t, home, name)
	return name
}
