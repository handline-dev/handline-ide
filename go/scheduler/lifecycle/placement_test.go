package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/protocol/sessiontoken"
)

// Rows E1 and E2 of docs/design/account-assignment-scenarios.md: placements racing for slots, on the real Launcher and usage
// adapter, the clock fixed at usageNow. No sleeps: the racers start together off a closed channel.

type placeRig struct {
	l *Launcher
	u *UsageAdapter
	q db.Q
}

// newPlaceRig is accounts (cap 2 each) with one agent running on each, and agents x and y to place.
func newPlaceRig(t *testing.T, accounts ...string) *placeRig {
	t.Helper()
	was := db.Now
	db.Now = func() int64 { return usageNow }
	t.Cleanup(func() { db.Now = was })
	statuses := map[string]map[string]any{}
	for _, a := range accounts {
		statuses[a] = liveAccount(10, 10, 90000)
	}
	env := usageHome(t, statuses)
	for _, a := range []string{"x", "y"} {
		teamtest.Set(t, env.Home, a, "vendor", "claude")
	}
	q := teamtest.DB(t, env.Home)
	for _, a := range accounts {
		teamtest.Account(t, env.Home, a)
		run := "run-" + a
		db.SetAgentState(q, run, states.Running, 0, "")
		db.SetAccount(q, run, a, "")
	}
	u := &UsageAdapter{Env: env, Q: q}
	l := &Launcher{Q: q, Home: env.Home, Roster: team.NewRoster(env.Home), Tokens: sessiontoken.New(env.Home), Ports: LaunchPorts{
		Usage: u, Accounts: fakeAccounts{true}, Vendors: fakeVendors{"claude"},
		CurrentAccount: func(db.Q, string) string { return "" }, TeamRules: func(string) []string { return nil },
		Routing: func() *team.Team { return nil }, RegisterWorker: func(string, string) error { return nil },
	}}
	return &placeRig{l: l, u: u, q: q}
}

