package team

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/paths"
	_ "modernc.org/sqlite"
)

func TestTrustRootsStandingConsent(t *testing.T) {
	home := newHome(t)
	tr := NewTrust(home)
	repo := filepath.Join(t.TempDir(), "repo")
	os.MkdirAll(filepath.Join(repo, "sub", "wt"), 0o755)
	if tr.IsDeclared(repo) {
		t.Fatal("declared from nowhere")
	}
	if ok, said := tr.Enact(repo, ""); !ok || !strings.HasSuffix(said, "is now a declared root") {
		t.Fatalf("%v %s", ok, said)
	}
	if !tr.IsDeclared(repo) || !tr.IsDeclared(filepath.Join(repo, "sub", "wt")) || !tr.IsDeclared(filepath.Join(repo, "not", "yet")) {
		t.Fatal("a root covers what is under it, existing or not")
	}
	if tr.IsDeclared(repo+"-other") || tr.IsDeclared(filepath.Dir(repo)) {
		t.Fatal("a sibling or a parent is not covered")
	}
	if ok, said := tr.Enact(filepath.Join(repo, "sub"), "x"); ok || !strings.Contains(said, "already inside a declared root") {
		t.Fatalf("%v %s", ok, said)
	}
	b, _ := tr.Store.Get("trust", "roots.json")
	if !strings.Contains(b, "\"why\": \"added by the founder\"") || !strings.HasPrefix(b, "[\n  {\n    \"root\": ") {
		t.Fatalf("the roots are the Python's indent-2 JSON: %s", b)
	}
	if got := tr.ListRoots(); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	if ok, _ := tr.Remove(filepath.Join(repo, "elsewhere")); ok {
		t.Fatal("removed a root that is not declared")
	}
	if ok, said := tr.Remove(repo); !ok || tr.IsDeclared(repo) {
		t.Fatalf("%v %s", ok, said)
	}
	if b, _ := tr.Store.Get("trust", "roots.json"); b != "[]" {
		t.Fatalf("%q", b)
	}
	t.Setenv("HANDLINE_AGENT", "worker")
	if ok, said := tr.Enact(repo, "x"); ok || !strings.Contains(said, "founder's to set") {
		t.Fatalf("an agent declared a root: %v %s", ok, said)
	}
}

func TestMCPServersAreApprovedByContent(t *testing.T) {
	home := newHome(t)
	tr := NewTrust(home)
	repo := filepath.Join(t.TempDir(), "repo")
	os.MkdirAll(repo, 0o755)
	mcp := filepath.Join(repo, ".mcp.json")
	os.WriteFile(mcp, []byte(`{"mcpServers": {"a": {"command": "x"}, "b": {"command": "y", "args": ["1"]}}}`), 0o644)
	if got := tr.MCPUnapproved(repo); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("%v", got)
	}
	if ok, said := tr.ApproveMCP(repo); ok || !strings.Contains(said, "not inside a declared root") {
		t.Fatalf("%v %s", ok, said)
	}
	tr.Enact(repo, "")
	if ok, said := tr.ApproveMCP(repo); !ok || said != "approved: a, b" {
		t.Fatalf("%v %s", ok, said)
	}
	if got := tr.MCPApproved(repo); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("%v", got)
	}
	os.WriteFile(mcp, []byte(`{"mcpServers": {"a": {"command": "x"}, "b": {"command": "CHANGED", "args": ["1"]}, "c": {}}}`), 0o644)
	if got := tr.MCPApproved(repo); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("a changed command is a new question: %v", got)
	}
	if got := tr.MCPUnapproved(repo); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("%v", got)
	}
	if got := MCPServers(filepath.Join(repo, "none")); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	// 16 hex of the sha256 of the entry as sorted, compact JSON, as the Python hashes it
	if h := MCPServers(repo)["a"]; h != "cf35b664f0e85cbf" {
		t.Fatalf("%s", h)
	}
	os.WriteFile(mcp, []byte(`{"mcpServers": {"b": {"command": "y", "args": ["1"]}}}`), 0o644)
	if h := MCPServers(repo)["b"]; h != "3c229334e4d2871e" {
		t.Fatalf("%s", h)
	}
	if raw, _ := tr.Store.Get("trust", "mcp_servers.json"); raw != `["a:cf35b664f0e85cbf", "b:3c229334e4d2871e"]` {
		t.Fatalf("%s", raw)
	}
}

