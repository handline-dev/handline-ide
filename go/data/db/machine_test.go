package db

import (
	"os"
	"testing"
)

func TestANodeIDIsMadeOnceKeptInItsFileAndSeededFromTheOldMetaRow(t *testing.T) {
	q, home := open(t), t.TempDir()
	a, err := NodeID(home, q)
	if err != nil || len(a) != 16 {
		t.Fatalf("%q %v", a, err)
	}
	if b, _ := NodeID(home, q); b != a {
		t.Fatalf("a second ask made another: %q %q", a, b)
	}
	if st, err := os.Stat(NodeIDFile(home)); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", st, err)
	}

	home = t.TempDir()
	SetMeta(q, MachineIDKey, "0123456789abcdef")
	if id, _ := NodeID(home, q); id != "0123456789abcdef" {
		t.Fatalf("the id daemon.status showed changed: %q", id)
	}
	if _, ok, _ := Meta(q, MachineIDKey); ok {
		t.Fatal("the meta row that seeded it is still there")
	}
	if id := NodeIDOf(home); id != "0123456789abcdef" {
		t.Fatalf("the file is not what is kept: %q", id)
	}
	if id := NodeIDOf(t.TempDir()); id != "" {
		t.Fatalf("a home with none read as %q", id)
	}
}

func TestTheOwnNodeRowIsWrittenOnceUnderTheOwnerAndKeepsItsJoinTime(t *testing.T) {
	q := open(t)
	if err := KeepOwnNode(q, "n1", "mac", 10); err != nil {
		t.Fatal(err)
	}
	if err := KeepOwnNode(q, "n1", "mac2", 20); err != nil {
		t.Fatal(err)
	}
	var n int
	var owner, person, joined, seen int64
	var name string
	q.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&n)
	q.QueryRow("SELECT id FROM people WHERE role = 'owner'").Scan(&owner)
	q.QueryRow("SELECT person, name, joined_at, last_seen FROM nodes WHERE id = 'n1'").Scan(&person, &name, &joined, &seen)
	if n != 1 || person != owner || name != "mac2" || joined != 10 || seen != 20 {
		t.Fatalf("%d %d/%d %s %d %d", n, person, owner, name, joined, seen)
	}
}
