// Command hl-import-v1 writes the new home from the old one, once, at the cutover to the new base (docs/design/people-workspaces.md,
// No migration). It is run by hand on a stopped daemon's home moved aside, is linked into nothing else, and is deleted a week after.
//
//	hl-import-v1 [-from ~/.handline-v1] [-to ~/.handline]
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/yamlcompat"
	_ "modernc.org/sqlite"
)

func main() {
	home, _ := os.UserHomeDir()
	from := flag.String("from", filepath.Join(home, ".handline-v1"), "the old home, its daemon stopped")
	to := flag.String("to", filepath.Join(home, ".handline"), "the new home: absent or empty")
	owner := flag.String("owner-name", "", "the owner's display name (default: the Mac account's full name)")
	flag.Parse()
	if *owner == "" {
		*owner = macName()
	}
	// a login is kept under its folder's exact path, so the paths are absolute
	src, err1 := filepath.Abs(*from)
	dst, err2 := filepath.Abs(*to)
	if err := errors.Join(err1, err2); err != nil {
		fmt.Fprintln(os.Stderr, "hl-import-v1:", err)
		os.Exit(1)
	}
	if err := run(src, dst, *owner, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "hl-import-v1:", err)
		os.Exit(1)
	}
}

// macName is the Mac account's full name, as the daemon names the owner at a first start.
func macName() string {
	if out, err := exec.Command("id", "-F").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		return strings.TrimSpace(string(out))
	}
	return "Owner"
}

// carried are the old home's entries copied as they are; everything else not rebuilt here is left behind and listed.
var carried = []string{"shared", "attachments", "browser", "proposals", "trust_proposals"}

// rebuilt are the old home's entries the import writes anew, or leaves behind on purpose (logs, sockets, the old databases).
var rebuilt = []string{"db.sqlite", "db.sqlite-wal", "db.sqlite-shm", "agents", "accounts", "config.yaml", "team.yaml", "daemon.sock", "logs", "run", "locks"}

// who are the columns that name a who, by table: a name becomes its key. bodies are the texts whose @name becomes <@key>.
var (
	who = map[string][]string{
		"tasks":      {"owner", "opened_by", "edited_by", "closed_by", "discarded_by", "queued_by", "reporter", "accepted_by"},
		"task_memos": {"author"}, "task_turns": {"agent", "pushed_by"}, "requests": {"requester", "target", "no_work_by"},
		"memo_tells": {"reader"}, "task_views": {"updated_by"}, "prefs": {"person"},
	}
	bodies = map[string][]string{"task_memos": {"body"}, "requests": {"body", "response"}}
	// tables are copied in this order, every row with its id.
	tables = []string{"tasks", "task_memos", "task_turns", "task_blocks", "task_tags", "task_requests", "requests", "memo_tells",
		"task_views", "prefs", "devices"}
)

type importer struct {
	from, to string
	old      *sql.DB
	tx       *sql.Tx
	owner    string
	keys     map[string]string         // an old name (dotted, or a person's or the daemon's word) -> its key
	accts    map[string]string         // an account's name -> its key
	wss      map[string]int64          // a workspace's name -> its id
	unmapped map[string]map[string]int // table.column -> a who word that names no row now -> how many rows hold it
	out      *os.File
}

