package usage

import (
	"database/sql"
	"sync"

	"github.com/handline/handline/go/data/db"
)

// SampleCache holds the usage_samples rows SamplesSince read, decoded, and on each read adds only the rows written since: decoding every
// row's windows is most of the time a two-week read takes. The rows are shared, so a caller never changes one. The zero value is ready.
type SampleCache struct {
	mu     sync.Mutex
	held   bool
	floor  int64 // every row from floor on is in rows
	lastID int64
	rows   Samples
}

// Since is SamplesSince(q, since).
func (c *SampleCache) Since(q db.Q, since int64) (Samples, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.held || since < c.floor || !c.catchUp(q) {
		rows, err := readSamples(q, since)
		if err != nil {
			return nil, err
		}
		c.held, c.floor, c.rows, c.lastID = true, since, rows, maxID(0, rows)
	}
	return SamplesFrom(c.rows, since), nil
}

// catchUp drops the rows pruning deleted and adds the rows written since; false when the held rows cannot be made what a read would give,
// and a read must start over.
func (c *SampleCache) catchUp(q db.Q) bool {
	var oldest sql.NullInt64 // of the held rows pruning left
	if q.QueryRow("SELECT MIN(ts) FROM usage_samples WHERE ts >= ? AND id <= ?", c.floor, c.lastID).Scan(&oldest) != nil {
		return false
	}
	if !oldest.Valid {
		c.rows = nil
	} else {
		c.rows = SamplesFrom(c.rows, oldest.Int64)
	}
	// Any other change to a held row (one deleted inside, or written back in time) changes the count.
	var n int
	if q.QueryRow("SELECT COUNT(*) FROM usage_samples WHERE ts >= ? AND id <= ?", c.floor, c.lastID).Scan(&n) != nil || n != len(c.rows) {
		return false
	}
	fresh, err := querySamples(q, "SELECT * FROM usage_samples WHERE id > ? AND ts >= ? ORDER BY ts, id", c.lastID, c.floor)
	if err != nil {
		return false
	}
	if len(fresh) == 0 {
		return true
	}
	if n := len(c.rows); n > 0 {
		last, _ := num(c.rows[n-1]["ts"])
		if first, _ := num(fresh[0]["ts"]); first < last {
			return false // written before a held row: it goes in the middle
		}
	}
	c.rows, c.lastID = append(c.rows, fresh...), maxID(c.lastID, fresh)
	return true
}

func maxID(id int64, rows Samples) int64 {
	for _, r := range rows {
		v, _ := num(r["id"])
		id = max(id, int64(v))
	}
	return id
}
