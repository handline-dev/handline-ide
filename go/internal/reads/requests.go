package reads

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

// The founder's requests: the tasks they opened (a message made a request, or a task of their own for a lead). A message that is not a task
// is the chat's, not a request. Each request's status is read from its tree of tasks and stored nowhere.

// Request statuses, in the order the first that holds is the status.
const (
	reqWaitingOnYou = "waiting_on_you" // a task assigned to the founder, or a decision card, anywhere in the tree
	reqReview       = "review"         // an open task has a PR
	reqDoing        = "doing"          // some task in the tree is open
	reqDone         = "done"           // the request's own task is done
)

const (
	// requestWindowS is how far back a request done is listed.
	requestWindowS = 30 * 24 * 3600
)

// requestNode is one task of a request's tree.
type requestNode struct {
	id, at, pr int64
	due        int64  // its timeout_at, 0 when its clock is not running
	acting     string // who acts on it now (db.ActingSQL)
	open       bool
	blocked    bool // it waits on another open task
}

// RequestRows is requests.list's rows, newest activity first, before filtering by state and paging.
func (e *Env) RequestRows(clock int64) ([]map[string]any, error) {
	since := clock - requestWindowS
	roots, err := query(e.DB, "SELECT id, title, owner, COALESCE(from_request, turn_origin) AS origin, created_at, done_evidence, done_at, discarded_by, discarded_at FROM tasks WHERE "+requestRootSQL+
		" AND (discarded_by IS NOT NULL AND discarded_at >= ? OR discarded_by IS NULL AND (done_evidence IS NULL OR done_at >= ?)) ORDER BY id", actor.OwnerKey, actor.OwnerKey, since, since)
	if err != nil {
		return nil, err
	}
	nodes, err := e.requestNodes(since)
	if err != nil {
		return nil, err
	}
	cards, err := e.cardTasks()
	if err != nil {
		return nil, err
	}
	unread, err := e.unreadAll()
	if err != nil {
		return nil, err
	}
	seen, err := e.resultMarks()
	if err != nil {
		return nil, err
	}
	origins, err := e.originAgents()
	if err != nil {
		return nil, err
	}
	facts, err := e.attentionFacts(clock)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, r := range roots {
		id := r.i("id")
		tree := nodes[id]
		status, holders, prs := reqDoing, map[string]bool{}, map[int64]bool{}
		open, done, last, waiting, review := 0, 0, r.i("created_at"), false, false
		var now, rootNow *requestNode // now: the open task under the root with the latest activity; the root's own when nothing is under it
		for i, n := range tree {
			last = max(last, n.at)
			if n.id != id {
				if n.open {
					open++
				} else {
					done++
				}
			}
			if !n.open {
				continue
			}
			if actor.IsPerson(n.acting) || cards[n.id] {
				waiting = true
			} else if n.acting != db.Gate {
				holders[n.acting] = true
				if n.id == id {
					rootNow = &tree[i]
				} else if now == nil || n.at >= now.at {
					now = &tree[i]
				}
			}
			if n.pr != 0 {
				review, prs[n.pr] = true, true
			}
		}
		switch {
		case r.set("done_evidence") || r.set("discarded_by"):
			status, holders = reqDone, map[string]bool{}
		case waiting:
			status = reqWaitingOnYou
		case review:
			status = reqReview
		}
		if now == nil {
			now = rootNow
		}
		agent := r.str("owner")
		nowAgent, blocked := any(nil), false
		if now != nil && status != reqDone {
			nowAgent, blocked = now.acting, now.blocked
		}
		row := map[string]any{"kind": "task", "task": id, "ref": nil, "title": r.str("title"), "status": status, "agent": agent,
			"holders": sortedKeys(holders), "open": open, "done": done, "prs": sortedNums(prs), "now": nowAgent, "blocked": blocked,
			"progress": progressLine(status, open, done, nowAgent, blocked), "unread_comments": unread[id].n,
			"created": ts(r["created_at"]), "last_activity": ts(last), "cursor": cursorOf(last, "t", id), "attention": nil, "ended": endedOf(r, seen)}
		if r.set("discarded_by") {
			row["progress"] = "discarded"
		}
		if status != reqDone {
			a, err := facts.of(id, tree, last, cards)
			if err != nil {
				return nil, err
			}
			if a != nil {
				row["attention"] = a
			}
		}
		if rid := r.i("origin"); rid != 0 {
			row["ref"] = origins[rid].ref
			row["origin"] = origins[rid].agent
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i]["cursor"].(string) > out[j]["cursor"].(string) })
	return out, nil
}

