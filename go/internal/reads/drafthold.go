package reads

import (
	"fmt"
	"sort"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
)

// waitEdge is one open task waiting on another: through a live block, or a parent on its open child; at is when that wait began.
type waitEdge struct{ waiter, at int64 }

// waitGraph is who waits on whom among the open tasks, read once per board, and each agent's task ends, oldest first.
type waitGraph struct {
	open    map[int64]row
	waiters map[int64][]waitEdge
	ends    map[string][]int64
	held    map[int64]heldResult
}

type heldResult struct {
	ids   []int64
	since int64
}

// graphOf is the board's wait graph, read on first use.
func (e *Env) graphOf(b *board) (*waitGraph, error) {
	if b.graph != nil {
		return b.graph, nil
	}
	g := &waitGraph{open: map[int64]row{}, waiters: map[int64][]waitEdge{}, ends: map[string][]int64{}, held: map[int64]heldResult{}}
	open, err := e.openTasks()
	if err != nil {
		return nil, err
	}
	for _, t := range open {
		g.open[t.i("id")] = t
	}
	for _, t := range open {
		if p := t.i("parent_task"); p != 0 && g.open[p] != nil {
			g.waiters[t.i("id")] = append(g.waiters[t.i("id")], waitEdge{p, t.i("created_at")})
		}
	}
	blocks, err := query(e.DB, "SELECT task_id, blocks_on, created_at FROM task_blocks WHERE dormant = 0")
	if err != nil {
		return nil, err
	}
	for _, k := range blocks {
		on, w := k.i("blocks_on"), k.i("task_id")
		if g.open[on] != nil && g.open[w] != nil {
			g.waiters[on] = append(g.waiters[on], waitEdge{w, k.i("created_at")})
		}
	}
	ends, err := query(e.DB, "SELECT agent, ts FROM events WHERE kind IN "+endKinds+" ORDER BY ts")
	if err != nil {
		return nil, err
	}
	for _, r := range ends {
		g.ends[r.str("agent")] = append(g.ends[r.str("agent")], r.i("ts"))
	}
	b.graph = g
	return g, nil
}

// heldOn is what waits on task x: the open tasks, not drafts, that wait on it through the wait graph's chains, sorted, and when the first of
// them began to. A task's wait begins at the latest along its path of each edge's time and each waiter's send or queue time.
func (e *Env) heldOn(x int64, b *board) ([]int64, int64, error) {
	g, err := e.graphOf(b)
	if err != nil {
		return nil, 0, err
	}
	if h, ok := g.held[x]; ok {
		return h.ids, h.since, nil
	}
	best := map[int64]int64{}
	type step struct{ id, since int64 }
	todo := []step{{x, 0}}
	for len(todo) > 0 {
		cur := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		for _, w := range g.waiters[cur.id] {
			t := g.open[w.waiter]
			queued := t.i("created_at")
			if t.set("queued_at") {
				queued = t.i("queued_at")
			} else if t.set("sent_at") {
				queued = t.i("sent_at")
			}
			began := max(cur.since, w.at, queued)
			if prev, seen := best[w.waiter]; w.waiter == x || seen && prev <= began {
				continue
			}
			best[w.waiter] = began
			todo = append(todo, step{w.waiter, began})
		}
	}
	var h heldResult
	for id, at := range best {
		if s := b.state(g.open[id]); s != "queued" && s != "doing" {
			continue
		}
		h.ids = append(h.ids, id)
		if h.since == 0 || at < h.since {
			h.since = at
		}
	}
	sort.Slice(h.ids, func(i, j int) bool { return h.ids[i] < h.ids[j] })
	g.held[x] = h
	return h.ids, h.since, nil
}

// endKinds are the events of an agent ending a task it held.
const endKinds = "('mark_task_done', 'task_handoff')"

// leftBehind is whether draft t is one an agent made and left without bringing it to the founder, and since when: its opener has ended a
// task since, the founder has not seen it at its newest comment, it waits on no open task, no founder turn is open on it, its parent or
// the task it serves, and that task is not one of the founder's already done.
func (e *Env) leftBehind(t row, b *board) (bool, int64, error) {
	id, opener := t.i("id"), t.str("opened_by")
	if actor.IsPerson(opener) || b.blocked[id] {
		return false, 0, nil
	}
	g, err := e.graphOf(b)
	if err != nil {
		return false, 0, err
	}
	var end int64
	for _, at := range g.ends[opener] {
		if at > t.i("created_at") {
			end = at
			break
		}
	}
	if end == 0 {
		return false, 0, nil
	}
	seen, err := queryOne(e.DB, "SELECT 1 AS n FROM cursors WHERE name = ? AND row_id >= COALESCE((SELECT MAX(id) FROM task_comments WHERE task_id = ?), 0)",
		db.SeenKey(e.person(), id), id)
	if err != nil || seen != nil {
		return false, 0, err
	}
	near := []any{id}
	if p := t.i("parent_task"); p != 0 {
		near = append(near, p)
	}
	served := t.i("request")
	if served != 0 && served != id {
		near = append(near, served)
		done, err := queryOne(e.DB, "SELECT * FROM tasks WHERE id = ? AND COALESCE(done_evidence, '') != ''", served)
		if err != nil {
			return false, 0, err
		}
		if done != nil { // its finish already told the founder when the task was theirs: they own it or it answers to them
			asker, err := e.askerOf(done)
			if err != nil {
				return false, 0, err
			}
			if actor.IsPerson(done.str("owner")) || asker == e.person() {
				return false, 0, nil
			}
		}
	}
	turns, err := query(e.DB, "SELECT u.agent FROM task_turns u JOIN tasks t ON t.id = u.task_id WHERE u.task_id IN (?"+strings.Repeat(", ?", len(near)-1)+
		") AND COALESCE(t.done_evidence, '') = '' AND COALESCE(t.discarded_by, '') = ''", near...)
	if err != nil {
		return false, 0, err
	}
	for _, u := range turns {
		if actor.IsPerson(u.str("agent")) {
			return false, 0, nil
		}
	}
	return true, end, nil
}

