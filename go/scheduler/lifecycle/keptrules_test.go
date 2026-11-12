package lifecycle

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
)

// keptRig is the launch rig with agents in two workspaces, shop (its lead and bob) and lab, and accounts a, b and c; keys by old name.
func keptRig(t *testing.T) (*launchRig, map[string]string, func(ws, key string, v any)) {
	t.Helper()
	r := newLaunchRig(t)
	teamtest.Seed(t, r.l.Home, "shop", "shop.bob", "lab")
	addAccounts(t, r, "a", "b", "c")
	keys := map[string]string{}
	for _, n := range []string{"shop", "shop.bob", "lab"} {
		keys[n] = teamtest.Key(t, r.l.Home, n)
	}
	set := func(ws, key string, v any) {
		t.Helper()
		id := int64(team.MachineRules)
		if ws != "" {
			var ok bool
			if id, ok, _ = team.WorkspaceID(r.l.Q, ws); !ok {
				t.Fatalf("no workspace %s", ws)
			}
		}
		if err := team.SetRule(r.l.Q, id, key, v, "p1", 1); err != nil {
			t.Fatal(err)
		}
	}
	return r, keys, set
}

func TestTheAccountsRuleLeavesItsWorkspacesAgentsOnlyTheAccountsItNames(t *testing.T) {
	r, keys, set := keptRig(t)
	shop, lab := keys["shop"], keys["lab"]
	if got := r.l.FindRoom(shop); got != "a" {
		t.Fatalf("no rule, the first fit: %q", got)
	}
	set("", team.RuleAccounts, []string{"c"})
	if got := r.l.FindRoom(shop); got != "c" {
		t.Fatalf("the machine's rule holds: %q", got)
	}
	set("shop", team.RuleAccounts, []string{"b"})
	if got := r.l.FindRoom(shop); got != "b" {
		t.Fatalf("the workspace's beats it: %q", got)
	}
	if got := r.l.FindRoom(lab); got != "c" {
		t.Fatalf("another workspace keeps the machine's: %q", got)
	}
	if got := r.l.ChooseAccount(shop, false, "a"); got != "" {
		t.Fatalf("a destination the rule leaves out is refused: %q", got)
	}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "a" }
	if got := r.l.ChooseAccount(shop, true, ""); got != "b" {
		t.Fatalf("a session on an account the rule leaves out starts on one it names: %q", got)
	}
	row := &db.AgentState{Agent: shop, ClosedAt: sql.NullInt64{Int64: 5, Valid: true}}
	const why = "is left out by its workspace's rule accounts b (set by "
	for name, got := range map[string]string{"RoomOn": r.l.RoomOn(r.l.Q, shop, row, "a"), "RoomPastFit": r.l.RoomPastFit(r.l.Q, shop, row, "a")} {
		if !strings.HasPrefix(got, why) {
			t.Errorf("%s: %q", name, got)
		}
	}
	// A resume of a session on an account the rule leaves out waits on none of that account's walls: it is carried to one it names.
	pct := 99
	r.l.Ports.Usage = fakeUsage{by: map[string]AccountStatus{"a": {Exists: true, ShortTrusted: &pct, ShortLabel: "5h", ShortReset: 1000}, "b": {Exists: true}}}
	r.l.Ports.AccountReading = func(db.Q, string) (string, string, bool) { return "a", "recorded", true }
	resume := &db.AgentState{Agent: shop, ClosedAt: sql.NullInt64{Int64: 5, Valid: true}, ResumeSession: sql.NullString{String: "s1", Valid: true}}
	if w := r.l.WakeWait(r.l.Q, shop, resume); w != nil {
		t.Fatalf("%+v", w)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}}
	teamtest.Set(t, r.l.Home, "shop", "account", "a")
	if got := r.l.FindRoom(shop); got != "" {
		t.Fatalf("a pin the rule leaves out takes it nowhere: %q", got)
	}
	if got := r.l.NoRoom(shop); !strings.HasPrefix(got, "it is pinned to a, which "+why) {
		t.Fatal(got)
	}
	if got := r.l.ChooseAccount(shop, true, ""); got != "" {
		t.Fatalf("nor does a resume: %q", got)
	}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "b" }
	if _, _, err := r.build(shop, []string{"--resume", "s1"}, false, ""); err == nil || !strings.Contains(err.Error(), "it is pinned to a, which "+why) {
		t.Fatalf("a resume's launch says why: %v", err)
	}
	teamtest.Set(t, r.l.Home, "shop", "account", "")
	if _, _, err := r.build(shop, nil, false, "a"); err == nil || !strings.Contains(err.Error(), "account 'a' "+why) {
		t.Fatalf("a launch onto it names the rule: %v", err)
	}
	set("shop", team.RuleAccounts, []string{"zz"})
	if got := r.l.NoRoom(shop); !strings.HasPrefix(got, "no account its workspace's rule accounts zz (set by ") {
		t.Fatal(got)
	}
	set("shop", team.RuleAccounts, []string{})
	if got := r.l.FindRoom(shop); got != "a" {
		t.Fatalf("an empty list on the workspace is no rule, over the machine's: %q", got)
	}
}

