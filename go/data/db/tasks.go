package db

import (
	"database/sql"
	"strings"

	"github.com/handline/handline/go/agentctl/verdict"
)

// OpenSQL is a task that is neither done nor discarded.
const OpenSQL = "done_evidence IS NULL AND discarded_by IS NULL"

// DefaultPriority is the priority a task is opened with.
const DefaultPriority = 2

// Task is one tasks row.
type Task struct {
	ID, CreatedAt, Priority                                int64
	Title, Owner, OpenedBy                                 string
	ParentTask, DoneAt, EditedAt, ClosedAt, DiscardedAt    sql.NullInt64
	MergedInto, QueuedAt, TimeoutS, TimeoutAt, DeliveredAt sql.NullInt64
	SentAt                                                 sql.NullInt64
	DoneEvidence, DoneVia, EditedBy, ClosedBy, DiscardedBy sql.NullString
	DiscardReason, QueuedBy, Expects                       sql.NullString
	Reporter                                               sql.NullString
	// Acting is who acts on the task now: its top turn's agent, else Owner (ActingSQL).
	Acting string
}

var taskCols = "id, title, owner, parent_task, opened_by, done_evidence, done_at, done_via, edited_by, edited_at, created_at, closed_by, closed_at, " +
	"discarded_by, discard_reason, discarded_at, priority, merged_into, queued_by, queued_at, timeout_s, timeout_at, delivered_at, sent_at, expects, " +
	"reporter, " + actingCol

// actingCol is ActingSQL for a query of the tasks table unaliased.
var actingCol = ActingSQL("tasks")

func (t *Task) dest() []any {
	return []any{&t.ID, &t.Title, &t.Owner, &t.ParentTask, &t.OpenedBy, &t.DoneEvidence, &t.DoneAt, &t.DoneVia, &t.EditedBy, &t.EditedAt, &t.CreatedAt,
		&t.ClosedBy, &t.ClosedAt, &t.DiscardedBy, &t.DiscardReason, &t.DiscardedAt, &t.Priority, &t.MergedInto, &t.QueuedBy, &t.QueuedAt,
		&t.TimeoutS, &t.TimeoutAt, &t.DeliveredAt, &t.SentAt, &t.Expects, &t.Reporter, &t.Acting}
}

// AnswersTo is who the task answers to: its reporter, or its opener when none was set.
func (t *Task) AnswersTo() string {
	if t.Reporter.String != "" {
		return t.Reporter.String
	}
	return t.OpenedBy
}

// Ended is db.task_ended: done or discarded.
func (t *Task) Ended() bool { return t.DoneEvidence.String != "" || t.DiscardedBy.String != "" }