func run(from, to, ownerName string, out *os.File) error {
	if err := preflight(from, to); err != nil {
		return err
	}
	old, err := sql.Open("sqlite", "file:"+filepath.Join(from, "db.sqlite")+"?mode=ro")
	if err != nil {
		return err
	}
	defer old.Close()
	d, err := store.Open(filepath.Join(to, "db.sqlite"))
	if err != nil {
		return err
	}
	defer d.Close()
	owner, err := actor.EnsureOwner(d, func() string { return ownerName })
	if err != nil {
		return err
	}
	// One connection with foreign keys off, so a task's parent or merge target may come after it.
	conn, err := d.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	im := &importer{from: from, to: to, old: old, tx: tx, owner: owner, out: out, unmapped: map[string]map[string]int{},
		accts: map[string]string{}, wss: map[string]int64{},
		keys: map[string]string{"human": owner, "founder": owner, "task-board": actor.Sys, "land": actor.Sys, "schedule": actor.Sys, actor.Gate: actor.Gate}}
	steps := []struct {
		name string
		f    func() error
	}{
		{"agents", im.agents}, {"accounts", im.accounts}, {"rows", im.rows}, {"agent state", im.agentState},
	}
	for _, s := range steps {
		if err := s.f(); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	if err := im.foreignKeys(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := im.files(); err != nil {
		return fmt.Errorf("files: %w (the database is written: remove %s and run again)", err, to)
	}
	im.report()
	return nil
}

// preflight refuses an old home whose daemon still answers, or a new home that holds anything.
func preflight(from, to string) error {
	if _, err := os.Stat(filepath.Join(from, "db.sqlite")); err != nil {
		return fmt.Errorf("no old home at %s: %w", from, err)
	}
	if c, err := net.Dial("unix", filepath.Join(from, "daemon.sock")); err == nil {
		c.Close()
		return fmt.Errorf("the daemon of %s still answers: stop it first", from)
	}
	ents, err := os.ReadDir(to)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(ents) > 0 {
		return fmt.Errorf("%s is not empty: the import writes a new home", to)
	}
	return nil
}

// oldAgent is one registered agent of the old home: a folder of agents/ with an agent.yaml.
type oldAgent struct {
	name, ws, agent string
	cfg             map[string]any
}

// agents writes a workspace for each first segment of the old names and an agent row for each registered agent, the lead (the
// agent named just the segment) as lead; reports to is today's parent name.
func (im *importer) agents() error {
	olds, err := im.readAgents()
	if err != nil {
		return err
	}
	roles, err := teamRoles(im.from)
	if err != nil {
		return err
	}
	cols, err := columns(im.tx, "agents")
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	byWS := map[string][]oldAgent{}
	for _, a := range olds {
		byWS[a.ws] = append(byWS[a.ws], a)
	}
	wss := make([]string, 0, len(byWS))
	for ws := range byWS {
		wss = append(wss, ws)
	}
	sort.Strings(wss)
	for _, ws := range wss {
		group := byWS[ws]
		sort.Slice(group, func(i, j int) bool { // the lead first, then by name
			if (group[i].agent == "") != (group[j].agent == "") {
				return group[i].agent == ""
			}
			return group[i].agent < group[j].agent
		})
		if err := names.Check(im.tx, names.Workspace, ws, 0); err != nil {
			return fmt.Errorf("workspace %s: %w", ws, err)
		}
		folder, _ := group[0].cfg["repo"].(string)
		if group[0].agent != "" {
			fmt.Fprintf(im.out, "workspace %s has no lead: its folder %q is %s's\n", ws, folder, group[0].name)
		}
		res, err := im.tx.Exec("INSERT INTO workspaces(name, folder, created_at) VALUES(?, ?, ?)", ws, expandHome(folder), now)
		if err != nil {
			return err
		}
		wid, _ := res.LastInsertId()
		im.wss[ws] = wid
		for _, a := range group {
			name := a.agent
			if name == "" {
				name = names.Lead
			}
			if err := names.Check(im.tx, names.Agent, name, wid); err != nil {
				return fmt.Errorf("agent %s: %w", a.name, err)
			}
			role := team.RoleWork
			if roles[a.name] == team.RoleCheck {
				role = team.RoleCheck
			}
			res, err := im.tx.Exec("INSERT INTO agents(workspace, name, owner, role, created_at) VALUES(?, ?, ?, ?, ?)", wid, name, im.owner, role, now)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			im.keys[a.name] = actor.Key(actor.Agent, id)
			if a.agent == "" {
				if _, err := im.tx.Exec("UPDATE workspaces SET lead = ? WHERE id = ?", id, wid); err != nil {
					return err
				}
			}
			if err := im.settings(id, a, cols); err != nil {
				return fmt.Errorf("agent %s: %w", a.name, err)
			}
		}
	}
	return nil
}

// settings writes each agent.yaml key that is an agents column; the rest are said in the report.
func (im *importer) settings(id int64, a oldAgent, cols []string) error {
	fixed := []string{"id", "workspace", "name", "owner", "role", "created_at"}
	keys := make([]string, 0, len(a.cfg))
	for k := range a.cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == "name" || k == "repo" {
			continue
		}
		if !slices.Contains(cols, k) || slices.Contains(fixed, k) {
			fmt.Fprintf(im.out, "agent %s: setting %s=%v is no column, not brought\n", a.name, k, a.cfg[k])
			continue
		}
		if a.cfg[k] == nil && k == "enabled" {
			continue
		}
		v, err := team.Setting(k, a.cfg[k])
		if err != nil {
			return err
		}
		// k is a column of the agents table, never input text.
		if _, err := im.tx.Exec("UPDATE agents SET "+k+" = ? WHERE id = ?", v, id); err != nil {
			return err
		}
	}
	return nil
}

func (im *importer) readAgents() ([]oldAgent, error) {
	ents, err := os.ReadDir(filepath.Join(im.from, "agents"))
	if err != nil {
		return nil, err
	}
	var out []oldAgent
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(im.from, "agents", e.Name(), "agent.yaml"))
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(im.out, "agents/%s has no agent.yaml: no agent, left behind\n", e.Name())
			continue
		}
		if err != nil {
			return nil, err
		}
		v, err := yamlcompat.Parse(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s/agent.yaml: %w", e.Name(), err)
		}
		cfg, _ := v.(map[string]any)
		ws, agent := team.SplitLegacy(e.Name())
		if strings.Contains(agent, ".") {
			return nil, fmt.Errorf("%s: a name of three segments has no place in a workspace", e.Name())
		}
		out = append(out, oldAgent{name: e.Name(), ws: ws, agent: agent, cfg: cfg})
	}
	return out, nil
}

