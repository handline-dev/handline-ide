package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
)

// A task's plan and the asks for one live in events, like the edits of its brief: each version is a task_plan event, the founder's OK a
// plan_ok event naming the version it approved (so a newer version clears it), an ask for a plan or for the core logic a plan_ask or
// core_ask event, and that ask's delivery to its agent a task_ask_sent event.
const (
	PlanKind     = "task_plan"
	PlanOKKind   = "plan_ok"
	PlanAskKind  = "plan_ask"
	CoreAskKind  = "core_ask"
	AskSentKind  = "task_ask_sent"
	planBaseSQL  = "(kind = 'task_new' OR kind = 'task_handoff' OR (kind = 'edit_task' AND (json_extract(payload, '$.body') IS NOT NULL OR json_extract(payload, '$.owner') IS NOT NULL)))"
	eventCols    = "id, ts, agent, COALESCE(payload, '')"
	taskEventSQL = "SELECT " + eventCols + " FROM events WHERE kind = ? AND " + eventTask + " = ? ORDER BY id"
)

// PlanEventsSQL is each task's count and newest id of the plan events above: what a watcher compares to see a plan or an ask change.
const PlanEventsSQL = "SELECT " + eventTask + ", COUNT(*), MAX(id) FROM events WHERE kind IN ('" + PlanKind + "', '" + PlanOKKind + "', '" +
	PlanAskKind + "', '" + CoreAskKind + "', '" + AskSentKind + "') GROUP BY 1"

// DoingTask is the task the agent is doing now (its current task, held and doing), 0 for none.
func DoingTask(q Q, agent string) (int64, error) {
	cur, held, err := CurrentTask(q, agent)
	if err != nil || !held {
		return 0, err
	}
	doing, err := IsDoing(q, cur)
	if err != nil || !doing {
		return 0, err
	}
	return cur, nil
}

// PlanVersion is one version of a task's plan.
type PlanVersion struct {
	ID, At int64
	By     string
	Text   string
}

// PlanMark is the founder's OK of a plan version.
type PlanMark struct {
	At int64
	By string
}

// Plan is a task's plan: its versions oldest first (the last is the plan), and the OK of the last one, nil when it has none.
type Plan struct {
	Versions []PlanVersion
	OK       *PlanMark
}

// Current is the plan as it stands.
func (p *Plan) Current() PlanVersion { return p.Versions[len(p.Versions)-1] }

// TaskAsk is an ask for a task's plan (Kind PlanAskKind) or core logic (CoreAskKind): To is the agent asked, Behind the task it was doing
// when asked (the ask waits until that is no longer its doing task), Sent when it was typed to the agent (0 while it waits), Dropped
// true when it was never sent because it no longer applied.
type TaskAsk struct {
	ID, At, Task int64
	Kind, By     string
	To, Note     string
	Behind       int64
	Sent         int64
	Dropped      bool
}

type taskEvent struct {
	id, at  int64
	agent   string
	payload map[string]any
}

