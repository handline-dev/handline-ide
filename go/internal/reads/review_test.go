package reads

import (
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

func TestABoardReadsTheTeamOnce(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	d := teamtest.Seed(t, e.Home, "lead", "lead.w")
	w := teamtest.Key(t, e.Home, "lead.w")
	b := &board{}
	if got := b.teamOf(e.Home).TaskRole(w); got != "work" {
		t.Fatalf("role %q", got)
	}
	if _, err := d.Exec("UPDATE agents SET role = 'check'"); err != nil {
		t.Fatal(err)
	}
	if got := b.teamOf(e.Home).TaskRole(w); got != "work" {
		t.Fatalf("the team was read again: %q", got)
	}
}