// teamRoles is each agent's role in team.yaml's agents map ("" for none).
func teamRoles(home string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(home, "team.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := yamlcompat.Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("team.yaml: %w", err)
	}
	out := map[string]string{}
	top, _ := v.(map[string]any)
	agents, _ := top["agents"].(map[string]any)
	for name, e := range agents {
		m, _ := e.(map[string]any)
		out[name], _ = m["role"].(string)
	}
	return out, nil
}

// accounts writes a row for each account folder, its label its old name, with its settings from config.yaml.
func (im *importer) accounts() error {
	ents, err := os.ReadDir(filepath.Join(im.from, "accounts"))
	if err != nil {
		return err
	}
	cfg, err := im.oldConfig()
	if err != nil {
		return err
	}
	sets := map[string]map[string]any{}
	for k, v := range cfg {
		if acct, col := usage.AccountSetting(k); acct != "" {
			if sets[acct] == nil {
				sets[acct] = map[string]any{}
			}
			sets[acct][col] = v
		}
	}
	for acct := range sets {
		if _, err := os.Stat(filepath.Join(im.from, "accounts", acct)); err != nil {
			fmt.Fprintf(im.out, "config.yaml: settings of account %s, which has no folder, not brought\n", acct)
		}
	}
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		set := sets[e.Name()]
		if set == nil {
			set = map[string]any{}
		}
		if _, ok := set["label"]; !ok {
			set["label"] = e.Name()
		}
		if err := usage.SetAccount(im.tx, e.Name(), set); err != nil {
			return err
		}
		var id int64
		if err := im.tx.QueryRow("SELECT id FROM accounts WHERE name = ?", e.Name()).Scan(&id); err != nil {
			return err
		}
		im.accts[e.Name()] = actor.Key(actor.Account, id)
	}
	return nil
}

func (im *importer) oldConfig() (map[string]any, error) {
	b, err := os.ReadFile(filepath.Join(im.from, "config.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := yamlcompat.Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("config.yaml: %w", err)
	}
	m, _ := v.(map[string]any)
	return m, nil
}

// rows copies each of tables, every row with its id: names in who columns become keys and @name in bodies <@key>.
func (im *importer) rows() error {
	for _, t := range tables {
		n, err := im.copyTable(t)
		if err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
		fmt.Fprintf(im.out, "%s: %d rows\n", t, n)
	}
	return nil
}

func (im *importer) copyTable(table string) (int, error) {
	oldCols, err := columns(im.old, table)
	if err != nil {
		return 0, err
	}
	newCols, err := columns(im.tx, table)
	if err != nil {
		return 0, err
	}
	var cols []string
	for _, c := range oldCols {
		if slices.Contains(newCols, c) {
			cols = append(cols, c)
		}
	}
	extra := map[string]any{}
	if table == "devices" && !slices.Contains(cols, "person") {
		extra["person"] = im.owner // personal mode: every paired device is the owner's
	}
	ins := append(slices.Clone(cols), keysOf(extra)...)
	stmt, err := im.tx.Prepare("INSERT INTO " + table + "(" + strings.Join(ins, ", ") + ") VALUES(" +
		strings.TrimSuffix(strings.Repeat("?, ", len(ins)), ", ") + ")")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	rows, err := im.old.Query("SELECT " + strings.Join(cols, ", ") + " FROM " + table)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return n, err
		}
		for i, c := range cols {
			if vals[i], err = im.fix(table, c, vals[i]); err != nil {
				return n, err
			}
		}
		for _, k := range keysOf(extra) {
			vals = append(vals, extra[k])
		}
		if _, err := stmt.Exec(vals...); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// fix is one old value as the new row holds it.
func (im *importer) fix(table, col string, v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return v, nil
	}
	switch {
	case slices.Contains(who[table], col):
		return im.key(table+"."+col, s), nil
	case slices.Contains(bodies[table], col):
		return im.mentions(s), nil
	case table == "prefs" && col == "nav":
		return im.nav(s)
	}
	return v, nil
}