// progressLine is a request in one line: `sub 3/5 done, now daemon-core, in review`.
func progressLine(status string, open, done int, now any, blocked bool) string {
	if status == reqDone {
		return "done"
	}
	var parts []string
	if open+done > 0 {
		parts = append(parts, fmt.Sprintf("sub %d/%d done", done, open+done))
	}
	if who, ok := now.(string); ok {
		parts = append(parts, "now "+who)
	}
	switch {
	case status == reqWaitingOnYou:
		parts = append(parts, "waiting on you")
	case blocked:
		parts = append(parts, "blocked")
	case status == reqReview:
		parts = append(parts, "in review")
	}
	return strings.Join(parts, ", ")
}

// cursorOf orders rows by last activity, then number (kind is "t", kept so a saved cursor still reads); text of one width sorts as it reads.
func cursorOf(last int64, kind string, id int64) string {
	return fmt.Sprintf("%012d:%s%012d", last, kind, id)
}

// requestRootSQL is a request's own task: one with no parent that answers to the founder (its reporter, else its opener), or one whose
// request is itself. Two args: human, human.
const requestRootSQL = "((COALESCE(reporter, opened_by) = ? AND parent_task IS NULL) OR request = id) AND owner != ?"

// requestTreeSQL is the ids of request N's tree, as requestNodes counts it: its own task, the tasks that serve it, and every task below
// those. Two args: N, N.
const requestTreeSQL = "WITH RECURSIVE story(id) AS (SELECT ? UNION SELECT id FROM tasks WHERE request = ? " +
	"UNION SELECT k.id FROM tasks k JOIN story ON k.parent_task = story.id) SELECT id FROM story"

// requestNodes is the tasks of every request's tree, by the root's number: the root, every task under it and every task that serves it
// (tasks.request), kept or ended since.
func (e *Env) requestNodes(since int64) (map[int64][]requestNode, error) {
	rows, err := query(e.DB, "WITH RECURSIVE sub(root, id) AS (SELECT id, id FROM tasks WHERE "+requestRootSQL+
		" UNION SELECT request, id FROM tasks WHERE request IS NOT NULL UNION SELECT s.root, k.id FROM tasks k JOIN sub s ON k.parent_task = s.id) "+
		"SELECT s.root, t.id, "+db.ActingSQL("t")+" AS acting, COALESCE(t.pr, 0) AS pr, (t.done_evidence IS NULL AND t.discarded_by IS NULL) AS open, "+
		"EXISTS(SELECT 1 FROM task_blocks b JOIN tasks w ON w.id = b.blocks_on WHERE b.task_id = t.id AND b.dormant = 0 AND w.done_evidence IS NULL AND w.discarded_by IS NULL) AS blocked, "+
		"COALESCE(t.timeout_at, 0) AS due, MAX(t.created_at, COALESCE(t.done_at, 0), COALESCE(t.edited_at, 0), COALESCE(t.discarded_at, 0), "+
		"COALESCE((SELECT MAX(m.created_at) FROM task_memos m WHERE m.task_id = t.id), 0)) AS at "+
		"FROM sub s JOIN tasks t ON t.id = s.id WHERE t.discarded_by IS NULL OR t.discarded_at >= ? ORDER BY t.id", actor.OwnerKey, actor.OwnerKey, since)
	if err != nil {
		return nil, err
	}
	out := map[int64][]requestNode{}
	for _, r := range rows {
		out[r.i("root")] = append(out[r.i("root")], requestNode{id: r.i("id"), acting: r.str("acting"), pr: r.i("pr"), open: r.i("open") == 1, blocked: r.i("blocked") == 1, at: r.i("at"), due: r.i("due")})
	}
	return out, nil
}

// cardTasks is the tasks with a decision card waiting on the founder: a queued request to them, not a board line.
func (e *Env) cardTasks() (map[int64]bool, error) {
	rows, err := query(e.DB, "SELECT DISTINCT l.task_id FROM task_requests l JOIN requests r ON r.id = l.request_id WHERE r.target = ? AND r.state = 'queued' AND "+notBoard, actor.OwnerKey)
	if err != nil {
		return nil, err
	}
	out := map[int64]bool{}
	for _, r := range rows {
		out[r.i("task_id")] = true
	}
	return out, nil
}

