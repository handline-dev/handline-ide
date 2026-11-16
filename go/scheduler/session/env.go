package session

import (
	"database/sql"
	"time"
)

// Env is what the session code works on: the database, the Handline home, and the clock (nil for the wall clock).
type Env struct {
	DB   *sql.DB
	Home string
	Now  func() time.Time
}

func (e *Env) now() int64 {
	if e.Now != nil {
		return e.Now().Unix()
	}
	return time.Now().Unix()
}