// key is the key word names now: a registered agent's, the owner's or the daemon's. A word that names no one now (an agent gone
// before the cutover) stays as it is, written out as it is, and is counted for the report.
func (im *importer) key(at, word string) string {
	if k, ok := im.keys[word]; ok && word != "" {
		return k
	}
	if word != "" {
		if im.unmapped[at] == nil {
			im.unmapped[at] = map[string]int{}
		}
		im.unmapped[at][word]++
	}
	return word
}

// mentions is text with each @name of a key as <@key>; any other @word stays text.
func (im *importer) mentions(text string) string {
	if !strings.Contains(text, "@") {
		return text
	}
	return addr.MentionRe.ReplaceAllStringFunc(text, func(m string) string {
		s := addr.MentionRe.FindStringSubmatch(m)
		word := strings.TrimRight(s[2], ".-") // a sentence's full stop is not part of the name
		k, ok := im.keys[word]
		if !ok {
			return m
		}
		return s[1] + "<@" + k + ">" + s[2][len(word):]
	})
}

// nav is the person's view preferences with each agent named by its key; a name that is no agent now is dropped, as the daemon
// refuses a pin it does not know.
func (im *importer) nav(text string) (string, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return "", fmt.Errorf("prefs.nav: %w", err)
	}
	for _, f := range []string{"pins", "order"} { // hidden holds status words, not agents
		list, ok := m[f].([]any)
		if !ok {
			continue
		}
		out := []any{}
		for _, v := range list {
			if k, ok := im.keys[fmt.Sprint(v)]; ok && actor.KindOf(k) == actor.Agent {
				out = append(out, k)
			} else {
				fmt.Fprintf(im.out, "prefs.nav.%s: %v names no agent now, dropped\n", f, v)
			}
		}
		m[f] = out
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// agentState brings each agent's worktrees, so a worktree made before keeps its name and its work; the rest of the state starts
// over.
func (im *importer) agentState() error {
	rows, err := im.old.Query("SELECT agent, COALESCE(worktree, ''), extra_worktrees FROM agent_state")
	if err != nil {
		return err
	}
	defer rows.Close()
	now := time.Now().Unix()
	for rows.Next() {
		var name, wt string
		var extra sql.NullString
		if err := rows.Scan(&name, &wt, &extra); err != nil {
			return err
		}
		k, ok := im.keys[name]
		if !ok || actor.KindOf(k) != actor.Agent || wt == "" && !extra.Valid {
			continue
		}
		if _, err := im.tx.Exec("INSERT INTO agent_state(agent, state, updated, worktree, extra_worktrees) VALUES(?, 'IDLE', ?, NULLIF(?, ''), ?)",
			k, now, wt, extra); err != nil {
			return err
		}
	}
	return rows.Err()
}

// foreignKeys refuses rows that point at a row the import did not bring.
func (im *importer) foreignKeys() error {
	rows, err := im.tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	var bad []string
	for rows.Next() {
		var table, parent string
		var rowid, fkid sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return err
		}
		bad = append(bad, fmt.Sprintf("%s row %d -> %s", table, rowid.Int64, parent))
	}
	if len(bad) > 0 {
		return fmt.Errorf("rows point at rows not brought: %s", strings.Join(bad, "; "))
	}
	return rows.Err()
}

// files copies the folders by key, each workspace's team rules to its base.md, config.yaml without the account settings (they are
// rows now) and the carried entries. Copies are APFS clones, so the old home stays whole to read from.
func (im *importer) files() error {
	for name, k := range im.keys {
		if actor.KindOf(k) != actor.Agent {
			continue
		}
		if err := clone(filepath.Join(im.from, "agents", name), filepath.Join(im.to, "agents", k)); err != nil {
			return err
		}
	}
	for name, k := range im.accts {
		if err := clone(filepath.Join(im.from, "accounts", name), filepath.Join(im.to, "accounts", k)); err != nil {
			return err
		}
		// v1 ran from <to>, so its login is keyed by <to>/accounts/<name>
		was := filepath.Join(im.to, "accounts", name, ".claude")
		if found, err := copyLogin(was, filepath.Join(im.to, "accounts", k, ".claude")); err != nil {
			return fmt.Errorf("account %s: %w", name, err)
		} else if !found {
			fmt.Fprintf(im.out, "account %s: no Keychain login under %s: it signs in again\n", name, was)
		}
	}
	if err := im.rules(); err != nil {
		return err
	}
	if err := im.config(); err != nil {
		return err
	}
	for _, e := range carried {
		if err := clone(filepath.Join(im.from, e), filepath.Join(im.to, e)); err != nil {
			return err
		}
	}
	// the old team rules are each workspace's base.md now
	return os.RemoveAll(filepath.Join(im.to, "shared", "team"))
}

