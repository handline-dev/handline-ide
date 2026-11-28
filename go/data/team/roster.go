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

// NewRoster is the roster of home, on its database's shared handle. A home with no database has no roster: every call says so.
func NewRoster(home string) *Roster {
	r := &Roster{Home: home, Store: protected.New(home)}
	switch d, err := store.SharedIfThere(paths.DBFile(home)); {
	case err != nil:
		r.err = err
	case d == nil:
		r.err = fmt.Errorf("%s has no database", home)
	default:
		r.DB = d
	}
	return r
}

// settingKinds are the agent settings, each a column of the agents table under the same name: s text, i integer, b true or false.
var settingKinds = map[string]byte{
	"vendor": 's', "model": 's', "effort": 's', "weight": 's', "mode": 's', "enabled": 'b', "context_tokens": 'i',
	"handoff_note": 'b', "park_timeout_s": 'i', "sandbox": 'b', "sandbox_allow": 's', "device": 's', "chrome": 'b',
	"mcp_config": 's', "schedules": 's', "peer_inbox": 'b', "account": 's', "ignore_cache_expiry": 'b', "bg_notice_s": 'i',
	"bg_grace_s": 'i', "bg_check_s": 'i', "notice_digest_s": 'i', "handoff_prompt": 's', "cache_renewal": 'b', "idle_fold": 'b',
	"idle_fold_tokens": 'i', "idle_fold_after_s": 'i', "fresh_after_task": 'b', "account_split": 's', "block_screen_capture": 'b',
	"session_mode":           's',
	"move_background_wait_s": 'i',
	"compact_at_task_end":    'b', "compact_now_stops_turn": 'b', "no_silence_reminder": 'b',
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

// row is where an agent is: its id (0 when there is none yet), its workspace's id (0 when there is none), name and default folder, and
// own, the folder of its workspace it works in ("" for the default).
type row struct {
	id, ws                    int64
	wsName, name, folder, own string // name: a new agent's, from "workspace/name"
	lead                      bool
}

// works is the folder the agent works in: its own, else its workspace's default.
func (r row) works() string {
	if r.own != "" {
		return r.own
	}
	return r.folder
}

// find is where who's agent is: who is its key, "workspace/name" (of an agent that may not be there yet: a new one's workspace and
// name), or any other word paths.AgentKeyIn takes.
func find(q db.Q, who string) (row, error) {
	wsName, agent, qualified := strings.Cut(who, "/")
	if !qualified {
		k, err := paths.AgentKeyIn(q, who)
		if err != nil || k == "" {
			return row{}, err
		}
		_, id, _ := actor.Parse(k)
		var r row
		var lead sql.NullInt64
		err = q.QueryRow(`SELECT a.id, w.id, w.name, w.folder, COALESCE(a.folder, ''), w.lead FROM agents a JOIN workspaces w ON w.id = a.workspace
			WHERE a.id = ?`, id).Scan(&r.id, &r.ws, &r.wsName, &r.folder, &r.own, &lead)
		r.lead = lead.Valid && lead.Int64 == r.id
		return r, err
	}
	r := row{wsName: wsName, name: agent}
	var lead sql.NullInt64
	err := q.QueryRow("SELECT id, folder, lead FROM workspaces WHERE name = ?", wsName).Scan(&r.ws, &r.folder, &lead)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	err = q.QueryRow("SELECT id, COALESCE(folder, '') FROM agents WHERE workspace = ? AND name = ?", r.ws, agent).Scan(&r.id, &r.own)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	r.lead = lead.Valid && lead.Int64 == r.id
	return r, err
}

// Model is the model the agent runs, its vendor's default when none is set; "" for an agent not registered.
func (r *Roster) Model(name string) (string, error) {
	cfg, err := r.Read(name)
	if _, ok := errors.AsType[*NotRegistered](err); ok {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	_, model, _ := Parts(cfg)
	return model, nil
}

// Read is the agent's settings: a key only when its column is set, repo the folder of its workspace it works in.
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
	if f := at.works(); f != "" {
		cfg["repo"] = f
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

// SetKeys sets settings of the agent (a nil value goes back to the default). repo is the folder it works in (setFolder). The founder's
// session only.
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
			if err := setFolder(q, at, folder); err != nil {
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

// OtherFolders are the folders of this agent's workspace it does not work in, which its session gets beside its own.
func (r *Roster) OtherFolders(name string) []string {
	if r.err != nil {
		return nil
	}
	at, err := find(r.DB, name)
	if err != nil || at.id == 0 {
		return nil
	}
	fs, _ := Folders(r.DB, at.ws) // unreadable, it reads none
	var out []string
	for _, f := range fs {
		if real(f) != real(at.works()) {
			out = append(out, f)
		}
	}
	return out
}

// RepoOf is the folder of its workspace this agent works in; "" when it has none.
func (r *Roster) RepoOf(name string) string {
	repo, _ := r.ReadRepo(name)
	return repo
}

// ReadRepo is RepoOf with the error of a roster that could not be read, so a caller can tell "no repo" from "not known".
func (r *Roster) ReadRepo(name string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	at, err := find(r.DB, name)
	if err != nil || at.id == 0 {
		return "", err
	}
	return at.works(), nil
}

// RegisterOpts is what Register is asked to set; a nil field is left as it is.
type RegisterOpts struct {
	Role, Weight, RoleFile, Repo string
	// By and Approver are who asked for the agent and who OK'd it, kept on its first role and standing context; By "" is Handline.
	By, Approver                           string
	Model, Effort, Vendor, Mode, MCPConfig *string
	// Account pins the agent to that account (PinFor).
	Account         *string
	ContextTokens   *int64
	Chrome, Sandbox *bool
	// SandboxAllow is sandbox_allow's entries, one per line; "" clears it.
	SandboxAllow *string
	// Device pins the agent to a device (a node id) for Manual placement; "" clears it.
	Device *string
	// SessionMode is how its sessions run (SessionModes); "" follows its workspace.
	SessionMode *string
}

func strp(s string) *string { return &s }

func present(p *string) bool { return p != nil && *p != "" }

// Register creates or updates an agent: its row (and its workspace's, for a new workspace), its folders and its role. A new agent is
// named workspace/name; a new workspace's first agent leads it. An agent there is may be named by any word paths.AgentKeyIn takes.
func (r *Roster) Register(name string, o RegisterOpts) (string, error) {
	if present(o.Effort) && !contains(ValidEffort, *o.Effort) {
		return "", refuse("effort must be one of %s, not %s", strings.Join(ValidEffort, ", "), quote(*o.Effort))
	}
	if present(o.Vendor) {
		if err := CheckVendor(*o.Vendor); err != nil {
			return "", err
		}
	}
	if o.Mode != nil && !contains(Modes, *o.Mode) {
		return "", refuse("mode must be one of %s, not %s", strings.Join(Modes, ", "), quote(*o.Mode))
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
	var made int64
	if err := inTx(r.DB, func(q db.Q) error {
		var err error
		made, err = r.register(q, name, o, mcp)
		return err
	}); err != nil {
		return "", err
	}
	d, err := r.folders(name, o)
	if err != nil || made == 0 {
		return d, err
	}
	return d, seedWorkspace(r.DB, actor.Key(actor.Workspace, made), time.Now().Unix())
}

// register is Register's rows, in one transaction with the name checks. made is the id of the workspace it made, 0 when it made none.
func (r *Roster) register(q db.Q, name string, o RegisterOpts, mcp string) (made int64, err error) {
	at, err := find(q, name)
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	owner, err := actor.Owner(q)
	if err != nil {
		return 0, err
	}
	repo := expandUser(o.Repo)
	if at.id == 0 && at.name == "" {
		return 0, refuse("a new agent is named as workspace/name, not %s", name)
	}
	if at.ws == 0 {
		if at.ws, at.folder, err = createWorkspace(q, at.wsName, repo); err != nil {
			return 0, err
		}
		made = at.ws
		repo = at.folder // resolved: set finds it unchanged
	}
	if o.Mode != nil {
		held, err := HeldFor(q, at.ws)
		if err != nil {
			return 0, err
		}
		if said := held.ModeRefusal(*o.Mode); said != "" {
			return 0, refuse("%s", said)
		}
	}
	isNew := at.id == 0
	if isNew {
		agent := at.name
		var lead sql.NullInt64
		if err := q.QueryRow("SELECT lead FROM workspaces WHERE id = ?", at.ws).Scan(&lead); err != nil {
			return 0, err
		}
		isLead := !lead.Valid // a new workspace's first agent leads it
		if err := names.Check(q, names.Agent, agent, at.ws); err != nil {
			return 0, err
		}
		vendor := DefaultVendor
		if present(o.Vendor) {
			vendor = *o.Vendor
		}
		model, effort := NewAgentDefaults(r.Home, vendor)
		res, err := q.Exec(`INSERT INTO agents(workspace, name, owner, model, effort, weight, created_at)
			VALUES(?, ?, ?, ?, ?, ?, ?)`, at.ws, agent, owner, nilIf(model, ""), effort, "light", now)
		if err != nil {
			return 0, err
		}
		if at.id, err = res.LastInsertId(); err != nil {
			return 0, err
		}
		if isLead {
			if _, err := q.Exec("UPDATE workspaces SET lead = ? WHERE id = ?", at.id, at.ws); err != nil {
				return 0, err
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
	if o.Mode != nil {
		add("mode", nilIf(*o.Mode, DefaultMode))
	}
	if o.Sandbox != nil { // said either way: a NULL follows the founder's default
		add("sandbox", *o.Sandbox)
	}
	if o.SandboxAllow != nil {
		add("sandbox_allow", nilIf(*o.SandboxAllow, ""))
	}
	if o.Device != nil {
		add("device", nilIf(*o.Device, ""))
	}
	if o.SessionMode != nil {
		add("session_mode", nilIf(*o.SessionMode, ""))
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
	if present(o.Account) {
		add("account", *o.Account)
	}
	return made, r.set(q, at, set)
}

func nilIf(s, def string) any {
	if s == def {
		return nil
	}
	return s
}

func nilIfFalse(b bool) any {
	if !b {
		return nil
	}
	return true
}

// RemoveEmptyJournals removes each agent's journal folder, which nothing writes any more, when it is empty; one with files in it is
// logged and left. Removed names the folders it removed.
func RemoveEmptyJournals(home string, log func(format string, a ...any)) (removed []string) {
	dirs, _ := filepath.Glob(filepath.Join(paths.AgentsDir(home), "*", "journal")) // a bad pattern is no folder to remove
	for _, dir := range dirs {
		if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
			continue // only a folder: a file or a link of that name is not ours to remove
		}
		if err := os.Remove(dir); err == nil {
			removed = append(removed, dir)
		} else if !os.IsNotExist(err) {
			log("  agents: %s left as it is: %v", dir, err)
		}
	}
	return removed
}

// folders makes the agent's folders and, when it has none, its role and standing context. Its key is read through r.DB, which may be the
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
	for _, sub := range []string{"inbox", "memory"} {
		if err := os.MkdirAll(filepath.Join(d, sub), 0o755); err != nil {
			return "", err
		}
	}
	role := o.Role
	if role == "" {
		role = "(describe the role)"
	}
	role = "# " + name + "\n\n" + role
	if o.RoleFile != "" {
		b, err := os.ReadFile(expandUser(o.RoleFile))
		if err != nil {
			return "", refuse("--role-file not found: %s", expandUser(o.RoleFile))
		}
		role = string(b)
	}
	by := o.By
	if by == "" {
		by = SeedActor
	}
	now := time.Now().Unix()
	for _, p := range [][2]string{{db.PartRole, role}, {db.PartBase, "# base - " + name + "\n\n(injected into every session, so it is fixed overhead.)"}} {
		was, err := db.CurrentInstruction(r.DB, key, p[0])
		if err != nil {
			return "", err
		}
		if was != nil && (p[0] == db.PartBase || o.RoleFile == "") {
			continue // an agent made again keeps what it has, unless it is given a role
		}
		if _, err := db.SetInstruction(r.DB, key, p[0], p[1], by, o.Approver, now); err != nil {
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

// Remove deletes the agent's row. When it leads a workspace others are still in, the oldest of them (NextLead) leads it from then on; the
// workspace goes with its last agent.
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
		if at.lead {
			next, err := NextLead(q, at.ws)
			if err != nil {
				return err
			}
			var lead any // NULL for the last agent, whose workspace goes below
			if next != "" {
				_, id, _ := actor.Parse(next)
				lead = id
			}
			if _, err := q.Exec("UPDATE workspaces SET lead = ? WHERE id = ?", lead, at.ws); err != nil {
				return err
			}
		}
		if _, err := q.Exec("DELETE FROM agents WHERE id = ?", at.id); err != nil {
			return err
		}
		if others == 0 {
			if err := ClearRules(q, at.ws); err != nil {
				return err
			}
			_, err = q.Exec("DELETE FROM workspaces WHERE id = ?", at.ws)
		}
		return err
	})
}

// NextLead is the key of the agent that leads workspace ws (its id) once its lead leaves: the oldest other agent in it (the lowest id),
// never a seat (a temp lent from an agent), "" when there is none.
func NextLead(q db.Q, ws int64) (string, error) {
	var id sql.NullInt64
	err := q.QueryRow(`SELECT MIN(a.id) FROM agents a JOIN workspaces w ON w.id = a.workspace
		WHERE a.workspace = ? AND a.hired_from IS NULL AND (w.lead IS NULL OR a.id != w.lead)`, ws).Scan(&id)
	if err != nil || !id.Valid {
		return "", err
	}
	return actor.Key(actor.Agent, id.Int64), nil
}

// Rename gives the agent old the name new in its workspace. A lead is renamed like any agent: its workspace keeps its name.
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
		if err := names.Check(q, names.Agent, new, at.ws); err != nil {
			return err
		}
		_, err = q.Exec("UPDATE agents SET name = ? WHERE id = ?", new, at.id)
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