func TestProposalsKeepTheirOrderAndMootOnesAreWithdrawn(t *testing.T) {
	home := newHome(t)
	tr := NewTrust(home)
	repo := filepath.Join(t.TempDir(), "repo")
	os.MkdirAll(repo, 0o755)
	p := Proposal{{Key: "id", Val: "ab12cd34"}, {Key: "root", Val: repo}, {Key: "why", Val: "needs it"}, {Key: "requested_by", Val: "worker"},
		{Key: "created", Val: int64(5)}, {Key: "state", Val: "proposed"}}
	if err := tr.SaveProposal(p); err != nil {
		t.Fatal(err)
	}
	got, ok, err := tr.LoadProposal("ab12cd34")
	if err != nil || !ok || got.Get("why") != "needs it" || got[0].Key != "id" || got[5].Key != "state" {
		t.Fatalf("%v %v %v", got, ok, err)
	}
	if _, ok, _ := tr.LoadProposal("nope"); ok {
		t.Fatal("a proposal from nowhere")
	}
	tr.Enact(repo, "")
	gone, err := tr.SettleMoot()
	if err != nil || !reflect.DeepEqual(gone, []string{"ab12cd34"}) {
		t.Fatalf("%v %v", gone, err)
	}
	again, _, _ := tr.LoadProposal("ab12cd34")
	if again.Get("state") != "withdrawn" || again.Get("verdict") != "moot: that root is already declared" {
		t.Fatalf("%v", again)
	}
	if list, _ := tr.Listing(); len(list) != 1 {
		t.Fatalf("%v", list)
	}
	if root, ok := RootRead("[trust] add /a b - asked by worker\n\nwhy", "worker"); !ok || root != "/a b" {
		t.Fatalf("%q %v", root, ok)
	}
	if _, ok := RootRead("[trust] add /a - asked by worker", "other"); ok {
		t.Fatal("a root read for another requester")
	}
}

