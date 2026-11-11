package rpc

import (
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/storetest"
)

// A paired device acts as its person, with that person's role; a device with no person is the owner's, one whose person is not there is
// unpaired, and a member's waits for team mode.
func TestADeviceActsAsItsPersonWithThatPersonsRole(t *testing.T) {
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner, err := actor.EnsureOwner(db, func() string { return "Owner" })
	if err != nil {
		t.Fatal(err)
	}
	r, err := db.Exec("INSERT INTO people(display_name, role, created_at) VALUES('Mia', 'member', 0)")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := r.LastInsertId()
	member := actor.Key(actor.Person, id)
	s := NewServer(nil, "x", 1, "network", t.Logf)
	s.DB = db
	for _, c := range []struct{ person, key, role string }{{"", owner, "owner"}, {owner, owner, "owner"}} {
		got, err := s.deviceCaller("d1", "phone", c.person)
		if err != nil || got.Kind != "device" || got.Key != c.key || got.Person != c.key || got.Role != c.role || got.Actor() != c.key {
			t.Errorf("person %q: %+v %v", c.person, got, err)
		}
	}
	for _, gone := range []string{"p99", "a1", "nobody"} {
		if got, err := s.deviceCaller("d1", "phone", gone); err == nil || err.(*Error).Data["gate"] != "unpaired" {
			t.Errorf("person %q is no one, got %+v %v", gone, got, err)
		}
	}
	if got, err := s.deviceCaller("d1", "phone", member); err == nil || err.(*Error).Data["gate"] != "role" {
		t.Errorf("a member's device: %+v %v", got, err)
	}
}
