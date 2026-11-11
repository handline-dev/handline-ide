package reads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/accounts/weblogin"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

func TestTheAccountsViewNeverCarriesTheAuthorizeURL(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if err := os.MkdirAll(filepath.Join(e.Home, "accounts", "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, st := range []string{weblogin.Waiting, "code_sent", weblogin.Done, weblogin.Failed, weblogin.Cancelled} {
		e.Login = func(string) *weblogin.Login {
			return &weblogin.Login{ID: "login_1", State: st, URL: "https://claude.example/authorize?s=secret"}
		}
		acct, ok := e.Account("main")
		if !ok {
			t.Fatal("no account")
		}
		l, _ := acct["login"].(map[string]any)
		if l["state"] != st || l["login_id"] != "login_1" {
			t.Fatalf("%s: %v", st, l)
		}
		list, err := e.accountsList(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal([]any{acct, list})
		if strings.Contains(string(raw), "claude.example") || strings.Contains(string(raw), `"url"`) {
			t.Fatalf("%s: the accounts view has the URL: %s", st, raw)
		}
	}
}

func TestAccountsListUsageIsOptInAndSaysWhyAnAccountCannotStart(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	dir := filepath.Join(e.Home, "accounts", "main")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "subscription.json"), []byte(`{"subscriptionType":"max"}`), 0o644)
	plain, _ := e.accountsList(nil, nil)
	if _, has := plain.(map[string]any)["accounts"].([]any)[0].(map[string]any)["assignable"]; has {
		t.Fatal("the usage fields are only there when asked for")
	}
	got, err := e.accountsList(nil, map[string]any{"usage": true})
	if err != nil {
		t.Fatal(err)
	}
	a := got.(map[string]any)["accounts"].([]any)[0].(map[string]any)
	if a["plan"] != "max" || a["assignable"] != false || !strings.Contains(a["assign_reason"].(string), "no status.json") || a["runway"] != "" || a["ctx_pct"] != nil {
		t.Fatalf("%v", a)
	}
	if _, err := e.accountsList(nil, map[string]any{"usage": "yes"}); err == nil {
		t.Fatal("a usage that is not a boolean is refused")
	}
}