// race places x and y at once and returns where each went ("" for nowhere), then lets both holds go for the next round.
func (r *placeRig) race(t *testing.T) map[string]string {
	t.Helper()
	got := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, a := range []string{"x", "y"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			acct, err := r.l.place(a, false, "")
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			got[a] = acct
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	for _, a := range []string{"x", "y"} {
		if _, err := db.AddEvent(r.q, a, "wake_dropped", db.KV{K: "why", V: "next round"}); err != nil {
			t.Fatal(err)
		}
	}
	return got
}

const raceRounds = 200

func TestTwoPlacementsForOneFreeSlotPlaceOne(t *testing.T) { // E1
	r := newPlaceRig(t, "alpha")
	for i := range raceRounds {
		got := r.race(t)
		if n := btoi(got["x"] == "alpha") + btoi(got["y"] == "alpha"); n != 1 || got["x"]+got["y"] != "alpha" {
			t.Fatalf("round %d: %v - exactly one may take alpha's one free slot, the other none", i, got)
		}
	}
}

func TestTwoPlacementsForTwoAccountsWithOneSlotEachPlaceOneEach(t *testing.T) { // E2
	r := newPlaceRig(t, "alpha", "beta")
	for i := range raceRounds {
		got := r.race(t)
		if got["x"] == "" || got["y"] == "" || got["x"] == got["y"] {
			t.Fatalf("round %d: %v - one on each, never both on one", i, got)
		}
	}
}

// A launch slower than WakeReserveS loses its hold at the look-back, before its session reports: the slot then reads free.
func TestAHeldSlotCountsUntilItsLookBackEndsThenGoes(t *testing.T) {
	r := newPlaceRig(t, "alpha")
	if acct, err := r.l.place("x", false, ""); err != nil || acct != "alpha" {
		t.Fatalf("placed on %q (%v)", acct, err)
	}
	now := usageNow
	r.u.Env.Now = func() float64 { return float64(now) }
	for _, c := range []struct {
		after int64
		want  int
	}{{usage.WakeReserveS - 1, 2}, {usage.WakeReserveS, 1}} {
		now = usageNow + c.after
		if n := r.u.ActiveOn(r.q, "alpha", ""); n != c.want {
			t.Fatalf("+%d s: %d mid-turn on alpha, want %d", c.after, n, c.want)
		}
	}
	// A session that has reported counts by its state, whatever the age of its hold.
	db.SetAgentState(r.q, "x", states.Running, 0, "")
	db.SetAccount(r.q, "x", "alpha", "")
	if n := r.u.ActiveOn(r.q, "alpha", ""); n != 2 {
		t.Fatalf("running: %d", n)
	}
}

func TestALaunchRefusedPastItsHoldLetsTheSlotGo(t *testing.T) {
	r := newPlaceRig(t, "alpha")
	teamtest.Set(t, r.l.Home, "x", "vendor", "claude", "effort", "bogus") // refused after the hold: effort is checked late
	if _, _, err := r.l.Build(&Site{Cwd: t.TempDir(), Env: map[string]string{}}, "x", nil, false, ""); err == nil {
		t.Fatal("built")
	}
	if _, _, ok, _ := db.LastEvent(r.q, "x", "wake_reserved"); !ok {
		t.Fatal("no hold was taken: the test does not reach the refusal it means to")
	}
	if _, _, ok, _ := db.LastEvent(r.q, "x", usage.HoldDroppedKind); !ok {
		t.Fatal("the hold was not let go")
	}
	if _, _, ok, _ := db.LastEvent(r.q, "x", "wake_dropped"); ok {
		t.Fatal("wake_dropped is the wake's own answer, not a launch's")
	}
	if n := r.u.ActiveOn(r.q, "alpha", ""); n != 1 {
		t.Fatalf("%d mid-turn on alpha: the refused launch still holds a slot", n)
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// lockedLaunch is a Launch whose decisions and the hold they lead to must all fall inside one Placing.
type lockedLaunch struct {
	fakeLaunch
	q      db.Q
	held   *bool
	missed *[]string
}

func (l lockedLaunch) Placing() func() {
	*l.held = true
	return func() {
		if _, _, ok, _ := db.LastEvent(l.q, "a", "wake_reserved"); !ok {
			*l.missed = append(*l.missed, "the hold was written after the lock let go")
		}
		*l.held = false
	}
}

func (l lockedLaunch) ChooseAccount(q db.Q, agent string) bool {
	if !*l.held {
		*l.missed = append(*l.missed, "ChooseAccount outside the lock")
	}
	return true
}

func (l lockedLaunch) AccountFor(q db.Q, agent string, resuming bool) string {
	if !*l.held {
		*l.missed = append(*l.missed, "AccountFor outside the lock")
	}
	return l.fakeLaunch.AccountFor(q, agent, resuming)
}

func TestAWakeChoosesAndHoldsItsSlotUnderThePlacementLock(t *testing.T) {
	m, d, tm := newManager(t)
	d.waitingMsg, d.tookPrompt, tm.launchOK = "Here is your request.", true, true
	var held bool
	var missed []string
	m.Launch = lockedLaunch{q: m.Q, held: &held, missed: &missed}
	db.SetTerminal(m.Q, "a", "t1", "/wt")
	db.SetClosed(m.Q, "a", "")
	db.AddEvent(m.Q, "a", "Stop")
	if ok, said, err := m.WakeAgentLocked("a", WakeOpts{Force: true}); err != nil || !ok {
		t.Fatalf("%v %q %v", ok, said, err)
	}
	if len(missed) > 0 || held {
		t.Fatalf("%v (still held: %v)", missed, held)
	}
}

// E5: a reading that lands between the pick's first look and its choice is the one the choice uses. alpha's status is old (idle
// floor), so the pick asks it fresh; the fresh reading says it is full, and nothing is placed there.
func TestAPlacementUsesTheReadingThatLandedBeforeItsChoice(t *testing.T) {
	r := newPlaceRig(t, "alpha")
	stale := liveAccount(10, 10, 90000)
	stale["collected_at"] = usageNow - usage.StaleAfterS - 60
	for _, w := range stale["windows"].([]any) {
		w.(map[string]any)["observed_at"] = usageNow - usage.StaleAfterS - 60
	}
	db.SetAgentState(r.q, "run-alpha", states.Idle, 0, "") // the idle floor needs nobody mid-turn
	writeUsage(t, r.u.Env.Home, "alpha", stale)
	asked := 0
	r.u.Env.Refresh = func(acct string) bool {
		asked++
		writeUsage(t, r.u.Env.Home, acct, liveAccount(usage.WallPct, 10, 90000))
		return true
	}
	if acct, err := r.l.place("x", false, ""); err != nil || acct != "" {
		t.Fatalf("placed on %q (%v): the reading that landed said full", acct, err)
	}
	if asked != 1 {
		t.Fatalf("asked fresh %d times: the test does not reach the read it means to", asked)
	}
}

func writeUsage(t *testing.T, home, acct string, raw map[string]any) {
	t.Helper()
	b, _ := json.Marshal(raw)
	if err := os.WriteFile(filepath.Join(teamtest.AccountDir(t, home, acct), "status.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