func taskRows(q Q, query string, args ...any) ([]*Task, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t := &Task{}
		if err := rows.Scan(t.dest()...); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTask is db.get_task: nil when there is no such task.
func GetTask(q Q, tid int64) (*Task, error) {
	ts, err := taskRows(q, "SELECT "+taskCols+" FROM tasks WHERE id = ?", tid)
	if err != nil || len(ts) == 0 {
		return nil, err
	}
	return ts[0], nil
}

// OpenTasks is db.open_tasks.
func OpenTasks(q Q) ([]*Task, error) {
	return taskRows(q, "SELECT "+taskCols+" FROM tasks WHERE "+OpenSQL+" ORDER BY priority, id")
}

// OpenTasksOf is the open tasks agent acts on (Task.Acting), by priority then id.
func OpenTasksOf(q Q, agent string) ([]*Task, error) {
	return taskRows(q, "SELECT "+taskCols+" FROM tasks WHERE "+actingCol+" = ? AND "+OpenSQL+" ORDER BY priority, id", agent)
}

// OpenChildTasks is db.open_child_tasks.
func OpenChildTasks(q Q, tid int64) ([]*Task, error) {
	return taskRows(q, "SELECT "+taskCols+" FROM tasks WHERE parent_task = ? AND "+OpenSQL+" ORDER BY priority, id", tid)
}

// TasksForRequest is db.tasks_for_request: the tasks a request is linked to, oldest first.
func TasksForRequest(q Q, rid int64) ([]*Task, error) {
	return taskRows(q, "SELECT "+taskCols+" FROM tasks WHERE id IN (SELECT task_id FROM task_requests WHERE request_id = ?) ORDER BY id", rid)
}

// TaskRequests is db.task_requests: the requests linked to a task, as they are now.
func TaskRequests(q Q, tid int64) ([]*Request, error) {
	return requestRows(q, "SELECT r."+strings.ReplaceAll(requestCols, ", ", ", r.")+
		" FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? ORDER BY r.id", tid)
}

// LiveDelivery is db.live_delivery: the request that asks owner to do the task (the latest task delivery to that owner, queued or
// landed and not answered), nil when none.
func LiveDelivery(q Q, tid int64, owner string) (*Request, error) {
	rows, err := TaskRequests(q, tid)
	if err != nil {
		return nil, err
	}
	var last *Request
	for _, r := range rows {
		if r.Kind.String == "task" && r.Target == owner && (r.State == "queued" || r.State == Landed && r.QueueOrder.Valid) {
			last = r
		}
	}
	return last, nil
}

// TaskBlockers is db.task_blockers: the tasks still blocking tid.
func TaskBlockers(q Q, tid int64) ([]*Task, error) {
	return taskRows(q, "SELECT "+taskCols+" FROM tasks WHERE id IN (SELECT blocks_on FROM task_blocks WHERE task_id = ? AND dormant = 0) "+
		"AND discarded_by IS NULL AND done_evidence IS NULL ORDER BY id", tid)
}

// NewTask is db.new_task, with the request it serves (SetTaskRequest). The clock of timeoutS starts when the task is doing, so timeout_at stays NULL. A reporter of "" is the opener.
func NewTask(q Q, title, owner, openedBy string, parent int64, priority int64, expects string, timeoutS int64, reporter string) (int64, error) {
	var p, e, ts, r any
	if parent != 0 {
		p = parent
	}
	if expects != "" {
		e = expects
	}
	if timeoutS != 0 {
		ts = timeoutS
	}
	if reporter != "" {
		r = reporter
	}
	res, err := q.Exec("INSERT INTO tasks(title, owner, parent_task, opened_by, created_at, priority, expects, timeout_s, reporter) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		title, owner, p, openedBy, Now(), priority, e, ts, r)
	if err != nil {
		return 0, err
	}
	tid, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return tid, SetTaskRequest(q, tid)
}

// LinkRequest is db.link_request.
func LinkRequest(q Q, tid, rid int64) error {
	if _, err := q.Exec("INSERT OR IGNORE INTO task_requests(task_id, request_id) VALUES (?, ?)", tid, rid); err != nil {
		return err
	}
	return RefreshSent(q, tid)
}

// RefreshSent is db.refresh_sent: sent_at is set once while a request that asks something is out on the task, cleared
// when none is left.
func RefreshSent(q Q, tid int64) error {
	_, err := q.Exec("UPDATE tasks SET sent_at = CASE WHEN EXISTS "+
		"(SELECT 1 FROM task_requests l JOIN requests r ON r.id = l.request_id WHERE l.task_id = tasks.id "+
		"AND (r.state = 'queued' OR (r.state = 'landed' AND r.kind = 'task' AND r.target = tasks.owner AND r.queue_order IS NOT NULL))) "+
		"THEN COALESCE(sent_at, ?) ELSE NULL END WHERE id = ?", Now(), tid)
	return err
}

// RefreshSentOfRequest is db.refresh_sent_of_request.
func RefreshSentOfRequest(q Q, rid int64) error {
	tids, err := IDs(q, "SELECT task_id FROM task_requests WHERE request_id = ?", rid)
	if err != nil {
		return err
	}
	for _, t := range tids {
		if err := RefreshSent(q, t); err != nil {
			return err
		}
	}
	return nil
}

// EndHeld is db.end_held: the task no longer holds the landed delivery that asked owner (anyone when "") for it.
func EndHeld(q Q, tid int64, owner string) error {
	var o any
	if owner != "" {
		o = owner
	}
	_, err := q.Exec("UPDATE requests SET queue_order = NULL WHERE state = 'landed' AND kind = 'task' AND queue_order IS NOT NULL "+
		"AND (? IS NULL OR target = ?) AND id IN (SELECT request_id FROM task_requests WHERE task_id = ?)", o, o, tid)
	return err
}

func unsend(q Q, tid int64) error {
	_, err := q.Exec("UPDATE tasks SET sent_at = NULL WHERE id = ?", tid)
	return err
}

// MarkTaskDone is db.mark_task_done: true only if this call wrote the first evidence. via is 'sha', 'request' or 'reason'.
func MarkTaskDone(q Q, tid int64, evidence, via string) (bool, error) {
	res, err := q.Exec("UPDATE tasks SET done_evidence = ?, done_at = ?, done_via = ? "+
		"WHERE id = ? AND done_evidence IS NULL AND discarded_by IS NULL", evidence, Now(), via, tid)
	if err != nil {
		return false, err
	}
	if err := unsend(q, tid); err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return true, EndHeld(q, tid, "")
	}
	return false, nil
}

// DiscardTask is db.discard_task: true only if this call ended the task.
func DiscardTask(q Q, tid int64, by, reason string) (bool, error) {
	res, err := q.Exec("UPDATE tasks SET discarded_by = ?, discarded_at = ?, discard_reason = ? WHERE id = ? AND "+OpenSQL, by, Now(), reason, tid)
	if err != nil {
		return false, err
	}
	if err := unsend(q, tid); err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return true, EndHeld(q, tid, "")
	}
	return false, nil
}

