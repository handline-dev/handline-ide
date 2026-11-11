// Package store opens the daemon's sqlite database and brings its schema up from the steps in migrations/, the first of which is the
// whole base (docs/design/people-workspaces.md).
package store

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite" // pure Go: the build needs no C toolchain
)

//go:embed migrations
var migrationFiles embed.FS

const (
	migrateWait   = 60 * time.Second
	busyTimeoutMS = 10000 // db.connect's timeout=10.0
	// cacheKiB is each connection's page cache. The default (2 MB) holds a sliver of a database of hundreds of MB, so every read of a list
	// went back to the file for its pages, a syscall each, and that was most of the daemon's read time.
	cacheKiB = 65536
)

// ErrSchema means the database cannot be used by this code as it stands.
var ErrSchema = errors.New("schema")

type migration struct {
	version  int
	name     string
	additive bool
	sql      string
}

// migrations are the steps manifest.json lists (tools/go_schema.py checks the files agree with it).
func migrations() ([]migration, error) {
	raw, err := migrationFiles.ReadFile("migrations/manifest.json")
	if err != nil {
		return nil, err
	}
	var steps []struct {
		Version  int    `json:"version"`
		Name     string `json:"name"`
		Additive bool   `json:"additive"`
	}
	if err := json.Unmarshal(raw, &steps); err != nil {
		return nil, fmt.Errorf("migrations/manifest.json: %w", err)
	}
	if len(steps) == 0 || steps[0].Version != baseVersion || steps[0].Name != baseName {
		return nil, fmt.Errorf("migrations/manifest.json: the first step is %d_%s", baseVersion, baseName)
	}
	var out []migration
	for i, st := range steps {
		if st.Version != baseVersion+i {
			return nil, fmt.Errorf("migrations/manifest.json: step %d is numbered %d", baseVersion+i, st.Version)
		}
		body, err := migrationFiles.ReadFile(fmt.Sprintf("migrations/%04d_%s.sql", st.Version, st.Name))
		if err != nil {
			return nil, err
		}
		out = append(out, migration{st.Version, st.Name, st.Additive, string(body)})
	}
	return out, nil
}

// LatestVersion is the schema version this code knows.
func LatestVersion() int {
	ms, err := migrations()
	if err != nil || len(ms) == 0 {
		return 0
	}
	return ms[len(ms)-1].version
}

// Open opens the database (WAL, synchronous NORMAL, foreign keys, a 10 s busy timeout) and brings it to the latest step under a lock
// file, so two processes opening a new database at once make it once. A database ahead of this code is used only when every step it
// does not know only added; the old layout, and a database with tables but no steps, are refused.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	var lock *os.File
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if lock, err = takeLock(path + ".migrate-lock"); err != nil {
			return nil, err
		}
		defer lock.Close() // closing the file drops the flock
		if err := create(path); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	if err := bringUp(db, path, lock != nil); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// dsn is the connection string: one connection per pragma set, since modernc applies _pragma to every connection the pool opens.
func dsn(path string) string {
	return "file:" + path + "?_pragma=busy_timeout(" + strconv.Itoa(busyTimeoutMS) + ")&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_pragma=cache_size(-" + strconv.Itoa(cacheKiB) + ")&_txlock=immediate"
}

// create makes an empty WAL database at path under the migrate lock: made aside and renamed in, so no opener sees it before it is WAL
// (two connections switching one file to WAL at once fail SQLITE_BUSY). Another process may have made it while this one waited.
func create(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	tmp := path + ".new"
	for _, f := range []string{tmp, tmp + "-wal", tmp + "-shm"} { // a crashed create's, which sqlite would replay
		os.Remove(f)
	}
	d, err := sql.Open("sqlite", "file:"+tmp+"?_pragma=journal_mode(WAL)")
	if err == nil {
		err = d.Ping()
		d.Close()
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func version(db *sql.DB) (int, error) {
	var v int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&v)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return 0, nil
	}
	return v, err
}

