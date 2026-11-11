package daemon

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/store"
)

// The first start makes the owner p1; a home whose owner came out another key is refused, since every write that names the person with
// no caller in hand writes actor.OwnerKey.
func TestTheDaemonStartsOnlyWhereTheOwnerIsOwnerKey(t *testing.T) {
	open := func() *sql.DB {
		t.Helper()
		d, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { d.Close() })
		return d
	}
	name := func() string { return "Owner" }
	fresh := open()
	if err := ensureOwner(fresh, name); err != nil {
		t.Fatalf("a fresh home: %v", err)
	}
	if k, _ := actor.Owner(fresh); k != actor.OwnerKey {
		t.Fatalf("the first owner is %s", k)
	}
	other := open()
	if _, err := other.Exec("INSERT INTO people(display_name, role, created_at) VALUES('Mia', 'member', 0)"); err != nil {
		t.Fatal(err)
	}
	if err := ensureOwner(other, name); err == nil {
		t.Fatal("a home whose owner is p2 started")
	}
}
