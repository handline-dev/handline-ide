package session

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/storetest"
)

type fixture struct {
	*Env
	t   *testing.T
	now int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	home := t.TempDir()
	db, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &fixture{t: t, now: 1_800_000_000}
	f.Env = &Env{DB: db, Home: home, Now: func() time.Time { return time.Unix(f.now, 0) }}
	return f
}

func (f *fixture) event(agent, kind string) int64 {
	f.t.Helper()
	r, err := f.DB.Exec("INSERT INTO events(ts, agent, kind) VALUES(?,?,?)", f.now, agent, kind)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := r.LastInsertId()
	return id
}
