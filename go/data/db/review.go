package db

import (
	"database/sql"
	"errors"

	"github.com/handline/handline/go/kit/actor"
)

// A story's sub-task (one opened as a draft) does not go to its land_by agent when its assignee finishes: it is Ready for the founder, a
// task_ready event (the land_by agent it waits to go to, and the comment that carries the result) with the founder's turn on top. The
// founder's verdict is a task_verdict event: ok hands it to that agent, redo sends it back to the assignee with a note. Step 6 counts
// these events (first-answer OK rate per role), so no column keeps them.
const (
	ReadyKind   = "task_ready"
	VerdictKind = "task_verdict"
	VerdictOK   = "ok"
	VerdictRedo = "redo"
)

// ReviewEventsSQL is each task's count and newest id of the events above: what a watcher compares to see a task become Ready or answered.
const ReviewEventsSQL = "SELECT " + eventTask + ", COUNT(*), MAX(id) FROM events WHERE kind IN ('" + ReadyKind + "', '" + VerdictKind + "') GROUP BY 1"

// Review is how a story's sub-task stands with the founder.
type Review struct {
	// Ready is whether it waits for the founder's verdict now: the newest Ready came after the newest verdict, and the founder's turn it put
	// on, hung on its comment, is still on top (only OK or Redo takes it off: the founder's comment is a question to the assignee).
	Ready bool
	// LandBy is who the newest Ready waits to go to; Memo the comment that carries the agent's result; At when it was made.
	LandBy   string
	Memo, At int64
	// Redo is how many times the founder sent it back; OK whether the newest verdict is an OK.
	Redo int
	OK   bool
}

// TaskReview is how the task stands with the founder, nil when it was never Ready.
func TaskReview(q Q, tid int64) (*Review, error) {
	ready, err := taskEvents(q, ReadyKind, tid)
	if err != nil || len(ready) == 0 {
		return nil, err
	}
	last := ready[len(ready)-1]
	r := &Review{LandBy: payloadStr(last.payload, "land_by"), Memo: payloadNum(last.payload, "memo"), At: last.at}
	verdicts, err := taskEvents(q, VerdictKind, tid)
	if err != nil {
		return nil, err
	}
	var newest int64
	for _, v := range verdicts {
		if payloadStr(v.payload, "verdict") == VerdictRedo {
			r.Redo++
		}
		r.OK, newest = payloadStr(v.payload, "verdict") == VerdictOK, v.id
	}
	if last.id > newest {
		turns, err := Turns(q, tid)
		if err != nil {
			return nil, err
		}
		r.Ready = len(turns) > 0 && actor.IsPerson(turns[len(turns)-1].Agent) && turns[len(turns)-1].Memo == r.Memo
	}
	return r, nil
}

// ReadyDue is whether a finish of the task goes to the founder first: it was opened as a draft (a story's sub-task) and the founder has
// not said OK since it was last sent back.
func ReadyDue(q Q, tid int64) (bool, error) {
	if draft, err := OpenedAsDraft(q, tid); err != nil || !draft {
		return false, err
	}
	var verdict string
	err := q.QueryRow("SELECT COALESCE(json_extract(payload, '$.verdict'), '') FROM events WHERE kind = ? AND "+eventTask+" = ? ORDER BY id DESC LIMIT 1",
		VerdictKind, tid).Scan(&verdict)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return verdict != VerdictOK, err
}

// AddReady records that the task is Ready for the founder, by its assignee, waiting to go to landBy, its result in comment memo.
func AddReady(q Q, tid int64, by, landBy string, memo int64) error {
	_, err := AddEvent(q, by, ReadyKind, KV{"task_id", tid}, KV{"land_by", landBy}, KV{"memo", memo})
	return err
}

// AddVerdict records the founder's verdict on a Ready task: VerdictOK, or VerdictRedo with its note's comment.
func AddVerdict(q Q, tid int64, by, verdict string, memo int64) error {
	payload := []KV{{"task_id", tid}, {"verdict", verdict}}
	if memo != 0 {
		payload = append(payload, KV{"memo", memo})
	}
	_, err := AddEvent(q, by, VerdictKind, payload...)
	return err
}
