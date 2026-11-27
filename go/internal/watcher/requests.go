package watcher

import (
	"database/sql"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/internal/reads"
)

// Requests is the requests topic: request.changed with the RequestRow of each request that appeared or changed. Its first look says nothing.
type Requests struct {
	Reads *reads.Env
	Now   func() time.Time             // nil is time.Now
	seen  map[string]map[string]string // by person
}

func (*Requests) Name() string { return "requests" }

func (*Requests) Tables() []string {
	return []string{"cursors", "events", "meta", "people", "requests", "task_blocks", "task_comments", "task_requests", "task_turns", "tasks"}
}

func (r *Requests) Scan(_ *sql.DB, emit Emit) error {
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	people, err := db.People(r.Reads.DB)
	if err != nil {
		return err
	}
	if r.seen == nil {
		r.seen = map[string]map[string]string{}
	}
	for _, person := range people {
		if err := r.scan(person, now().Unix(), emit); err != nil {
			return err
		}
	}
	return nil
}

// scan is one person's requests: an event of one is that person's alone (its Agent).
func (r *Requests) scan(person string, clock int64, emit Emit) error {
	rows, err := r.Reads.For(person).RequestRows(clock)
	if err != nil {
		return err
	}
	before, looked := r.seen[person]
	seen := map[string]string{}
	r.seen[person] = seen
	for _, row := range rows {
		key := encoded(row["task"]) + "/" + encoded(row["ref"])
		seen[key] = encoded(row)
		if !looked || before[key] == seen[key] {
			continue
		}
		if _, err := emit(stream.Entry{Topic: "requests", Type: "request.changed", Data: row, Title: row["title"].(string), Agent: person,
			Target: map[string]any{"screen": "requests"}}); err != nil {
			return err
		}
	}
	return nil
}