func tooNew(path string, have, latest int) error {
	return fmt.Errorf("%w: %s is at schema version %d, newer than the %d this code knows: update handline before using it",
		ErrSchema, path, have, latest)
}

func hasAdditiveColumn(q interface {
	Query(string, ...any) (*sql.Rows, error)
}) (bool, error) {
	rows, err := q.Query("PRAGMA table_info(schema_version)")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == "additive" {
			return true, nil
		}
	}
	return false, rows.Err()
}

// newerStepsOnlyAdd is true when every step this code does not know was recorded as additive: it only added
// tables and columns, so this code, which names none of them, runs on the database as it is. A step recorded
// without the mark, or a database with no marks at all, is not trusted.
func newerStepsOnlyAdd(db *sql.DB, latest int) (bool, error) {
	if ok, err := hasAdditiveColumn(db); err != nil || !ok {
		return false, err
	}
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_version WHERE version > ? AND additive = 0", latest).Scan(&n)
	return n == 0, err
}

// The base is step 100, not 1, and is not additive: the old layout ran steps 1 to 51, so an old binary finds the new base newer than it
// knows and refuses it rather than running its own steps on top. A database with any step below the base is the old layout, which this
// code never reads.
const (
	baseVersion = 100
	baseName    = "base"
)

func refuseOldLayout(db *sql.DB, path string) error {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_version WHERE version < ?", baseVersion).Scan(&n)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil
	}
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: %s is the old layout: move the home aside and bring it over with hl-import-v1", ErrSchema, path)
	}
	return nil
}

// bringUp brings db to the latest step; locked is whether the caller holds the migrate lock already.
func bringUp(db *sql.DB, path string, locked bool) error {
	ms, err := migrations()
	if err != nil {
		return err
	}
	latest := ms[len(ms)-1].version
	if err := refuseOldLayout(db, path); err != nil {
		return err
	}
	if have, err := version(db); err != nil {
		return err
	} else if have == latest {
		return nil
	} else if have > latest {
		if ok, err := newerStepsOnlyAdd(db, latest); err != nil {
			return err
		} else if !ok {
			return tooNew(path, have, latest)
		}
		return nil
	}
	if !locked {
		lock, err := takeLock(path + ".migrate-lock")
		if err != nil {
			return err
		}
		defer lock.Close() // closing the file drops the flock
	}
	have, err := version(db)
	if err != nil {
		return err
	}
	if have > latest {
		if ok, err := newerStepsOnlyAdd(db, latest); err != nil {
			return err
		} else if !ok {
			return tooNew(path, have, latest)
		}
		return nil // another process brought it ahead while this one waited for the lock
	}
	if have == 0 {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master").Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("%w: %s predates schema versions: move the home aside and bring it over with hl-import-v1", ErrSchema, path)
		}
	}
	for _, m := range ms {
		if m.version <= have {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec("CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY, " +
			"name TEXT NOT NULL, applied_at INTEGER NOT NULL, additive INTEGER NOT NULL DEFAULT 0)"); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec("INSERT INTO schema_version(version, name, applied_at, additive) VALUES(?, ?, ?, ?)",
			m.version, m.name, time.Now().Unix(), m.additive); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// takeLock is the lock a migrating daemon takes on the file, so two daemons exclude each other.
func takeLock(file string) (*os.File, error) {
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(migrateWait)
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			break
		} else if err != syscall.EWOULDBLOCK {
			f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			holder := "another process"
			if b, _ := os.ReadFile(file); len(strings.TrimSpace(string(b))) > 0 {
				holder = strings.TrimSpace(string(b))
			}
			f.Close()
			return nil, fmt.Errorf("%w: %s has been busy migrating for %.0fs, held by pid %s: wait for it, or end it and try again",
				ErrSchema, file, migrateWait.Seconds(), holder)
		}
		time.Sleep(50 * time.Millisecond)
	}
	f.Truncate(0)
	f.WriteString(strconv.Itoa(os.Getpid()))
	return f, nil
}