func taskEvents(q Q, kind string, tid int64) ([]taskEvent, error) {
	rows, err := q.Query(taskEventSQL, kind, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []taskEvent
	for rows.Next() {
		var e taskEvent
		var p string
		if err := rows.Scan(&e.id, &e.at, &e.agent, &p); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(p), &e.payload) != nil {
			e.payload = map[string]any{}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func payloadStr(p map[string]any, k string) string {
	s, _ := p[k].(string)
	return s
}

func payloadNum(p map[string]any, k string) int64 {
	f, _ := p[k].(float64)
	return int64(f)
}

// TaskPlan is the task's plan, nil when none was written.
func TaskPlan(q Q, tid int64) (*Plan, error) {
	evs, err := taskEvents(q, PlanKind, tid)
	if err != nil || len(evs) == 0 {
		return nil, err
	}
	p := &Plan{}
	for _, e := range evs {
		p.Versions = append(p.Versions, PlanVersion{ID: e.id, At: e.at, By: e.agent, Text: payloadStr(e.payload, "text")})
	}
	var id, at int64
	var by, payload string
	err = q.QueryRow("SELECT "+eventCols+" FROM events WHERE kind = ? AND "+eventTask+" = ? ORDER BY id DESC LIMIT 1", PlanOKKind, tid).Scan(&id, &at, &by, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	} else if err != nil {
		return nil, err
	}
	var mark map[string]any
	_ = json.Unmarshal([]byte(payload), &mark)
	base, err := PlanBase(q, tid)
	if err != nil {
		return nil, err
	}
	// The OK is of this version, and of the brief and assignee it was written for.
	if ok, _ := mark["ok"].(bool); ok && payloadNum(mark, "plan") == p.Current().ID && p.Current().ID > base {
		p.OK = &PlanMark{At: at, By: by}
	}
	return p, nil
}

// AddPlan keeps text as the task's newest plan version, by its author.
func AddPlan(q Q, tid int64, by, text string) (int64, error) {
	return AddEvent(q, by, PlanKind, KV{"task_id", tid}, KV{"text", text})
}

// MarkPlan records the founder's OK (ok) of plan version plan, or takes it back (!ok).
func MarkPlan(q Q, tid int64, by string, plan int64, ok bool) error {
	_, err := AddEvent(q, by, PlanOKKind, KV{"task_id", tid}, KV{"plan", plan}, KV{"ok", ok})
	return err
}

// AddTaskAsk records an ask for the task's plan or core logic, which the board sends its agent once to is free of behind.
func AddTaskAsk(q Q, kind string, tid int64, by, to, note string, behind int64) (int64, error) {
	payload := []KV{{"task_id", tid}, {"to", to}, {"behind", behind}}
	if note != "" {
		payload = append(payload, KV{"note", note})
	}
	return AddEvent(q, by, kind, payload...)
}

// MarkAskSent records that ask went out as request rid, or (rid 0) that it was dropped unsent.
func MarkAskSent(q Q, a *TaskAsk, rid int64) error {
	_, err := AddEvent(q, a.By, AskSentKind, KV{"task_id", a.Task}, KV{"ask", a.ID}, KV{"request_id", rid})
	return err
}

// TaskAsks are the task's asks of kind, oldest first.
func TaskAsks(q Q, kind string, tid int64) ([]*TaskAsk, error) {
	evs, err := taskEvents(q, kind, tid)
	if err != nil || len(evs) == 0 {
		return nil, err
	}
	sent, err := taskEvents(q, AskSentKind, tid)
	if err != nil {
		return nil, err
	}
	var out []*TaskAsk
	for _, e := range evs {
		a := &TaskAsk{ID: e.id, At: e.at, Task: tid, Kind: kind, By: e.agent, To: payloadStr(e.payload, "to"), Note: payloadStr(e.payload, "note"), Behind: payloadNum(e.payload, "behind")}
		for _, s := range sent {
			if payloadNum(s.payload, "ask") == e.id {
				a.Sent, a.Dropped = s.at, payloadNum(s.payload, "request_id") == 0
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// UnsentAsks are every ask not yet sent nor dropped, oldest first.
func UnsentAsks(q Q) ([]*TaskAsk, error) {
	// The sent marker is found by its task, as the events_kind_task index reads it, then by the ask it names.
	rows, err := q.Query("SELECT DISTINCT "+eventTask+" FROM events e WHERE kind IN (?, ?) AND NOT EXISTS (SELECT 1 FROM events s WHERE s.kind = ? "+
		"AND CASE WHEN json_valid(s.payload) THEN json_extract(s.payload, '$.task_id') END = "+eventTask+" AND json_extract(s.payload, '$.ask') = e.id)",
		PlanAskKind, CoreAskKind, AskSentKind)
	if err != nil {
		return nil, err
	}
	var tids []int64
	for rows.Next() {
		var tid sql.NullInt64
		if err := rows.Scan(&tid); err != nil {
			rows.Close()
			return nil, err
		}
		if tid.Valid {
			tids = append(tids, tid.Int64)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []*TaskAsk
	for _, tid := range tids {
		for _, kind := range []string{PlanAskKind, CoreAskKind} {
			asks, err := TaskAsks(q, kind, tid)
			if err != nil {
				return nil, err
			}
			for _, a := range asks {
				if a.Sent == 0 {
					out = append(out, a)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// OpenedAsDraft is whether the task was opened as a draft (task.new draft), the only drafts the plans setting asks about on its own.
func OpenedAsDraft(q Q, tid int64) (bool, error) {
	var n int
	err := q.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'task_new' AND "+eventTask+" = ? AND json_extract(payload, '$.draft') = 1", tid).Scan(&n)
	return n > 0, err
}

// WritingPlan is whether a plan ask was sent to agent since the stamp and no plan has come for its task since, while that task is still
// an open draft the agent is to work.
func WritingPlan(q Q, agent string, since int64) (bool, error) {
	tids, err := IDs(q, "SELECT DISTINCT json_extract(a.payload, '$.task_id') FROM events s JOIN events a ON a.id = json_extract(s.payload, '$.ask') "+
		"AND a.kind = ? WHERE s.kind = ? AND s.ts >= ? AND json_extract(s.payload, '$.request_id') != 0 AND json_extract(a.payload, '$.to') = ?",
		PlanAskKind, AskSentKind, since, agent)
	if err != nil {
		return false, err
	}
	for _, tid := range tids {
		t, err := GetTask(q, tid)
		if err != nil {
			return false, err
		}
		if t == nil || t.Ended() || t.Owner != agent {
			continue
		}
		st, err := TaskState(q, t)
		if err != nil {
			return false, err
		}
		if st != "draft" {
			continue
		}
		state, _, _, err := PlanState(q, tid)
		if err != nil || state == "writing" {
			return state == "writing", err
		}
	}
	return false, nil
}

// PlanBase is the newest event that makes the task's plan due again: its opening, a change of its brief, or of who works it.
func PlanBase(q Q, tid int64) (int64, error) {
	var id sql.NullInt64
	err := q.QueryRow("SELECT MAX(id) FROM events WHERE "+planBaseSQL+" AND "+eventTask+" = ?", tid).Scan(&id)
	return id.Int64, err
}

// PlanState is how a task's plan stands, the word a client shows: none (no plan and none asked), asked (an ask waits for its agent),
// writing (the ask was sent and no plan has come since), written (a plan not yet OK), ok.
func PlanState(q Q, tid int64) (string, *Plan, *TaskAsk, error) {
	p, err := TaskPlan(q, tid)
	if err != nil {
		return "", nil, nil, err
	}
	asks, err := TaskAsks(q, PlanAskKind, tid)
	if err != nil {
		return "", nil, nil, err
	}
	var open *TaskAsk // the newest ask that came after the plan and was not dropped
	for _, a := range asks {
		if (p == nil || a.ID > p.Current().ID) && !a.Dropped {
			open = a
		}
	}
	switch {
	case open != nil && open.Sent == 0:
		return "asked", p, open, nil
	case open != nil:
		return "writing", p, open, nil
	case p != nil && p.OK != nil:
		return "ok", p, nil, nil
	case p != nil:
		return "written", p, nil, nil
	}
	return "none", nil, nil, nil
}

// PlanDue is whether the task needs its plan asked for: none written since its brief or assignee last changed, and no ask out since.
func PlanDue(q Q, tid int64) (bool, error) {
	base, err := PlanBase(q, tid)
	if err != nil {
		return false, err
	}
	p, err := TaskPlan(q, tid)
	if err != nil {
		return false, err
	}
	if p != nil && p.Current().ID > base {
		return false, nil
	}
	asks, err := TaskAsks(q, PlanAskKind, tid)
	if err != nil {
		return false, err
	}
	for _, a := range asks {
		if a.ID > base || a.Sent == 0 {
			return false, nil
		}
	}
	return true, nil
}

// PlanWrite is one plan written to answer a sent ask: its agent, when the ask was sent and when the plan came.
type PlanWrite struct {
	Agent        string
	Sent, Answer int64
}

// PlanWrites are the newest n plans written to answer a sent ask, newest first.
func PlanWrites(q Q, n int) ([]PlanWrite, error) {
	rows, err := q.Query("SELECT json_extract(a.payload, '$.to'), s.ts, (SELECT p.ts FROM events p WHERE p.kind = ? "+
		"AND CASE WHEN json_valid(p.payload) THEN json_extract(p.payload, '$.task_id') END = json_extract(s.payload, '$.task_id') "+
		"AND p.id > s.id AND p.agent = json_extract(a.payload, '$.to') ORDER BY p.id LIMIT 1) AS answer "+
		"FROM events s JOIN events a ON a.id = json_extract(s.payload, '$.ask') AND a.kind = ? "+
		"WHERE s.kind = ? AND json_extract(s.payload, '$.request_id') != 0 AND answer IS NOT NULL ORDER BY s.id DESC LIMIT ?",
		PlanKind, PlanAskKind, AskSentKind, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanWrite
	for rows.Next() {
		var w PlanWrite
		if err := rows.Scan(&w.Agent, &w.Sent, &w.Answer); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// PlanSentKind records that a task's delivery (request_id) carried its plan at a version, so its tell shows that delivery whole.
const PlanSentKind = "task_plan_sent"

// AddPlanSent records that request rid delivered task tid with its plan at version.
func AddPlanSent(q Q, tid, rid int64, version int) error {
	_, err := AddEvent(q, "", PlanSentKind, KV{"task_id", tid}, KV{"request_id", rid}, KV{"version", version})
	return err
}

// CarriesPlan is whether request rid delivered its task with the task's plan.
func CarriesPlan(q Q, rid int64) (bool, error) {
	var n int
	err := q.QueryRow("SELECT COUNT(*) FROM events WHERE kind = ? AND "+eventRequest+" = ?", PlanSentKind, rid).Scan(&n)
	return n > 0, err
}