func account(t *testing.T, home, name string) string {
	dir := filepath.Join(accountDir(t, home, name), ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoginStateIsAPresenceNeverAReadOfTheCredential(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	dir := account(t, home, "alpha")
	if in, known := a.LoginState("alpha"); !known || in {
		t.Skip("a keychain item of this machine answered for the path") // macOS: the service name is per path
	}
	os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte("secret"), 0o600)
	if in, known := a.LoginState("alpha"); !known || !in || !a.LoggedIn("alpha") || !a.MayStart("alpha") {
		t.Fatal("a credential file is a login")
	}
	if got := LoginWord(true, true, false) + LoginWord(false, true, true) + LoginWord(false, false, true); got != "logged in"+"no"+"?" {
		t.Fatalf("%q", got)
	}
	os.WriteFile(filepath.Join(accountDir(t, home, "alpha"), "vendor"), []byte("nope\n"), 0o644)
	if in, known := a.LoginState("alpha"); !known || in {
		t.Fatal("a vendor with no driver cannot be driven")
	}
}

func TestAnExpiredLoginIsMarkedAndGoesWithANewerCredential(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	dir := account(t, home, "alpha")
	cred := filepath.Join(dir, ".credentials.json")
	os.WriteFile(cred, []byte("c"), 0o600)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(cred, old, old)
	now := time.Now()
	if ok, err := a.StallAccount("alpha", "authentication_failed", "401 on refresh", now); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if ok, _ := a.StallAccount("alpha", "authentication_failed", "second sighting", now); ok {
		t.Fatal("the first sighting stands")
	}
	time.Sleep(10 * time.Millisecond)
	m := a.Stalled("alpha")
	if m == nil || m.Why != "401 on refresh" || m.Error != "authentication_failed" {
		t.Fatalf("%+v", m)
	}
	if in, known := a.LoginState("alpha"); !known || in {
		t.Fatal("an account found with an expired login is not logged in, whatever its credential says")
	}
	if at, ok := a.GoneSince("alpha", 99); !ok || at != m.At {
		t.Fatalf("%v %v", at, ok)
	}
	modifiedMu.Lock()
	delete(modified, modifiedKey{home, "alpha"})
	modifiedMu.Unlock()
	os.Chtimes(cred, time.Now().Add(time.Minute), time.Now().Add(time.Minute)) // a login writes it anew
	if a.Stalled("alpha") != nil {
		t.Fatal("a login after the mark clears it")
	}
	if _, err := os.Stat(filepath.Join(accountDir(t, home, "alpha"), "login_expired.json")); err == nil {
		t.Fatal("the mark file stayed")
	}
}

func TestEnvForSetsTheAccountAndDropsAnotherAccountsConfigDir(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	account(t, home, "alpha")
	account(t, home, "beta")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(accountDir(t, home, "beta"), ".claude"))
	t.Setenv("XDG_DATA_HOME", "/somewhere/users/own")
	t.Setenv("OPENCODE_CONFIG_CONTENT", "{}")
	env := a.EnvFor("alpha", "worker")
	if env["CLAUDE_CONFIG_DIR"] != filepath.Join(accountDir(t, home, "alpha"), ".claude") || env["XDG_DATA_HOME"] != "/somewhere/users/own" {
		t.Fatalf("%v", env)
	}
	if _, has := env["OPENCODE_CONFIG_CONTENT"]; has {
		t.Fatal("another session's whole config was inherited")
	}
	for _, k := range []string{"HANDLINE_HOME", "HANDLINE_ACCOUNT", "HANDLINE_AGENT"} {
		if env[k] == "" {
			t.Errorf("%s not set", k)
		}
	}
	if env["HANDLINE_ACCOUNT"] != "alpha" || env["HANDLINE_AGENT"] != "worker" {
		t.Fatalf("%v", env)
	}
	if a.AccountNamedBy(filepath.Join(accountDir(t, home, "beta"), ".claude")) != "beta" || a.AccountNamedBy("/elsewhere") != "" {
		t.Fatal("AccountNamedBy")
	}
}

func TestEnvFromDropsTheOldPrefixAndKeepsHandlines(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	account(t, home, "alpha")
	base := map[string]string{"OCP_AGENT": "aud", "OCP_HOME": "/old/home", "OCP_ACCOUNT": "beta", "OCP_TMUX_SOCKET": "/old/sock",
		"HANDLINE_TMUX_SOCKET": "/new/sock", "HANDLINE_LIVE_ACCOUNTS": "delta", "PATH": "/bin"}
	env := a.EnvFrom(base, "alpha", "aud.c")
	for k := range env {
		if strings.HasPrefix(k, "OCP_") {
			t.Errorf("%s was carried into the new session", k)
		}
	}
	want := map[string]string{"HANDLINE_HOME": home, "HANDLINE_ACCOUNT": "alpha", "HANDLINE_AGENT": "aud.c",
		"HANDLINE_TMUX_SOCKET": "/new/sock", "HANDLINE_LIVE_ACCOUNTS": "delta", "PATH": "/bin"}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
	if base["OCP_AGENT"] != "aud" {
		t.Fatal("the caller's environment was changed")
	}
}

