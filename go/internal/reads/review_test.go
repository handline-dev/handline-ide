package reads

import (
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

func TestABoardReadsTheTeamOnce(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	d := teamtest.Seed(t, e.Home, "lead", "lead.w")
	lead, w := teamtest.Key(t, e.Home, "lead"), teamtest.Key(t, e.Home, "lead.w")
	b := &board{}
	if got := b.leadOf(e.Home, w); got != lead {
		t.Fatalf("lead %q", got)
	}
	if _, err := d.Exec("UPDATE agents SET reports_to = 'p1'"); err != nil {
		t.Fatal(err)
	}
	if got := b.leadOf(e.Home, w); got != lead {
		t.Fatalf("the team was read again: %q", got)
	}
}
