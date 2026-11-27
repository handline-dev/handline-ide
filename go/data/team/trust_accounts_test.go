package team

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/testfiles"
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
		t.Fatalf("the roots are indent-2 JSON: %s", b)
	}
	if got := tr.ListRoots(); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	t.Setenv("HANDLINE_AGENT", "worker")
	if ok, said := tr.Enact(t.TempDir(), "x"); ok || !strings.Contains(said, "founder's to set") {
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
	// 16 hex of the sha256 of the entry as sorted, compact JSON: fixed expected answers
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

func TestEnvFromSetsHandlinesOwnAndKeepsTheRest(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	account(t, home, "alpha")
	base := map[string]string{"HANDLINE_AGENT": "aud", "HANDLINE_TMUX_SOCKET": "/new/sock", "HANDLINE_LIVE_ACCOUNTS": "delta", "PATH": "/bin"}
	env := a.EnvFrom(base, "alpha", "aud/c")
	want := map[string]string{"HANDLINE_HOME": home, "HANDLINE_ACCOUNT": "alpha", "HANDLINE_AGENT": "aud/c",
		"HANDLINE_TMUX_SOCKET": "/new/sock", "HANDLINE_LIVE_ACCOUNTS": "delta", "PATH": "/bin"}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
	if base["HANDLINE_AGENT"] != "aud" {
		t.Fatal("the caller's environment was changed")
	}
}

func TestEnvFromDropsAHostClaudeSessionsVariables(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	account(t, home, "alpha")
	host := map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_CHILD_SESSION": "1", "CLAUDE_CODE_SESSION_ID": "host",
		"CLAUDE_CODE_ENTRYPOINT": "claude-desktop", "CLAUDE_EFFORT": "low", "CLAUDE_PID": "7", "CLAUDE_AGENT_SDK_VERSION": "1",
		"PATH": "/bin"}
	env := a.EnvFrom(host, "alpha", "worker")
	for k := range env {
		if strings.HasPrefix(k, "CLAUDE") && k != "CLAUDE_CONFIG_DIR" {
			t.Errorf("%s was carried into the new session", k)
		}
	}
	if env["CLAUDE_CONFIG_DIR"] != filepath.Join(accountDir(t, home, "alpha"), ".claude") || env["PATH"] != "/bin" {
		t.Fatalf("%v", env)
	}
	if host["CLAUDECODE"] != "1" {
		t.Fatal("the caller's environment was changed")
	}

	// Outside a Claude Code session a CLAUDE_CODE_ setting is the user's own.
	own := a.EnvFrom(map[string]string{"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "9"}, "alpha", "worker")
	if own["CLAUDE_CODE_MAX_OUTPUT_TOKENS"] != "9" {
		t.Fatalf("the user's own setting went: %v", own)
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
}

func TestSettingsProblemAndChanges(t *testing.T) {
	good := map[string]string{"model": "opus", "repo": "", "effort": "high", "weight": "light", "browser": "off",
		"context_k": "200", "idle_close": "", "handoff_note": "default", "sandbox": "on", "mode": "auto"}
	if p := SettingsProblem(good); p != "" {
		t.Fatalf("%s", p)
	}
	for field, c := range map[string][2]string{"model": {"", "model cannot be empty"}, "effort": {"huge", "effort must be one of low, medium, high, xhigh, max"},
		"weight": {"mid", "weight must be one of light, heavy"}, "context_k": {"0", "context must be a whole number of thousands of tokens, at least 1"},
		"idle_close": {"x", "idle close must be a whole number of minutes (0 = never), or blank to follow the global time"},
		"browser":    {"ie", "browser must be off or chrome"}, "mode": {"wild", "mode must be one of default, auto, acceptEdits, plan"},
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
	current := Settings{"model": "sonnet", "effort": "medium", "weight": "light", "repo": "/r", "browser": "off",
		"context_k": "200", "idle_close": "", "handoff_note": "default", "sandbox": "on", "mode": "auto"}
	next := map[string]string{"model": "opus", "effort": "medium", "weight": "light", "repo": "", "browser": "chrome",
		"context_k": "100", "idle_close": "", "handoff_note": "on"}
	got, err := SettingsChanges(current, next)
	want := []Change{{"model", "sonnet", "opus"}, {"browser", "off", "chrome"}, {"context_k", "200", "100"}, {"handoff_note", "default", "on"}}
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
	r.Register("worker/lead", RegisterOpts{})
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
	if ok, said := r.SetByAgent(nil, "worker", map[string]string{"repo": "/x"}, "worker", "", nil, idle); ok || !strings.Contains(said, "repo needs the founder's approval: ask with `hl propose settings worker repo=<value>") {
		t.Fatalf("%v %s", ok, said)
	}
	if ok, said := r.SetByAgent(nil, "worker", map[string]string{"model": "x"}, "other", "", nil, idle); ok || !strings.Contains(said, "other may not change worker's settings") || !strings.Contains(said, "`hl propose settings ") {
		t.Fatalf("%v %s", ok, said)
	}
}

// A new account's folder is a random hex name its row records, found from its key and its name; a folder named by its key, as an
// account made before it has, is found as before. A name shaped like a key is refused, and an add that fails takes its row away.
func TestAddMakesTheRowFirstAndAFailedAddLeavesNone(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	if _, err := a.Add("c4", ""); !errors.Is(err, ErrBadAccountName) {
		t.Fatalf("a key-shaped name: %v", err)
	}
	cdir, err := a.Add("main", "")
	if err != nil {
		t.Fatal(err)
	}
	key, folder := paths.AccountKey(home, "main"), paths.AccountFolder(home, "main")
	if !regexp.MustCompile(`^[0-9a-f]{6}$`).MatchString(folder) || folder == key {
		t.Fatalf("a hex folder, not the key: %q (key %q)", folder, key)
	}
	for _, who := range []string{"main", key} {
		if got, err := AccountConfigDir(home, who); err != nil || got != cdir || got != filepath.Join(home, "accounts", folder, ".claude") {
			t.Fatalf("the config dir by %s: %s %v", who, got, err)
		}
	}
	if a.AccountNamedBy(cdir) != "main" || !slices.Contains(paths.ListAccounts(home), "main") {
		t.Fatal("the account its folder names, and listed")
	}
	old := accountDir(t, home, "older")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if old != filepath.Join(home, "accounts", paths.AccountKey(home, "older")) || a.AccountNamedBy(filepath.Join(old, ".claude")) != "older" {
		t.Fatalf("a folder named by its key still resolves: %s", old)
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

// A launch's HANDLINE_AGENT is the agent's name, workspace/name when another agent has its name.
func TestEnvForNamesTheAgentByItsName(t *testing.T) {
	home := newHome(t)
	account(t, home, "alpha")
	r := NewRoster(home)
	for _, n := range []string{"shop/lead", "shop/quill", "blog/lead"} {
		o := RegisterOpts{}
		if n != "shop/quill" { // a workspace's lead holds its folder
			o.Repo = testfiles.GitCheckout(t)
		}
		if _, err := r.Register(n, o); err != nil {
			t.Fatal(err)
		}
	}
	a, k := Accounts{Home: home}, keyer(home)
	for who, want := range map[string]string{"shop/quill": "quill", "shop": "shop/lead"} {
		if got := a.EnvFor("alpha", k(who))["HANDLINE_AGENT"]; got != want {
			t.Errorf("%s: %q, want %q", who, got, want)
		}
	}
}

// An email is one account on every machine: the one logged in as it, else one made for it, named after the runner's folder, then
// after the email, with -2 when the name is taken.
func TestByEmailFindsTheAccountOfAnEmailOrMakesOne(t *testing.T) {
	home := newHome(t)
	a := Accounts{Home: home}
	dir, err := a.Add("main", "")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"a@example.com"}}`), 0o644)
	for _, c := range []struct{ email, name, want string }{
		{"A@example.com", "alpha", "main"}, // the master's own, whatever the runner calls its folder
		{"b@example.com", "alpha", "alpha"},
		{"c@example.com", "alpha", "alpha-2"}, // another email in a folder of the same name is another account
		{"b@example.com", "beta", "alpha"},
		{"d.e@example.com", "c4", "d-e"}, // a key-shaped folder name is no account's name
		{"f@example.com", "9f3e0b", "f"}, // nor is a hex folder name
	} {
		got, err := a.ByEmail(c.email, c.name)
		if err != nil || got != c.want {
			t.Fatalf("%s in %s: %q, %v; want %q", c.email, c.name, got, err, c.want)
		}
	}
	if a.RecordedEmail("alpha-2") != "c@example.com" || a.LoggedIn("alpha-2") {
		t.Fatalf("a made account keeps its email and is logged in nowhere here: %q", a.RecordedEmail("alpha-2"))
	}
	if _, err := a.ByEmail("", "x"); err == nil {
		t.Fatal("no email finds no account")
	}
}
