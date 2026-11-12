package db

import (
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/textsafe"
)

// What a task is, read off the rows (actions/tasks.py: task_state and the facts it is built from). The one copy the supervisor uses.

// Asks is actions._asks: a linked request that is work still being asked for. A report to the founder is a say, born landed, so
// linking one never makes a task doing.
func Asks(r *Request) bool {
	return r.State == "queued" || (r.State == Landed && r.Kind.String == "task" && r.QueueOrder.Valid)
}

// SinceRelease keeps the events of task (an SQL expression) newer than its last release: a release takes back a begun task.
func SinceRelease(task string) string {
	return " AND id > COALESCE((SELECT MAX(r.id) FROM events r WHERE r.kind = 'release_task' AND json_valid(r.payload) AND " +
		"json_extract(r.payload, '$.task_id') = " + task + "), 0)"
}

// Begun is actions._begun: begun by task_begin or task_self_start, and not released since.
func Begun(q Q, tid int64) (bool, error) {
	var one int
	err := q.QueryRow("SELECT 1 FROM events WHERE kind IN ('task_begin', 'task_self_start') AND "+eventTask+" = ?"+SinceRelease("?")+" LIMIT 1", tid, tid).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// ReadsDoing is actions.reads_doing: the task has been sent to its owner, or it was begun on a founder message (TaskFacts.Asked).
func ReadsDoing(q Q, tid int64) (bool, error) {
	t, err := GetTask(q, tid)
	if err != nil || t == nil {
		return false, err
	}
	begun, err := Begun(q, tid)
	return verdict.TaskFacts{Begun: begun, Sent: t.SentAt.Valid}.Asked(), err
}

// HoldsCurrent is actions.holds_current: the task may be delivered (wayFree).
func HoldsCurrent(q Q, t *Task) (bool, error) {
	if t.Acting == actor.OwnerKey || t.Acting == Gate {
		return true, nil
	}
	if held, ok, err := CurrentTask(q, t.Acting); err != nil || ok && held == t.ID {
		return err == nil, err
	}
	return wayFree(q, t.Acting)
}

// wayFree: agent holds no open, unblocked task.
func wayFree(q Q, agent string) (bool, error) {
	held, ok, err := CurrentTask(q, agent)
	if err != nil || !ok {
		return err == nil, err
	}
	cur, err := GetTask(q, held)
	if err != nil || cur == nil || cur.Ended() {
		return err == nil, err
	}
	bs, err := TaskBlockers(q, held)
	return len(bs) > 0, err
}

func isCurrent(q Q, t *Task) (bool, error) {
	if t.Acting == actor.OwnerKey || t.Acting == Gate {
		return true, nil
	}
	held, ok, err := CurrentTask(q, t.Acting)
	return ok && held == t.ID, err
}

// HeldByHolder is whether the holder rule keeps request r back from agent: it delivers a task of agent's that is not the one agent holds.
func HeldByHolder(q Q, agent string, r *Request) (bool, error) {
	tasks, err := TasksForRequest(q, r.ID)
	if err != nil {
		return false, err
	}
	for _, t := range tasks {
		if t.Acting != agent {
			continue
		}
		if holds, err := HoldsCurrent(q, t); err != nil || !holds {
			return !holds, err
		}
	}
	return false, nil
}

func IsDoing(q Q, tid int64) (bool, error) {
	t, err := GetTask(q, tid)
	if err != nil || t == nil {
		return false, err
	}
	st, err := TaskState(q, t)
	return st == "doing", err
}

// TaskState is actions.task_state, decided by verdict.TaskState from this task's facts. A fact the verdict cannot use is not read: an
// ended task reads no events, and one not asked for no holder or blockers.
func TaskState(q Q, t *Task) (string, error) {
	f := verdict.TaskFacts{Discarded: t.DiscardedBy.String != "", Done: t.DoneEvidence.String != "", Sent: t.SentAt.Valid,
		Queued: t.QueuedBy.String != ""}
	if t.Ended() {
		return verdict.TaskState(f), nil
	}
	var err error
	if f.Begun, err = Begun(q, t.ID); err != nil {
		return "", err
	}
	if f.Asked() {
		if f.Held, err = isCurrent(q, t); err != nil {
			return "", err
		}
	}
	if f.Held {
		bs, err := TaskBlockers(q, t.ID)
		if err != nil {
			return "", err
		}
		f.Blocked = len(bs) > 0
		if f.PauseHolds, err = pauseHolds(q, t, f.Begun); err != nil {
			return "", err
		}
	}
	return verdict.TaskState(f), nil
}

// pauseHolds: new work is paused and the task has not reached the agent that acts on it, nor did that agent begin it itself. The founder
// and the gates act on theirs with no delivery, so the pause holds none of theirs.
func pauseHolds(q Q, t *Task, begun bool) (bool, error) {
	if begun || t.DeliveredAt.Valid || t.Acting == actor.OwnerKey || t.Acting == Gate {
		return false, nil
	}
	p, err := GetPause(q, time.Now())
	return p != nil, err
}

// ChildBlockerOf is actions.child_blocker_of: the open child task of someone else that blocks tid, nil for none.
func ChildBlockerOf(q Q, tid int64) (*Task, error) {
	t, err := GetTask(q, tid)
	if err != nil || t == nil {
		return nil, err
	}
	blockers, err := TaskBlockers(q, tid)
	if err != nil {
		return nil, err
	}
	var children []*Task
	for _, b := range blockers {
		if b.ParentTask.Valid && b.ParentTask.Int64 == tid {
			children = append(children, b)
		}
	}
	for _, b := range children {
		if b.Acting != t.Acting {
			continue
		}
		st, err := TaskState(q, b)
		if err != nil {
			return nil, err
		}
		if st == "queued" || st == "doing" {
			return nil, nil
		}
	}
	for _, b := range children {
		if b.Acting != t.Acting {
			return b, nil
		}
	}
	return nil, nil
}

func currentOpen(q Q, agent string) (*Task, error) {
	tid, ok, err := CurrentTask(q, agent)
	if err != nil || !ok {
		return nil, err
	}
	t, err := GetTask(q, tid)
	if err != nil || t == nil || t.Ended() {
		return nil, err
	}
	return t, nil
}

// ChildWait is actions.child_wait: what the agent waits on when its current task is held by a child of its own ("waiting for T<n> (owner)").
func ChildWait(q Q, agent string) (string, error) {
	t, err := currentOpen(q, agent)
	if err != nil || t == nil {
		return "", err
	}
	child, err := ChildBlockerOf(q, t.ID)
	if err != nil || child == nil {
		return "", err
	}
	return "waiting for T" + itoa(child.ID) + " (" + addr.Ref(child.Acting) + ")", nil
}

// BlockWait is actions.block_wait: what the agent is held up by when its current task is blocked ("blocked by T<n> (owner), ...").
func BlockWait(q Q, agent string) (string, error) {
	t, err := currentOpen(q, agent)
	if err != nil || t == nil {
		return "", err
	}
	bs, err := TaskBlockers(q, t.ID)
	if err != nil || len(bs) == 0 {
		return "", err
	}
	parts := make([]string, len(bs))
	for i, b := range bs {
		parts[i] = "T" + itoa(b.ID) + " (" + addr.Ref(b.Acting) + ")"
	}
	return "blocked by " + strings.Join(parts, ", "), nil
}

// CurrentTaskStillAsking is actions._current_task_still_asking: the agent's current task still has its own request out, the one thing left
// to deliver. A begun task always does, until it is done; a blocked one never does, so its owner moves on.
func CurrentTaskStillAsking(q Q, agent string) (bool, error) {
	t, err := currentOpen(q, agent)
	if err != nil || t == nil {
		return false, err
	}
	if bs, err := TaskBlockers(q, t.ID); len(bs) > 0 || err != nil {
		return false, err
	}
	if b, err := Begun(q, t.ID); b || err != nil {
		return b, err
	}
	return t.SentAt.Valid, nil
}

// NewestRequest is actions._newest_request: the after_request a reset event carries.
func NewestRequest(q Q) (int64, error) {
	var n int64
	err := q.QueryRow("SELECT COALESCE(MAX(id), 0) FROM requests").Scan(&n)
	return n, err
}

// resetEvents are the events after which a task's earlier answers no longer mean it was handled.
var resetEvents = []string{"reopen_task", "unblock_task", "task_unblocked", "queue_task", "reject_task"}

// WaitingTask is a task that could become current and its own queued request, nil when it was never delivered.
type WaitingTask struct {
	Task *Task
	Req  *Request
}

// WaitingTasks is actions._waiting_tasks: the agent's open tasks that could become current, by priority then age. Skips one blocked,
// still a draft, or with nothing of its own left to deliver.
func WaitingTasks(q Q, agent string) ([]WaitingTask, error) {
	open, err := OpenTasks(q)
	if err != nil {
		return nil, err
	}
	var out []WaitingTask
	for _, t := range open {
		if t.Acting != agent || t.DoneEvidence.String != "" {
			continue
		}
		bs, err := TaskBlockers(q, t.ID)
		if err != nil {
			return nil, err
		}
		if len(bs) > 0 {
			continue
		}
		sent := t.SentAt.Valid
		linked, err := TaskRequests(q, t.ID)
		if err != nil {
			return nil, err
		}
		var req *Request
		for _, r := range linked {
			if Asks(r) && r.Kind.String != "say" { // a notice linked to the task never delivers it
				req = r
				break
			}
		}
		var after int64
		if !sent {
			if after, err = ResetAfter(q, t.ID); err != nil {
				return nil, err
			}
			delivered := false
			for _, r := range linked {
				if r.Target == agent && r.ID > after && !neverReached(r.State) && r.Kind.String != "say" { // a memo notice delivers nothing
					delivered = true
				}
			}
			if delivered {
				continue // delivered and answered since it last came due: not a reason to send it again
			}
		}
		// sent_at counts a queued notice linked to the task (an unblock, a refusal), which asks nothing: a draft with one is still a draft.
		if req != nil || t.QueuedBy.Valid {
			out = append(out, WaitingTask{t, req})
		}
	}
	return out, nil
}

// ResetAfter is actions._reset_after: the highest request id a reset event on the task has covered.
func ResetAfter(q Q, tid int64) (int64, error) {
	args := make([]any, 0, len(resetEvents)+1)
	for _, k := range resetEvents {
		args = append(args, k)
	}
	args = append(args, tid)
	var n int64
	err := q.QueryRow("SELECT COALESCE(MAX(CAST(json_extract(payload, '$.after_request') AS INTEGER)), 0) FROM events WHERE kind IN ("+marks(len(resetEvents))+") AND "+eventTask+" = ?", args...).Scan(&n)
	return n, err
}

func neverReached(state string) bool {
	for _, s := range neverReachedTarget {
		if s == state {
			return true
		}
	}
	return false
}

// MemoSummary is actions.memo_summary: the task's memos in one short line for a delivery or a start; "" when it has none.
func MemoSummary(q Q, tid int64, cap int) (string, error) {
	rows, err := q.Query("SELECT author, body, created_at, edited_at FROM task_memos WHERE task_id = ? ORDER BY id", tid)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var author, body string
		var created float64 // a memo's stamp carries a fraction: Python writes time.time()
		var edited sql.NullFloat64
		if err := rows.Scan(&author, &body, &created, &edited); err != nil {
			return "", err
		}
		at := created
		if edited.Valid && edited.Float64 != 0 {
			at = edited.Float64
		}
		line := textsafe.Oneline(body)
		if utf8.RuneCountInString(line) > 160 {
			line = string([]rune(line)[:160])
		}
		parts = append(parts, author+" "+time.Unix(int64(at), 0).Format("01-02 15:04")+": "+line)
	}
	if err := rows.Err(); err != nil || len(parts) == 0 {
		return "", err
	}
	text := "Comments - " + strings.Join(parts, " | ")
	if utf8.RuneCountInString(text) <= cap {
		return text, nil
	}
	return strings.TrimRight(string([]rune(text)[:cap]), " \t\n\r") + "…", nil
}
