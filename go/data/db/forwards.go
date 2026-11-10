package db

// A request the daemon sends on someone's behalf forwards something: a comment (memo_tells), a turn's return (the task_turn_pop that sent
// it names it), or the task itself, delivered when it is queued or sent again to its new holder (task_forward). One that forwards a
// comment or a turn's return is drawn as that comment or turn and never as a request of its own; ForwardedSQL is that rule, in one place.

const (
	ForwardQueue  = "queue"  // a queued task delivered to its free holder, or raised on the founder's board
	ForwardResend = "resend" // a delivery sent again to the task's new holder, carrying what the one it replaces did
)

// RecordForward records that request rid, sent for task tid on someone's behalf, forwards carries; a resend carries the tells of from to reader.
func RecordForward(q Q, tid, rid int64, carries string, from int64, reader string) error {
	if carries == ForwardResend {
		// WHERE true: an upsert's SELECT needs it to read ON CONFLICT as the upsert's
		if _, err := q.Exec("INSERT INTO memo_tells(memo_id, reader, request_id) SELECT memo_id, ?, ? FROM memo_tells WHERE request_id = ? AND true"+keepReturnTell,
			reader, rid, from); err != nil {
			return err
		}
	}
	_, err := AddEvent(q, "", "task_forward", KV{K: "task_id", V: tid}, KV{K: "request_id", V: rid}, KV{K: "carries", V: carries}, KV{K: "from", V: from})
	return err
}

// ForwardedSQL selects the ids of the task requests that forward a comment still on its task or a turn's return, directly or through
// resends, whatever became of them: one withdrawn is hidden too.
var ForwardedSQL = "WITH RECURSIVE resend(src, id) AS MATERIALIZED (SELECT json_extract(payload, '$.from'), " + eventRequest +
	" FROM events WHERE kind = 'task_forward' AND json_extract(payload, '$.carries') = '" + ForwardResend + "'), " +
	"fwd(id) AS (SELECT t.request_id FROM memo_tells t JOIN task_memos m ON m.id = t.memo_id JOIN requests r ON r.id = t.request_id WHERE r.kind = 'task' " +
	"UNION SELECT " + eventRequest + " FROM events WHERE kind = 'task_turn_pop' AND " + eventRequest + " IS NOT NULL " +
	"UNION SELECT resend.id FROM resend JOIN fwd ON resend.src = fwd.id) SELECT id FROM fwd"
