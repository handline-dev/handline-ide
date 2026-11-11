package store

import (
	"path/filepath"
	"testing"
)

func TestALentHandleIsTheSharedOneUntilTakenBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	takeBack := Lend(path, d)
	if got, _ := SharedIfThere(path); got != d {
		t.Fatal("the shared handle is not the lent one")
	}
	takeBack()
	got, err := Shared(path)
	if err != nil || got == d {
		t.Fatalf("still lent after it was taken back: %v", err)
	}
	ForgetShared(path)
}