// draftCall is why a draft is the founder's to run: what waits on it, or that its agent left it behind; since when.
type draftCall struct {
	held   []int64
	left   bool
	since  int64
	opener string
}

// draftCallOf is draft t's call, ok false when neither holds.
func (e *Env) draftCallOf(t row, b *board) (draftCall, bool, error) {
	held, since, err := e.heldOn(t.i("id"), b)
	if err != nil {
		return draftCall{}, false, err
	}
	c := draftCall{held: held, since: since, opener: t.str("opened_by")}
	if len(held) > 0 {
		return c, true, nil
	}
	left, at, err := e.leftBehind(t, b)
	if err != nil || !left {
		return draftCall{}, false, err
	}
	c.left, c.since = true, at
	return c, true, nil
}

// text is a draft's Task.status text for its call.
func (c draftCall) text() string {
	if len(c.held) == 0 {
		return addr.Ref(c.opener) + " left it for you to run"
	}
	return "waits for you to run it: " + taskList(c.held) + " wait on it"
}

func taskList(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("T%d", id)
	}
	return strings.Join(parts, ", ")
}

// runItem is a ForYou item for a draft the founder has to run.
type runItem struct {
	id, parent int64
	title      string
	call       draftCall
}

// runItems are the drafts the founder has to run (draftCallOf), one item for each; drafts that share one open parent are one item on it.
func (e *Env) runItems(b *board) ([]map[string]any, error) {
	g, err := e.graphOf(b)
	if err != nil {
		return nil, err
	}
	open := make([]row, 0, len(g.open))
	for _, t := range g.open {
		open = append(open, t)
	}
	sort.Slice(open, func(i, j int) bool { return open[i].i("id") < open[j].i("id") })
	title := map[int64]string{}
	var items []runItem
	byParent := map[int64]int{}
	for _, t := range open {
		title[t.i("id")] = t.str("title")
		if b.state(t) != "draft" {
			continue
		}
		c, ok, err := e.draftCallOf(t, b)
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, runItem{id: t.i("id"), parent: t.i("parent_task"), title: t.str("title"), call: c})
			byParent[t.i("parent_task")]++
		}
	}
	var out []map[string]any
	merged := map[int64]map[string]any{}
	for _, it := range items {
		_, parentOpen := title[it.parent]
		if it.parent == 0 || !parentOpen || byParent[it.parent] < 2 {
			out = append(out, forYouItem("task", fmt.Sprint(it.id), it.call.opener, it.title, "run", ts(it.call.since)))
			continue
		}
		if m := merged[it.parent]; m != nil {
			if s := ts(it.call.since).(string); s < m["since"].(string) {
				m["since"] = s
			}
			continue
		}
		m := forYouItem("task", fmt.Sprint(it.parent), it.call.opener, title[it.parent], "run", ts(it.call.since))
		merged[it.parent] = m
		out = append(out, m)
	}
	return out, nil
}

// holdsOf is PendingRequest.holds of a row on task tid: what waits on it, and for a run row on a parent what waits on its drafts.
func (e *Env) holdsOf(tid int64, run bool, b *board) ([]int64, int64, error) {
	held, since, err := e.heldOn(tid, b)
	if err != nil {
		return nil, 0, err
	}
	if run {
		kids, err := query(e.DB, "SELECT *, "+taskActing+" FROM tasks WHERE parent_task = ? AND "+openTask, tid)
		if err != nil {
			return nil, 0, err
		}
		for _, k := range kids {
			if b.state(k) != "draft" {
				continue
			}
			h, s, err := e.heldOn(k.i("id"), b)
			if err != nil {
				return nil, 0, err
			}
			held = append(held, h...)
			if s != 0 && (since == 0 || s < since) {
				since = s
			}
		}
	}
	out := []int64{}
	have := map[int64]bool{tid: true}
	for _, h := range held {
		if !have[h] {
			have[h] = true
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, since, nil
}