func TestAgentCapHoldsAWakeWhileThatManyOfItsWorkspaceAreMidTurn(t *testing.T) {
	r, keys, set := keptRig(t)
	shop, bob, lab := keys["shop"], keys["shop.bob"], keys["lab"]
	row := &db.AgentState{Agent: shop, ClosedAt: sql.NullInt64{Int64: 5, Valid: true}}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, midTurn: []string{bob, lab}}
	if w := r.l.WakeWait(r.l.Q, shop, row); w != nil {
		t.Fatalf("no rule: %+v", w)
	}
	set("shop", team.RuleAgentCap, int64(2))
	if w := r.l.WakeWait(r.l.Q, shop, row); w != nil {
		t.Fatalf("one of two mid-turn, another workspace's not counted: %+v", w)
	}
	set("shop", team.RuleAgentCap, int64(1))
	w := r.l.WakeWait(r.l.Q, shop, row)
	if w == nil || w.Workspace != "shop" {
		t.Fatalf("%+v", w)
	}
	if got := DescribeWakeWait(w); !strings.HasPrefix(got, "waiting for a slot in workspace shop (1 of 1 of its agents mid-turn; its rule agent_cap 1 (set by ") {
		t.Fatal(got)
	}
	if got := r.l.WakeWaitReason(r.l.Q, shop, row); got != DescribeWakeWait(w) {
		t.Fatal(got)
	}
	labRow := &db.AgentState{Agent: lab, ClosedAt: sql.NullInt64{Int64: 5, Valid: true}}
	if w := r.l.WakeWait(r.l.Q, lab, labRow); w != nil {
		t.Fatalf("another workspace is not held: %+v", w)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, midTurn: []string{shop}}
	if w := r.l.WakeWait(r.l.Q, shop, row); w != nil {
		t.Fatalf("the agent itself is not counted against its own wake: %+v", w)
	}
	set("shop", team.RuleAgentCap, int64(0))
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, midTurn: []string{bob}}
	if w := r.l.WakeWait(r.l.Q, shop, row); w != nil {
		t.Fatalf("0 is no rule: %+v", w)
	}
}

// capLaunch: the agent's workspace has its agent_cap mid-turn, and another account has room.
type capLaunch struct{ slowLaunch }

func (capLaunch) WorkspaceWait(db.Q, string) *WakeWait {
	return &WakeWait{Slot: true, Workspace: "shop", Full: 1, Busy: 1, Rule: &team.KeptRule{Key: team.RuleAgentCap, Value: int64(1), SetBy: "p1"}}
}

func TestAWakeAtItsWorkspacesCapWaitsEvenForAPersonAndMovesNowhere(t *testing.T) {
	m, d, tm := newManager(t)
	d.waitingMsg, d.tookPrompt, tm.launchOK = "Here is your request.", true, true
	m.Launch = capLaunch{}
	db.SetTerminal(m.Q, "a", "t1", "/wt")
	db.SetClosed(m.Q, "a", "sess-1")
	ok, said, err := m.WakeAgentLocked("a", WakeOpts{Force: true, Person: true})
	if err != nil || ok || !strings.HasPrefix(said, "waiting for a slot in workspace shop") || len(tm.relaunched) != 0 {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if _, _, moved, _ := db.LastEvent(m.Q, "a", "moved_account"); moved {
		t.Fatal("no account move frees a workspace's cap")
	}
	db.SetTerminal(m.Q, "a", "", "")
	m.Agents = fakeAgents{repo: "/repo"}
	db.AddPending(m.Q, "a", "x", 1)
	if said, err := m.StartForWork("a"); err != nil || !strings.HasPrefix(said, "a: not started - waiting for a slot in workspace shop") {
		t.Fatalf("nor does a start for waiting work: %q %v", said, err)
	}
}

// nowhereLaunch: no account takes the agent's resume (its pin is one its workspace's accounts rule leaves out).
type nowhereLaunch struct{ fakeLaunch }

func (nowhereLaunch) AccountFor(db.Q, string, bool) string { return "" }
func (nowhereLaunch) NoRoom(string) string                 { return "it is pinned to a, which is left out" }

func TestAResumeWithNowhereToGoTypesNothing(t *testing.T) {
	m, d, tm := newManager(t)
	d.waitingMsg, d.tookPrompt, tm.launchOK = "Here is your request.", true, true
	m.Launch = nowhereLaunch{}
	db.SetTerminal(m.Q, "a", "t1", "/wt")
	db.SetClosed(m.Q, "a", "sess-1")
	db.AddEvent(m.Q, "a", "Stop")
	ok, said, err := m.WakeAgentLocked("a", WakeOpts{Force: true})
	if err != nil || ok || said != "it is pinned to a, which is left out" || len(tm.relaunched) != 0 {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if row, _ := db.GetAgentState(m.Q, "a"); row.WakeFailures != 0 {
		t.Fatalf("not counted against it: %d", row.WakeFailures)
	}
}
