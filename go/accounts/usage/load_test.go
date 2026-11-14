package usage

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
)

const clock = int64(1_800_000_000)

func setAgent(t *testing.T, q db.Q, agent, state, account string) {
	t.Helper()
	if err := db.SetAgentState(q, agent, state, 0, ""); err != nil {
		t.Fatal(err)
	}
	if account != "" {
		if err := db.SetAccount(q, agent, account, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func event(t *testing.T, q db.Q, ts int64, agent, kind, account string) {
	t.Helper()
	if account != "" {
		if _, err := q.Exec("INSERT INTO events(ts, agent, account, kind) VALUES(?,?,?,?)", ts, agent, account, kind); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := db.AddEventAt(q, ts, agent, kind); err != nil {
		t.Fatal(err)
	}
}

func TestActiveCountsRunningMovedAndReservedAgentsPerAccount(t *testing.T) {
	q := openDB(t)
	e := testEnv(t.TempDir(), clock)
	setAgent(t, q, "run", "RUNNING", "a")
	setAgent(t, q, "idle", "IDLE", "a")
	setAgent(t, q, "moved", "IDLE", "b")
	event(t, q, clock-50, "moved", "moved_account", "")
	setAgent(t, q, "woken", "IDLE", "a")
	event(t, q, clock-100, "woken", "wake_reserved", "c")
	setAgent(t, q, "late", "IDLE", "a")
	event(t, q, clock-WakeReserveS-1, "late", "wake_reserved", "c") // too old to hold a slot
	setAgent(t, q, "seen", "IDLE", "a")
	event(t, q, clock-100, "seen", "wake_reserved", "c")
	event(t, q, clock-90, "seen", "Stop", "") // seen running: the reservation is spent
	setAgent(t, q, "failed", "IDLE", "a")
	event(t, q, clock-100, "failed", "wake_reserved", "c")
	event(t, q, clock-90, "failed", "wake_failed", "")
	setAgent(t, q, "dropped", "IDLE", "a")
	event(t, q, clock-100, "dropped", "wake_reserved", "c")
	event(t, q, clock-90, "dropped", "wake_dropped", "") // the wake gave up: the reservation is let go
	setAgent(t, q, "closed", "RUNNING", "a")
	db.SetClosed(q, "closed", "s")
	by, err := e.ActiveByAccount(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(by["a"]) != 1 || by["a"][0] != "run" || len(by["b"]) != 1 || by["b"][0] != "moved" || len(by["c"]) != 1 || by["c"][0] != "woken" {
		t.Fatalf("%v", by)
	}
	n, _ := e.ActiveAgents(q, "run")
	if n["a"] != 0 || n["b"] != 1 || n["c"] != 1 {
		t.Fatalf("a placement is counted against the slot it holds: %v", n)
	}
	// a moved agent counts only until its next Stop
	event(t, q, clock-10, "moved", "Stop", "")
	if by, _ = e.ActiveByAccount(q); len(by["b"]) != 0 {
		t.Fatalf("%v", by)
	}
}

// costsAt is Costs whose 5-hour factor is $1 a point on each of accts, so an agent costs its USD per minute / 60 percent per second.
// samples are agents' costs in that unit, and ratios the week's points per 5-hour point by account, "*" for any other.
func costsAt(samples []float64, ratios map[string]float64, accts ...string) *Costs {
	c := &Costs{pace: &DollarPace{}, factors: map[string]AccountFactors{}}
	for _, r := range samples {
		c.pace.Samples = append(c.pace.Samples, DollarSample{R: r * 60})
	}
	for _, a := range accts {
		f := AccountFactors{Short: Factor{USDPerPct: 1, Source: FactorOwn}}
		r, ok := ratios[a]
		if !ok {
			r, ok = ratios["*"]
		}
		if ok && r > 0 {
			f.Long = Factor{USDPerPct: 1 / r, Source: FactorOwn}
		}
		c.factors[a] = f
	}
	return c
}

// seedCosts writes the dollar pace (n samples of usdPerMin) and the factors the daemon would have kept.
func seedCosts(t *testing.T, q db.Q, usdPerMin float64, n int, factors map[string]AccountFactors) {
	t.Helper()
	p := DollarPace{At: 1}
	for range n {
		p.Samples = append(p.Samples, DollarSample{R: usdPerMin})
	}
	putMeta(t, q, dollarPaceMeta, p)
	putMeta(t, q, factorMeta, map[string]any{"at": 1, "accounts": factors})
}

func putMeta(t *testing.T, q db.Q, name string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := db.SetMeta(q, name, string(b)); err != nil {
		t.Fatal(err)
	}
}

func TestCostsPriceAnAgentByItsModelsDollarsOverTheAccountsFactor(t *testing.T) {
	q := openDB(t)
	e := testEnv(t.TempDir(), clock)
	e.AgentConfig = func(agent string) (team.Config, error) {
		return team.Config{"model": map[string]string{"x": "opus", "y": "sonnet"}[agent], "effort": "high"}, nil
	}
	c, _ := e.LoadCosts(q)
	if p, src, n := c.Describe("a", "x"); p != TypicalAgentBurnPerS || src != "starting value" || n != 0 {
		t.Fatalf("nothing measured: %v %q %d", p, src, n)
	}
	p := DollarPace{At: 1}
	for range 5 {
		p.Samples = append(p.Samples, DollarSample{R: 0.6, M: "opus"}, DollarSample{R: 0.2, M: "sonnet"})
	}
	putMeta(t, q, dollarPaceMeta, p)
	putMeta(t, q, factorMeta, map[string]any{"at": 1, "accounts": map[string]AccountFactors{
		"a": {Short: Factor{USDPerPct: 0.4, Source: FactorOwn}, Long: Factor{USDPerPct: 3.2, Source: FactorOwn}}}})
	c, _ = e.LoadCosts(q)
	// $0.60 a minute over $0.40 a point: 1.5 points a minute.
	if p, src, n := c.Describe("a", "x"); src != "opus" || n != 5 || math.Abs(p-1.5/60) > 1e-12 {
		t.Fatalf("%v %q %d", p, src, n)
	}
	if _, src, n := c.Describe("a", ""); src != "all agents" || n != 10 {
		t.Fatalf("%q %d", src, n)
	}
	if p, src, _ := c.Describe("other", "x"); p != TypicalAgentBurnPerS || src != "starting value" {
		t.Fatalf("an account with no factor: %v %q", p, src)
	}
	if r, ok := c.LongPerShort("a"); !ok || r != 0.125 {
		t.Fatalf("a 5-hour point is an eighth of a week point: %v %v", r, ok)
	}
	if _, ok := c.LongPerShort("other"); ok {
		t.Fatal("no factor, no ratio")
	}
	if b, ok := c.Burn("a", []string{"x", "y"}); !ok || math.Abs(b-(1.5+0.5)/60) > 1e-12 {
		t.Fatalf("burn %v %v", b, ok)
	}
	if _, ok := c.Burn("other", []string{"x"}); ok {
		t.Fatal("no factor, no burn")
	}
	if m := median([]float64{1, 2, 3, 10}); m != 2.5 {
		t.Fatalf("%v", m)
	}
}

func TestReadAllGivesEveryAccountTheBurnOfItsAgents(t *testing.T) {
	q := openDB(t)
	home := t.TempDir()
	e := testEnv(home, clock)
	for _, a := range []string{"a", "b"} {
		os.MkdirAll(teamtest.AccountDir(t, home, a), 0o755)
	}
	seedCosts(t, q, 0.6, 5, map[string]AccountFactors{"b": {Short: Factor{USDPerPct: 0.4, Source: FactorOwn}}})
	setAgent(t, q, "run", "RUNNING", "b")
	loads, err := e.ReadAll(q, "")
	if err != nil {
		t.Fatal(err)
	}
	a, b := loads["a"], loads["b"]
	if a.Burn != nil || b.Burn == nil || math.Abs(*b.Burn-1.5/60) > 1e-12 || b.Active != 1 {
		t.Fatalf("%+v %+v", a, b)
	}
	if loads, _ := e.ReadAll(q, "run"); loads["b"].Active != 0 || *loads["b"].Burn != 0 {
		t.Fatalf("the agent being placed is left out: %+v", loads["b"])
	}
}

func TestReadForAgentTakesContextFromTheAgentsOwnFreshSnapshotOnly(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	live := map[string]any{"windows": []any{}, "collected_at": clock - 5, "ctx_pct": 40, "session_id": "own", "model_id": "m", "account": "a", "cost_usd": 2.5}
	writeStatus(t, home, "a", map[string]any{"windows": []any{}, "collected_at": clock - 5, "ctx_pct": 99, "session_id": "other"})
	write := func(agent string, v any) {
		teamtest.Seed(t, home, agent)
		b, _ := json.Marshal(v)
		os.WriteFile(paths.AgentStatusFile(home, agent), b, 0o644)
	}
	if st := e.ReadForAgent("none", ""); st.Account != "" || st.Exists {
		t.Fatalf("an agent on no account knows nothing: %+v", st)
	}
	if st := e.ReadForAgent("fresh", "a"); st.CtxPct != nil || st.SessionID != "" {
		t.Fatalf("never ran: another agent's context was borrowed: %+v", st)
	}
	write("x", live)
	if st := e.ReadForAgent("x", "a"); st.CtxPct == nil || *st.CtxPct != 40 || st.SessionID != "own" || st.ModelID != "m" {
		t.Fatalf("%+v", st)
	}
	stale := map[string]any{"windows": []any{}, "collected_at": clock - 5000, "ctx_pct": 40, "session_id": "old"}
	write("y", stale)
	if st := e.ReadForAgent("y", "a"); st.CtxPct != nil || st.SessionID != "" {
		t.Fatalf("a stale snapshot is not trusted: %+v", st)
	}
	write("z", "{broken")
	if st := e.ReadForAgent("z", "a"); st.SessionID != "" {
		t.Fatalf("%+v", st)
	}
	if got := e.ReportedAccount("x"); got != "a" {
		t.Fatalf("%q", got)
	}
	if e.ReportedAccount("y") != "" || e.ReportedAccount("nobody") != "" {
		t.Fatal("a stale or missing snapshot reports no account")
	}
	if c, ok := e.LiveCost("a"); !ok || c != 2.5 {
		t.Fatalf("%v %v", c, ok)
	}
	if _, ok := e.LiveCost("b"); ok {
		t.Fatal("cost on an account nothing reports on")
	}
}

func TestAccountVerdictSaysWhyAnAccountCannotTakeWork(t *testing.T) {
	home := t.TempDir()
	e := testEnv(home, clock)
	writeStatus(t, home, "a", map[string]any{"collected_at": clock - 5, "windows": []any{
		map[string]any{"minutes": 300, "used_percent": 10, "resets_at": clock + 3600, "observed_at": clock - 5},
		map[string]any{"minutes": 10080, "used_percent": 10, "resets_at": clock + 90000, "observed_at": clock - 5}}})
	st := e.Read("a")
	ld := &Load{Account: "a", Active: 0}
	if ok, why := e.AccountVerdict("a", st, ld, nil, false); ok || why != "not logged in" {
		t.Fatalf("%v %q", ok, why)
	}
	if ok, why := e.AccountVerdict("a", st, ld, nil, true); !ok || why != "5h 10%" {
		t.Fatalf("%v %q", ok, why)
	}
	if ok, why := e.AccountVerdict("a", st, ld, []Reason{{"a", "ranked", ""}}, true); !ok || why != "ranked" {
		t.Fatalf("%v %q", ok, why)
	}
	if ok, why := e.AccountVerdict("a", st, &Load{Account: "a", Active: 9}, nil, true); !ok || why != "5h 10%" {
		t.Fatalf("an account with many agents on it still takes one: %v %q", ok, why)
	}
	os.WriteFile(filepath.Join(teamtest.AccountDir(t, home, "a"), "vendor"), []byte("nonesuch\n"), 0o644)
	if ok, why := e.AccountVerdict("a", st, ld, nil, true); ok || why == "" {
		t.Fatalf("a vendor with no driver takes nothing: %v %q", ok, why)
	}
}

func TestAClosedRunningAgentHoldingAReservationCountsOnTheReservedAccountOnly(t *testing.T) {
	q := openDB(t)
	e := testEnv(t.TempDir(), clock)
	setAgent(t, q, "gone", "RUNNING", "a")
	db.SetClosed(q, "gone", "s")
	event(t, q, clock-100, "gone", "wake_reserved", "c")
	by, err := e.ActiveByAccount(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(by["a"]) != 0 || len(by["c"]) != 1 {
		t.Fatalf("a closed session is not running on the account it last had: %v", by)
	}
}

func TestByKindPricesEachKindOnceAndSaysWhatItRestsOn(t *testing.T) {
	kinds := map[string][2]string{"a": {"opus", "high"}, "b": {"opus", "high"}, "c": {"haiku", "low"}}
	c := &Costs{pace: &DollarPace{}, kindOf: func(n string) (string, string, bool) { k, ok := kinds[n]; return k[0], k[1], ok }}
	got := c.ByKind([]string{"a", "b", "c", "gone"})
	want := []string{
		"haiku low " + fmt.Sprintf("$%.3f", TypicalAgentUSDPerMin) + "/min (starting value)",
		"opus high " + fmt.Sprintf("$%.3f", TypicalAgentUSDPerMin) + "/min (starting value)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
}
