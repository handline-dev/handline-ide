package reads

import (
	"fmt"
	"sort"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

// doingStarts begin a window; doingEnds close it. A delivery that landed with the holder counts as a start, as in task.time.
var doingStarts = map[string]bool{"task_begin": true, "task_start": true, "delivered": true}
var doingEnds = map[string]bool{"release_task": true, "queue_task": true, "draft_task": true, "reopen_task": true, "reject_task": true,
	"task_report": true, "mark_task_done": true, "discard_task": true, "close_task": true, "edit_task": true, "task_handoff": true}

// doingWindows is the spans in which agents worked task tid in doing, from the events: a window opens at a begin or a landed delivery and
// closes at the first of a state change out of doing, a hand-off, the task's end, or its holder starting another task by a begin or a delivery.
// A turn moves the work to the agent it is on while it lasts; a turn on the founder is no one's work.
func (e *Env) doingWindows(tid, ended, now int64) ([]usage.DoingWindow, error) {
	evs, err := query(e.DB, "SELECT ts, agent, kind, payload FROM events WHERE "+eventTask+" = ? AND kind IN "+
		"('task_begin', 'task_start', 'release_task', 'queue_task', 'draft_task', 'reopen_task', 'reject_task', 'task_report', 'mark_task_done', "+
		"'discard_task', 'close_task', 'edit_task', 'task_handoff', 'task_turn_push', 'task_turn_pop') ORDER BY id", tid)
	if err != nil {
		return nil, err
	}
	evs, err = withDeliveries(e, tid, evs)
	if err != nil {
		return nil, err
	}
	var out []usage.DoingWindow
	var base, holder string // who began the doing, and who the open window is of
	var turns []string
	var from int64
	acting := func() string {
		if n := len(turns); n > 0 {
			return turns[n-1]
		}
		return base
	}
	closeAt := func(at int64) {
		if holder != "" && at > from {
			out = append(out, usage.DoingWindow{Agent: holder, Lo: from, Hi: at})
		}
		holder = ""
	}
	// the open window goes on under whoever acts now
	move := func(at int64) {
		if base == "" || acting() == holder {
			return
		}
		closeAt(at)
		if who := acting(); !actor.IsPerson(who) {
			holder, from = who, at
		}
	}
	for _, ev := range evs {
		kind, p := ev.str("kind"), decodeObject(ev.str("payload"))
		switch {
		case kind == "task_turn_push":
			turns = append(turns, text(p["to"]))
			move(ev.i("ts"))
		case kind == "task_turn_pop":
			for i := len(turns) - 1; i >= 0; i-- {
				if turns[i] == text(p["agent"]) {
					turns = append(turns[:i], turns[i+1:]...)
					break
				}
			}
			move(ev.i("ts"))
		case doingStarts[kind]:
			if base == "" {
				base = ev.str("agent")
				move(ev.i("ts"))
			}
		case doingEnds[kind]:
			if _, ok := p["owner"]; kind == "edit_task" && !ok {
				continue
			}
			closeAt(ev.i("ts"))
			base = ""
		}
	}
	end := now
	if ended != 0 {
		end = ended
	}
	closeAt(end)
	// A holder is in one window at a time: another task's start after this window's, or in the same second with a lower task number (so
	// exactly one of two simultaneous starts keeps its window), cuts it there.
	kept := out[:0]
	for _, w := range out {
		if w.Hi > end {
			w.Hi = end
		}
		next, err := queryOne(e.DB, "SELECT MIN(ts) AS ts FROM (SELECT ts FROM events WHERE agent = ? AND kind IN ('task_begin', 'task_start') AND "+
			"(ts > ? OR (ts = ? AND "+eventTask+" < ?)) AND "+eventTask+" != ? UNION ALL SELECT r.landed_at FROM requests r JOIN task_requests l "+
			"ON l.request_id = r.id WHERE r.target = ? AND (r.landed_at > ? OR (r.landed_at = ? AND l.task_id < ?)) AND l.task_id != ?)",
			w.Agent, w.Lo, w.Lo, tid, tid, w.Agent, w.Lo, w.Lo, tid, tid)
		if err != nil {
			return nil, err
		}
		if next.set("ts") && next.i("ts") < w.Hi {
			w.Hi = next.i("ts")
		}
		if w.Hi > w.Lo {
			kept = append(kept, w)
		}
	}
	out = kept
	return out, nil
}

// statsRetro is stats.retro: a founder's task's wall time, and the tokens spent inside the doing windows of its subtree, per agent.
func (e *Env) statsRetro(_ *rpc.Conn, params map[string]any) (any, error) {
	return atNow(params, e.retroAt)
}

func (e *Env) retroAt(tid, now int64) (any, error) {
	t, err := db.GetTask(e.DB, tid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, noTask(tid)
	}
	if !actor.IsPerson(t.AnswersTo()) {
		return nil, badParam(fmt.Sprintf("T%d does not answer to the founder", tid), "task")
	}
	subtree, err := query(e.DB, "WITH RECURSIVE sub(id) AS (SELECT ? UNION SELECT t.id FROM tasks t JOIN sub ON t.parent_task = sub.id) "+
		"SELECT t.id, COALESCE(t.done_at, t.discarded_at, t.closed_at) AS ended FROM tasks t JOIN sub ON t.id = sub.id ORDER BY t.id", tid)
	if err != nil {
		return nil, err
	}
	var ws []usage.DoingWindow
	var of []int64
	for _, s := range subtree {
		got, err := e.doingWindows(s.i("id"), s.i("ended"), now)
		if err != nil {
			return nil, err
		}
		for range got {
			of = append(of, s.i("id"))
		}
		ws = append(ws, got...)
	}
	uses, err := usage.WindowUses(e.DB, ws, usage.PricesFromConfig((&usage.Env{Home: e.Home, Now: func() float64 { return float64(now) }}).CurrentConfig()))
	if err != nil {
		return nil, err
	}
	type tally struct {
		secs, tokens int64
		cost         float64
		tasks        map[int64]bool
	}
	per := map[string]*tally{}
	for i, w := range ws {
		a := per[w.Agent]
		if a == nil {
			a = &tally{tasks: map[int64]bool{}}
			per[w.Agent] = a
		}
		a.secs += w.Hi - w.Lo
		a.tokens += uses[i].Tokens
		a.cost += uses[i].Cost
		a.tasks[of[i]] = true
	}
	names := make([]string, 0, len(per))
	for n := range per {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := per[names[i]], per[names[j]]
		return a.tokens > b.tokens || (a.tokens == b.tokens && names[i] < names[j])
	})
	agents := []any{}
	var tokens int64
	var cost float64
	for _, n := range names {
		a := per[n]
		ids := []int64{}
		for id := range a.tasks {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		agents = append(agents, map[string]any{"agent": n, "doing_seconds": a.secs, "tokens": a.tokens, "cost": round(a.cost, 4), "tasks": ids})
		tokens += a.tokens
		cost += a.cost
	}
	end := now
	if t.DoneAt.Valid {
		end = t.DoneAt.Int64
	} else if t.DiscardedAt.Valid {
		end = t.DiscardedAt.Int64
	}
	return map[string]any{"task": tid, "wall_seconds": end - t.CreatedAt, "tokens": tokens, "cost": round(cost, 4), "agents": agents}, nil
}
