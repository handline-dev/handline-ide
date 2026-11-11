package compose

import (
	"github.com/handline/handline/go/data/team/teamtest"
	"os"
	"testing"

	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
)

// ownersRig is wired(), with a terminal agent (bob) beside the structured one (alice) and an agent_view row for an agent that is gone.
func ownersRig(t *testing.T) (p *Parts, bobRow string, count func(table string) int) {
	t.Helper()
	p, _, _, _ = wired(t)
	d := p.db
	bobRow = `{"name":"bob","state":"idle","python":"wrote this"}`
	for agent, view := range map[string]string{"bob": bobRow, "ghost": `{"name":"ghost"}`} {
		if _, err := d.Exec("INSERT INTO agent_view(agent, view, updated) VALUES(?, ?, 7)", agent, view); err != nil {
			t.Fatal(err)
		}
	}
	count = func(table string) int {
		var n int
		if err := d.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	return p, bobRow, count
}

func storedView(t *testing.T, d dbpkg.Q, agent string) (view string, found bool) {
	t.Helper()
	err := d.QueryRow("SELECT view FROM agent_view WHERE agent = ?", agent).Scan(&view)
	return view, err == nil
}

func TestTheGoTickOwnsTheUsagePollAndDropsTheRowOfAGoneAgent(t *testing.T) {
	p, bobRow, count := ownersRig(t)
	d := p.db
	samples := count("usage_samples")
	tk := p.Tick(cfgOf(p), TickConfig{})
	if _, err := tk.Tick.Run(); err != nil {
		t.Fatal(err)
	}
	if _, ok := storedView(t, d, "ghost"); ok {
		t.Fatal("the row of a gone agent stays")
	}
	if v, ok := storedView(t, d, "bob"); !ok || v != bobRow {
		t.Fatalf("a registered terminal agent's row is not the Go side's to drop or rewrite: %q %v", v, ok)
	}
	if got := count("usage_samples"); got <= samples {
		t.Fatalf("this tick polls usage: %d samples, was %d", got, samples)
	}
}

func TestTheGoTickAsksADueAccountForItsUsage(t *testing.T) {
	p, _, _ := ownersRig(t)
	cfg := cfgOf(p)
	cfg.Log = t.Logf
	tk := p.Tick(cfg, TickConfig{})
	if _, err := tk.Tick.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.StatusFile(p.home, teamtest.Account(t, p.home, "a1"))); err != nil {
		t.Fatalf("account a1 was not read: %v", err)
	}
}
