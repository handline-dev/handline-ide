package actor

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/kit/store"
)

func TestParse(t *testing.T) {
	for _, c := range []struct {
		key  string
		kind Kind
		id   int64
		ok   bool
	}{
		{"p1", Person, 1, true}, {"w2", Workspace, 2, true}, {"a17", Agent, 17, true}, {"c3", Account, 3, true}, {"d4", Device, 4, true},
		{"sys", Reserved, 0, true}, {"gate", Reserved, 0, true},
		{"", None, 0, false}, {"p", None, 0, false}, {"p0", None, 0, false}, {"p01", None, 0, false}, {"p-1", None, 0, false},
		{"x1", None, 0, false}, {"p+1", None, 0, false}, {"a 1", None, 0, false}, {"human", None, 0, false}, {"a1x", None, 0, false}, {"P1", None, 0, false},
	} {
		k, id, ok := Parse(c.key)
		if k != c.kind || id != c.id || ok != c.ok {
			t.Errorf("Parse(%q) = %q %d %v, want %q %d %v", c.key, k, id, ok, c.kind, c.id, c.ok)
		}
	}
	if Key(Agent, 17) != "a17" || !IsPerson("p9") || IsPerson("a9") || IsPerson("human") {
		t.Fatal("Key or IsPerson")
	}
}

func open(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestFirstStartMakesOneOwner(t *testing.T) {
	db := open(t)
	if _, err := Owner(db); !errors.Is(err, ErrNoOwner) {
		t.Fatalf("before the first start: %v", err)
	}
	k, err := EnsureOwner(db, func() string { return "Ron Kim" })
	if err != nil || k != "p1" {
		t.Fatalf("%q %v", k, err)
	}
	again, err := EnsureOwner(db, func() string { t.Fatal("the name is asked only at the first start"); return "" })
	var n int
	var name, role string
	db.QueryRow("SELECT COUNT(*), MAX(display_name), MAX(role) FROM people").Scan(&n, &name, &role)
	if err != nil || again != k || n != 1 || name != "Ron Kim" || role != "owner" {
		t.Fatalf("second start: %q %v; %d people, %q %q", again, err, n, name, role)
	}
	if _, err := EnsureOwner(open(t), func() string { return "" }); err == nil {
		t.Fatal("an owner with no name")
	}
	// People but no owner (a member left alone by a hand edit): the next start makes one rather than failing on every start.
	db = open(t)
	db.Exec("INSERT INTO people(display_name, role, created_at) VALUES('Jisoo', 'member', 0)")
	if k, err := EnsureOwner(db, func() string { return "Ron" }); err != nil || k != "p2" {
		t.Fatalf("no owner among people: %q %v", k, err)
	}
}
