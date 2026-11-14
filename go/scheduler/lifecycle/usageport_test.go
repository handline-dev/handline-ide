package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/storetest"
)

const usageNow = int64(1_800_000_000)

func usageHome(t *testing.T, accounts map[string]map[string]any) *usage.Env {
	t.Helper()
	home := t.TempDir()
	for name, raw := range accounts {
		dir := teamtest.AccountDir(t, home, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if raw != nil {
			b, _ := json.Marshal(raw)
			os.WriteFile(filepath.Join(dir, "status.json"), b, 0o644)
		}
	}
	return &usage.Env{Home: home, Now: func() float64 { return float64(usageNow) },
		LoginState: func(string) (bool, bool) { return true, true }, PlanKey: func(string) string { return "" },
		AgentConfig: func(string) (team.Config, error) { return team.Config{}, nil }}
}

func liveAccount(short, week int, weekResetIn int64) map[string]any {
	win := func(minutes, pct int, resetIn int64) map[string]any {
		return map[string]any{"minutes": minutes, "used_percent": pct, "resets_at": usageNow + resetIn, "observed_at": usageNow - 5, "carried": false, "lost": false}
	}
	return map[string]any{"collected_at": usageNow - 5, "windows": []any{win(300, short, 3600), win(10080, week, weekResetIn)}}
}

func TestTheUsageAdapterPicksTheAccountFurthestBehindItsPaceAndCountsLoadWithoutTheAgent(t *testing.T) {
	env := usageHome(t, map[string]map[string]any{"soon": liveAccount(50, 10, 3600), "later": liveAccount(5, 10, 90000), "never": nil})
	q := openDB(t)
	u := &UsageAdapter{Env: env, Q: q}
	if got := u.PickAccount([]string{"soon", "later", "never"}, ""); got != "soon" {
		t.Fatalf("%q", got)
	}
	if got := u.PickAccount([]string{"never"}, ""); got != "never" {
		t.Fatalf("a never-run account is a real place to go: %q", got)
	}
	if got := u.PickAccount([]string{}, ""); got != "" {
		t.Fatalf("no candidate picks nobody: %q", got)
	}
	// Two agents on "soon" keep no start off it; the one being placed is left out of the count.
	for _, a := range []string{"w1", "w2"} {
		db.SetAgentState(q, a, states.Running, 0, "")
		db.SetAccount(q, a, "soon", "")
	}
	if got := u.PickAccount([]string{"soon", "later"}, ""); got != "soon" {
		t.Fatalf("no cap: %q", got)
	}
	if n := activeOn(t, u, q, "soon", "w1"); n != 1 {
		t.Fatalf("%d", n)
	}
	db.SetAgentState(q, "w3", states.Running, 0, "")
	db.SetAccount(q, "w3", "later", "")
	if got := u.MidTurn(q, "w1"); !slices.Equal(slices.Sorted(slices.Values(got)), []string{"w2", "w3"}) {
		t.Fatalf("mid-turn on every account, the agent left out: %v", got)
	}
	db.SetAgentState(q, "w3", states.Idle, 0, "")
	if !u.Admit("soon", "w1", "claude").Fits || !u.Admit("soon", "", "claude").Fits {
		t.Fatalf("fit, however many are on it")
	}
	if adm := u.Admit("soon", "w1", "opencode"); adm.Fits || len(adm.Reasons) != 1 {
		t.Fatalf("another vendor's account does not fit, with its reason: %+v", adm)
	}
	if !u.NotFor("soon", "opencode") || u.NotFor("soon", "claude") {
		t.Fatal("another vendor's account is not for this agent")
	}
	st := u.Read("later")
	if !st.Exists || st.Stale || st.ShortTrusted == nil || *st.ShortTrusted != 5 || st.ShortLabel != "5h" || st.LongLabel != "7d" {
		t.Fatalf("%+v", st)
	}
}

func TestALoadNobodyCanReadPicksNothing(t *testing.T) {
	env := usageHome(t, map[string]map[string]any{"a": liveAccount(5, 5, 90000)})
	q := openDB(t)
	u := &UsageAdapter{Env: env, Q: q}
	q.(interface{ Close() error }).Close()
	if r := u.PickReasons([]string{"a"}, ""); r != nil {
		t.Fatalf("reasons from a load nobody could read: %v", r)
	}
	if got := u.PickAccount([]string{"a"}, ""); got != "" {
		t.Fatalf("a pick on an unreadable load: %q", got)
	}
	if _, err := u.ActiveOn(q, "a", ""); err == nil {
		t.Fatal("an unreadable count read without an error")
	}
	if why, room := u.StartFit("a", ""); why == "" || room >= 0 {
		t.Fatalf("an unreadable load fits: %q, room %d", why, room)
	}
	if got := u.MidTurn(q, ""); got != nil {
		t.Fatalf("an unreadable count names nobody: %v", got)
	}
}

func TestTheUsageAdapterReadsWhichAccountIsPaused(t *testing.T) {
	env := usageHome(t, map[string]map[string]any{"rest": liveAccount(5, 5, 90000), "on": liveAccount(5, 5, 90000)})
	q, err := storetest.OpenAt(t, filepath.Join(env.Home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Close() })
	if err := usage.SetAccount(q, "rest", map[string]any{"paused": true}); err != nil {
		t.Fatal(err)
	}
	u := &UsageAdapter{Env: env, Q: q}
	if !u.Read("rest").Paused || u.Read("on").Paused {
		t.Fatalf("rest %+v, on %+v", u.Read("rest"), u.Read("on"))
	}
}

// activeOn is u.ActiveOn, failing the test on a count that cannot be read.
func activeOn(t *testing.T, u *UsageAdapter, q db.Q, acct, agent string) int {
	t.Helper()
	n, err := u.ActiveOn(q, acct, agent)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
