package team

import (
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/kit/testfiles"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// newHome is a home as the daemon's first start leaves it: a database with the owner, p1, and no agents. The caller is the founder.
func newHome(t *testing.T) string {
	t.Helper()
	t.Setenv("HANDLINE_TEST_ROOT", t.TempDir())
	t.Setenv("HANDLINE_AGENT", "")
	home := filepath.Join(t.TempDir(), "home")
	d, err := storetest.OpenAt(t, paths.DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	t.Cleanup(func() { store.ForgetShared(paths.DBFile(home)) })
	return home
}

// keyer turns a word that names an agent (ws/name, ws for its lead) into its key in home; a word no agent has (human, nobody) stays as it is.
func keyer(home string) func(string) string {
	return func(n string) string {
		if k := paths.AgentKey(home, n); k != "" {
			return k
		}
		return n
	}
}

// register registers each of names: ws/agent, or a bare ws for that workspace's lead (named lead).
func register(t *testing.T, r *Roster, names ...string) {
	t.Helper()
	for _, n := range names {
		if !strings.Contains(n, "/") {
			n += "/lead"
		}
		if _, err := r.Register(n, RegisterOpts{}); err != nil {
			t.Fatalf("register %s: %v", n, err)
		}
	}
}

func TestRegisterMakesTheWorkspaceAndItsLeadAndReadsBack(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	repo := testfiles.GitCheckout(t)
	if _, err := r.Register("shop/lead", RegisterOpts{Repo: repo, Model: strp("opus")}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register("shop/ui", RegisterOpts{Effort: strp("high"), Sandbox: new(bool)}); err != nil {
		t.Fatal(err)
	}
	d, _ := store.Shared(paths.DBFile(home))
	var ws, folder, lead, owner string
	d.QueryRow("SELECT w.name, w.folder, a.name, a.owner FROM workspaces w JOIN agents a ON a.id = w.lead").Scan(&ws, &folder, &lead, &owner)
	if ws != "shop" || folder != repo || lead != names.Lead || owner != "p1" {
		t.Fatalf("workspace %s at %s led by %s, owned by %s", ws, folder, lead, owner)
	}
	cfg, err := r.Read("shop/ui")
	if err != nil {
		t.Fatal(err)
	}
	want := Config{"name": "shop/ui", "repo": repo, "model": DefaultModel, "effort": "high", "weight": "light", "enabled": true, "sandbox": false}
	if len(cfg) != len(want) {
		t.Fatalf("%v, want %v", cfg, want)
	}
	for k, v := range want {
		if cfg[k] != v {
			t.Fatalf("%s: %v, want %v (%v)", k, cfg[k], v, cfg)
		}
	}
	if lead, _ := r.Read("shop"); lead["model"] != "opus" {
		t.Fatalf("the lead: %v", lead)
	}
	// Again: only what is asked changes.
	if _, err := r.Register("shop/ui", RegisterOpts{Model: strp("haiku")}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := r.Read("shop/ui"); cfg["model"] != "haiku" || cfg["effort"] != "high" || cfg["sandbox"] != false {
		t.Fatalf("%v", cfg)
	}
	tm, _ := LoadTeam(home)
	k := keyer(home)
	if tm.ContactOf("shop") != k("shop") || tm.TeamOf(k("shop/ui")) != "shop" {
		t.Fatalf("shop's contact %q, shop.ui's workspace %q", tm.ContactOf("shop"), tm.TeamOf(k("shop/ui")))
	}
	if _, err := os.Stat(filepath.Join(paths.AgentDir(home, "shop/ui"), "memory")); err != nil {
		t.Error(err)
	}
	for _, part := range []string{db.PartRole, db.PartBase} {
		if in, err := db.CurrentInstruction(r.DB, k("shop/ui"), part); err != nil || in == nil || in.Actor != SeedActor {
			t.Errorf("%s: %+v %v", part, in, err)
		}
	}
	if got := paths.ListAgents(home); strings.Join(got, ",") != k("shop")+","+k("shop/ui") {
		t.Fatalf("listed %v", got)
	}
}

func TestOnlyTheLeadMovesTheWorkspaceFolder(t *testing.T) {
	r := NewRoster(newHome(t))
	one, two := testfiles.GitCheckout(t), testfiles.GitCheckout(t)
	register(t, r, "shop", "shop/ui")
	if _, err := r.Register("shop/ui", RegisterOpts{Repo: one}); err != nil {
		t.Fatalf("a workspace with no folder yet takes one from any agent: %v", err)
	}
	if _, err := r.Register("shop/ui", RegisterOpts{Repo: two}); err == nil || !strings.Contains(err.Error(), "set the default on its lead") {
		t.Fatalf("%v", err)
	}
	if _, err := r.Register("shop/lead", RegisterOpts{Repo: two}); err != nil {
		t.Fatal(err)
	}
	if r.RepoOf("shop/ui") != two {
		t.Fatalf("%q", r.RepoOf("shop/ui"))
	}
}

func TestRegisterRefusesWhatTheNameRulesRefuse(t *testing.T) {
	r := NewRoster(newHome(t))
	register(t, r, "shop", "shop/ui")
	for _, c := range []struct{ name, why string }{
		{"human/lead", "reserved"}, {"gate/lead", "reserved"}, {"Shop2/lead", "lowercase"}, {"shop/a.b", "lowercase"}, {"blog/ui", ""},
		{"ui/lead", "an agent"}, {"blog/shop", "a workspace"},
	} {
		_, err := r.Register(c.name, RegisterOpts{})
		var ref *names.Refusal
		if c.why == "" && err != nil || c.why != "" && (!errors.As(err, &ref) || !strings.Contains(err.Error(), c.why)) {
			t.Errorf("%s: %v, want %q", c.name, err, c.why)
		}
	}
}

func TestOnlyTheFoundersSessionRegisters(t *testing.T) {
	r := NewRoster(newHome(t))
	t.Setenv("HANDLINE_AGENT", "worker")
	if _, err := r.Register("evil/lead", RegisterOpts{}); err == nil {
		t.Fatal("an agent registered")
	}
	if len(paths.ListAgents(r.Home)) != 0 {
		t.Fatal("a refusal left a row behind")
	}
	t.Setenv("HANDLINE_AGENT", "")
	if _, err := r.Register("worker/lead", RegisterOpts{Mode: strp("bypass")}); err == nil || !strings.Contains(err.Error(), "mode must be one of") {
		t.Fatalf("bypass was taken as a mode: %v", err)
	}
	register(t, r, "worker")
	t.Setenv("HANDLINE_AGENT", "worker")
	if err := r.SetKeys("worker", nil); err == nil {
		t.Fatal("an agent set its own settings")
	}
}

func TestRouteFollowsWorkspaces(t *testing.T) {
	home := newHome(t)
	register(t, NewRoster(home), "shop", "shop/ui", "shop/api", "blog", "blog/writer")
	tm, err := LoadTeam(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		from, to string
		depth    int
		ok       bool
		why      string
	}{
		{"shop/ui", "shop/api", 0, true, "are in workspace shop"},
		{"shop/ui", "shop", 0, true, "are in workspace shop"},
		{"shop", "shop/ui", 0, true, ""},
		{"shop/ui", "blog", 0, true, "leads workspace blog"},
		{"shop/ui", "blog/writer", 0, false, "another workspace"},
		{"shop/ui", actor.OwnerKey, 0, true, "its owner"},
		{actor.OwnerKey, "blog/writer", 0, true, "the founder"},
		{"shop/ui", "shop/ui", 0, false, "cannot ask itself"},
		{"shop/ui", "nobody", 0, false, "unknown target"},
		{"shop/ui", "shop/api", DefaultMaxChainDepth, false, "chain depth"},
	} {
		d := tm.Route(keyer(home)(c.from), keyer(home)(c.to), c.depth)
		if d.Allowed != c.ok || !strings.Contains(d.Reason, c.why) {
			t.Errorf("%s -> %s: %v %q, want %v %q", c.from, c.to, d.Allowed, d.Reason, c.ok, c.why)
		}
	}
}

// Agents are peers: no row says whom an agent reports to.
func TestAgentsHaveNoReportsToColumn(t *testing.T) {
	home := newHome(t)
	register(t, NewRoster(home), "web", "web/dev")
	d, _ := store.Shared(paths.DBFile(home))
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM pragma_table_info('agents') WHERE name = 'reports_to'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("reports_to columns %d, %v", n, err)
	}
}

func TestContactAndRoleOfAnAgent(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	register(t, r, "web", "web/dev", "web/auditor", "ops", "ops/dev")
	d, _ := store.Shared(paths.DBFile(home))
	d.Exec("UPDATE agents SET role = 'check' WHERE name = 'auditor'")
	tm, _ := LoadTeam(home)
	k := keyer(home)
	for _, c := range []struct{ agent, role string }{{"web/dev", RoleWork}, {"web/auditor", RoleCheck}, {"web", RoleWork}} {
		if ro := tm.TaskRole(k(c.agent)); ro != c.role {
			t.Errorf("%s: role %s, want %s", c.agent, ro, c.role)
		}
	}
	if tm.ContactOf("web") != k("web") || tm.ContactOf("ops") != k("ops") || tm.ContactOf("none") != "" {
		t.Error("contacts")
	}
	if tm.TeamOf(k("ops/dev")) != "ops" || strings.Join(tm.Members("ops"), ",") != k("ops")+","+k("ops/dev") {
		t.Error("workspace or members")
	}
	var none *Team
	if none.ContactOf("web") != "" || none.TaskRole("web/auditor") != RoleWork {
		t.Error("no team: no contact, work")
	}
	empty, err := LoadTeam(t.TempDir())
	if err != nil || len(empty.Agents) != 0 || len(paths.ListAgents(t.TempDir())) != 0 {
		t.Fatalf("a home with no database has no agents: %v", err)
	}
}

func TestRemoveAndRename(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	register(t, r, "shop/lead")
	d0, _ := store.Shared(paths.DBFile(home))
	seat, err := NewSeat(d0, keyer(home)("shop/lead"), 1) // older than ui, but a seat never leads
	if err != nil {
		t.Fatal(err)
	}
	register(t, r, "shop/ui", "shop/api", "solo/lead")
	key := keyer(home)
	lead, ui, api, solo := key("shop/lead"), key("shop/ui"), key("shop/api"), key("solo")
	d, _ := store.Shared(paths.DBFile(home))
	ws := func() (n int64) {
		d.QueryRow("SELECT id FROM workspaces WHERE name = 'shop'").Scan(&n)
		return n
	}()
	leadOf := func() string {
		var id sql.NullInt64
		d.QueryRow("SELECT lead FROM workspaces WHERE name = 'shop'").Scan(&id)
		if !id.Valid {
			return ""
		}
		return actor.Key(actor.Agent, id.Int64)
	}
	if next, err := NextLead(d, ws); err != nil || next != ui {
		t.Fatalf("next lead %q, %v: want the oldest other agent, %s", next, err, ui)
	}
	if err := r.Rename(lead, "boss"); err != nil {
		t.Fatalf("a lead with agents renames like any agent: %v", err)
	}
	if got := leadOf(); got != lead || paths.WorkspaceKey(home, "shop") == "" {
		t.Fatalf("lead %s after its rename; the workspace keeps its name", got)
	}
	if err := r.Remove("nobody"); !IsRefusal(err) {
		t.Fatalf("a name with no row is refused, not removed: %v", err)
	}
	if IsRefusal(errors.New("disk I/O error")) {
		t.Fatal("a failure is no refusal")
	}
	for _, c := range []struct{ from, to, why string }{
		{ui, "api", "already the name"},
		{ui, "shop", "a workspace"},
		{ui, "a.b", "is not a"},
	} {
		if err := r.Rename(c.from, c.to); err == nil || !strings.Contains(err.Error(), c.why) {
			t.Errorf("%s -> %s: %v, want %q", c.from, c.to, err, c.why)
		}
	}
	if _, err := r.Register("nowhere", RegisterOpts{}); err == nil || !IsRefusal(err) {
		t.Fatalf("a new agent is named as workspace/name: %v", err)
	}
	if err := r.Remove(lead); err != nil {
		t.Fatalf("a lead with agents goes: %v", err)
	}
	if got := leadOf(); got != ui {
		t.Fatalf("lead %s after the lead left, want the oldest other agent %s", got, ui)
	}
	if err := r.Remove(api); err != nil {
		t.Fatal(err)
	}
	if next, _ := NextLead(d, ws); next != "" {
		t.Fatalf("next lead %q of a workspace of one", next)
	}
	for _, k := range []string{seat, ui} { // agent.retire takes a lead's idle seats with it
		if err := r.Remove(k); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(paths.ListAgents(home), ","); got != solo {
		t.Fatalf("left %s", got)
	}
	var n int
	d.QueryRow("SELECT COUNT(*) FROM workspaces").Scan(&n)
	if n != 1 {
		t.Fatalf("%d workspaces: the last agent's goes with it", n)
	}
}

// accountDir gives the account named name a row in home's database, when it has none, and is its folder, which the caller makes.
func accountDir(t *testing.T, home, name string) string {
	t.Helper()
	d, err := storetest.OpenAt(t, paths.DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec("INSERT INTO accounts(name, owner, created_at) VALUES(?, 'p1', 0) ON CONFLICT(name) DO NOTHING", name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.ForgetShared(paths.DBFile(home)) })
	return paths.AccountDir(home, name)
}

func TestARosterOfAHomeWithNoDatabaseMakesNone(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if _, err := NewRoster(home).Read("shop"); err == nil {
		t.Fatal("read a roster with no database")
	}
	if _, err := os.Stat(paths.DBFile(home)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a database was made: %v", err)
	}
}

func TestTheRosterFindsAnAgentByItsKey(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	register(t, r, "shop", "shop/ui")
	ui := keyer(home)("shop/ui")
	if err := r.SetKeys(ui, []yamlcompat.Pair{{Key: "model", Val: "opus"}}); err != nil {
		t.Fatal(err)
	}
	if cfg, err := r.Read(ui); err != nil || cfg["model"] != "opus" {
		t.Fatalf("%v %v", cfg, err)
	}
	if _, err := r.Read("a999"); err == nil {
		t.Fatal("read a key no agent has")
	}
}

func runGit(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	return string(out), err
}

// A new agent's model and effort, when it names none, are config.yaml's default for its vendor; an existing agent keeps its own.
func TestANewAgentTakesItsVendorsDefaults(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	repo := testfiles.GitCheckout(t)
	if _, err := r.Register("shop/lead", RegisterOpts{Repo: repo}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(paths.ConfigFile(home), []byte("default_model_claude: opus\ndefault_effort_claude: high\ndefault_model_codex: gpt-5.1-codex\n"+
		"default_effort_codex: bogus\n"), 0o644)
	if _, err := r.Register("shop/ui", RegisterOpts{}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register("shop/cx", RegisterOpts{Vendor: strp("codex")}); err != nil {
		t.Fatal(err)
	}
	for agent, want := range map[string][2]string{"shop": {DefaultModel, DefaultEffort}, "shop/ui": {"opus", "high"},
		"shop/cx": {"gpt-5.1-codex", DefaultEffort}} {
		cfg, _ := r.Read(agent)
		if cfg["model"] != want[0] || cfg["effort"] != want[1] {
			t.Errorf("%s: %v %v, want %v", agent, cfg["model"], cfg["effort"], want)
		}
	}
	if _, err := r.Register("shop/own", RegisterOpts{Model: strp("haiku")}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := r.Read("shop/own"); cfg["model"] != "haiku" || cfg["effort"] != "high" {
		t.Errorf("a model named at register beats the default: %v", cfg)
	}
	if m, e := NewAgentDefaults(t.TempDir(), "codex"); m != "" || e != DefaultEffort {
		t.Errorf("codex unset: %q %q", m, e)
	}
}
