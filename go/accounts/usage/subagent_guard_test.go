package usage

import (
	"testing"

	"github.com/handline/handline/go/data/db"
)

// The guard is off by default: a fresh config records the start and refuses nothing, even when one more subagent would take the account
// past its 5h wall (Predict); the setting on refuses it and records nothing.
func TestTheSubagentGuardRefusesOnlyWhenTheSettingIsOn(t *testing.T) {
	home, q := t.TempDir(), openDB(t)
	accountAt(t, home, 50, 60, 10, 86400)
	// 50 points left with an hour to the reset. The parent alone, at $0.80/min on $1 a point, fills 0.8%/min and lasts 62 minutes; with
	// its subagent at the same cost, 31.
	seedCosts(t, q, 0.8, 6, map[string]AccountFactors{"a": {Short: Factor{USDPerPct: 1, Source: FactorOwn}}})
	setAgent(t, q, "x", "RUNNING", "a")
	starts := func() int {
		var n int
		if err := q.QueryRow("SELECT COUNT(*) FROM events WHERE kind = ?", db.SubagentStart).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	e := testEnv(home, clock)
	e.Config = func() Config { return Config{} }
	if why, err := e.StartSubagent(q, "x", "s1", clock, db.SubagentMeta{}); err != nil || why != "" || starts() != 1 {
		t.Fatalf("a fresh config: %q %v, %d starts", why, err, starts())
	}
	e.Config = func() Config { return Config{SubagentGuardKey: true} }
	if why, err := e.StartSubagent(q, "x", "s1", clock, db.SubagentMeta{}); err != nil || why == "" || starts() != 1 {
		t.Fatalf("the guard on: %q %v, %d starts", why, err, starts())
	}
}
