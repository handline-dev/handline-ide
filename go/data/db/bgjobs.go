package db

import "database/sql"

// BackgroundStartedKind is the event a Bash call that started something in the background is recorded as (session.NoteBackground).
const BackgroundStartedKind = "background_started"

// BgJobStarted is when agent's background job id started in its session, at or after from: its background_started event (a shell),
// else the first Stop that listed it; 0 when neither did.
func BgJobStarted(q Q, agent string, from int64, id string) int64 {
	var at sql.NullInt64
	_ = q.QueryRow("SELECT MIN(ts) FROM events e WHERE agent = ?1 AND ts >= ?2 AND (kind = ?4 AND json_extract(payload, '$.job') = ?3 "+
		"OR kind = 'Stop' AND EXISTS (SELECT 1 FROM json_each(e.payload, '$.background_tasks') WHERE json_extract(value, '$.id') = ?3))",
		agent, from, id, BackgroundStartedKind).Scan(&at)
	return at.Int64
}

// BgJobOutput is the file agent's background shell id writes its output to, as its background_started event at or after from names
// it; "" for none.
func BgJobOutput(q Q, agent string, from int64, id string) string {
	var out sql.NullString
	_ = q.QueryRow("SELECT json_extract(payload, '$.output') FROM events WHERE agent = ? AND kind = ? AND ts >= ? AND "+
		"json_extract(payload, '$.job') = ? ORDER BY id DESC LIMIT 1", agent, BackgroundStartedKind, from, id).Scan(&out)
	return out.String
}
