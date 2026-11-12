package reads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/accounts/weblogin"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

func TestTheAccountsViewNeverCarriesTheAuthorizeURL(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if err := os.MkdirAll(teamtest.AccountDir(t, e.Home, "main"), 0o755); err != nil {
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
	dir := teamtest.AccountDir(t, e.Home, "main")
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
	if err := os.MkdirAll(teamtest.AccountDir(t, e.Home, "main"), 0o755); err != nil {
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
	dir := teamtest.AccountDir(t, e.Home, "main")
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
	dir := teamtest.AccountDir(t, e.Home, "main")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	status := `{"collected_at": 1000, "windows": [` +
		`{"minutes": 10080, "used_percent": 40, "resets_at": 90000, "observed_at": 1000},` +
		`{"minutes": 300, "used_percent": 10, "resets_at": 3600, "observed_at": 1000}]}`
	if err := os.WriteFile(paths.StatusFile(e.Home, teamtest.Account(t, e.Home, "main")), []byte(status), 0o644); err != nil {
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
	if err := os.MkdirAll(teamtest.AccountDir(t, e.Home, "main"), 0o755); err != nil {
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
	if err := os.MkdirAll(teamtest.AccountDir(t, e.Home, "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"cold", "fresh", "warm"} {
		addAgent(t, e, n, "")
		if _, err := e.DB.Exec("INSERT INTO agent_state(agent, state, account, updated) VALUES(?, 'IDLE', 'main', 1)", teamtest.Key(t, e.Home, n)); err != nil {
			t.Fatal(err)
		}
	}
	cold, fresh, warm := teamtest.Key(t, e.Home, "cold"), teamtest.Key(t, e.Home, "fresh"), teamtest.Key(t, e.Home, "warm")
	for n, view := range map[string]string{cold: `{"account": null}`, warm: `{"account": "main"}`} {
		if _, err := e.DB.Exec("INSERT INTO agent_view(agent, view, updated) VALUES(?, ?, 1)", n, view); err != nil {
			t.Fatal(err)
		}
	}
	acct, _ := e.Account("main")
	if got, _ := json.Marshal(acct["agents"]); string(got) != `["`+fresh+`","`+warm+`"]` {
		t.Fatalf("agents %s", got)
	}
}

// Each account says its vendor: the one account.add recorded, else the default.
func TestAccountsListSaysEachAccountsVendor(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for _, a := range []string{"main", "other"} {
		if err := os.MkdirAll(teamtest.AccountDir(t, e.Home, a), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(teamtest.AccountDir(t, e.Home, "other"), "vendor"), []byte("opencode\n"), 0o644); err != nil {
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
		dir := teamtest.AccountDir(t, e.Home, a)
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
	if err := usage.SetAccount(e.DB, "ended", map[string]any{"month_cycle": true, "sub_end": "2020-01-01"}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(teamtest.AccountDir(t, e.Home, "oc"), "vendor"), []byte("opencode\n"), 0o644)
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
		dir := teamtest.AccountDir(t, e.Home, acct)
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
	if got := emails(&rpc.Conn{Caller: rpc.Caller{Role: "owner"}}); got["main"] != "main@example.com" || got["bare"] != nil || len(got) != 2 {
		t.Fatalf("founder: %v", got)
	}
	for name, c := range map[string]*rpc.Conn{
		"agent":      {Caller: rpc.Caller{Role: "agent", Kind: "agent", Key: "amy", Info: map[string]any{"kind": "agent", "name": "amy"}}},
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
	dir, err := team.AccountConfigDir(e.Home, teamtest.Account(t, e.Home, "main"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"now@example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := e.accountsList(&rpc.Conn{Caller: rpc.Caller{Role: "owner"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a := got.(map[string]any)["accounts"].([]any)[0].(map[string]any); a["email"] != "now@example.com" {
		t.Fatalf("%v", a)
	}
	if _, err := os.Stat(filepath.Join(teamtest.AccountDir(t, e.Home, "main"), "identity.json")); !os.IsNotExist(err) {
		t.Fatalf("the read wrote identity.json: %v", err)
	}
}

func TestAccountsListSaysAPausedAccountIsPausedAndTakesNoAgent(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	status := fmt.Sprintf(`{"collected_at": %d, "windows": [{"minutes": 300, "used_percent": 10, "resets_at": %d, "observed_at": %d},
		{"minutes": 10080, "used_percent": 10, "resets_at": %d, "observed_at": %d}]}`, now-5, now+3600, now-5, now+90000, now-5)
	for _, a := range []string{"rest", "good"} {
		dir := teamtest.AccountDir(t, e.Home, a)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "status.json"), []byte(status), 0o644)
		os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte("{}"), 0o600)
	}
	if err := usage.SetAccount(e.DB, "rest", map[string]any{"paused": true}); err != nil {
		t.Fatal(err)
	}
	got, err := e.accountsList(nil, map[string]any{"usage": true})
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]map[string]any{}
	for _, a := range got.(map[string]any)["accounts"].([]any) {
		rows[a.(map[string]any)["name"].(string)] = a.(map[string]any)
	}
	rest, good := rows["rest"], rows["good"]
	np, _ := rest["not_picked"].(map[string]any)
	if rest["paused"] != true || rest["cap_effective"] != 0 || np["kind"] != "paused" {
		t.Errorf("rest: paused %v, cap_effective %v, not_picked %v", rest["paused"], rest["cap_effective"], rest["not_picked"])
	}
	if good["paused"] != false || good["not_picked"] != nil {
		t.Errorf("good: paused %v, not_picked %v", good["paused"], good["not_picked"])
	}
}

// Scenario B3 and B7 (docs/design/account-assignment-scenarios.md), the 7 / 2 fixture: alpha, cap 2, has 7 agents, 3 mid-turn and 4 at
// their prompt with background work running. agents_running is the mid-turn count the cap holds to; the 4 are counted apart, not in it.
func TestAccountsListRosterCountsMidTurnAsTheCapDoesAndBackgroundWorkApart(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	dir := teamtest.AccountDir(t, e.Home, "alpha")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "status.json"), []byte(fmt.Sprintf(`{"collected_at": %d, "windows": [{"minutes": 300, "used_percent": 10, "resets_at": %d, "observed_at": %d}, {"minutes": 10080, "used_percent": 10, "resets_at": %d, "observed_at": %d}]}`, now-5, now+3600, now-5, now+90000, now-5)), 0o644)
	os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte("{}"), 0o600)
	if err := usage.SetAccount(e.DB, "alpha", map[string]any{"active_cap": 2}); err != nil {
		t.Fatal(err)
	}
	bg := map[string]bool{}
	for i, n := range []string{"ann", "bob", "cal", "dee", "eve", "fay", "gus"} {
		addAgent(t, e, n, "")
		state, key := "RUNNING", teamtest.Key(t, e.Home, n)
		if i >= 3 {
			state, bg[key] = "IDLE", true
		}
		if _, err := e.DB.Exec("INSERT INTO agent_state(agent, state, account, updated) VALUES(?, ?, 'alpha', 1)", key, state); err != nil {
			t.Fatal(err)
		}
	}
	// A running agent's session may run background work too: it is mid-turn, so it is counted there only.
	bg[teamtest.Key(t, e.Home, "ann")] = true
	// hal runs on beta while its view still says alpha (just moved): it counts on beta only, in neither of alpha's figures.
	os.MkdirAll(teamtest.AccountDir(t, e.Home, "beta"), 0o755)
	addAgent(t, e, "hal", "")
	hal := teamtest.Key(t, e.Home, "hal")
	bg[hal] = true
	if _, err := e.DB.Exec("INSERT INTO agent_state(agent, state, account, updated) VALUES(?, 'RUNNING', 'beta', 1)", hal); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Exec(`INSERT INTO agent_view(agent, view, updated) VALUES(?, '{"account": "alpha"}', 1)`, hal); err != nil {
		t.Fatal(err)
	}
	e.Delivery = fakeDelivery{background: bg}

	got, err := e.accountsList(nil, map[string]any{"roster": true})
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]map[string]any{}
	for _, r := range got.(map[string]any)["accounts"].([]any) {
		rows[r.(map[string]any)["name"].(string)] = r.(map[string]any)
	}
	a, b := rows["alpha"], rows["beta"]
	if b["agents_assigned"] != 1 || b["agents_running"] != 1 || b["agents_background"] != 0 {
		t.Fatalf("beta: assigned %v running %v background %v", b["agents_assigned"], b["agents_running"], b["agents_background"])
	}
	if a["agents_assigned"] != 7 || a["agents_running"] != 3 || a["agents_background"] != 4 || a["cap_effective"] != 2 {
		t.Fatalf("assigned %v running %v background %v cap %v", a["agents_assigned"], a["agents_running"], a["agents_background"], a["cap_effective"])
	}
	by, err := e.usageEnv().ActiveByAccount(e.DB)
	if err != nil || len(by["alpha"]) != a["agents_running"] {
		t.Fatalf("the cap's own count %v (%v) is not agents_running %v", by["alpha"], err, a["agents_running"])
	}
	plain, _ := e.accountsList(nil, nil)
	if _, has := plain.(map[string]any)["accounts"].([]any)[0].(map[string]any)["agents_running"]; has {
		t.Fatal("the roster counts are only there when asked for")
	}
	if _, err := e.accountsList(nil, map[string]any{"roster": 1}); err == nil {
		t.Fatal("a roster that is not a boolean is refused")
	}
}
