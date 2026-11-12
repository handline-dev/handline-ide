package team

import (
	"testing"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
)

type fallbackScreen struct{ term.Blind }

// A terminal is read in its agent's vendor's wording: codex's for a codex agent's terminal, the fallback for a claude agent's, one the
// agent left, or one of no agent.
func TestScreenForIsTheVendorOfTheTerminalsAgent(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	if _, err := r.Register("shop", RegisterOpts{Vendor: strp("codex")}); err != nil {
		t.Fatal(err)
	}
	register(t, r, "shop.ui")
	cx, cl := paths.AgentKey(home, "shop"), paths.AgentKey(home, "shop.ui")
	q, err := store.Shared(paths.DBFile(home))
	if err != nil {
		t.Fatal(err)
	}
	// %4 was codex's terminal; it moved to %1.
	for _, at := range [][2]string{{cx, "%4"}, {cx, "%1"}, {cl, "%2"}} {
		if err := db.SetTerminal(q, at[0], at[1], ""); err != nil {
			t.Fatal(err)
		}
	}
	fallback := func(string) term.Driver { return fallbackScreen{} }
	screen := ScreenFor(home, q, fallback)
	if got := screen("%1").Name(); got != "codex" {
		t.Fatalf("codex agent's terminal: %q", got)
	}
	for _, h := range []string{"%2", "%4", "%9"} {
		if _, ok := screen(h).(fallbackScreen); !ok {
			t.Errorf("%s: %T", h, screen(h))
		}
	}
}