func (im *importer) rules() error {
	for name, id := range im.wss {
		b, err := os.ReadFile(filepath.Join(im.from, "shared", "team", name+".md"))
		if errors.Is(err, os.ErrNotExist) {
			continue // the daemon seeds the house rules for a workspace with none
		}
		if err != nil {
			return err
		}
		dir := filepath.Join(im.to, "workspaces", actor.Key(actor.Workspace, id))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "base.md"), b, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, ".seeded"), nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// config writes config.yaml without its per-account lines, which the daemon reads from the rows only.
func (im *importer) config() error {
	b, err := os.ReadFile(filepath.Join(im.from, "config.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var keep []string
	for _, line := range strings.SplitAfter(string(b), "\n") {
		k, _, _ := strings.Cut(line, ":")
		if acct, _ := usage.AccountSetting(strings.TrimSpace(k)); acct != "" && !strings.HasPrefix(line, " ") {
			continue
		}
		keep = append(keep, line)
	}
	return os.WriteFile(filepath.Join(im.to, "config.yaml"), []byte(strings.Join(keep, "")), 0o644)
}

// report says what the import did not bring: who words that name no one now, and the old home's entries left behind.
func (im *importer) report() {
	ats := keysOf(im.unmapped)
	for _, at := range ats {
		words := keysOf(im.unmapped[at])
		parts := make([]string, len(words))
		for i, w := range words {
			parts[i] = fmt.Sprintf("%s (%d)", w, im.unmapped[at][w])
		}
		fmt.Fprintf(im.out, "%s: kept as text, no one now: %s\n", at, strings.Join(parts, ", "))
	}
	ents, _ := os.ReadDir(im.from)
	var left []string
	for _, e := range ents {
		if !slices.Contains(carried, e.Name()) && !slices.Contains(rebuilt, e.Name()) {
			left = append(left, e.Name())
		}
	}
	fmt.Fprintf(im.out, "left behind in %s: %s\n", im.from, strings.Join(left, " "))
}

// copyLogin copies a Claude login to a moved account folder and says whether there was one: on macOS it is a Keychain item named
// by the folder's path. The old item stays, for going back.
var copyLogin = func(oldDir, newDir string) (bool, error) {
	if runtime.GOOS != "darwin" {
		return true, nil
	}
	if _, err := os.Stat(filepath.Join(newDir, ".credentials.json")); err == nil {
		return true, nil // a file login moved with the folder
	}
	user := claudecode.KeychainAccount()
	secret, err := exec.Command("security", "find-generic-password", "-a", user, "-s", claudecode.KeychainService(oldDir), "-w").Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 44 { // errSecItemNotFound
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the Keychain login: %w", err)
	}
	add := fmt.Sprintf("add-generic-password -U -a %s -s \"%s\" -X %s\n", user, claudecode.KeychainService(newDir),
		hex.EncodeToString(bytes.TrimRight(secret, "\n")))
	// the secret goes on stdin, out of argv; security -i reads a line of at most 4096 bytes, so a longer one goes in argv
	cmd := exec.Command("security", "-i")
	cmd.Stdin = strings.NewReader(add)
	if len(add) > 4000 {
		cmd = exec.Command("security", "add-generic-password", "-U", "-a", user, "-s", claudecode.KeychainService(newDir), "-w",
			string(bytes.TrimRight(secret, "\n")))
	}
	out, err := cmd.CombinedOutput()
	if err == nil && len(bytes.TrimSpace(out)) > 0 {
		err = errors.New(string(bytes.TrimSpace(out)))
	}
	if err != nil {
		return false, fmt.Errorf("copying the Keychain login: %w", err)
	}
	return true, nil
}

// clone copies src to dst as an APFS clone, keeping links and modes; a src that is not there is nothing to copy.
func clone(src, dst string) error {
	if _, err := os.Lstat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if out, err := exec.Command("cp", "-cRp", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("cp %s %s: %w: %s", src, dst, err, out)
	}
	return nil
}

func columns(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, table string) ([]string, error) {
	rows, err := q.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no table %s", table)
	}
	return out, rows.Err()
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func idOf(key string) int64 { _, id, _ := actor.Parse(key); return id }

func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		h, _ := os.UserHomeDir()
		return filepath.Join(h, rest)
	}
	return p
}