func TestUsersOfAndRemovePlan(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	account(t, home, "alpha")
	r := NewRoster(home)
	register(t, r, "wa", "wb")
	k := keyer(home)
	wa, wb := k("wa"), k("wb")
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("CREATE TABLE agent_state (agent TEXT PRIMARY KEY, account TEXT, account_source TEXT, closed_at INTEGER)")
	db.Exec("INSERT INTO agent_state VALUES (?, 'alpha', 'recorded', NULL), (?, 'alpha', 'assumed', 99)", wa, wb)
	users, err := a.UsersOf(db, "alpha")
	if err != nil || !reflect.DeepEqual(users, []string{wa}) {
		t.Fatalf("%v %v", users, err)
	}
	if ok, why, _ := a.RemovePlan(db, "alpha"); ok || !strings.Contains(why, "session open for "+wa+" - close it on the screen (c) first") {
		t.Fatalf("%v %s", ok, why)
	}
	if ok, why, _ := a.RemovePlan(db, "nope"); ok || why != "no account 'nope'" {
		t.Fatalf("%v %s", ok, why)
	}
	db.Exec("UPDATE agent_state SET closed_at = 5 WHERE agent = ?", wa)
	if ok, _, _ := a.RemovePlan(db, "alpha"); !ok {
		t.Fatal("a closed agent strands nothing")
	}
	if acct, _ := CurrentAccount(db, wa); acct != "alpha" {
		t.Fatal(acct)
	}
	if rd, ok, _ := ReadAccount(db, wa, nil); !ok || rd.Source != "recorded" {
		t.Fatalf("%+v", rd)
	}
	if rd, ok, _ := ReadAccount(db, wb, func() string { return "beta" }); !ok || rd != (AccountReading{"beta", "reported"}) {
		t.Fatalf("%+v", rd)
	}
	if rd, ok, _ := ReadAccount(db, wb, nil); !ok || rd.Source != "assumed" {
		t.Fatalf("%+v", rd)
	}
	if _, ok, _ := ReadAccount(db, "nobody", nil); ok {
		t.Fatal("nothing says")
	}
	dest, err := a.Retire("alpha", time.Date(2026, 10, 7, 1, 2, 3, 0, time.UTC))
	if err != nil || filepath.Base(dest) != "alpha-20261007-010203" {
		t.Fatalf("%s %v", dest, err)
	}
}

