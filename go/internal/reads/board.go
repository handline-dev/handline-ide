package reads

import (
	"sort"

	"github.com/handline/handline/go/protocol/rpc"
)

// asksSQL is db.Asks over a requests row r: work still being asked for.
const asksSQL = "(r.state = 'queued' OR (r.state = 'landed' AND r.kind = 'task' AND r.queue_order IS NOT NULL))"

// taskBoard is task.board: the open tasks the caller works, or owns while someone outside its reports works them (mine), and those
// its reports work (reports), each with who holds it now. The founder's board is the tasks assigned to it, unless it asks for
// everyone's.
func (e *Env) taskBoard(c *rpc.Conn, params map[string]any) (any, error) {
	everyone, err := optBool(params, "everyone")
	if err != nil {
		return nil, err
	}
	me := c.Caller.Actor()
	var owners map[string]bool
	switch {
	case everyone:
	case me == human:
		owners = map[string]bool{human: true}
	default:
		if owners, err = e.reach(me); err != nil {
			return nil, err
		}
	}
	// In SQL, before any per-task read: a task in reach by who acts on it, or, on an agent's board, one it answers to someone in reach for.
	q, args := "SELECT *, "+taskActing+" FROM tasks WHERE "+openTask, []any{}
	if owners != nil {
		for n := range owners {
			args = append(args, n)
		}
		in := "(" + placeholders(len(args)) + ")"
		if me == human {
			q += " AND " + actingSQL + " IN " + in
		} else {
			q += " AND (" + actingSQL + " IN " + in + " OR " + askerSQL + " IN " + in + ")"
			args = append(args, args...)
		}
	}
	open, err := query(e.DB, q+" ORDER BY priority, id", args...)
	if err != nil {
		return nil, err
	}
	b, err := e.loadBoard()
	if err != nil {
		return nil, err
	}
	mine, reports := []any{}, []any{}
	for _, t := range open {
		reach := owners == nil || owners[actingOf(t)]
		task, err := e.taskOf(t, b.state(t), b)
		if err != nil {
			return nil, err
		}
		now, err := e.heldBy(t.i("id"), me, b.blocked[t.i("id")])
		if err != nil {
			return nil, err
		}
		var holder any
		if agents := now["agents"].([]any); len(agents) > 0 {
			holder = agents[0]
		}
		r := map[string]any{"task": task, "holder": holder, "now": now}
		if actingOf(t) == me || !reach {
			mine = append(mine, r)
		} else {
			reports = append(reports, r)
		}
	}
	return map[string]any{"mine": mine, "reports": reports}, nil
}

// heldBy is TaskBoardRow.now for task tid, as caller me reads it; blocked is whether the board knows of an open blocker.
func (e *Env) heldBy(tid int64, me string, blocked bool) (map[string]any, error) {
	now := func(kind string, agents []string, tasks []int64) map[string]any {
		a, t := []any{}, []any{}
		for _, x := range agents {
			a = append(a, x)
		}
		for _, x := range tasks {
			t = append(t, x)
		}
		return map[string]any{"kind": kind, "agents": a, "tasks": t}
	}
	if blocked {
		blockers, err := e.blockers(tid)
		if err != nil {
			return nil, err
		}
		var ids []int64
		for _, b := range blockers {
			ids = append(ids, b.i("id"))
		}
		return now("waiting", nil, ids), nil
	}
	targets := func(q string, args ...any) ([]string, error) {
		rows, err := query(e.DB, q, args...)
		if err != nil {
			return nil, err
		}
		var out []string
		for _, r := range rows {
			out = append(out, r.str("target"))
		}
		sort.Strings(out)
		return out, nil
	}
	held, err := targets("SELECT DISTINCT r.target FROM task_requests l CROSS JOIN requests r ON l.request_id = r.id WHERE l.task_id = ? AND "+asksSQL, tid)
	if err != nil {
		return nil, err
	}
	if len(held) > 0 {
		return now("held", held, nil), nil
	}
	kids, err := targets("SELECT DISTINCT r.target FROM requests r JOIN task_requests l ON l.request_id = r.id JOIN tasks c ON c.id = l.task_id "+
		"WHERE c.parent_task = ? AND "+openTask+" AND "+asksSQL, tid)
	if err != nil {
		return nil, err
	}
	if len(kids) > 0 {
		return now("sub_task", kids, nil), nil
	}
	holds, err := query(e.DB, "SELECT c.id, c.owner, "+actingSQLc+" AS acting FROM tasks c WHERE c.parent_task = ? AND "+openTask+" AND EXISTS (SELECT 1 FROM task_requests a "+
		"JOIN task_requests p ON p.request_id = a.request_id WHERE a.task_id = c.id AND p.task_id = ?) ORDER BY c.priority, c.id", tid, tid)
	if err != nil {
		return nil, err
	}
	if len(holds) > 0 {
		var agents []string
		var ids []int64
		for _, h := range holds {
			agents, ids = append(agents, actingOf(h)), append(ids, h.i("id"))
		}
		return now("held_as_sub_task", agents, ids), nil
	}
	last, err := query(e.DB, "SELECT r.state, r.target FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? ORDER BY r.id DESC LIMIT 1", tid)
	if err != nil {
		return nil, err
	}
	if len(last) == 1 && last[0].str("state") == "landed" && last[0].str("target") != me && last[0].str("target") != human {
		return now("result_in", nil, nil), nil
	}
	return now("nobody", nil, nil), nil
}
