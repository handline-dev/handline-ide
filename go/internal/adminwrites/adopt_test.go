package adminwrites

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/data/team"
)

// fakeCodex is codex's account facts as adopt needs them: the login is auth.json in its folder.
type fakeCodex struct{}

func (fakeCodex) Name() string                       { return "codex" }
func (fakeCodex) ConfigDirName() string              { return ".codex" }
func (fakeCodex) ConfigDirEnv() string               { return "CODEX_HOME" }
func (fakeCodex) LoginArgv() []string                { return []string{"codex", "login"} }
func (fakeCodex) WebLoginArgv() ([]string, bool)     { return nil, false }
func (fakeCodex) CredentialModified(string) *float64 { return nil }
func (fakeCodex) CredentialFile() string             { return "auth.json" }
func (fakeCodex) CredentialPresent(dir string) (bool, bool) {
	_, err := os.Stat(filepath.Join(dir, "auth.json"))
	return err == nil, true
}

func TestDiscoverAnswersTheLoginsFoundAndAdoptMakesAnAccountOfOne(t *testing.T) {
	r := newRig(t)
	user := t.TempDir()
	was := agent.UserHome
	agent.UserHome = func() string { return user }
	t.Cleanup(func() { agent.UserHome = was })
	if prior, ok := agent.Lookup("codex"); ok {
		t.Cleanup(func() { agent.Register(prior) })
	}
	agent.Register(fakeCodex{})
	if got := r.result(r.one("accounts.discover", `{}`)); len(got["found"].([]any)) != 0 {
		t.Fatalf("nothing there: %v", got)
	}
	os.MkdirAll(filepath.Join(user, ".claude"), 0o755)
	os.WriteFile(filepath.Join(user, ".claude", ".credentials.json"), []byte("{}"), 0o600)
	codex := filepath.Join(user, ".codex")
	os.MkdirAll(codex, 0o755)
	found := r.result(r.one("accounts.discover", `{}`))["found"].([]any)
	if len(found) != 2 {
		t.Fatalf("%v", found)
	}
	claude, cx := found[0].(map[string]any), found[1].(map[string]any)
	if claude["vendor"] != "claude" || claude["source"] != "~/.claude" || claude["logged_in"] != true || claude["adoptable"] != false {
		t.Fatalf("claude: %v", claude)
	}
	if cx["vendor"] != "codex" || cx["source"] != "~/.codex" || cx["logged_in"] != false || cx["adoptable"] != true {
		t.Fatalf("codex: %v", cx)
	}
	refusals := []struct{ params, code, key, want string }{
		{`{"vendor":"claude","source":"~/.claude","request_id":"a1"}`, "bad_request", "field", "vendor"},
		{`{"vendor":"codex","source":"~/.nowhere","request_id":"a2"}`, "not_found", "what", "source"},
		{`{"vendor":"codex","source":"~/.codex","request_id":"a3"}`, "refused", "gate", "state"},
		{`{"vendor":"codex","source":"~/.codex","account":"c4","request_id":"a4"}`, "bad_request", "field", "account"},
	}
	os.WriteFile(filepath.Join(codex, "auth.json"), []byte("{}"), 0o600)
	for i, c := range refusals {
		if i == 2 {
			os.Rename(filepath.Join(codex, "auth.json"), filepath.Join(codex, "gone"))
		}
		if i == 3 {
			os.Rename(filepath.Join(codex, "gone"), filepath.Join(codex, "auth.json"))
		}
		m := r.one("account.adopt", c.params)
		if r.code(m) != c.code {
			t.Errorf("%s: %v", c.params, m)
		}
		if data, _ := m["error"].(map[string]any)["data"].(map[string]any); data[c.key] != c.want {
			t.Errorf("%s: %s %v", c.params, c.key, data)
		}
	}
	var told []string
	r.e.Changed = func(a string) { told = append(told, a) }
	res := r.result(r.one("account.adopt", `{"vendor":"codex","source":"~/.codex","request_id":"a5"}`))
	acct, _ := res["account"].(string)
	if acct == "" || res["accepted"] != true || len(told) != 1 || told[0] != acct {
		t.Fatalf("%v %v", res, told)
	}
	if !(team.Accounts{Home: r.home}).LoggedIn(acct) || team.AccountVendor(r.home, acct) != "codex" {
		t.Fatalf("%s: a codex account, logged in", acct)
	}
}
