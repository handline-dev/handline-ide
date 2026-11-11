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
	sharedMu sync.Mutex
	shared   = map[string]*sql.DB{}
)

// Shared is the process's handle on the database at path, opened (and made, when there is none) on first use. Not closed by callers.
func Shared(path string) (*sql.DB, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if d := shared[path]; d != nil {
		return d, nil
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

// Lend makes d, a handle the caller opened and closes, the shared one for path, so code given only the home uses the caller's pool.
// The returned func, called before the caller closes d, puts back the handle d stood in for, if any.
func Lend(path string, d *sql.DB) (takeBack func()) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	was := shared[path]
	shared[path] = d
	return func() {
		sharedMu.Lock()
		defer sharedMu.Unlock()
		if shared[path] != d {
			return
		}
		if was != nil {
			shared[path] = was
		} else {
			delete(shared, path)
		}
	}
}

// ForgetShared closes the handle on path, for a test that removes its home.
func ForgetShared(path string) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if d := shared[path]; d != nil {
		d.Close()
		delete(shared, path)
	}
}
