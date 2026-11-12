package team

import (
	"database/sql"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/storetest"
)

func TestARuleValueIsOfItsKeysKind(t *testing.T) {
	for _, c := range []struct {
		key  string
		in   any
		want any
		bad  string
	}{
		{RuleAgentCap, 3.0, int64(3), ""},
		{RuleAgentCap, 0.0, int64(0), ""},
		{RuleAgentCap, -1.0, nil, "whole number"},
		{RuleAgentCap, 1.5, nil, "whole number"},
		{RuleAgentCap, "3", nil, "whole number"},
		{"lease_max.browser", 2.0, int64(2), ""},
		{"lease_max.", 2.0, nil, "not a kept rule"},
		{RuleAccounts, []any{"alpha", " gamma ", "alpha"}, []string{"alpha", "gamma"}, ""},
		{RuleAccounts, []any{}, []string{}, ""},
		{RuleAccounts, []any{""}, nil, "list of names"},
		{RuleToolsDeny, "WebFetch", nil, "list of names"},
		{RuleModeMax, "acceptEdits", "acceptEdits", ""},
		{RuleModeMax, "", "", ""},
		{RuleModeMax, "bypass", nil, "one of plan, default, acceptEdits, auto"},
		{RuleReviewRequired, true, true, ""},
		{RuleReviewRequired, 1.0, nil, "true or false"},
		{"land_by", "x", nil, "not a kept rule"},
	} {
		got, err := ParseRule(c.key, c.in)
		if c.bad != "" {
			if err == nil || !strings.Contains(err.Error(), c.bad) {
				t.Errorf("%s %v: %v, want %q", c.key, c.in, err, c.bad)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %v: %#v %v", c.key, c.in, got, err)
		}
	}
}

// keptHome is a home with workspaces shop and lab, and its database.
func keptHome(t *testing.T) (string, *sql.DB) {
	t.Helper()
	home := t.TempDir()
	workspaces(t, home, "shop", "lab")
	d, err := storetest.OpenAt(t, paths.DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return home, d
}

// Each key: the workspace's value beats the machine's, the machine's holds with none on the workspace, and with neither there is no
// rule. A second workspace sees only the machine's.
func TestAWorkspacesRuleBeatsTheMachineDefaultWhichBeatsNone(t *testing.T) {
	for _, c := range []struct {
		key           string
		machine, mine any
	}{
		{RuleAccounts, []string{"alpha"}, []string{"gamma"}},
		{RuleAgentCap, int64(3), int64(1)},
		{"lease_max.simulator", int64(2), int64(1)},
		{RuleToolsDeny, []string{"WebFetch"}, []string{"Bash"}},
		{RuleModeMax, "acceptEdits", "plan"},
		{RuleReviewRequired, false, true},
	} {
		_, d := keptHome(t)
		shop, _, _ := WorkspaceID(d, "shop")
		lab, _, _ := WorkspaceID(d, "lab")
		get := func(ws int64) *KeptRule {
			rules, err := KeptRules(d, ws)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rules {
				if r.Key == c.key {
					return &r
				}
			}
			return nil
		}
		if r := get(shop); r != nil {
			t.Errorf("%s: set nowhere, yet %+v", c.key, r)
		}
		must(t, SetRule(d, MachineRules, c.key, c.machine, "p1", 10))
		if r := get(shop); r == nil || !reflect.DeepEqual(r.Value, c.machine) || r.Source != "machine" {
			t.Errorf("%s: the machine's holds: %+v", c.key, r)
		}
		must(t, SetRule(d, shop, c.key, c.mine, "p1", 20))
		if r := get(shop); r == nil || !reflect.DeepEqual(r.Value, c.mine) || r.Source != "workspace" || r.SetBy != "p1" || r.SetAt != 20 {
			t.Errorf("%s: the workspace's beats it: %+v", c.key, r)
		}
		if r := get(lab); r == nil || !reflect.DeepEqual(r.Value, c.machine) {
			t.Errorf("%s: another workspace keeps the machine's: %+v", c.key, r)
		}
		must(t, ClearRule(d, shop, c.key))
		if r := get(shop); r == nil || r.Source != "machine" {
			t.Errorf("%s: cleared, the machine's holds again: %+v", c.key, r)
		}
	}
}

func TestAWorkspacesNoRuleStillBeatsTheMachines(t *testing.T) {
	_, d := keptHome(t)
	shop, _, _ := WorkspaceID(d, "shop")
	must(t, SetRule(d, MachineRules, RuleAgentCap, int64(3), "p1", 1))
	must(t, SetRule(d, shop, RuleAgentCap, int64(0), "p1", 2))
	rules, _ := KeptRules(d, shop)
	if len(rules) != 1 || rules[0].Value != int64(0) || RuleOn(rules[0].Value) || RulesBlock(rules) != "" {
		t.Fatalf("0 on the workspace is no rule there: %+v", rules)
	}
}

// The rules in force render under the team's text, in rule order, the ones that wait for a session start saying so; a change shows in
// the text, so the rules hash tells running agents (RulesNote).
func TestTheRulesInForceAreRenderedUnderTheTeamsText(t *testing.T) {
	home, d := keptHome(t)
	shop, _, _ := WorkspaceID(d, "shop")
	must(t, os.MkdirAll(paths.WorkspaceDir(home, "shop"), 0o755))
	must(t, os.WriteFile(RulesFile(home, "shop"), []byte("be kind\n"), 0o644))
	tm := &Team{Teams: map[string][]string{"shop": {"a"}, "lab": {"b"}}}
	before := Rules(home, "a", tm, KeptBlock(d))
	if !reflect.DeepEqual(before, []string{"# Rules for team shop\nbe kind"}) {
		t.Fatalf("no rule: the text alone, as before: %q", before)
	}
	must(t, SetRule(d, shop, RuleReviewRequired, true, "p1", 1))
	must(t, SetRule(d, shop, RuleToolsDeny, []string{"WebFetch", "Bash"}, "p1", 1))
	must(t, SetRule(d, MachineRules, RuleAgentCap, int64(3), "p1", 1))
	must(t, SetRule(d, MachineRules, "lease_max.simulator", int64(1), "p1", 1))
	must(t, SetRule(d, MachineRules, RuleAccounts, []string{}, "p1", 1))
	want := "# Rules for team shop\nbe kind\n\nKept rules (the owner's; the daemon holds to them): agent_cap 3; lease_max.simulator 1; " +
		"tools_deny WebFetch, Bash (from the next session); review_required on."
	if got := Rules(home, "a", tm, KeptBlock(d)); !reflect.DeepEqual(got, []string{want}) {
		t.Fatalf("got %q", got)
	}
	// lab has no text: the machine's rules alone make its part
	if got := Rules(home, "b", tm, KeptBlock(d)); len(got) != 1 || !strings.HasPrefix(got[0], "# Rules for team lab\nKept rules (the owner's; the daemon holds to them): agent_cap 3; lease_max.simulator 1.") {
		t.Fatalf("lab: %q", got)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestRemovingAWorkspacesLastAgentTakesItsRulesWithIt(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	register(t, r, "solo", "other")
	d, err := storetest.OpenAt(t, paths.DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	solo, _, _ := WorkspaceID(d, "solo")
	other, _, _ := WorkspaceID(d, "other")
	must(t, SetRule(d, solo, RuleAgentCap, int64(1), "p1", 1))
	must(t, SetRule(d, other, RuleAgentCap, int64(1), "p1", 1))
	must(t, r.Remove("solo"))
	var left []int64
	rows, _ := d.Query("SELECT workspace FROM workspace_settings")
	for rows.Next() {
		var ws int64
		rows.Scan(&ws)
		left = append(left, ws)
	}
	rows.Close()
	if !reflect.DeepEqual(left, []int64{other}) {
		t.Fatalf("left %v, want only %d's", left, other)
	}
}