// resultMarks is when the founder last saw each ended task, and the time at or before which every result counts as seen.
type resultMarks struct {
	at    map[int64]int64
	floor int64
}

func (e *Env) resultMarks() (resultMarks, error) {
	out := resultMarks{at: map[int64]int64{}}
	if v, ok := queryMeta(e, db.ResultSeenFloorMeta); ok {
		out.floor, _ = strconv.ParseInt(v, 10, 64)
	}
	rows, err := query(e.DB, "SELECT name, row_id FROM cursors WHERE name LIKE ?", db.ResultSeenPrefix+"%")
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		id, _ := strconv.ParseInt(strings.TrimPrefix(r.str("name"), db.ResultSeenPrefix), 10, 64)
		out.at[id] = r.i("row_id")
	}
	return out, nil
}

// endedOf is RequestRow.ended for a root row: nil while it is open.
func endedOf(r row, seen resultMarks) any {
	outcome, at := "done", r.i("done_at")
	if r.set("discarded_by") {
		outcome, at = "discarded", r.i("discarded_at")
	} else if !r.set("done_evidence") {
		return nil
	}
	return map[string]any{"outcome": outcome, "at": ts(at), "seen": at <= seen.floor || at <= seen.at[r.i("id")]}
}

// unseenResults counts the requests ended in the window that the founder has not seen (RequestRow.ended.seen false).
func (e *Env) unseenResults(clock int64) (int, error) {
	since := clock - requestWindowS
	seen, err := e.resultMarks()
	if err != nil {
		return 0, err
	}
	rows, err := query(e.DB, "SELECT id, done_evidence, done_at, discarded_by, discarded_at FROM tasks WHERE "+requestRootSQL+
		" AND (discarded_by IS NOT NULL AND discarded_at >= ? OR discarded_by IS NULL AND done_evidence IS NOT NULL AND done_at >= ?)", actor.OwnerKey, actor.OwnerKey, since, since)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if !endedOf(r, seen).(map[string]any)["seen"].(bool) {
			n++
		}
	}
	return n, nil
}

type origin struct{ ref, agent string }

// originAgents is, by message, the ref and the agent it was sent to, for the requests made from a message (from_request or turn_origin).
func (e *Env) originAgents() (map[int64]origin, error) {
	rows, err := query(e.DB, "SELECT id, ref, target FROM requests WHERE id IN (SELECT from_request FROM tasks WHERE from_request IS NOT NULL) "+
		"OR id IN (SELECT turn_origin FROM tasks WHERE turn_origin IS NOT NULL)")
	if err != nil {
		return nil, err
	}
	out := map[int64]origin{}
	for _, r := range rows {
		out[r.i("id")] = origin{ref: db.MsgPrefix + r.str("ref"), agent: r.str("target")}
	}
	return out, nil
}

func sortedKeys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedNums(m map[int64]bool) []int64 {
	out := []int64{}
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// requestsList is requests.list {state?, before?, limit?}: the rows of RequestRows with the status asked for, a page after the cursor.
func (e *Env) requestsList(_ *rpc.Conn, params map[string]any) (any, error) {
	state, err := optStr(params, "state")
	if err != nil {
		return nil, err
	}
	if state != nil && !slices.Contains([]string{reqWaitingOnYou, reqReview, reqDoing, reqDone}, *state) {
		return nil, badParam("state is waiting_on_you, review, doing or done", "state")
	}
	unseen, ok := params["unseen"].(bool)
	if !ok && params["unseen"] != nil {
		return nil, badParam("unseen is true or false", "unseen")
	}
	before, err := optStr(params, "before")
	if err != nil {
		return nil, err
	}
	limit, err := pageLimit(params)
	if err != nil {
		return nil, err
	}
	rows, err := e.RequestRows(time.Now().Unix())
	if err != nil {
		return nil, err
	}
	var kept []any
	more := false
	for _, r := range rows {
		if (state != nil && r["status"] != *state) || (before != nil && r["cursor"].(string) >= *before) {
			continue
		}
		if ended, finished := r["ended"].(map[string]any); unseen && (!finished || ended["seen"] == true) {
			continue
		}
		if len(kept) == limit {
			more = true
			break
		}
		kept = append(kept, r)
	}
	if kept == nil {
		kept = []any{}
	}
	return map[string]any{"items": kept, "more": more}, nil
}