func TestStatusPickNamesNoAccountThatIsNotLoggedIn(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if err := os.MkdirAll(filepath.Join(e.Home, "accounts", "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := e.statusPick(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if m["picked"] != nil || len(m["candidates"].([]any)) != 0 || len(m["agent_costs"].([]string)) != 0 {
		t.Fatalf("%v", m)
	}
}

func TestStatusPickNamesAnAccountThatIsLoggedIn(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	dir := filepath.Join(e.Home, "accounts", "main")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(`{}`), 0o600)
	got, err := e.statusPick(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if len(m["candidates"].([]any)) != 1 {
		t.Fatalf("%v", m)
	}
}

func TestAccountsListServesTheNextFiveHourAndWeekResetFromTheStatusFile(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	dir := filepath.Join(e.Home, "accounts", "main")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	status := `{"collected_at": 1000, "windows": [` +
		`{"minutes": 10080, "used_percent": 40, "resets_at": 90000, "observed_at": 1000},` +
		`{"minutes": 300, "used_percent": 10, "resets_at": 3600, "observed_at": 1000}]}`
	if err := os.WriteFile(paths.StatusFile(e.Home, "main"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := e.accountsList(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := got.(map[string]any)["accounts"].([]any)[0].(map[string]any)
	five, week := a["five_hour"].(map[string]any), a["weekly"].(map[string]any)
	if five["resets_at"] != "1970-01-01T01:00:00Z" || week["resets_at"] != "1970-01-02T01:00:00Z" || five["source"] != "measured" {
		t.Fatalf("%v", a)
	}
}

func TestAStalledAccountRowSaysStalledWithItsReason(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if err := os.MkdirAll(filepath.Join(e.Home, "accounts", "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	if acct, _ := e.Account("main"); acct["stalled"] != nil {
		t.Fatalf("a usable account is not stalled: %v", acct["stalled"])
	}
	at := time.Unix(1000, 0)
	if _, err := (team.Accounts{Home: e.Home}).StallAccount("main", "oauth_org_not_allowed", "bob's turn ended on oauth_org_not_allowed", at); err != nil {
		t.Fatal(err)
	}
	acct, _ := e.Account("main")
	s, _ := acct["stalled"].(map[string]any)
	if s["reason"] != "oauth_org_not_allowed" || s["error"] != "oauth_org_not_allowed" || s["since"] != "1970-01-01T00:16:40Z" ||
		s["detail"] != "bob's turn ended on oauth_org_not_allowed" {
		t.Fatalf("%v", acct["stalled"])
	}
}

// An account lists the agents its views put on it: a cold agent's view is on none, whatever account it last ran on.
func TestAnAccountListsTheAgentsItsViewsPutOnIt(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if err := os.MkdirAll(filepath.Join(e.Home, "accounts", "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"cold", "fresh", "warm"} {
		addAgent(t, e, n, "")
		if _, err := e.DB.Exec("INSERT INTO agent_state(agent, state, account, updated) VALUES(?, 'IDLE', 'main', 1)", n); err != nil {
			t.Fatal(err)
		}
	}
	for n, view := range map[string]string{"cold": `{"account": null}`, "warm": `{"account": "main"}`} {
		if _, err := e.DB.Exec("INSERT INTO agent_view(agent, view, updated) VALUES(?, ?, 1)", n, view); err != nil {
			t.Fatal(err)
		}
	}
	acct, _ := e.Account("main")
	if got, _ := json.Marshal(acct["agents"]); string(got) != `["fresh","warm"]` {
		t.Fatalf("agents %s", got)
	}
}

// Each account says its vendor: the one account.add recorded, else the default.
func TestAccountsListSaysEachAccountsVendor(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for _, a := range []string{"main", "other"} {
		if err := os.MkdirAll(filepath.Join(e.Home, "accounts", a), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(e.Home, "accounts", "other", "vendor"), []byte("opencode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := e.accountsList(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	vendors := map[string]any{}
	for _, a := range got.(map[string]any)["accounts"].([]any) {
		vendors[a.(map[string]any)["name"].(string)] = a.(map[string]any)["vendor"]
	}
	if vendors["main"] != "claude" || vendors["other"] != "opencode" {
		t.Fatalf("%v", vendors)
	}
}

func TestAccountsListSaysWhyAnAccountIsNotPickedAndNothingForACandidate(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	status := fmt.Sprintf(`{"collected_at": %d, "windows": [{"minutes": 300, "used_percent": 10, "resets_at": %d, "observed_at": %d},
		{"minutes": 10080, "used_percent": 10, "resets_at": %d, "observed_at": %d}]}`, now-5, now+3600, now-5, now+90000, now-5)
	for _, a := range []string{"ended", "good", "out", "unknown", "oc", "new"} {
		dir := filepath.Join(e.Home, "accounts", a)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if a != "new" {
			os.WriteFile(filepath.Join(dir, "status.json"), []byte(status), 0o644)
		}
		if a != "out" {
			os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte("{}"), 0o600)
		}
	}
	os.WriteFile(filepath.Join(e.Home, "config.yaml"), []byte("month_cycle_ended: true\nsub_end_ended: \"2020-01-01\"\n"), 0o644)
	os.WriteFile(filepath.Join(e.Home, "accounts", "oc", "vendor"), []byte("opencode\n"), 0o644)
	login := e.LoginState
	e.LoginState = func(a string) (bool, bool) {
		if a == "unknown" {
			return false, false
		}
		return login(a)
	}
	got, err := e.accountsList(nil, map[string]any{"usage": true})
	if err != nil {
		t.Fatal(err)
	}
	np := map[string]any{}
	for _, a := range got.(map[string]any)["accounts"].([]any) {
		m := a.(map[string]any)
		np[m["name"].(string)] = m["not_picked"]
	}
	ended, _ := np["ended"].(map[string]any)
	out, _ := np["out"].(map[string]any)
	if ended["kind"] != "subscription_ended" || !strings.Contains(ended["text"].(string), "2020-01-01") ||
		!strings.Contains(ended["text"].(string), "may be out of date") {
		t.Errorf("ended: %v", np["ended"])
	}
	if out["kind"] != "login" || out["text"] != "not logged in" {
		t.Errorf("out: %v", np["out"])
	}
	for _, a := range []string{"good", "unknown", "new"} {
		if v, has := np[a]; !has || v != nil {
			t.Errorf("%s: a candidate's not_picked is null, an unknown login and a never-run one (bootstrap) included: %v", a, np)
		}
	}
	// Judged by its own vendor's pick, never refused as "not claude"; with no opencode driver in this test its reading counts, so it is
	// a candidate.
	if v := np["oc"]; v != nil {
		t.Errorf("oc: %v", v)
	}
}

// The email is the founder's: an agent's, a member's or an in-process call's accounts.list has no email key at all.
func TestAccountsListGivesTheEmailToTheFounderOnly(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for acct, identity := range map[string]string{"main": `{"email": "main@example.com"}`, "bare": `{}`} {
		dir := filepath.Join(e.Home, "accounts", acct)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "identity.json"), []byte(identity), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	emails := func(c *rpc.Conn) map[string]any {
		t.Helper()
		got, err := e.accountsList(c, nil)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]any{}
		for _, a := range got.(map[string]any)["accounts"].([]any) {
			a := a.(map[string]any)
			if email, has := a["email"]; has {
				out[a["name"].(string)] = email
			}
		}
		return out
	}
	if acct, ok := e.Account("main"); !ok {
		t.Fatal("no account")
	} else if _, has := acct["email"]; has {
		t.Fatalf("the accounts topic's Account, which agents hear, has the email: %v", acct)
	}
	if got := emails(&rpc.Conn{Caller: rpc.Caller{Role: "founder"}}); got["main"] != "main@example.com" || got["bare"] != nil || len(got) != 2 {
		t.Fatalf("founder: %v", got)
	}
	for name, c := range map[string]*rpc.Conn{
		"agent":      {Caller: rpc.Caller{Role: "agent", Info: map[string]any{"kind": "agent", "name": "amy"}}},
		"member":     {Caller: rpc.Caller{Role: "member"}},
		"in-process": nil,
	} {
		if got := emails(c); len(got) != 0 {
			t.Fatalf("%s: %v", name, got)
		}
	}
}

// An account with no record shows the email Claude Code is logged in to now, and the read records nothing.
func TestAccountsListShowsTheLoggedInEmailOfAnUnrecordedAccountAndWritesNothing(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	dir, err := team.AccountConfigDir(e.Home, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"now@example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := e.accountsList(&rpc.Conn{Caller: rpc.Caller{Role: "founder"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a := got.(map[string]any)["accounts"].([]any)[0].(map[string]any); a["email"] != "now@example.com" {
		t.Fatalf("%v", a)
	}
	if _, err := os.Stat(filepath.Join(e.Home, "accounts", "main", "identity.json")); !os.IsNotExist(err) {
		t.Fatalf("the read wrote identity.json: %v", err)
	}
}
