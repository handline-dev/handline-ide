package reads

import (
	"sort"
	"strconv"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
)

// unreadSQL counts, per task, the comments by others above the founder's marker or the floor, whichever is higher.
const unreadSQL = "SELECT m.task_id AS task, COUNT(*) AS n, MIN(m.id) AS first, MAX(m.created_at) AS last FROM task_memos m " +
	"LEFT JOIN cursors c ON c.name = 'task_seen:human:' || m.task_id WHERE m.author != 'human' AND m.id > MAX(?, COALESCE(c.row_id, 0)) "

type unread struct{ n, first, last int64 }

func (e *Env) seenFloor() int64 {
	v, ok := queryMeta(e, db.SeenFloorMeta)
	if !ok {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

func queryMeta(e *Env, name string) (string, bool) {
	r, err := queryOne(e.DB, "SELECT value FROM meta WHERE name = ?", name)
	if err != nil || r == nil {
		return "", false
	}
	return r.str("value"), true
}

func (e *Env) unreadOf(tid int64) (unread, error) {
	r, err := queryOne(e.DB, unreadSQL+"AND m.task_id = ? GROUP BY m.task_id", e.seenFloor(), tid)
	if err != nil || r == nil {
		return unread{}, err
	}
	return unread{r.i("n"), r.i("first"), r.i("last")}, nil
}

func (e *Env) unreadAll() (map[int64]unread, error) {
	rows, err := query(e.DB, unreadSQL+"GROUP BY m.task_id", e.seenFloor())
	if err != nil {
		return nil, err
	}
	out := map[int64]unread{}
	for _, r := range rows {
		out[r.i("task")] = unread{r.i("n"), r.i("first"), r.i("last")}
	}
	return out, nil
}

func withUnread(task map[string]any, u unread) {
	task["unread"] = u.n
	if u.n > 0 {
		task["first_unread"] = u.first
	} else {
		task["first_unread"] = nil
	}
}

func noUnread(task map[string]any) { withUnread(task, unread{}) }

// requests.pending: what waits on the founder to act, by task, and the counts the bell and the sidebar badges show.
func (e *Env) requestsPending(_ *rpc.Conn, _ map[string]any) (any, error) {
	all, err := e.unreadAll()
	if err != nil {
		return nil, err
	}
	row := func(task any, title, kind string, items []int64, u unread, at any) map[string]any {
		out := map[string]any{"task": task, "title": title, "kind": kind, "items": items, "unread": u.n, "first_unread": nil, "last_at": at,
			"source": "", "id": "", "agent": nil, "ask": nil, "warning": nil}
		if u.n > 0 {
			out["first_unread"] = u.first
		}
		return out
	}
	forYou, _, err := e.ForYou()
	if err != nil {
		return nil, err
	}
	warnings, err := e.founderWarnings()
	if err != nil {
		return nil, err
	}
	warningOf := map[string]founderWarning{}
	for _, w := range warnings {
		warningOf[w.id] = w
	}
	rank := map[string]int{"comment": 0, "warning": 1, "ask": 2, "decide": 3, "allow": 4}
	answer, byTask := []map[string]any{}, map[int64]map[string]any{}
	for _, it := range forYou {
		if _, deferred := it["later"]; deferred {
			continue
		}
		kind, id := it["kind"].(string), it["id"].(string)
		ask := map[string]string{"allow": "allow", "decide": "decide", "review": "decide"}[it["asks"].(string)]
		var line, agent any
		agent = it["agent"]
		var warning map[string]any
		var tid int64
		switch kind {
		case "task":
			tid, _ = strconv.ParseInt(id, 10, 64)
		case "comment":
			line = it["title"]
			ask = "ask"
			if memo, _ := strconv.ParseInt(id, 10, 64); memo != 0 {
				if r, err := queryOne(e.DB, "SELECT task_id FROM task_memos WHERE id = ?", memo); err != nil {
					return nil, err
				} else if r != nil {
					tid = r.i("task_id")
				}
			}
		case "machine":
			w := warningOf[id]
			tid, warning, ask = w.task, w.view, "warning"
			if tid != 0 { // the row's title is the task's, so the warning's words go here
				line = w.title
			}
		}
		var items []int64
		if tid != 0 {
			// a request the founder queued to themself asks them nothing
			open, err := query(e.DB, "SELECT r.id FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? AND r.target = ? "+
				"AND r.requester != ? AND r.state = 'queued' AND r.kind != 'say' ORDER BY r.id", tid, human, human)
			if err != nil {
				return nil, err
			}
			for _, o := range open {
				items = append(items, o.i("id"))
			}
			turn, err := queryOne(e.DB, "SELECT agent, "+db.TurnAskSQL("u")+" AS ask, pushed_by FROM task_turns u WHERE task_id = ? ORDER BY pos DESC LIMIT 1", tid)
			if err != nil {
				return nil, err
			}
			onTop := turn != nil && turn.str("agent") == human
			// a task the founder owns with no turn and no request open on it is the Tasks page's Needs you, not their turn
			if len(items) == 0 && kind == "task" && !onTop {
				continue
			}
			if onTop && kind == "task" {
				agent = turn.str("pushed_by")
				if a := turn.str("ask"); a != "" {
					line = a
				}
			}
			if prev := byTask[tid]; prev != nil {
				if rank[ask] > rank[prev["kind"].(string)] {
					prev["kind"], prev["source"], prev["id"], prev["agent"] = ask, kind, id, agent
				}
				if prev["ask"] == nil {
					prev["ask"] = line
				}
				if warning != nil {
					prev["warning"] = warning
				}
				continue
			}
		}
		var out map[string]any
		if tid != 0 {
			t, err := queryOne(e.DB, "SELECT title FROM tasks WHERE id = ?", tid)
			if err != nil {
				return nil, err
			}
			title := it["title"].(string)
			if t != nil {
				title = t.str("title")
			}
			if len(items) == 0 {
				items = []int64{}
			}
			out = row(tid, title, ask, items, all[tid], it["since"])
			byTask[tid] = out
		} else {
			out = row(nil, it["title"].(string), ask, []int64{}, unread{}, it["since"])
		}
		out["source"], out["id"], out["agent"], out["ask"], out["warning"] = kind, id, agent, line, warning
		if out["kind"] == "" {
			out["kind"] = "ask"
		}
		answer = append(answer, out)
	}
	// Notes (the bell's dot, not its count): @human comments unread on their task and in their author's chat, on a task no row above has.
	notes, err := query(e.DB, "SELECT m.id, m.task_id, m.author, m.body, m.created_at, t.title FROM task_memos m JOIN tasks t ON t.id = m.task_id "+
		"LEFT JOIN cursors c ON c.name = 'task_seen:human:' || m.task_id WHERE m.author != 'human' AND m.body LIKE '%@human%' AND m.id > MAX(?, COALESCE(c.row_id, 0)) "+
		"AND COALESCE(t.discarded_by, '') = '' AND NOT EXISTS (SELECT 1 FROM requests r WHERE r.memo_id = m.id AND r.target = ? AND r.read_at IS NOT NULL) "+
		"AND COALESCE((SELECT r.ts FROM requests r JOIN cursors s ON r.id = s.row_id WHERE s.name = 'for_you_seen:' || m.author), 0) < m.created_at "+
		"ORDER BY m.id DESC", e.seenFloor(), human)
	if err != nil {
		return nil, err
	}
	noted := 0
	for _, m := range notes {
		tid := m.i("task_id")
		if byTask[tid] != nil || !db.ToFounder(m.str("body")) {
			continue
		}
		out := row(tid, m.str("title"), "note", []int64{}, all[tid], ts(m["created_at"]))
		out["source"], out["id"], out["agent"], out["ask"] = "comment", strconv.FormatInt(m.i("id"), 10), m.str("author"), firstLine(withoutHumanTag(m.str("body")))
		byTask[tid] = out
		answer = append(answer, out)
		noted++
	}
	sort.SliceStable(answer, func(i, j int) bool { return answer[i]["last_at"].(string) > answer[j]["last_at"].(string) })

	// Tasks the founder answers for that have comments they have not read, and are not already a question above.
	comments := []map[string]any{}
	for tid, u := range all {
		if byTask[tid] != nil {
			continue
		}
		t, err := queryOne(e.DB, "SELECT *, "+taskActing+" FROM tasks WHERE id = ? AND COALESCE(discarded_by, '') = ''", tid)
		if err != nil || t == nil {
			if err != nil {
				return nil, err
			}
			continue
		}
		if who, err := e.askerOf(t); err != nil {
			return nil, err
		} else if who != human && actingOf(t) != human {
			continue
		}
		comments = append(comments, row(tid, t.str("title"), "comment", []int64{}, u, ts(u.last)))
	}
	sort.SliceStable(comments, func(i, j int) bool {
		if a, b := comments[i]["last_at"].(string), comments[j]["last_at"].(string); a != b {
			return a > b
		}
		return comments[i]["task"].(int64) > comments[j]["task"].(int64)
	})

	needs, drafts, err := e.needsYouAndDrafts()
	if err != nil {
		return nil, err
	}
	results, err := e.unseenResults(time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return map[string]any{"answer": answer, "comments": comments,
		"counts": map[string]any{"answer": len(answer) - noted, "notes": noted, "comments": len(comments), "needs_you": needs, "drafts": drafts, "unseen_results": results}}, nil
}

// needsYouAndDrafts counts the open tasks the founder owns or that wait on them (Task.waiting.on is founder), and the open drafts.
func (e *Env) needsYouAndDrafts() (needs, drafts int, err error) {
	b, err := e.loadBoard()
	if err != nil {
		return 0, 0, err
	}
	open, err := query(e.DB, "SELECT *, "+taskActing+" FROM tasks WHERE "+openTask+" ORDER BY id")
	if err != nil {
		return 0, 0, err
	}
	for _, t := range open {
		switch st := b.state(t); st {
		case "draft":
			drafts++
		case "queued", "doing":
			if actingOf(t) == human {
				needs++
				continue
			}
			task, err := e.taskOf(t, st, b)
			if err != nil {
				return 0, 0, err
			}
			if w, ok := task["waiting"].(map[string]any); ok && w["on"] == "founder" {
				needs++
			}
		}
	}
	return needs, drafts, nil
}
