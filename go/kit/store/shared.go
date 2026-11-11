package store

import (
	"database/sql"
	"errors"
	"os"
	"sync"
)

// The rows that say who is in a home (people, workspaces, agents) are read by code that is given only the home, as it was given the
// files before. Shared is that code's handle on the home's database: one per path for the process, opened on first use.
var (
	sharedMu  sync.Mutex
	shared    = map[string]*sql.DB{}
	forgotten = map[string]bool{}
)

// ErrForgotten is Shared on a path ForgetShared closed: the home is going, and opening it again would write into a folder being removed.
var ErrForgotten = errors.New("the database's home is gone")

// Shared is the process's handle on the database at path, opened (and made, when there is none) on first use. Not closed by callers.
func Shared(path string) (*sql.DB, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if d := shared[path]; d != nil {
		return d, nil
	}
	if forgotten[path] {
		return nil, ErrForgotten
	}
	d, err := Open(path)
	if err != nil {
		return nil, err
	}
	shared[path] = d
	return d, nil
}

// SharedIfThere is Shared, or nil when there is no database at path yet: a read of a home with none finds nothing and makes none.
func SharedIfThere(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return Shared(path)
}

// ForgetShared closes the handle on path, for a test that removes its home; it is not opened again.
func ForgetShared(path string) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	forgotten[path] = true
	if d := shared[path]; d != nil {
		d.Close()
		delete(shared, path)
	}
}
