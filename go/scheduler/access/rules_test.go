package access

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/protocol/rpc"
)

// A kept rule goes into each launch of the agent and of its temps, once however often it is kept; access.list gives it, and
// access.rule_remove takes it back and says so on the agents topic.
func TestAKeptRuleIsInEveryLaunchOfTheAgentAndItsTempsUntilRemoved(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	teamtest.Set(t, home, "a", "vendor", "claude")
	srv := rpc.NewServer(founder{}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	srv.DB = teamtest.DB(t, home) // the roster's rows too, for a temp of the agent
	srv.Names = srv.DB
	var events []map[string]any
	Register(srv, home, func(topic, typ string, data map[string]any) error { events = append(events, data); return nil }, nil)
	a := teamtest.Key(t, home, "a")
	now := time.Unix(1000, 0)
	for _, r := range []string{"Bash(npm test)", "Bash(go test ./...)", "Bash(npm test)"} {
		if err := AddRule(home, a, r, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.DB.Exec("INSERT INTO agents(id, workspace, name, owner, created_at, hired_from) "+
		"SELECT 9001, workspace, 'a-temp', owner, created_at, ? FROM agents WHERE 'a' || id = ?", a, a); err != nil {
		t.Fatal(err)
	}
	want := []string{"Bash(go test ./...)", "Bash(npm test)"}
	for _, who := range []string{a, "a9001"} {
		if rs, err := RulesForLaunch(srv.DB, home, who); err != nil || !slices.Equal(rs, want) {
			t.Fatalf("%s launches with %v %v", who, rs, err)
		}
	}
	if err := AddRule(home, a, "Bash(*)", now); err != nil { // kept before a check tightened: not carried
		t.Fatal(err)
	}
	if rs, _ := RulesForLaunch(srv.DB, home, a); !slices.Equal(rs, want) {
		t.Fatalf("an unsafe kept rule is launched with: %v", rs)
	}
	RemoveRule(home, a, "Bash(*)")
	got := call(t, srv, [2]string{"access.list", `{"agent":"a"}`},
		[2]string{"access.rule_remove", `{"agent":"a","rule":"Bash(npm test)","request_id":"r1"}`},
		[2]string{"access.rule_remove", `{"agent":"a","rule":"Bash(nope)","request_id":"r2"}`},
		[2]string{"access.rule_remove", `{"agent":"a","rule":"","request_id":"r3"}`})
	if rs := got[0]["result"].(map[string]any)["rules"].([]any); len(rs) != 2 {
		t.Fatalf("list: %v", got[0])
	}
	if r := got[1]["result"].(map[string]any); r["removed"] != true || r["applies_from"] != "next_session" {
		t.Fatalf("%v", got[1])
	}
	if r := got[2]["result"].(map[string]any); r["removed"] != false {
		t.Fatalf("a rule never kept: %v", got[2])
	}
	if e, _ := got[3]["error"].(map[string]any); fmt.Sprint(e["data"].(map[string]any)["field"]) != "rule" {
		t.Fatalf("%v", got[3])
	}
	if rs, _ := RulesForLaunch(srv.DB, home, "a9001"); !slices.Equal(rs, []string{"Bash(go test ./...)"}) {
		t.Fatalf("after the removal a temp launches with %v", rs)
	}
	if len(events) == 0 || fmt.Sprint(events[0]["rules"]) == "" || len(events[0]["rules"].([]map[string]any)) != 1 {
		t.Fatalf("events %v", events)
	}
}

func TestSafeRule(t *testing.T) {
	home := "/Users/x/.handline"
	for _, c := range []struct {
		rule string
		safe bool
	}{
		{"Bash(npm test)", true},
		{"Bash(tools/check.sh origin/main)", true},
		{"Bash(go test ./internal/prompts -run Always)", true},
		{"Bash(npm test *)", false}, // a wildcard covers any arguments
		{"Bash(npm test:*)", false},
		{"Bash(cat /Users/x/*)", false},
		{"Bash(*)", false},
		{"Bash()", false},
		{"Bash( npm test)", false},
		{"Bash(npm test\nrm -rf /)", false},
		{"Bash(tmux -L handline send-keys -t a1 x)", false},
		{"Bash(security find-generic-password -s x)", false},
		{"Bash(git push origin HEAD:main --force)", false},
		{"Bash(cat /USERS/X/.HANDLINE/db.sqlite)", false},
		{"Bash(cat ~/.ssh/id_ed25519)", false},
		{"Bash(sqlite3 db.sqlite .tables)", false},
		{"Read(//tmp/x)", false},
		{"Bash(git -C . push origin x)", false},
		{"Bash(cat $HANDLINE_HOME/db.sqlite)", false},
		{"Bash(t''mux ls)", false},
		{"Bash(hl  term a1)", false},
		{"Bash(npm test && rm x)", false},
		{"Bash(npm test > out)", false},
		{"Bash(printf a\\n)", false},
		{"Bash(echo (x))", false},
		{"Bash(cat ~/notes)", false},
		{"Bash(npm test", false},
	} {
		if got := SafeRule(home, c.rule); got != c.safe {
			t.Errorf("%q: safe %v, want %v", c.rule, got, c.safe)
		}
	}
}

// TestMain sets the process environment once (storetest.Main): the founder is the caller, and the protected store has a stand-in home.
func TestMain(m *testing.M) {
	storetest.Main(m, []string{"HANDLINE_AGENT"}, nil)
}
