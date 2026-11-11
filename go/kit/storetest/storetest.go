// Package storetest opens a migrated database for a test without migrating it again: the schema is built once per test binary and each
// test gets a copy of the bytes. The migrations are most of a database-backed test's time.
package storetest

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/store"
)

var (
	once     sync.Once
	template []byte
	templErr error
)

// Owner is the display name of the owner every test database has, p1.
const Owner = "Owner"

// migrated is a new database as the daemon's first start leaves it: schema at the latest version and the owner, p1, made; WAL
// checkpointed away by the close.
func migrated() ([]byte, error) {
	once.Do(func() {
		dir, err := os.MkdirTemp("", "storetest")
		if err != nil {
			templErr = err
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "db.sqlite")
		d, err := store.Open(path)
		if err != nil {
			templErr = err
			return
		}
		if _, templErr = actor.EnsureOwner(d, func() string { return Owner }); templErr != nil {
			d.Close()
			return
		}
		if templErr = d.Close(); templErr == nil {
			template, templErr = os.ReadFile(path)
		}
	})
	return template, templErr
}

// OpenAt is store.Open(path): when no database is at path yet, the migrated template is copied there first (its folder made), so the
// schema is already at the latest version; one that exists is opened as it is. It returns the connection and the error as store.Open
// does, and is not closed for the caller.
func OpenAt(t testing.TB, path string) (*sql.DB, error) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		return store.Open(path)
	}
	b, err := migrated()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return nil, err
	}
	return store.Open(path)
}

// Open is a new database in the test's temp dir, closed when the test ends.
func Open(t testing.TB) *sql.DB {
	t.Helper()
	d, err := OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// Main is a package's TestMain for tests that run in parallel: the process environment is set once, not by t.Setenv in each test. The
// variables in blank are cleared (the founder is the caller, whatever session runs the suite) and HANDLINE_TEST_ROOT is a stand-in user
// home for the protected store. onStart, if set, runs before the tests.
func Main(m *testing.M, blank []string, onStart func()) {
	for _, k := range blank {
		os.Setenv(k, "")
	}
	root, err := os.MkdirTemp("", "testroot")
	if err != nil {
		panic(err)
	}
	os.Setenv("HANDLINE_TEST_ROOT", root)
	if onStart != nil {
		onStart()
	}
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
