package storetest

import "testing"

// Each copy is at the latest schema and independent of the others and of the template.
func TestACopyIsMigratedAndIndependent(t *testing.T) {
	a, b := Open(t), Open(t)
	var v, latest int
	if err := a.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&v); err != nil || v < 1 {
		t.Fatalf("version %d: %v", v, err)
	}
	if _, err := a.Exec("INSERT INTO meta(name, value, updated) VALUES('x', '1', 1)"); err != nil {
		t.Fatal(err)
	}
	if err := b.QueryRow("SELECT COUNT(*) FROM meta WHERE name = 'x'").Scan(&latest); err != nil || latest != 0 {
		t.Fatalf("a write in one copy shows in another: %d %v", latest, err)
	}
	c := Open(t)
	if err := c.QueryRow("SELECT COUNT(*) FROM meta WHERE name = 'x'").Scan(&latest); err != nil || latest != 0 {
		t.Fatalf("a write shows in the template: %d %v", latest, err)
	}
}

// A database that is already at the path is opened as it is, not replaced by the template.
func TestOpenAtKeepsADatabaseThatExists(t *testing.T) {
	path := t.TempDir() + "/db.sqlite"
	a, err := OpenAt(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Exec("INSERT INTO meta(name, value, updated) VALUES('kept', '1', 1)"); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b, err := OpenAt(t, path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var n int
	if err := b.QueryRow("SELECT COUNT(*) FROM meta WHERE name = 'kept'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("the existing database was replaced: %d %v", n, err)
	}
}
