package db

import "database/sql"

// Block is one delivery_blocks row: an agent the daemon cannot give work to, kept for the founder's list.
type Block struct {
	Agent, Kind, Detail, Work, Origin, Title string
	Since, ShowAt, Seen                      int64
	RequestID                                sql.NullInt64
}

const blockCols = "agent, kind, detail, work, since, show_at, seen, request_id, origin, title"

func scanBlocks(rows *sql.Rows) ([]*Block, error) {
	defer rows.Close()
	var out []*Block
	for rows.Next() {
		b := &Block{}
		if err := rows.Scan(&b.Agent, &b.Kind, &b.Detail, &b.Work, &b.Since, &b.ShowAt, &b.Seen, &b.RequestID, &b.Origin, &b.Title); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BlockGet is db.block_get: nil when the agent has no block.
func BlockGet(q Q, agent string) (*Block, error) {
	rows, err := q.Query("SELECT "+blockCols+" FROM delivery_blocks WHERE agent = ?", agent)
	if err != nil {
		return nil, err
	}
	bs, err := scanBlocks(rows)
	if err != nil || len(bs) == 0 {
		return nil, err
	}
	return bs[0], nil
}

// Blocks is db.blocks.
func Blocks(q Q) ([]*Block, error) {
	rows, err := q.Query("SELECT " + blockCols + " FROM delivery_blocks ORDER BY since, agent")
	if err != nil {
		return nil, err
	}
	return scanBlocks(rows)
}

// BlockOpen is db.block_open.
func BlockOpen(q Q, agent, kind, detail, work string, now, showAt int64, origin, title string) error {
	_, err := q.Exec("INSERT OR REPLACE INTO delivery_blocks(agent, kind, detail, work, since, show_at, seen, request_id, origin, title) VALUES(?,?,?,?,?,?,?,NULL,?,?)",
		agent, kind, detail, work, now, showAt, now, origin, title)
	return err
}

// BlockUpdate is db.block_update.
func BlockUpdate(q Q, agent, kind, detail, work string, now int64, title string) error {
	_, err := q.Exec("UPDATE delivery_blocks SET kind = ?, detail = ?, work = ?, seen = ?, title = ? WHERE agent = ?", kind, detail, work, now, title, agent)
	return err
}

// BlockSetRequest is db.block_set_request.
func BlockSetRequest(q Q, agents []string, rid int64) error {
	for _, a := range agents {
		if _, err := q.Exec("UPDATE delivery_blocks SET request_id = ? WHERE agent = ?", rid, a); err != nil {
			return err
		}
	}
	return nil
}

// BlockClear is db.block_clear.
func BlockClear(q Q, agent string) error {
	_, err := q.Exec("DELETE FROM delivery_blocks WHERE agent = ?", agent)
	return err
}
