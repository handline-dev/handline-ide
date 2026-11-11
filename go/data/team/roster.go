package team

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/protected"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// Roster is the agents of one home: their rows in the agents table, their folders and the founder's grants.
type Roster struct {
	DB    db.Q
	Home  string
	Store *protected.Store
	err   error // why the database could not be opened: every read and write says it
}

// NewRoster is the roster of home, on its database's shared handle (made, empty, when home has none yet).
func NewRoster(home string) *Roster {
	r := &Roster{Home: home, Store: protected.New(home)}
	if d, err := store.Shared(paths.DBFile(home)); err != nil {
		r.err = err
	} else {
		r.DB = d
	}
	return r
}

// settingKinds are the agent settings, each a column of the agents table under the same name: s text, i integer, b true or false.
var settingKinds = map[string]byte{
	"vendor": 's', "model": 's', "effort": 's', "weight": 's', "mode": 's', "enabled": 'b', "context_tokens": 'i',
	"extra_worktrees": 'i', "handoff_note": 'b', "park_timeout_s": 'i', "sandbox": 'b', "sandbox_domains": 's', "chrome": 'b',
	"mcp_config": 's', "schedules": 's', "peer_inbox": 'b', "account": 's', "ignore_cache_expiry": 'b', "bg_notice_s": 'i',
	"bg_grace_s": 'i', "bg_check_s": 'i', "notice_digest_s": 'i', "handoff_prompt": 's', "cache_renewal": 'b', "idle_fold": 'b',
	"idle_fold_tokens": 'i', "idle_fold_after_s": 'i', "fresh_after_task": 'b',
}

// settingKeys is settingKinds' keys in order, for the SELECT.
var settingKeys = func() []string {
	out := make([]string, 0, len(settingKinds))
	for k := range settingKinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}()

// NotRegistered is the error for a name no agent row has.
type NotRegistered struct{ Name string }

func (e *NotRegistered) Error() string { return fmt.Sprintf("agent '%s' is not registered", e.Name) }

// IsRefusal is whether err is a Roster's answer to the caller (a refused value or name, a missing agent), not a failure.
func IsRefusal(err error) bool {
	var r *names.Refusal
	var n *NotRegistered
	return errors.As(err, &r) || errors.As(err, &n)
}

func refuse(format string, a ...any) error { return &names.Refusal{Msg: fmt.Sprintf(format, a...)} }

// row is where an old name's agent is: its id (0 when there is none yet) and its workspace's id (0 when there is none), name and folder.
type row struct {
	id, ws         int64
	wsName, folder string
	lead           bool
}

func find(q db.Q, legacy string) (row, error) {
	wsName, agent := SplitLegacy(legacy)
	r := row{wsName: wsName}
	var lead sql.NullInt64
	err := q.QueryRow("SELECT id, folder, lead FROM workspaces WHERE name = ?", wsName).Scan(&r.ws, &r.folder, &lead)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if agent == "" {
		r.id, r.lead = lead.Int64, lead.Valid
		return r, nil
	}
	err = q.QueryRow("SELECT id FROM agents WHERE workspace = ? AND name = ?", r.ws, agent).Scan(&r.id)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	r.lead = lead.Valid && lead.Int64 == r.id
	return r, err
}

// Read is the agent's settings: a key only when its column is set, repo its workspace's folder.
func (r *Roster) Read(name string) (Config, error) {
	if r.err != nil {
		return nil, r.err
	}
	at, err := find(r.DB, name)
	if err != nil {
		return nil, err
	}
	if at.id == 0 {
		return nil, &NotRegistered{name}
	}
	vals := make([]any, len(settingKeys))
	for i := range settingKeys {
		vals[i] = new(any)
	}
	if err := r.DB.QueryRow("SELECT "+strings.Join(settingKeys, ", ")+" FROM agents WHERE id = ?", at.id).Scan(vals...); err != nil {
		return nil, err
	}
	cfg := Config{"name": name}
	if at.folder != "" {
		cfg["repo"] = at.folder
	}
	for i, k := range settingKeys {
		v := *(vals[i].(*any))
		if v == nil {
			continue
		}
		if settingKinds[k] == 'b' {
			n, _ := v.(int64)
			v = n != 0
		}
		cfg[k] = v
	}
	return cfg, nil
}

