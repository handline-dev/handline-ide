package taskwrites

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
)

// SetSeenFloor sets once the highest comment number as the floor, so comments from before the markers do not all show as new.
func SetSeenFloor(q db.Q) error {
	if _, set, err := db.Meta(q, db.SeenFloorMeta); err != nil || set {
		return err
	}
	var top sql.NullInt64
	if err := q.QueryRow("SELECT MAX(id) FROM task_memos").Scan(&top); err != nil {
		return err
	}
	return db.SetMeta(q, db.SeenFloorMeta, strconv.FormatInt(top.Int64, 10))
}

// MarkSeen moves the read marker only forward (to the newest comment when upto is 0); a queued question is never read away.
func MarkSeen(tx db.Q, tid, upto int64) (bool, error) {
	if upto == 0 {
		var top sql.NullInt64
		if err := tx.QueryRow("SELECT MAX(id) FROM task_memos WHERE task_id = ?", tid).Scan(&top); err != nil {
			return false, err
		}
		upto = top.Int64
	}
	cur, err := db.Cursor(tx, db.SeenKey(tid))
	if err != nil {
		return false, err
	}
	moved := upto > cur
	if moved {
		if err := db.SetCursor(tx, db.SeenKey(tid), upto); err != nil {
			return false, err
		}
	}
	ids, err := db.IDs(tx, "SELECT r.id FROM requests r JOIN events e ON r.id = CASE WHEN json_valid(e.payload) THEN json_extract(e.payload, '$.request_id') END "+
		"WHERE e.kind = 'request_comment_told' AND CASE WHEN json_valid(e.payload) THEN json_extract(e.payload, '$.task_id') END = ? "+
		"AND r.target = ? AND r.read_at IS NULL", tid, db.Human)
	if err != nil {
		return false, err
	}
	return moved || len(ids) > 0, db.MarkRead(tx, ids)
}

func seenMethod(c *rpc.Conn, p map[string]any) (any, error) {
	tid, err := taskID(p)
	if err != nil {
		return nil, err
	}
	var upto int64
	if p["upto"] != nil {
		n, ok := rpc.AsInt(p["upto"])
		if !ok || n < 0 {
			return nil, badParam("upto is a comment number", "upto")
		}
		upto = n
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		if t, err := getTask(tx, tid); err != nil {
			return nil, err
		} else if t == nil {
			nf := rpc.NotFoundError(fmt.Sprintf("no task T%d", tid))
			nf.Data = map[string]any{"what": "task", "id": tid}
			return nil, nf
		}
		if _, err := MarkSeen(tx, tid, upto); err != nil {
			return nil, err
		}
		return accepted(fmt.Sprintf("T%d's comments are marked seen", tid), nil), nil
	})
}
