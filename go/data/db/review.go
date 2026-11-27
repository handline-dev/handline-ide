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
	// LandBy is who the newest Ready waits to go to; Comment the comment that carries the agent's result; At when it was made.
	LandBy      string
	Comment, At int64
	// Redo is how many times the founder sent it back; OK whether the newest verdict is an OK, Auto whether that OK was automatic.
	Redo     int
	OK, Auto bool
	// By is who made the newest Ready: the agent that finished it, whose review level and record it counts in.
	By string
}

// TaskReview is how the task stands with the founder, nil when it was never Ready.
func TaskReview(q Q, tid int64) (*Review, error) {
	ready, err := taskEvents(q, ReadyKind, tid)
	if err != nil || len(ready) == 0 {
		return nil, err
	}
	last := ready[len(ready)-1]
	r := &Review{LandBy: payloadStr(last.payload, "land_by"), Comment: payloadNum(last.payload, "comment"), At: last.at, By: last.agent}
	verdicts, err := taskEvents(q, VerdictKind, tid)
	if err != nil {
		return nil, err
	}
	var newest int64
	for _, v := range verdicts {
		if payloadStr(v.payload, "verdict") == VerdictRedo {
			r.Redo++
		}
		r.OK, r.Auto, newest = payloadStr(v.payload, "verdict") == VerdictOK, v.agent == AutoVerdictBy, v.id
	}
	if last.id > newest {
		turns, err := Turns(q, tid)
		if err != nil {
			return nil, err
		}
		r.Ready = len(turns) > 0 && actor.IsPerson(turns[len(turns)-1].Agent) && turns[len(turns)-1].Comment == r.Comment
	}
	return r, nil
}

// servesFounderSQL walks up from a task through its parents (a move to a new parent keeps the task's own request) to one that serves a
// founder's request (tasks.request, set when it was opened) or that the founder owns.
const servesFounderSQL = "WITH RECURSIVE up(id) AS (SELECT ? UNION SELECT t.parent_task FROM tasks t JOIN up ON t.id = up.id " +
	"WHERE t.parent_task IS NOT NULL) SELECT EXISTS(SELECT 1 FROM tasks WHERE id IN (SELECT id FROM up) AND (request IS NOT NULL OR owner = ?))"

// ServesFounder is whether the task, or a task above it, serves a founder's request or is the founder's own.
func ServesFounder(q Q, tid int64) (bool, error) {
	var yes bool
	err := q.QueryRow(servesFounderSQL, tid, actor.OwnerKey).Scan(&yes)
	return yes, err
}

// ReadyDue is whether a finish of the task goes to the founder first: it was opened as a draft (a story's sub-task), or, with the review
// flow on, it serves the founder; and the founder has not said OK since it was last sent back.
func ReadyDue(q Q, tid int64, reviewFlow bool) (bool, error) {
	due, err := OpenedAsDraft(q, tid)
	if err == nil && !due && reviewFlow {
		due, err = ServesFounder(q, tid)
	}
	if err != nil || !due {
		return false, err
	}
	var verdict string
	err = q.QueryRow("SELECT COALESCE(json_extract(payload, '$.verdict'), '') FROM events WHERE kind = ? AND "+eventTask+" = ? ORDER BY id DESC LIMIT 1",
		VerdictKind, tid).Scan(&verdict)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return verdict != VerdictOK, err
}

// AddReady records that the task is Ready for the founder, by its assignee, waiting to go to landBy, its result in comment `comment`; more
// is the assignee's check report.
func AddReady(q Q, tid int64, by, landBy string, comment int64, more ...KV) error {
	_, err := AddEvent(q, by, ReadyKind, append([]KV{{"task_id", tid}, {"land_by", landBy}, {"comment", comment}}, more...)...)
	return err
}

// MaySkipCount is how many of agent's finishes judged at its level at work under model (the Ready's judged_by), since that level was set
// (after event `after`), could have gone on without the founder: at 1 in n, the first of them and every n-th after go to the founder.
func MaySkipCount(q Q, agent, work, model string, after int64) (int, error) {
	var n int
	err := q.QueryRow("SELECT COUNT(*) FROM events r WHERE r.kind = ? AND r.id > ? AND json_extract(r.payload, '$.may_skip') = 1 AND "+
		"r.agent = ? AND json_extract(r.payload, '$.model') = ? AND json_extract(r.payload, '$.judged_by') = ?",
		ReadyKind, after, agent, model, work).Scan(&n)
	return n, err
}

// AddVerdict records the founder's verdict on a Ready task: VerdictOK, or VerdictRedo with its note's comment.
func AddVerdict(q Q, tid int64, by, verdict string, comment int64) error {
	payload := []KV{{"task_id", tid}, {"verdict", verdict}}
	if comment != 0 {
		payload = append(payload, KV{"comment", comment})
	}
	_, err := AddEvent(q, by, VerdictKind, payload...)
	return err
}

// ReadyWorks are the kinds of work the newest Ready of task tid recorded.
func ReadyWorks(q Q, tid int64) ([]string, error) {
	var works []string
	rows, err := q.Query("SELECT j.value FROM events r, json_each(r.payload, '$.works') j WHERE r.id = "+
		"(SELECT MAX(id) FROM events WHERE kind = ? AND json_extract(payload, '$.task_id') = ?)", ReadyKind, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			return nil, err
		}
		works = append(works, w)
	}
	return works, rows.Err()
}