// SetKeys sets settings of the agent (a nil value goes back to the default). repo sets its workspace's folder, which only the lead
// may change. The founder's session only.
func (r *Roster) SetKeys(name string, set []yamlcompat.Pair) error {
	if err := requireFounder(); err != nil {
		return err
	}
	if r.err != nil {
		return r.err
	}
	at, err := find(r.DB, name)
	if err != nil {
		return err
	}
	if at.id == 0 {
		return &NotRegistered{name}
	}
	return inTx(r.DB, func(q db.Q) error { return r.set(q, at, set) })
}

func (r *Roster) set(q db.Q, at row, set []yamlcompat.Pair) error {
	for _, kv := range set {
		if kv.Key == "repo" {
			folder, _ := kv.Val.(string)
			if folder == at.folder {
				continue
			}
			if !at.lead && at.folder != "" {
				return refuse("repo is the folder of workspace %s (%s): set it on its lead", at.wsName, at.folder)
			}
			if _, err := q.Exec("UPDATE workspaces SET folder = ? WHERE id = ?", folder, at.ws); err != nil {
				return err
			}
			continue
		}
		kind, ok := settingKinds[kv.Key]
		if !ok {
			return refuse("%s is not an agent setting", kv.Key)
		}
		val := kv.Val
		if val == nil && kv.Key == "enabled" { // the one NOT NULL setting: its default is on
			val = true
		}
		v, err := column(kv.Key, kind, val)
		if err != nil {
			return err
		}
		// kv.Key is one of settingKinds', never input text.
		if _, err := q.Exec("UPDATE agents SET "+kv.Key+" = ? WHERE id = ?", v, at.id); err != nil {
			return err
		}
	}
	return nil
}

// column is v as the column of kind holds it; nil stays nil (the default).
func column(key string, kind byte, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch x := v.(type) {
	case string:
		if kind == 's' {
			return x, nil
		}
	case bool:
		if kind == 'b' {
			if x {
				return 1, nil
			}
			return 0, nil
		}
	case int64:
		if kind == 'i' {
			return x, nil
		}
	case int:
		if kind == 'i' {
			return int64(x), nil
		}
	}
	return nil, refuse("%s cannot be %v", key, v)
}

// RepoOf is the folder this agent works in, its workspace's; "" when it has none.
func (r *Roster) RepoOf(name string) string {
	if r.err != nil {
		return ""
	}
	at, err := find(r.DB, name)
	if err != nil || at.id == 0 {
		return ""
	}
	return at.folder
}

// Registered is the old names of every agent, oldest first.
func (r *Roster) Registered() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	t, err := ReadTeam(r.DB)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), t.order...), nil
}

// RegisterOpts is what Register is asked to set; a nil field is left as it is.
type RegisterOpts struct {
	Role, Weight, RoleFile, Repo           string
	Model, Effort, Vendor, Mode, MCPConfig *string
	ContextTokens, ExtraWorktrees          *int64
	Chrome, Sandbox                        *bool
}

func strp(s string) *string { return &s }

func present(p *string) bool { return p != nil && *p != "" }

// Register creates or updates an agent: its row (and its workspace's, for a new workspace), its folders and its ROLE.md. A bare name
// is a workspace's lead, which a new workspace gets as its first agent; ws.agent is an agent of workspace ws.
func (r *Roster) Register(name string, o RegisterOpts) (string, error) {
	if present(o.Effort) && !contains(ValidEffort, *o.Effort) {
		return "", refuse("effort must be one of %s, not %s", strings.Join(ValidEffort, ", "), pyQuote(*o.Effort))
	}
	if present(o.Vendor) {
		if err := CheckVendor(*o.Vendor); err != nil {
			return "", err
		}
	}
	if o.Mode != nil && !contains(Modes, *o.Mode) {
		return "", refuse("mode must be one of %s, not %s", strings.Join(Modes, ", "), pyQuote(*o.Mode))
	}
	if o.ExtraWorktrees != nil && (*o.ExtraWorktrees < 0 || *o.ExtraWorktrees > MaxExtraWorktrees) {
		return "", refuse("extra_worktrees is 0 to %d, not %d", MaxExtraWorktrees, *o.ExtraWorktrees)
	}
	var mcp string
	if present(o.MCPConfig) {
		var err error
		if mcp, err = MCPConfigText(*o.MCPConfig); err != nil {
			return "", err
		}
	}
	// First, so an agent's refusal leaves nothing written: the grant is the founder's alone.
	if err := requireFounder(); err != nil {
		return "", err
	}
	if r.err != nil {
		return "", r.err
	}
	if err := inTx(r.DB, func(q db.Q) error { return r.register(q, name, o, mcp) }); err != nil {
		return "", err
	}
	return r.folders(name, o)
}

