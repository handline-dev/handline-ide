package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/textsafe"
)

// What a task is, read off the rows: its state and the facts it is built from. The one copy the supervisor uses.

// Asks is whether a linked request that is work still being asked for. A report to the founder is a say, born landed, so
// linking one never makes a task doing.
func Asks(r *Request) bool {
	return r.State == "queued" || (r.State == Landed && r.Kind.String == "task" && r.QueueOrder.Valid)
}

// SinceRelease keeps the events of task (an SQL expression) newer than its last release: a release takes back a begun task.
func SinceRelease(task string) string {
	return " AND id > COALESCE((SELECT MAX(r.id) FROM events r WHERE r.kind = 'release_task' AND json_valid(r.payload) AND " +
		"json_extract(r.payload, '$.task_id') = " + task + "), 0)"
}

// Begun is whether the task was begun by task_begin or task_self_start, and not released since.
func Begun(q Q, tid int64) (bool, error) {
	var one int
	err := q.QueryRow("SELECT 1 FROM events WHERE kind IN ('task_begin', 'task_self_start') AND "+eventTask+" = ?"+SinceRelease("?")+" LIMIT 1", tid, tid).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// ReadsDoing is whether the task has been sent to its owner, or it was begun on a founder message (TaskFacts.Asked).
func ReadsDoing(q Q, tid int64) (bool, error) {
	t, err := GetTask(q, tid)
	if err != nil || t == nil {
		return false, err
	}
	begun, err := Begun(q, tid)
	return verdict.TaskFacts{Begun: begun, Sent: t.SentAt.Valid}.Asked(), err
}

// HoldsCurrent is whether the task may be delivered: the founder's or a gate's, its agent's current one, or its agent's way is free (wayFree).
func HoldsCurrent(q Q, t *Task) (bool, error) {
	if t.Acting == actor.OwnerKey || t.Acting == Gate {
		return true, nil
	}
	if held, ok, err := CurrentTask(q, t.Acting); err != nil || ok && held == t.ID {
		return err == nil, err
	}
	return wayFree(q, t.Acting)
}

// wayFree: agent holds no open task that does not wait (TaskWaits).
func wayFree(q Q, agent string) (bool, error) {
	held, ok, err := CurrentTask(q, agent)
	if err != nil || !ok {
		return err == nil, err
	}
	cur, err := GetTask(q, held)
	if err != nil || cur == nil || cur.Ended() {
		return err == nil, err
	}
	return TaskWaits(q, held)
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

// TaskState is the task's state, decided by verdict.TaskState from this task's facts. A fact the verdict cannot use is not read: an
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

// ChildBlockerOf is the open child task of someone else that blocks tid, nil for none.
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

// ChildWait is what the agent waits on when its current task is held by a child of its own ("waiting for T<n> (owner)").
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

// BlockWait is what the agent is held up by when its current task is blocked ("blocked by T<n> (owner), ...").
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

// CurrentTaskStillAsking is whether the agent's current task still has its own request out, the one thing left
// to deliver. A begun task always does, until it is done; one that waits (TaskWaits) never does, so its owner moves on.
func CurrentTaskStillAsking(q Q, agent string) (bool, error) {
	t, err := currentOpen(q, agent)
	if err != nil || t == nil {
		return false, err
	}
	if waits, err := TaskWaits(q, t.ID); waits || err != nil {
		return false, err
	}
	if b, err := Begun(q, t.ID); b || err != nil {
		return b, err
	}
	return t.SentAt.Valid, nil
}

// NewestRequest is the newest request's id, 0 for none: the after_request a reset event carries.
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

// WaitingTasks is the agent's open tasks that could become current, by priority then age. Skips one blocked,
// still a draft, or with nothing of its own left to deliver. Blocked counts a task waiting on a child of its own (TaskWaits).
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
		waits, err := TaskWaits(q, t.ID)
		if err != nil {
			return nil, err
		}
		if waits {
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

// ResetAfter is the highest request id a reset event on the task has covered.
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

// MemoCap is the runes of comments a delivery or a start shows when the receiver has none of its own on the task.
const MemoCap = 500

// MemoSummary is task tid's comments for a delivery to receiver or its start, one per line, each whole, newest last; "" when it has none.
// They are the comments after receiver's own last one; with none of its own, the newest that fit cap runes (always the newest). The
// comments left out are only counted. Comments that request told already carried to receiver are left out uncounted (told 0: none).
func MemoSummary(q Q, tid int64, receiver string, told int64, cap int) (string, error) {
	shown, earlier, err := memosShown(q, tid, receiver, told, cap)
	if err != nil || len(shown) == 0 && earlier == 0 {
		return "", err
	}
	lines := make([]string, len(shown))
	for i, m := range shown {
		lines[i] = m.line
	}
	return summary(tid, lines, earlier), nil
}

// DeliveryMemoTexts is the texts of the comments request told brings receiver on task tid: those it carries, then those MemoSummary shows
// with the same cap.
func DeliveryMemoTexts(q Q, tid int64, receiver string, told int64, cap int) ([]string, error) {
	rows, err := q.Query("SELECT m.body FROM task_memos m JOIN memo_tells t ON t.memo_id = m.id WHERE m.task_id = ? AND t.reader = ? AND t.request_id = ? ORDER BY m.id",
		tid, receiver, told)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		out = append(out, body)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	shown, _, err := memosShown(q, tid, receiver, told, cap)
	if err != nil {
		return nil, err
	}
	for _, m := range shown {
		out = append(out, m.body)
	}
	return out, nil
}

type shownMemo struct{ line, body string }

// memosShown is MemoSummary's comments, and how many earlier ones it leaves out.
func memosShown(q Q, tid int64, receiver string, told int64, cap int) ([]shownMemo, int, error) {
	rows, err := q.Query("SELECT id, author, body, created_at, edited_at FROM task_memos m WHERE task_id = ? AND NOT EXISTS "+
		"(SELECT 1 FROM memo_tells WHERE memo_id = m.id AND reader = ? AND request_id = ?) ORDER BY id", tid, receiver, told)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var memos []shownMemo
	own := -1 // the index of receiver's last comment
	for rows.Next() {
		var id int64
		var author, body string
		var created float64 // a memo's stamp is float seconds, with a fraction
		var edited sql.NullFloat64
		if err := rows.Scan(&id, &author, &body, &created, &edited); err != nil {
			return nil, 0, err
		}
		at := created
		if edited.Valid && edited.Float64 != 0 {
			at = edited.Float64
		}
		if author == receiver {
			own = len(memos)
		}
		memos = append(memos, shownMemo{fmt.Sprintf("comment %d %s %s: %s", id, author, time.Unix(int64(at), 0).Format("01-02 15:04"), textsafe.Oneline(body)), body})
	}
	if err := rows.Err(); err != nil || len(memos) == 0 {
		return nil, 0, err
	}
	from := own + 1
	if own < 0 {
		from = len(memos) - 1
		for n := utf8.RuneCountInString(memos[from].line); from > 0; from-- {
			if n += utf8.RuneCountInString(memos[from-1].line) + 1; n > cap {
				break
			}
		}
	}
	return memos[from:], from, nil
}

func summary(tid int64, shown []string, earlier int) string {
	head := "Comments, newest last"
	switch {
	case len(shown) == 0:
		return fmt.Sprintf("Comments: %d, none since yours (hl task show T%d)", earlier, tid)
	case earlier > 0:
		head += fmt.Sprintf(" (%d earlier: hl task show T%d)", earlier, tid)
	}
	return head + ":\n" + strings.Join(shown, "\n")
}