func TestSettingsProblemAndChanges(t *testing.T) {
	good := map[string]string{"model": "opus", "repo": "", "effort": "high", "weight": "light", "browser": "off",
		"browser_sites": "", "context_k": "200", "idle_close": "", "handoff_note": "default", "sandbox": "on", "mode": "auto"}
	if p := SettingsProblem(good); p != "" {
		t.Fatalf("%s", p)
	}
	for field, c := range map[string][2]string{"model": {"", "model cannot be empty"}, "effort": {"huge", "effort must be one of low, medium, high, xhigh, max"},
		"weight": {"mid", "weight must be one of light, heavy"}, "context_k": {"0", "context must be a whole number of thousands of tokens, at least 1"},
		"idle_close": {"x", "idle close must be a whole number of minutes (0 = never), or blank to follow the global time"},
		"browser":    {"ie", "browser must be off, chrome or playwright"}, "mode": {"wild", "mode must be one of default, auto, acceptEdits, plan"},
		"repo": {"/no/such/folder", "repo '/no/such/folder' is not a folder"}, "model ": {"on", "model 'on' would not be read back as text"}} {
		form := map[string]string{}
		for k, v := range good {
			form[k] = v
		}
		form[strings.TrimSpace(field)] = c[0]
		if got := SettingsProblem(form); got != c[1] {
			t.Errorf("%s=%q: %q, want %q", field, c[0], got, c[1])
		}
	}
	form := map[string]string{}
	for k, v := range good {
		form[k] = v
	}
	form["browser"], form["browser_sites"] = "playwright", ""
	if SettingsProblem(form) != "name at least one site for the sandboxed browser" {
		t.Fatal("a sandboxed browser needs sites")
	}
	current := Settings{"model": "sonnet", "effort": "medium", "weight": "light", "repo": "/r", "browser": "off",
		"browser_sites": "", "context_k": "200", "idle_close": "", "handoff_note": "default", "sandbox": "on", "mode": "auto"}
	next := map[string]string{"model": "opus", "effort": "medium", "weight": "light", "repo": "", "browser": "playwright",
		"browser_sites": "https://a.com, https://b.com", "context_k": "100", "idle_close": "", "handoff_note": "on"}
	got, err := SettingsChanges(current, next)
	want := []Change{{"model", "sonnet", "opus"}, {"browser", "off", "playwright"}, {"browser_sites", "", "https://a.com https://b.com"},
		{"context_k", "200", "100"}, {"handoff_note", "default", "on"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v %v", got, err)
	}
	if !Privileged("repo") || Privileged("model") || Privileged("handoff_note") || !Privileged("anything_new") {
		t.Fatal("privileged")
	}
}

type fakeIdle struct{ set string }

func (f *fakeIdle) Parse(text string) error { return nil }
func (f *fakeIdle) Set(text, agent string) (bool, string) {
	f.set = text + "@" + agent
	return true, ""
}

func TestSaveSettingsWritesThroughRegisterAndTheRest(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	r.Register("worker", RegisterOpts{})
	idle := &fakeIdle{}
	changes := []Change{{"model", "sonnet", "opus"}, {"context_k", "200", "100"}, {"handoff_note", "default", "on"}, {"idle_close", "", "30"},
		{"browser", "off", "chrome"}}
	ok, said := r.SaveSettings("worker", changes, idle)
	if !ok || said != "saved: model opus, context_k 100, handoff_note on, idle_close 30, browser chrome" || idle.set != "30@worker" {
		t.Fatalf("%v %s %s", ok, said, idle.set)
	}
	cfg, _ := r.Read("worker")
	if cfg["model"] != "opus" || cfg["effort"] != "medium" || cfg["context_tokens"] != int64(100000) || cfg["chrome"] != true || cfg["handoff_note"] != true {
		t.Fatalf("%v", cfg)
	}
	if ok, said := r.SaveSettings("worker", []Change{{"effort", "medium", "huge"}}, idle); ok || !strings.HasPrefix(said, "not saved: effort must be one of") {
		t.Fatalf("%v %s", ok, said)
	}
	if ok, said := r.SetByAgent(nil, "worker", map[string]string{"repo": "/x"}, "worker", "", nil, idle); ok || !strings.Contains(said, "repo needs the founder's approval") {
		t.Fatalf("%v %s", ok, said)
	}
	if ok, said := r.SetByAgent(nil, "worker", map[string]string{"model": "x"}, "other", "", nil, idle); ok || !strings.Contains(said, "other may not change worker's settings") {
		t.Fatalf("%v %s", ok, said)
	}
}

// An account's folder is named by its row's key: a name shaped like a key is refused, and an add that fails takes its row away.
func TestAddMakesTheRowFirstAndAFailedAddLeavesNone(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	if _, err := a.Add("c4", ""); !errors.Is(err, ErrBadAccountName) {
		t.Fatalf("a key-shaped name: %v", err)
	}
	if _, err := a.Add("main", ""); err != nil {
		t.Fatal(err)
	}
	if paths.AccountDir(home, "main") != filepath.Join(home, "accounts", paths.AccountKey(home, "main")) || paths.AccountKey(home, "main") == "" {
		t.Fatal("the folder by key")
	}
	os.Chmod(paths.AccountsDir(home), 0o555)
	defer os.Chmod(paths.AccountsDir(home), 0o755)
	if _, err := a.Add("spare", ""); err == nil {
		t.Fatal("no folder could be made")
	}
	if paths.AccountKey(home, "spare") != "" {
		t.Fatal("a failed add left its row")
	}
}

// A codex account is logged out with codex's own logout under its CODEX_HOME, never claude's.
func TestACodexAccountLogsOutWithCodex(t *testing.T) {
	bin := t.TempDir() // a codex whose login status says logged in: CredentialPresent asks it once auth.json is there
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\necho 'Logged in using ChatGPT' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home := newHome(t)
	a := Accounts{Home: home}
	acct := accountDir(t, home, "cx")
	if err := os.MkdirAll(acct, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acct, "vendor"), []byte("codex\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(acct, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if argv, _ := a.LogoutArgv("cx"); argv != nil {
		t.Fatalf("not logged in: %v", argv)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	argv, env := a.LogoutArgv("cx")
	if strings.Join(argv, " ") != "codex logout" || env["CODEX_HOME"] != dir {
		t.Fatalf("%v %v", argv, env)
	}
}
