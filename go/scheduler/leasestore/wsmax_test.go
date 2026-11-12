package leasestore

import (
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/scheduler/leases"
)

// lease_max.<kind>: the workspace's own row beats the machine's, which beats none; a second workspace and another kind follow only
// the machine's.
func TestWorkspaceMaxIsTheWorkspacesRuleOverTheMachines(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, folder, created_at) VALUES (4, 'web', '/w', 1), (5, 'app', '/a', 1)`); err != nil {
		t.Fatal(err)
	}
	s := Store{DB: db}
	key := team.RuleLeaseMax + leases.Simulator
	check := func(when, ws string, want int, by string) {
		t.Helper()
		lim, err := s.WorkspaceMax(leases.Simulator)
		if err != nil {
			t.Fatal(err)
		}
		l, own := lim.Own[ws]
		if !own {
			l = lim.Machine
		}
		if l.N != want || l.SetBy != by {
			t.Fatalf("%s, %s: %d by %q, want %d by %q", when, ws, l.N, l.SetBy, want, by)
		}
	}
	check("no rule", "w4", 0, "")
	if err := team.SetRule(db, team.MachineRules, key, 3, "u1", 1); err != nil {
		t.Fatal(err)
	}
	check("machine 3", "w4", 3, "u1")
	if err := team.SetRule(db, 4, key, 1, "u2", 2); err != nil {
		t.Fatal(err)
	}
	check("w4's own 1", "w4", 1, "u2")
	check("w4's own 1", "w5", 3, "u1")
	if lim, _ := s.WorkspaceMax(leases.Browser); lim.Machine.N != 0 || len(lim.Own) != 0 {
		t.Fatalf("another kind: %+v", lim)
	}
	if err := team.SetRule(db, 4, key, 0, "u2", 3); err != nil {
		t.Fatal(err)
	}
	check("w4's own 0 beats the machine's 3", "w4", 0, "u2")
}