// register is Register's rows, in one transaction with the name checks.
func (r *Roster) register(q db.Q, name string, o RegisterOpts, mcp string) error {
	at, err := find(q, name)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	owner, err := actor.Owner(q)
	if err != nil {
		return err
	}
	repo := expandUser(o.Repo)
	if at.ws == 0 {
		if err := names.Check(q, names.Workspace, at.wsName, 0); err != nil {
			return err
		}
		res, err := q.Exec("INSERT INTO workspaces(name, folder, created_at) VALUES(?, ?, ?)", at.wsName, repo, now)
		if err != nil {
			return err
		}
		if at.ws, err = res.LastInsertId(); err != nil {
			return err
		}
		at.folder = repo
	}
	isNew := at.id == 0
	if isNew {
		_, agent := SplitLegacy(name)
		isLead := agent == ""
		reportsTo := owner
		if isLead {
			agent = names.Lead
		} else if lead, err := find(q, at.wsName); err != nil {
			return err
		} else if lead.id != 0 {
			reportsTo = actor.Key(actor.Agent, lead.id)
		}
		if err := names.Check(q, names.Agent, agent, at.ws); err != nil {
			return err
		}
		res, err := q.Exec(`INSERT INTO agents(workspace, name, owner, reports_to, model, effort, weight, created_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, at.ws, agent, owner, reportsTo, DefaultModel, DefaultEffort, "light", now)
		if err != nil {
			return err
		}
		if at.id, err = res.LastInsertId(); err != nil {
			return err
		}
		if isLead {
			if _, err := q.Exec("UPDATE workspaces SET lead = ? WHERE id = ?", at.id, at.ws); err != nil {
				return err
			}
			at.lead = true
		}
	}
	var set []yamlcompat.Pair
	add := func(k string, v any) { set = append(set, yamlcompat.Pair{Key: k, Val: v}) }
	if present(o.Model) {
		add("model", *o.Model)
	}
	if present(o.Effort) {
		add("effort", *o.Effort)
	}
	if o.Weight != "" {
		add("weight", o.Weight)
	}
	if repo != "" {
		add("repo", repo)
	}
	if o.ContextTokens != nil && *o.ContextTokens != 0 {
		add("context_tokens", *o.ContextTokens)
	}
	if o.ExtraWorktrees != nil {
		add("extra_worktrees", nilIfZero(*o.ExtraWorktrees))
	}
	if o.Mode != nil {
		add("mode", nilIf(*o.Mode, DefaultMode))
	}
	if o.Sandbox != nil { // said either way: a NULL follows the founder's default
		add("sandbox", *o.Sandbox)
	}
	if o.Chrome != nil {
		add("chrome", nilIfFalse(*o.Chrome))
	}
	if o.MCPConfig != nil {
		add("mcp_config", nilIf(mcp, ""))
	}
	if present(o.Vendor) {
		add("vendor", nilIf(*o.Vendor, DefaultVendor))
	}
	return r.set(q, at, set)
}

func nilIf(s, def string) any {
	if s == def {
		return nil
	}
	return s
}

func nilIfZero(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

func nilIfFalse(b bool) any {
	if !b {
		return nil
	}
	return true
}

// folders makes the agent's folders and, when it has none, its ROLE.md and base.md. Its key is read through r.DB, which may be the
// transaction that made its row.
func (r *Roster) folders(name string, o RegisterOpts) (string, error) {
	key, err := paths.AgentKeyIn(r.DB, name)
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", &NotRegistered{name}
	}
	d := paths.AgentDir(r.Home, key)
	for _, sub := range []string{"inbox", "journal", "memory"} {
		if err := os.MkdirAll(filepath.Join(d, sub), 0o755); err != nil {
			return "", err
		}
	}
	rolePath := filepath.Join(d, "ROLE.md")
	if o.RoleFile != "" {
		b, err := os.ReadFile(expandUser(o.RoleFile))
		if err != nil {
			return "", refuse("--role-file not found: %s", expandUser(o.RoleFile))
		}
		if err := os.WriteFile(rolePath, b, 0o644); err != nil {
			return "", err
		}
	} else if _, err := os.Stat(rolePath); err != nil {
		role := o.Role
		if role == "" {
			role = "(describe the role)"
		}
		if err := os.WriteFile(rolePath, []byte("# "+name+"\n\n"+role+"\n"), 0o644); err != nil {
			return "", err
		}
	}
	base := filepath.Join(d, "base.md")
	if _, err := os.Stat(base); err != nil {
		body := "# base - " + name + "\n\n(injected into every session, so it is fixed overhead.)\n"
		if err := os.WriteFile(base, []byte(body), 0o644); err != nil {
			return "", err
		}
	}
	return d, nil
}

// inTx runs f in a transaction of q, or on q itself when q is one already.
func inTx(q db.Q, f func(db.Q) error) error {
	b, ok := q.(interface{ Begin() (*sql.Tx, error) })
	if !ok {
		return f(q)
	}
	tx, err := b.Begin()
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// NewRosterOn is the roster of home on q: the transaction a write is part of.
func NewRosterOn(q db.Q, home string) *Roster {
	return &Roster{DB: q, Home: home, Store: protected.New(home)}
}

// Remove deletes the agent's row. A lead goes only as its workspace's last agent, and its workspace with it.
func (r *Roster) Remove(name string) error {
	if r.err != nil {
		return r.err
	}
	return inTx(r.DB, func(q db.Q) error {
		at, err := find(q, name)
		if err != nil {
			return err
		}
		if at.id == 0 {
			return &NotRegistered{name}
		}
		var others int
		if err := q.QueryRow("SELECT COUNT(*) FROM agents WHERE workspace = ? AND id != ?", at.ws, at.id).Scan(&others); err != nil {
			return err
		}
		if at.lead && others > 0 {
			return refuse("%s leads workspace %s, which has %d other agent(s): retire them first", name, at.wsName, others)
		}
		if at.lead {
			if _, err := q.Exec("UPDATE workspaces SET lead = NULL WHERE id = ?", at.ws); err != nil {
				return err
			}
		}
		if _, err := q.Exec("DELETE FROM agents WHERE id = ?", at.id); err != nil {
			return err
		}
		if others == 0 {
			_, err = q.Exec("DELETE FROM workspaces WHERE id = ?", at.ws)
		}
		return err
	})
}

// Rename gives the agent old the name new, within its workspace (ws.a to ws.b). A lead's name is its workspace's, so renaming a lead
// renames a workspace, which only one with no other agent may do while they are named by old dotted names (until base-v2 1b).
func (r *Roster) Rename(old, new string) error {
	if r.err != nil {
		return r.err
	}
	return inTx(r.DB, func(q db.Q) error {
		at, err := find(q, old)
		if err != nil {
			return err
		}
		if at.id == 0 {
			return &NotRegistered{old}
		}
		oldWs, _ := SplitLegacy(old)
		newWs, newAgent := SplitLegacy(new)
		if at.lead != (newAgent == "") {
			return refuse("a workspace's lead is named by the workspace (%s), any other agent as workspace.agent: %s cannot become %s", oldWs, old, new)
		}
		if at.lead {
			var others int
			if err := q.QueryRow("SELECT COUNT(*) FROM agents WHERE workspace = ? AND id != ?", at.ws, at.id).Scan(&others); err != nil {
				return err
			}
			if others > 0 {
				return refuse("renaming %s renames workspace %s, which has %d other agent(s)", old, oldWs, others)
			}
			if err := names.Check(q, names.Workspace, newWs, 0); err != nil {
				return err
			}
			_, err = q.Exec("UPDATE workspaces SET name = ? WHERE id = ?", newWs, at.ws)
			return err
		}
		if newWs != oldWs {
			return refuse("%s cannot move to workspace %s by a rename", old, newWs)
		}
		if err := names.Check(q, names.Agent, newAgent, at.ws); err != nil {
			return err
		}
		_, err = q.Exec("UPDATE agents SET name = ? WHERE id = ?", newAgent, at.id)
		return err
	})
}

// requireFounder is an error when the caller is an agent's session: the roster is the founder's to change.
func requireFounder() error {
	if protected.InAgentSession() {
		return protected.ErrNotTheFounder
	}
	return nil
}
