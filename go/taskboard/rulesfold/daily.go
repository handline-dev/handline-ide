package rulesfold

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

const (
	// Every is how long a person's rules wait between two folds, unless a backlog waits.
	Every = 24 * time.Hour
	// LookEvery is how often the daemon looks whether a fold is due.
	LookEvery = time.Hour
)

// Daily folds each person's new words into their rules once a day, and a backlog (the first fold over all history, or one a stopped
// daemon left) a batch after another until it is read. It is a level trigger: it looks at the cursors, never at what happened since.
type Daily struct {
	DB *sql.DB
	// Model is the model to ask now, false when no account may be spent.
	Model func() (Model, bool)
	// On is whether the fold runs (the rules_fold setting).
	On  func() bool
	Log func(format string, args ...any)
	Now func() time.Time

	running  atomic.Bool
	lastLook time.Time
}

// Check looks, at most once in LookEvery, whether a person's fold is due, and runs the due ones in the background; it never waits on a
// model. ctx ends a fold in flight.
func (d *Daily) Check(ctx context.Context) {
	now := d.Now()
	if now.Sub(d.lastLook) < LookEvery || !d.On() || !d.running.CompareAndSwap(false, true) {
		return
	}
	d.lastLook = now
	go func() {
		defer d.running.Store(false)
		people, err := People(d.DB)
		if err != nil {
			d.Log("rules fold: %v", err)
			return
		}
		for _, p := range people {
			d.run(ctx, p)
		}
	}()
}

// run steps person's fold when it is due, and on while each step leaves more to read.
func (d *Daily) run(ctx context.Context, person string) {
	due, err := Due(d.DB, person, d.Now())
	if err != nil || !due {
		if err != nil {
			d.Log("rules fold %s: %v", person, err)
		}
		return
	}
	for ctx.Err() == nil {
		model, ok := d.Model()
		if !ok {
			return
		}
		res, err := Step(ctx, d.DB, person, model, true)
		if err != nil {
			d.Log("rules fold %s: %v", person, err)
			if err := tried(d.DB, person, d.Now()); err != nil {
				d.Log("rules fold %s: %v", person, err)
			}
			return
		}
		d.Log("rules fold %s: %d items read, %d rules, %d dropped, %d agent lessons, %d past an agent's cap", person, res.Items,
			len(res.Rules), len(res.Dropped), len(res.Lessons), len(res.Skipped))
		if !res.More {
			return
		}
	}
}

// People are the keys of the people who may write comments.
func People(q db.Q) ([]string, error) {
	return db.Collect[string](q.Query("SELECT id FROM people WHERE removed_at IS NULL ORDER BY id"))(func(rows *sql.Rows) (string, error) {
		var id int64
		err := rows.Scan(&id)
		return actor.Key(actor.Person, id), err
	})
}

// Due is whether person's fold should run now: there is something to read, and either more than a batch waits or a day has passed since
// the last fold. A fold that failed waits a day from its try, backlog or not.
func Due(q db.Q, person string, now time.Time) (bool, error) {
	cur, err := ReadCursors(q, person)
	if err != nil {
		return false, err
	}
	b, err := ReadBatch(q, person, cur, Budget)
	if err != nil || b.Next == cur {
		return false, err
	}
	var written, failed int64
	if err := q.QueryRow("SELECT COALESCE(MAX(CASE WHEN name != ? THEN updated END), 0), COALESCE(MAX(CASE WHEN name = ? THEN updated END), 0) "+
		"FROM cursors WHERE name IN (?, ?, ?)", cursorName(person, sourceTried), cursorName(person, sourceTried),
		cursorName(person, sourceComments), cursorName(person, sourceChat), cursorName(person, sourceTried)).Scan(&written, &failed); err != nil {
		return false, err
	}
	if failed > written {
		return now.Sub(time.Unix(failed, 0)) >= Every, nil
	}
	return b.Full || now.Sub(time.Unix(written, 0)) >= Every, nil
}

// tried records a fold of person's that failed at now.
func tried(q db.Q, person string, now time.Time) error {
	_, err := q.Exec("INSERT INTO cursors(name, row_id, updated) VALUES(?, 0, ?) ON CONFLICT(name) DO UPDATE SET updated = excluded.updated",
		cursorName(person, sourceTried), now.Unix())
	return err
}
