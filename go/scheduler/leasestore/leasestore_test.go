package leasestore

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/scheduler/leases"
)

func TestTheStoreKeepsLeasesAndNeverReusesAnID(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO workspaces (id, name, folder, created_at) VALUES (4, 'web', '/w', 1);
		INSERT INTO agents (id, workspace, name, owner, created_at) VALUES (7, 4, 'dev', 'p1', 1)`); err != nil {
		t.Fatal(err)
	}
	s := Store{DB: db}
	hold := leases.Lease{ID: 1, Kind: leases.Simulator, Agent: "a7", Workspace: "w4", Since: 100, Hold: true}
	cmd := leases.Lease{ID: 2, Kind: leases.Browser, Agent: "a7", Workspace: "w4", Since: 200, PID: 55}
	for _, l := range []leases.Lease{hold, cmd} {
		if err := s.Add(l); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Remove(2); err != nil {
		t.Fatal(err)
	}
	got, last, err := s.Load()
	if err != nil || last != 2 || !reflect.DeepEqual(got, []leases.Lease{hold}) {
		t.Fatalf("load: %+v %d %v", got, last, err)
	}
	if w := s.WorkspaceOf("a7"); w != "w4" {
		t.Fatalf("a7's workspace: %q", w)
	}
	for _, who := range []string{"a8", "p1", "dev"} {
		if w := s.WorkspaceOf(who); w != "" {
			t.Fatalf("%s's workspace: %q", who, w)
		}
	}
}

func TestAnEmptyStoreHasGivenNoID(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got, last, err := (Store{DB: db}).Load(); err != nil || last != 0 || len(got) != 0 {
		t.Fatalf("load: %+v %d %v", got, last, err)
	}
}
