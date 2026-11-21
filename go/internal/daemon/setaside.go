package daemon

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/joined"
)

// joinedKey is the meta row a board made for a joined master keeps the join's time in: a board without it, on a home that joined, is
// the one it ran alone on.
const joinedKey = "master_joined_at"

// SetBoardAside runs before the board opens, with the daemon's singleton held. On a home that joined a master (hl device join), the
// board it ran alone on is checkpointed and moved aside, unchanged (db.set-aside-<unix>.sqlite next to it), so a fresh one opens.
func SetBoardAside(home, file string, now time.Time) error {
	m, ok := joined.Of(home)
	if !ok {
		return nil
	}
	if _, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	raw, err := sql.Open("sqlite", "file:"+file+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	var at string
	err = raw.QueryRow("SELECT value FROM meta WHERE name = ?", joinedKey).Scan(&at)
	if err == nil && at == strconv.FormatInt(m.At, 10) {
		return raw.Close()
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		raw.Close()
		return fmt.Errorf("read the board before setting it aside: %w", err)
	}
	var busy, pages, done int
	err = raw.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &pages, &done)
	if cerr := raw.Close(); err == nil {
		err = cerr
	}
	if err == nil && busy != 0 {
		err = errors.New("a reader holds it")
	}
	if err != nil {
		return fmt.Errorf("checkpoint the board before setting it aside: %w", err)
	}
	aside := filepath.Join(filepath.Dir(file), "db.set-aside-"+strconv.FormatInt(now.Unix(), 10)+".sqlite")
	if err := os.Link(file, aside); err != nil { // never over a board set aside before
		return err
	}
	if err := os.Remove(file); err != nil {
		return err
	}
	for _, f := range []string{file + "-wal", file + "-shm"} {
		os.Remove(f) // empty after the checkpoint
	}
	return nil
}

// markJoined keeps the join's time on the board a joined home opened, so the next start leaves it where it is.
func markJoined(home string, q dbpkg.Q) error {
	m, ok := joined.Of(home)
	if !ok {
		return nil
	}
	return dbpkg.SetMeta(q, joinedKey, strconv.FormatInt(m.At, 10))
}