// StampTaskDelivered is db.stamp_task_delivered: request rid reached its target's session, so the task it delivers to that
// same owner is delivered from now. Once.
func StampTaskDelivered(q Q, rid int64) error {
	_, err := q.Exec("UPDATE tasks SET delivered_at = ? WHERE delivered_at IS NULL AND owner = "+
		"(SELECT target FROM requests WHERE id = ? AND (kind = 'task' OR (target = ? AND COALESCE(kind, '') NOT IN ('say', 'report')))) AND id IN "+
		"(SELECT task_id FROM task_requests WHERE request_id = ?)", Now(), rid, Human, rid)
	return err
}

// SetTaskQueued is db.set_task_queued: approve a task to run (by set) or send it back to draft (""), true only if it changed.
func SetTaskQueued(q Q, tid int64, by string) (bool, error) {
	var res sql.Result
	var err error
	if by != "" {
		res, err = q.Exec("UPDATE tasks SET queued_by = ?, queued_at = ? WHERE id = ? AND "+OpenSQL+" AND queued_by IS NULL", by, Now(), tid)
	} else {
		res, err = q.Exec("UPDATE tasks SET queued_by = NULL, queued_at = NULL WHERE id = ? AND "+OpenSQL+" AND queued_by IS NOT NULL", tid)
	}
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// AddBlock is db.add_block, and the request the wait brings the blocker into (inheritRequest).
func AddBlock(q Q, tid, blocksOn int64) error {
	if _, err := q.Exec("INSERT OR IGNORE INTO task_blocks(task_id, blocks_on, created_at) VALUES (?, ?, ?)", tid, blocksOn, Now()); err != nil {
		return err
	}
	return inheritRequest(q, tid, blocksOn)
}

// RemoveBlock is db.remove_block.
func RemoveBlock(q Q, tid, blocksOn int64) (bool, error) {
	res, err := q.Exec("DELETE FROM task_blocks WHERE task_id = ? AND blocks_on = ?", tid, blocksOn)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// BlocksOnIDs is db.blocks_on_ids: the raw edges the task itself names.
func BlocksOnIDs(q Q, tid int64) ([]int64, error) {
	return IDs(q, "SELECT blocks_on FROM task_blocks WHERE task_id = ?", tid)
}

// TasksBlockedBy is db.tasks_blocked_by: every task with a block row naming tid, closed or not.
func TasksBlockedBy(q Q, tid int64) ([]int64, error) {
	return IDs(q, "SELECT task_id FROM task_blocks WHERE blocks_on = ? ORDER BY task_id", tid)
}

const openDecisionCardsSQL = "(SELECT MIN(l.request_id) FROM task_requests l JOIN tasks t ON t.id = l.task_id " +
	"WHERE t.owner = ? AND t.discarded_by IS NULL AND t.done_evidence IS NULL GROUP BY t.id)"

// TaskOfCard is decisions.task_of_card: the decision task whose card is request rid, in whatever state it is now; 0 for none.
func TaskOfCard(q Q, rid int64) (int64, error) {
	tasks, err := TasksForRequest(q, rid)
	if err != nil {
		return 0, err
	}
	for _, t := range tasks {
		if t.Expects.String != "decision" {
			continue
		}
		first, err := TaskRequests(q, t.ID)
		if err != nil {
			return 0, err
		}
		if len(first) > 0 && first[0].ID == rid {
			return t.ID, nil
		}
	}
	return 0, nil
}

// FounderClosed is decisions.answer_of's reading of a decision task marked done: its evidence is the founder's answer only when the
// founder owns it and the last of its settle, done and reopen events is the founder's own settle or done.
func FounderClosed(q Q, t *Task) (bool, error) {
	if t.Owner != Human {
		return false, nil
	}
	var kind, agent string
	err := q.QueryRow("SELECT kind, agent FROM events WHERE "+eventTask+" = ? AND kind IN ('settle_decision_task', 'mark_task_done', 'reopen_task') "+
		"ORDER BY id DESC LIMIT 1", t.ID).Scan(&kind, &agent)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return kind == "settle_decision_task" || kind == "mark_task_done" && agent == Human, nil
}

// OpenDecisionCardIDs is db.open_decision_card_ids: the queued cards of the founder's open decision tasks, oldest first; about
// agent (its requester) when given, whose body starts with prefix when given, and is exactly body when hasBody.
func OpenDecisionCardIDs(q Q, agent, prefix string, body *string) ([]int64, error) {
	query := "SELECT id FROM requests WHERE target = ? AND state = 'queued' AND id IN " + openDecisionCardsSQL
	args := []any{Human, Human}
	if agent != "" {
		query += " AND requester = ?"
		args = append(args, agent)
	}
	if prefix != "" {
		query += " AND substr(body, 1, ?) = ?"
		args = append(args, len([]rune(prefix)), prefix)
	}
	if body != nil {
		query += " AND body = ?"
		args = append(args, *body)
	}
	return IDs(q, query+" ORDER BY id", args...)
}

// TaskEdit is what EditTask writes; a nil field is left as it is. TimeoutS, given, always writes, and starts timeout_at from now only when
// the task is doing (IsDoing, the caller's read of that at the time), else leaves it paused.
type TaskEdit struct {
	Owner, Title       *string
	Reporter           *string
	Priority, TimeoutS *int64
	Estimate           *int64 // story points; 0 clears it
	IsDoing            bool
}

// EditTask is db.edit_task: a changed owner, reporter, title, priority, estimate or timeout, stamped with who and when. The caller has checked permission and
// that something changed; this only writes. A new owner has not had the task yet, so delivered_at and sent_at clear.
func EditTask(q Q, tid int64, by string, ed TaskEdit) error {
	now := Now()
	sets := []string{"edited_by = ?", "edited_at = ?"}
	args := []any{by, now}
	if ed.Owner != nil {
		sets = append(sets, "owner = ?", "delivered_at = NULL", "sent_at = NULL")
		args = append(args, *ed.Owner)
	}
	if ed.Reporter != nil {
		sets = append(sets, "reporter = ?")
		args = append(args, *ed.Reporter)
	}
	if ed.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *ed.Title)
	}
	if ed.Priority != nil {
		sets = append(sets, "priority = ?")
		args = append(args, *ed.Priority)
	}
	if ed.Estimate != nil {
		var points any
		if *ed.Estimate != 0 {
			points = *ed.Estimate
		}
		sets = append(sets, "estimate = ?")
		args = append(args, points)
	}
	if ed.TimeoutS != nil {
		var at any
		if ed.IsDoing {
			at = now + *ed.TimeoutS
		}
		sets = append(sets, "timeout_s = ?", "timeout_at = ?", "timeout_left = NULL")
		args = append(args, *ed.TimeoutS, at)
	}
	args = append(args, tid)
	_, err := q.Exec("UPDATE tasks SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// DormantBlockers are the open tasks tid waits on through a dormant block (one agent holds both ends).
func DormantBlockers(q Q, tid int64) ([]int64, error) {
	return IDs(q, "SELECT b.blocks_on FROM task_blocks b JOIN tasks t ON t.id = b.blocks_on WHERE b.task_id = ? AND b.dormant = 1 "+
		"AND t.discarded_by IS NULL AND t.done_evidence IS NULL ORDER BY b.blocks_on", tid)
}

// SyncSelfBlocks is db.sync_self_blocks, after tid changed owner from oldOwner: a wait that one agent now holds both ends of goes dormant
// (nobody else could finish it), and a dormant one whose ends differ again counts again. A same-owner wait added on purpose is left as it was.
// It answers the (blocked, blocker) pairs that went dormant.
func SyncSelfBlocks(q Q, tid int64, oldOwner string) ([][2]int64, error) {
	rows, err := q.Query("SELECT b.task_id, b.blocks_on, b.dormant, a.owner, c.owner FROM task_blocks b JOIN tasks a ON a.id = b.task_id "+
		"JOIN tasks c ON c.id = b.blocks_on WHERE b.task_id = ?1 OR b.blocks_on = ?1", tid)
	if err != nil {
		return nil, err
	}
	type wait struct {
		blocked, blocker int64
		dormant          bool
		ao, co           string
	}
	var waits []wait
	for rows.Next() {
		var w wait
		var d int64
		if err := rows.Scan(&w.blocked, &w.blocker, &d, &w.ao, &w.co); err != nil {
			rows.Close()
			return nil, err
		}
		w.dormant = d != 0
		waits = append(waits, w)
	}
	rows.Close()
	var went [][2]int64
	for _, w := range waits {
		other := w.ao
		if w.blocked == tid {
			other = w.co
		}
		switch {
		case w.ao == w.co && other != oldOwner:
			if _, err := q.Exec("UPDATE task_blocks SET dormant = 1 WHERE task_id = ? AND blocks_on = ?", w.blocked, w.blocker); err != nil {
				return nil, err
			}
			went = append(went, [2]int64{w.blocked, w.blocker})
		case w.ao != w.co && w.dormant:
			if _, err := q.Exec("UPDATE task_blocks SET dormant = 0 WHERE task_id = ? AND blocks_on = ?", w.blocked, w.blocker); err != nil {
				return nil, err
			}
		}
	}
	return went, nil
}

// TasksToClock is db.tasks_to_clock: every open task, since whether it is doing is not a column.
func TasksToClock(q Q) ([]*Task, error) {
	return taskRows(q, "SELECT "+taskCols+" FROM tasks WHERE done_evidence IS NULL AND discarded_by IS NULL ORDER BY id")
}

// MaxTaskTimeoutS is actions.MAX_TASK_TIMEOUT_S: no task is given longer than this. DefaultTaskTimeoutS is actions.DEFAULT_TASK_TIMEOUT_S, what
// a task never given a timeout gets: the same.
const (
	MaxTaskTimeoutS     = 4 * 3600
	DefaultTaskTimeoutS = MaxTaskTimeoutS
)

// AdvanceTaskTimeout is db.advance_task_timeout: starts, resumes or re-arms a task's clock: what a pause left, else timeout_s, from now;
// defaultS for a task never given one.
func AdvanceTaskTimeout(q Q, tid, now, defaultS int64) error {
	_, err := q.Exec("UPDATE tasks SET timeout_s = COALESCE(timeout_s, ?), timeout_at = ? + COALESCE(timeout_left, timeout_s, ?), timeout_left = NULL WHERE id = ?",
		defaultS, now, defaultS, tid)
	return err
}

// PauseTaskTimeout is db.pause_task_timeout: no deadline runs while a task is not doing; what was left is kept for the resume.
func PauseTaskTimeout(q Q, tid, now int64) error {
	_, err := q.Exec("UPDATE tasks SET timeout_left = MAX(timeout_at - ?, 0), timeout_at = NULL WHERE id = ? AND timeout_at IS NOT NULL", now, tid)
	return err
}

// RestartTaskTimeout: a task that waits gets its whole length back for when the wait ends. Set again on each tick it waits, so nothing
// is lost or repeated across a restart. A clock that never started is left unstarted.
func RestartTaskTimeout(q Q, tid, defaultS int64) error {
	_, err := q.Exec("UPDATE tasks SET timeout_left = COALESCE(timeout_s, ?), timeout_at = NULL WHERE id = ? AND (timeout_at IS NOT NULL OR timeout_left IS NOT NULL)",
		defaultS, tid)
	return err
}

// Waiter is an open task that waits on another, and since when: its block's creation, or its child's.
type Waiter struct{ ID, Since int64 }

// WaitersOn is the open tasks that wait on task tid, by id: those it blocks (not dormantly) and its parent.
func WaitersOn(q Q, tid int64) ([]Waiter, error) {
	rows, err := q.Query("SELECT x.id, MIN(x.since) FROM (SELECT task_id AS id, created_at AS since FROM task_blocks WHERE blocks_on = ?1 AND task_id != ?1 AND dormant = 0 "+
		"UNION ALL SELECT parent_task, created_at FROM tasks WHERE id = ?1 AND parent_task IS NOT NULL) x JOIN tasks w ON w.id = x.id "+
		"WHERE w.done_evidence IS NULL AND w.discarded_by IS NULL GROUP BY x.id ORDER BY x.id", tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Waiter
	for rows.Next() {
		var w Waiter
		if err := rows.Scan(&w.ID, &w.Since); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ReopenedAt is when task tid was last reopened, 0 for never.
func ReopenedAt(q Q, tid int64) (int64, error) {
	var at int64
	err := q.QueryRow("SELECT COALESCE(MAX(ts), 0) FROM events WHERE kind = 'reopen_task' AND "+eventTask+" = ?", tid).Scan(&at)
	return at, err
}

// ClearCursorsOfEndedTasks drops the cursors named prefix<task id> of tasks that are done or discarded.
func ClearCursorsOfEndedTasks(q Q, prefix string) error {
	_, err := q.Exec("DELETE FROM cursors WHERE substr(name, 1, length(?1)) = ?1 AND CAST(substr(name, length(?1) + 1) AS INTEGER) IN "+
		"(SELECT id FROM tasks WHERE done_evidence IS NOT NULL OR discarded_by IS NOT NULL)", prefix)
	return err
}

// TaskWaits is whether a task waits on something open: a task it is blocked by, or a child of its own.
func TaskWaits(q Q, tid int64) (bool, error) {
	if bs, err := TaskBlockers(q, tid); err != nil || len(bs) > 0 {
		return len(bs) > 0, err
	}
	kids, err := OpenChildTasks(q, tid)
	return len(kids) > 0, err
}

// Overdue is whether a task's holder was told it ran past its timeout and nobody has answered since: the deadline in force is the one that
// notice set (an edit or a release replaces it).
func Overdue(q Q, t *Task) (bool, error) {
	if !t.TimeoutAt.Valid {
		return false, nil
	}
	var one int
	err := q.QueryRow("SELECT 1 FROM events WHERE kind = 'task_timeout_exceeded' AND "+eventTask+" = ? AND json_extract(payload, '$.next') = ? LIMIT 1",
		t.ID, t.TimeoutAt.Int64).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// AllTasks is db.all_tasks.
func AllTasks(q Q) ([]*Task, error) {
	return taskRows(q, "SELECT "+taskCols+" FROM tasks ORDER BY priority, id")
}

// HolderOf is what kind of holder a task's owner is, for verdict.Overdue.
func HolderOf(owner string) verdict.Holder {
	switch owner {
	case Human:
		return verdict.FounderHolds
	case Gate:
		return verdict.GateHolds
	}
	return verdict.AgentHolds
}
