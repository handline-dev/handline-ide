package proposalwrites

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
)

// rule is key's rule in force for workspace ws, nil for none.
func rule(t *testing.T, r *rig, ws int64, key string) *team.KeptRule {
	t.Helper()
	rules, err := team.KeptRules(r.db, ws)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range rules {
		if k.Key == key {
			return &k
		}
	}
	return nil
}

func TestWorkspaceRuleSetKeepsTheOwnersRuleOnAWorkspaceOrTheMachine(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bobWithKid(t, r)
	ws, _, _ := team.WorkspaceID(r.db, "bob")
	if m := r.call("workspace.rule.set", `{"workspace":"bob","key":"agent_cap","value":2,"request_id":"r1"}`); m["result"] == nil {
		t.Fatalf("%v", m)
	}
	if m := r.call("workspace.rule.set", `{"key":"agent_cap","value":5,"request_id":"r2"}`); m["result"] == nil {
		t.Fatalf("no workspace: the machine default: %v", m)
	}
	if k := rule(t, r, ws, "agent_cap"); k == nil || k.Value != int64(2) || k.Source != "workspace" || k.SetBy != actor.OwnerKey {
		t.Fatalf("%+v", k)
	}
	if m := r.call("workspace.rule.set", `{"workspace":"`+paths.WorkspaceKey(r.home, "bob")+`","key":"agent_cap","value":null,"request_id":"r3"}`); m["result"] == nil {
		t.Fatalf("null clears it, by the workspace's key too: %v", m)
	}
	if k := rule(t, r, ws, "agent_cap"); k == nil || k.Value != int64(5) || k.Source != "machine" {
		t.Fatalf("cleared, the machine's holds: %+v", k)
	}
	if m := r.call("workspace.rule.set", `{"workspace":"bob","key":"tools_deny","value":["WebFetch"],"request_id":"r4"}`); m["result"] == nil {
		t.Fatalf("%v", m)
	}
	if k := rule(t, r, ws, "tools_deny"); k == nil || !reflect.DeepEqual(k.Value, []string{"WebFetch"}) {
		t.Fatalf("%+v", k)
	}
}

func TestWorkspaceRuleSetRefusesWhatIsNoRuleAndWhoIsNotTheOwner(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bobWithKid(t, r)
	for _, c := range []struct{ params, says string }{
		{`{"workspace":"bob","key":"agent_cap","value":"x","request_id":"b1"}`, "whole number"},
		{`{"workspace":"bob","key":"land_by","value":"x","request_id":"b2"}`, "not a kept rule"},
		{`{"workspace":"bob","key":"land_by","value":null,"request_id":"b3"}`, "not a kept rule"},
		{`{"workspace":"bob","key":"lease_max.gpu","value":1,"request_id":"b4"}`, "no lease kind gpu"},
		{`{"workspace":"bob","key":"accounts","value":["nobody"],"request_id":"b5"}`, "no account nobody"},
		{`{"workspace":"bob","key":"mode_max","value":"bypass","request_id":"b6"}`, "one of plan"},
		{`{"workspace":"bob","key":"agent_cap","request_id":"b7"}`, "value"},
		{`{"workspace":"nope","key":"agent_cap","value":1,"request_id":"b8"}`, "no workspace nope"},
	} {
		if m := r.call("workspace.rule.set", c.params); m["error"] == nil || !strings.Contains(refusalOf(m), c.says) {
			t.Errorf("%s: %v", c.params, m)
		}
	}
	if m := r.callAs("bob", "workspace.rule.set", `{"workspace":"bob","key":"agent_cap","value":1,"request_id":"b9"}`); m["error"] == nil {
		t.Fatalf("an agent may not set a rule, its own workspace's either: %v", m)
	}
	var n int
	r.db.QueryRow("SELECT COUNT(*) FROM workspace_settings").Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows kept", n)
	}
}

func TestAnAccountRuleNamesAccountsThatAreThere(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	bobWithKid(t, r)
	res, err := r.db.Exec("INSERT INTO accounts(name, owner, created_at) VALUES('alpha', 'p1', 0)")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if err := os.MkdirAll(filepath.Join(r.home, "accounts", actor.Key(actor.Account, id)), 0o755); err != nil {
		t.Fatal(err)
	}
	if m := r.call("workspace.rule.set", `{"workspace":"bob","key":"accounts","value":["alpha"],"request_id":"a1"}`); m["result"] == nil {
		t.Fatalf("%v", m)
	}
}

func TestAWorkspaceNoAgentIsLeftInTakesItsRulesWithIt(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := r.db.Exec("INSERT INTO workspaces(name, folder, created_at) VALUES('empty', '', 0)"); err != nil {
		t.Fatal(err)
	}
	ws, _, _ := team.WorkspaceID(r.db, "empty")
	if m := r.call("workspace.rule.set", `{"workspace":"empty","key":"review_required","value":true,"request_id":"d1"}`); m["result"] == nil {
		t.Fatalf("%v", m)
	}
	if m := r.call("workspace.delete", `{"workspace":"empty","request_id":"d2"}`); m["result"] == nil {
		t.Fatalf("%v", m)
	}
	var n int
	r.db.QueryRow("SELECT COUNT(*) FROM workspace_settings WHERE workspace = ?", ws).Scan(&n)
	if n != 0 {
		t.Fatalf("%d rules left behind", n)
	}
}
