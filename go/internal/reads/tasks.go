package reads

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/settings"
	"github.com/handline/handline/go/taskboard/attachments"
	"github.com/handline/handline/go/taskboard/taskresult"
)

const eventTask = "CASE WHEN json_valid(payload) THEN json_extract(payload, '$.task_id') END"

// everyKind is every event kind, found by one index seek per kind: a query on eventTask that names no kind puts "kind IN everyKind"
// in front of it, so events_kind_task answers it instead of a scan that reads every event's payload.
const everyKind = "(WITH RECURSIVE k(kind) AS (SELECT MIN(kind) FROM events UNION ALL SELECT (SELECT MIN(kind) FROM events WHERE kind > k.kind) " +
	"FROM k WHERE k.kind IS NOT NULL) SELECT kind FROM k)"

// taskEventsSQL is every event of one task, oldest first: task.get's events.
const taskEventsSQL = "SELECT id, ts, agent, kind, payload FROM events WHERE kind IN " + everyKind + " AND " + eventTask + " = ? ORDER BY id"

// duration is the spec's Duration in the largest whole unit: 2h, 90m, 1d.
func duration(secs int64) string {
	for _, u := range []struct {
		name string
		size int64
	}{{"d", 86400}, {"h", 3600}, {"m", 60}} {
		if secs%u.size == 0 {
			return fmt.Sprintf("%d%s", secs/u.size, u.name)
		}
	}
	return fmt.Sprintf("%ds", secs)
}

// openTask is ended in SQL, negated: a task with no done evidence that was not discarded.
const openTask = "COALESCE(done_evidence, '') = '' AND COALESCE(discarded_by, '') = ''"

// board is what the state of a task needs, read in a handful of queries (db.TaskState over every task at once): the
// tasks begun on a founder's message, the ones with a request out, the ones with an open blocker, what every owner
// holds now.
type board struct {
	begun    map[int64]bool
	sent     map[int64]bool
	blocked  map[int64]bool
	current  map[string]int64
	paused   bool       // the founder's pause of new work is in force
	team     *team.Team // read on first use, once per board
	teamRead bool
}

// teamOf is the team file, read once per board.
func (b *board) teamOf(home string) *team.Team {
	if !b.teamRead {
		b.team, _ = team.LoadTeam(home)
		b.teamRead = true
	}
	return b.team
}

// leadOf is team.LeadOf of the board's team file.
func (b *board) leadOf(home, agent string) string { return b.teamOf(home).LeadOf(agent) }

func (e *Env) loadBoard() (*board, error) {
	b := &board{begun: map[int64]bool{}, sent: map[int64]bool{}, blocked: map[int64]bool{}, current: map[string]int64{}}
	if p, err := db.GetPause(e.DB, time.Now()); err != nil {
		return nil, err
	} else {
		b.paused = p != nil
	}
	begunTask := "CASE WHEN json_valid(e.payload) THEN json_extract(e.payload, '$.task_id') END"
	rows, err := query(e.DB, "SELECT DISTINCT "+begunTask+" AS t FROM events e WHERE kind IN ('task_begin', 'task_self_start')"+db.SinceRelease(begunTask))
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r["t"] != nil {
			b.begun[r.i("t")] = true
		}
	}
	if rows, err = query(e.DB, "SELECT id FROM tasks WHERE sent_at IS NOT NULL"); err != nil {
		return nil, err
	}
	for _, r := range rows {
		b.sent[r.i("id")] = true
	}
	if rows, err = query(e.DB, "SELECT DISTINCT b.task_id FROM task_blocks b JOIN tasks t ON t.id = b.blocks_on WHERE b.dormant = 0 AND "+
		"t.discarded_by IS NULL AND t.done_evidence IS NULL"); err != nil {
		return nil, err
	}
	for _, r := range rows {
		b.blocked[r.i("task_id")] = true
	}
	if rows, err = query(e.DB, "SELECT agent, current_task FROM agent_state WHERE current_task IS NOT NULL"); err != nil {
		return nil, err
	}
	for _, r := range rows {
		b.current[r.str("agent")] = r.i("current_task")
	}
	return b, nil
}

// holds is whether the task is the one its owner holds: the founder and the gates hold none, and every task of theirs reads as held.
func (b *board) holds(t row) bool {
	held, has := b.current[actingOf(t)]
	return actor.IsPerson(actingOf(t)) || actingOf(t) == db.Gate || has && held == t.i("id")
}

// busy says which other task t's acting agent holds now, as "<agent> is on T<n>"; empty when it holds none or holds t.
func (b *board) busy(t row) string {
	if cur, has := b.current[actingOf(t)]; has && cur != t.i("id") {
		return fmt.Sprintf("%s is on T%d", actingOf(t), cur)
	}
	return ""
}

// state is one of draft, queued, doing, done, discarded: verdict.TaskState of the board's facts for t.
func (b *board) state(t row) string {
	id := t.i("id")
	// as db.pauseHolds: the pause holds a task its acting agent was not told and did not begin
	pauseHolds := b.paused && !b.begun[id] && !t.set("delivered_at") && !actor.IsPerson(actingOf(t)) && actingOf(t) != db.Gate
	return verdict.TaskState(verdict.TaskFacts{Discarded: t.str("discarded_by") != "", Done: t.str("done_evidence") != "",
		Begun: b.begun[id], Sent: b.sent[id], Held: b.holds(t), Blocked: b.blocked[id], Queued: t.str("queued_by") != "", PauseHolds: pauseHolds})
}

// openTasks are the open tasks in priority then number order.
func (e *Env) openTasks() ([]row, error) {
	return query(e.DB, "SELECT *, "+taskActing+" FROM tasks WHERE "+openTask+" ORDER BY priority, id")
}

// reach is the agents whose tasks agent sees: itself and everyone under it, walked down the supervisor edges the
// stored views record (the Python daemon wrote them into agent_view each tick).
// Reach is the agent and the agents under it that are registered: what an agent session may read of tasks and notifications.
func (e *Env) Reach(agent string) (map[string]bool, error) { return e.reach(agent) }

func (e *Env) reach(agent string) (map[string]bool, error) {
	views, err := query(e.DB, "SELECT agent, view FROM agent_view")
	if err != nil {
		return nil, err
	}
	boss := map[string]string{}
	for _, v := range views {
		if p, _ := decodeObject(v.str("view"))["parent"].(string); p != "" {
			boss[v.str("agent")] = p
		}
	}
	registered := map[string]bool{}
	for _, n := range e.listAgents() {
		registered[n] = true
	}
	seen, todo := map[string]bool{agent: true}, []string{agent}
	for len(todo) > 0 {
		top := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		for a, b := range boss {
			if b == top && registered[a] && !seen[a] {
				seen[a] = true
				todo = append(todo, a)
			}
		}
	}
	return seen, nil
}

func (e *Env) tasksList(c *rpc.Conn, params map[string]any) (any, error) {
	owner, err := optStr(params, "owner")
	if err != nil {
		return nil, err
	}
	acting, err := optStr(params, "acting") // who acts on it now: its top turn's agent, else its owner
	if err != nil {
		return nil, err
	}
	closer, err := optStr(params, "reporter") // who the task answers to: its reporter, else as askerOf
	if err != nil {
		return nil, err
	}
	opener, err := optStr(params, "from")
	if err != nil {
		return nil, err
	}
	state, err := optStr(params, "state")
	if err != nil {
		return nil, err
	}
	parent, err := optInt(params, "parent")
	if err != nil {
		return nil, err
	}
	under, err := optInt(params, "under")
	if err != nil {
		return nil, err
	}
	request, err := optInt(params, "request")
	if err != nil {
		return nil, err
	}
	before, err := optInt(params, "before")
	if err != nil {
		return nil, err
	}
	ids, err := optTaskIDs(params, "ids")
	if err != nil {
		return nil, err
	}
	if ids != nil {
		if params["before"] != nil {
			return nil, badParam("ids does not go with before", "before")
		}
		if params["order"] == "closed" {
			return nil, badParam("ids does not go with order closed", "order")
		}
	}
	byClose, closeAt, closeID, err := closedOrder(params, state, before)
	if err != nil {
		return nil, err
	}
	limit, err := pageLimit(params)
	if err != nil {
		return nil, err
	}
	if ids != nil && params["limit"] == nil {
		limit = max(len(ids), 1)
	}
	withEnded, err := optBool(params, "all")
	if err != nil {
		return nil, err
	}
	everyone, err := optBool(params, "everyone")
	if err != nil {
		return nil, err
	}
	tags, err := optTags(params, "tags")
	if err != nil {
		return nil, err
	}
	turn, err := optBool(params, "my_turn") // the open tasks the caller acts on that wait on it: queued or doing, not draft or answered
	if err != nil {
		return nil, err
	}
	if turn {
		if acting != nil {
			return nil, badParam("my_turn is the tasks the caller acts on: it takes no acting", "acting")
		}
		me := c.Caller.Actor()
		acting = &me
	}
	var owners map[string]bool
	if !everyone && c.Caller.Role == "agent" {
		if owners, err = e.reach(c.Caller.Key); err != nil {
			return nil, err
		}
	}
	for _, name := range []*string{owner, acting, closer} {
		if name != nil && owners != nil && !owners[*name] {
			return nil, rpc.Refused(fmt.Sprintf("%s is not you or one of your reports: everyone shows every team's tasks", *name), "", "")
		}
	}
	// The filter is SQL, so a board of thousands is not loaded to show a few. Only the state's last step (draft, queued or doing) is
	// the board's, read for the rows the SQL kept.
	where, args := []string{"1 = 1"}, []any{}
	cond := func(sql string, a ...any) {
		where = append(where, sql)
		args = append(args, a...)
	}
	if !(withEnded || ids != nil || under != nil || request != nil || (state != nil && (*state == "done" || *state == "discarded" || *state == "closed"))) {
		cond(openTask)
	}
	if state != nil {
		switch *state {
		case "discarded":
			cond("COALESCE(discarded_by, '') != ''")
		case "done":
			cond("COALESCE(discarded_by, '') = '' AND COALESCE(done_evidence, '') != ''")
		case "closed":
			cond("(COALESCE(discarded_by, '') != '' OR COALESCE(done_evidence, '') != '')")
		}
	}
	for _, f := range []struct {
		col string
		v   *string
	}{{"owner", owner}, {db.ActingSQL("tasks"), acting}, {"opened_by", opener}, {askerSQL, closer}} {
		if f.v != nil {
			cond(f.col+" = ?", *f.v)
		}
	}
	if owners != nil { // a task is in scope by its assignee or its owner
		names := make([]any, 0, len(owners))
		for n := range owners {
			names = append(names, n)
		}
		in := "(" + placeholders(len(names)) + ")"
		cond("(owner IN "+in+" OR "+askerSQL+" IN "+in+")", append(names, names...)...)
	}
	if parent != nil {
		cond("parent_task = ?", *parent)
	}
	if under != nil {
		cond("id IN (WITH RECURSIVE story(id) AS (SELECT ? UNION SELECT k.id FROM tasks k JOIN story ON k.parent_task = story.id) SELECT id FROM story)", *under)
	}
	if request != nil {
		cond("id IN ("+requestTreeSQL+")", *request, *request)
	}
	if ids != nil {
		cond("id IN ("+placeholders(len(ids))+")", ids...)
	}
	if before != nil {
		cond("id < ?", *before)
	}
	if byClose && closeID != 0 {
		cond("("+closedAtSQL+" < ? OR ("+closedAtSQL+" = ? AND id < ?))", closeAt, closeAt, closeID)
	}
	for _, tag := range tags {
		cond("EXISTS (SELECT 1 FROM task_tags g WHERE g.task_id = tasks.id AND g.tag = ?)", tag)
	}
	page := ""
	sqlDecides := state == nil && !turn // my_turn leaves out drafts, which only the board knows
	if state != nil {
		sqlDecides = *state == "done" || *state == "discarded" || *state == "closed"
	}
	if state != nil && (*state == "open" || *state == "draft") {
		limit = math.MaxInt32 // every row: a board's open and draft tasks are not paged
	}
	if sqlDecides { // SQL alone decides: it pages too
		page = fmt.Sprintf(" LIMIT %d", limit+1)
	}
	order := "id DESC"
	if byClose {
		order = closedAtSQL + " DESC, id DESC"
	}
	shown, err := query(e.DB, "SELECT *, "+taskActing+", "+closedAtSQL+" AS closed_at FROM tasks WHERE "+strings.Join(where, " AND ")+" ORDER BY "+order+page, args...)
	if err != nil {
		return nil, err
	}
	b, err := e.loadBoard()
	if err != nil {
		return nil, err
	}
	type item struct {
		t     row
		state string
	}
	var rows []item
	for _, t := range shown {
		if st := b.state(t); (state == nil || stateIs(*state, st)) && (!turn || st == "queued" || st == "doing") {
			rows = append(rows, item{t, st})
		}
	}
	var listed []int64
	for _, r := range rows[:min(limit, len(rows))] {
		listed = append(listed, r.t.i("id"))
	}
	spent, err := e.usageEnv().TaskSpend(e.DB, listed)
	if err != nil {
		return nil, err
	}
	items := []any{}
	var last row
	for i, r := range rows {
		if i >= limit {
			break
		}
		task, err := e.taskOf(r.t, r.state, b)
		if err != nil {
			return nil, err
		}
		if s := spent[r.t.i("id")]; s != nil {
			task["spent_usd"] = s.USD
		}
		if c.Caller.Role == "agent" {
			noUnread(task)
		}
		items = append(items, task)
		last = r.t
	}
	out := map[string]any{"items": items, "more": len(rows) > limit}
	if byClose && len(rows) > limit {
		out["next"] = closedCursor(last.i("closed_at"), last.i("id"))
	}
	return out, nil
}

// stateIs is whether a task in state st is in the state a list asks for: a state of its own, or open (queued or doing) or closed (done or
// discarded).
func stateIs(want, st string) bool {
	switch want {
	case "open":
		return st == "queued" || st == "doing"
	case "closed":
		return st == "done" || st == "discarded"
	}
	return st == want
}

// optTaskIDs is a list of at most maxLimit task numbers under key, nil when absent.
func optTaskIDs(params map[string]any, key string) ([]any, error) {
	v, has := params[key]
	if !has || v == nil {
		return nil, nil
	}
	items, ok := v.([]any)
	if !ok || len(items) > maxLimit {
		return nil, badParam(fmt.Sprintf("%s is a list of at most %d task numbers", key, maxLimit), key)
	}
	out := make([]any, len(items))
	for i, it := range items {
		n, ok := rpc.AsInt(it)
		if !ok {
			return nil, badParam(key+" is a list of task numbers", key)
		}
		out[i] = n
	}
	return out, nil
}

// closedAtSQL is when a task was closed: the time it was discarded, else the time it was done.
const closedAtSQL = "(CASE WHEN COALESCE(discarded_by, '') != '' THEN COALESCE(discarded_at, 0) ELSE COALESCE(done_at, 0) END)"

func closedCursor(at, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%d", at, id)))
}

// closedOrder reads order and cursor: whether the list goes by close time, and where the cursor says to go on from (id 0 is the start).
func closedOrder(params map[string]any, state *string, before *int64) (byClose bool, at, id int64, err error) {
	order, err := optStr(params, "order")
	if err != nil {
		return false, 0, 0, err
	}
	cursor, err := optStr(params, "cursor")
	if err != nil {
		return false, 0, 0, err
	}
	if order != nil && *order != "id" && *order != "closed" {
		return false, 0, 0, badParam("order is id or closed", "order")
	}
	byClose = order != nil && *order == "closed"
	switch {
	case byClose && (state == nil || (*state != "done" && *state != "discarded" && *state != "closed")):
		return false, 0, 0, badParam("order closed needs state done, discarded or closed", "order")
	case !byClose && state != nil && *state == "closed":
		return false, 0, 0, badParam("state closed needs order closed", "order")
	case byClose && before != nil:
		return false, 0, 0, badParam("order closed pages by cursor, not before", "before")
	case !byClose && cursor != nil:
		return false, 0, 0, badParam("cursor goes with order closed", "cursor")
	case cursor == nil:
		return byClose, 0, 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*cursor)
	if err == nil {
		_, err = fmt.Sscanf(string(raw), "%d:%d", &at, &id)
	}
	if err != nil || id <= 0 {
		return false, 0, 0, badParam("cursor is not one this daemon gave", "cursor")
	}
	return true, at, id, nil
}

func (e *Env) firstRequest(tid int64) (row, error) {
	return queryOne(e.DB, "SELECT r.* FROM task_requests l JOIN requests r ON r.id = l.request_id "+
		"WHERE l.task_id = ? ORDER BY l.rowid LIMIT 1", tid)
}

// askerSQL is askerOf as a column of a tasks row. The + keeps the planner off requests_requester, which would read every request
// of the founder's for each task.
const askerSQL = "COALESCE(NULLIF(reporter, ''), CASE WHEN EXISTS (SELECT 1 FROM task_requests l JOIN requests r ON r.id = l.request_id " +
	"WHERE l.task_id = tasks.id AND +r.requester = '" + actor.OwnerKey + "') THEN '" + actor.OwnerKey + "' ELSE opened_by END)"

// askerOf is actions.task_asker: who a task answers to when it closes. Its reporter when one was set; else the founder, if any request
// linked to it came from the founder; else whoever opened it.
func (e *Env) askerOf(t row) (string, error) {
	if r := t.str("reporter"); r != "" {
		return r, nil
	}
	r, err := queryOne(e.DB, "SELECT 1 AS yes FROM task_requests l JOIN requests r ON r.id = l.request_id "+
		"WHERE l.task_id = ? AND +r.requester = ? LIMIT 1", t.i("id"), actor.OwnerKey) // + as in askerSQL
	if err != nil {
		return "", err
	}
	if r != nil {
		return actor.OwnerKey, nil
	}
	return t.str("opened_by"), nil
}

func (e *Env) blockers(tid int64) ([]row, error) {
	return query(e.DB, "SELECT t.*, "+taskActingT+" FROM tasks t JOIN task_blocks b ON b.blocks_on = t.id "+
		"WHERE b.task_id = ? AND b.dormant = 0 AND t.discarded_by IS NULL AND t.done_evidence IS NULL ORDER BY t.id", tid)
}

// taskOf is the spec's Task: the row as the board shows it, without its body.
func (e *Env) taskOf(t row, state string, b *board) (map[string]any, error) {
	waits, err := e.waitsOf(t.i("id"))
	if err != nil {
		return nil, err
	}
	rec, err := e.holdRec(t.i("id"))
	if err != nil {
		return nil, err
	}
	hold := waitLine(rec, state, b.busy(t))
	clock, err := e.taskClock(t, state)
	if err != nil {
		return nil, err
	}
	asker, err := e.askerOf(t)
	if err != nil {
		return nil, err
	}
	reqs, err := query(e.DB, "SELECT r.*, MAX(COALESCE(r.ts, 0), COALESCE(r.state_at, 0)) AS touched "+
		"FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? ORDER BY r.id", t.i("id"))
	if err != nil {
		return nil, err
	}
	blockedBy, waitList := openWaits(waits), make([]any, len(waits))
	for i, w := range waits {
		waitList[i] = map[string]any{"task": w.i("task"), "open": w.i("open") == 1}
	}
	waiting, waitSince, err := e.waitingOf(t, state, waits, rec)
	if err != nil {
		return nil, err
	}
	st, err := e.statusOf(t, state, waiting, waitSince)
	if err != nil {
		return nil, err
	}
	turns, err := e.turnsOf(t.i("id"), false)
	if err != nil {
		return nil, err
	}
	task := map[string]any{
		"blocked_by": blockedBy, "waits": waitList, "waiting": mapOrNil(waiting), "time": clock, "status": st.Map(),
		"asker": asker, "id": t.i("id"), "from": t.str("opened_by"), "to": t.str("owner"), "acting": actingOf(t), "title": t.str("title"), "state": state, "time_box": nil,
		"parent": t.intOrNil("parent_task"), "request": t.intOrNil("request"), "role": b.teamOf(e.Home).TaskRole(t.str("owner")), "created": ts(t["created_at"]), "delivered": ts(t["delivered_at"]),
		"turns": turns, "done": ts(t["done_at"]), "priority": t.i("priority"), "hold_reason": hold, "block_reason": blockReason(blockedBy), "evidence": t.strOrNil("done_evidence"), "spent_usd": nil,
	}
	active := max(t.i("created_at"), t.i("edited_at"), t.i("done_at"), t.i("discarded_at"))
	linked := make([]map[string]any, len(reqs))
	for i, r := range reqs {
		active = max(active, r.i("touched"))
		linked[i] = map[string]any{"id": r.i("id"), "ref": r.str("ref"), "name": db.MsgNameOf(e.DB, r.str("ref")), "from": r.str("requester"), "to": r.str("target"),
			"state": r.str("state"), "at": ts(r["ts"])}
	}
	tagRows, err := query(e.DB, "SELECT tag FROM task_tags WHERE task_id = ? ORDER BY tag", t.i("id"))
	if err != nil {
		return nil, err
	}
	tags := make([]string, len(tagRows))
	for i, r := range tagRows {
		tags[i] = r.str("tag")
	}
	task["last_activity"], task["edited_by"], task["requests"], task["tags"] = ts(active), t.strOrNil("edited_by"), linked, tags
	u, err := e.unreadOf(t.i("id"))
	if err != nil {
		return nil, err
	}
	withUnread(task, u)
	task["discarded"] = nil
	task["pr"] = t.intOrNil("pr")
	task["watch"] = nil
	if t.set("watch_vendor") {
		task["watch"] = map[string]any{"vendor": t.str("watch_vendor"), "target": t.str("watch_target"), "until": t.str("watch_until"),
			"label": t.str("watch_label"), "result": t.strOrNil("watch_result")}
	}
	if t.str("discarded_by") != "" {
		d := map[string]any{"by": t.str("discarded_by"), "reason": t.str("discard_reason"), "at": ts(t["discarded_at"]), "withdrew": []any{}, "batch": nil}
		ev, err := queryOne(e.DB, "SELECT payload FROM events WHERE kind = 'discard_task' AND "+eventTask+" = ? ORDER BY id DESC LIMIT 1", t.i("id"))
		if err != nil {
			return nil, err
		}
		p := decodeObject(ev.str("payload"))
		if w, ok := p["withdrew"].([]any); ok {
			d["withdrew"] = w
		}
		d["batch"] = p["batch"]
		task["discarded"] = d
	}
	task["mismatch"] = taskMismatch(t, state, reqs)
	if t.i("timeout_s") != 0 {
		task["time_box"] = duration(t.i("timeout_s"))
	}
	return task, nil
}

// mapOrNil is m, or an untyped nil for a nil map, which would not read as nil.
func mapOrNil(m map[string]any) any {
	if m == nil {
		return nil
	}
	return m
}

// waitsOf is the tasks tid waits on through a live block, open or not.
func (e *Env) waitsOf(tid int64) ([]row, error) {
	return query(e.DB, "SELECT b.blocks_on AS task, b.created_at, t.discarded_by IS NULL AND t.done_evidence IS NULL AS open "+
		"FROM task_blocks b JOIN tasks t ON t.id = b.blocks_on WHERE b.task_id = ? AND b.dormant = 0 ORDER BY b.created_at, b.blocks_on", tid)
}

// openWaits is the open tasks of waits, in number order.
func openWaits(waits []row) []int64 {
	on := []int64{}
	for _, w := range waits {
		if w.i("open") == 1 {
			on = append(on, w.i("task"))
		}
	}
	slices.Sort(on)
	return on
}

// blockReason is the block_reason of a task blockedBy holds, nil when nothing does.
func blockReason(blockedBy []int64) any {
	if len(blockedBy) == 0 {
		return nil
	}
	return "waiting on " + tids(blockedBy)
}

// lookOf is what the web's look() reads of a task another one lists (a sub-task, a blocker), so the list draws each row's state without
// reading that task: the fields taskOf gives them, from the same reads.
func (e *Env) lookOf(t row, state string, b *board) (map[string]any, error) {
	waits, err := e.waitsOf(t.i("id"))
	if err != nil {
		return nil, err
	}
	rec, err := e.holdRec(t.i("id"))
	if err != nil {
		return nil, err
	}
	waiting, _, err := e.waitingOf(t, state, waits, rec)
	if err != nil {
		return nil, err
	}
	return map[string]any{"state": state, "waiting": mapOrNil(waiting), "hold_reason": waitLine(rec, state, b.busy(t)), "block_reason": blockReason(openWaits(waits)),
		"evidence": t.strOrNil("done_evidence")}, nil
}

func tids(ids []int64) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = fmt.Sprintf("T%d", id)
	}
	return strings.Join(out, ", ")
}

// waitingOf is the spec's Task.waiting for an open task: behind its open blockers (a child blocks its parent), else on an open request to
// the founder, else the cause the supervisor's sweep stored with its reason, and since when, unix seconds. nil when nothing waits.
func (e *Env) waitingOf(t row, state string, waits []row, rec map[string]any) (map[string]any, int64, error) {
	if state != "queued" && state != "doing" {
		return nil, 0, nil
	}
	var on []int64
	var since int64
	for _, w := range waits {
		if w.i("open") == 1 {
			on = append(on, w.i("task"))
			if since == 0 || w.i("created_at") < since {
				since = w.i("created_at")
			}
		}
	}
	if len(on) > 0 {
		sort.Slice(on, func(i, j int) bool { return on[i] < on[j] })
		return map[string]any{"on": "task", "text": "blocked by " + tids(on), "tasks": on, "since": ts(since)}, since, nil
	}
	asked, err := queryOne(e.DB, "SELECT MIN(r.ts) AS since FROM requests r JOIN task_requests l ON l.request_id = r.id "+
		"WHERE l.task_id = ? AND r.target = ? AND "+notBoard+" AND "+founderOpen, t.i("id"), actor.OwnerKey)
	if err != nil {
		return nil, 0, err
	}
	if asked.set("since") {
		return map[string]any{"on": "founder", "text": "needs you", "tasks": []int64{}, "since": ts(asked.i("since"))}, asked.i("since"), nil
	}
	if len(rec) == 0 {
		return nil, 0, nil
	}
	cause, since := holdCause(rec)
	return map[string]any{"on": cause, "text": pyStr(rec["why"]), "tasks": []int64{}, "since": ts(since)}, since, nil
}

// holdCause is a hold record's cause and since when, unix seconds.
func holdCause(rec map[string]any) (string, int64) {
	cause, _ := rec["on"].(string)
	if cause == "" {
		cause = "owner" // kept before the sweep stored a cause
	}
	since, _ := recInt(rec["since"])
	return cause, since
}

// recInt is a whole number of a decoded record.
func recInt(v any) (int64, bool) {
	n, ok := v.(interface{ Int64() (int64, error) })
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	return i, err == nil
}

// taskMismatch is where a task's state and its delivery disagree, or nil, as actions.task_mismatch has it.
func taskMismatch(t row, state string, reqs []row) any {
	if state == "done" || state == "discarded" {
		return nil
	}
	sent := t.set("sent_at")
	if t.str("queued_by") == "" && sent {
		return "draft with work out - its opener has not queued it"
	}
	if state == "queued" && !sent && !actor.IsPerson(actingOf(t)) {
		return "approved, but no request is out"
	}
	return nil
}

// holdRec is the record the supervisor's sweep keeps of why a waiting task waits (supervisor/holds.go); empty when there is none.
func (e *Env) holdRec(tid int64) (map[string]any, error) {
	m, err := queryOne(e.DB, "SELECT value FROM meta WHERE name = ?", fmt.Sprintf("taskhold:%d", tid))
	if err != nil {
		return nil, err
	}
	return decodeObject(m.str("value")), nil
}

// waitLine is what shows beside a waiting task: the reason the sweep recorded for it and how long it has stood. A queued task nothing has
// recorded a reason for names the other task its acting agent is on, or says it is not looked at yet when there is none.
func waitLine(rec map[string]any, state, busy string) any {
	if state != "queued" && state != "doing" {
		return nil
	}
	if len(rec) == 0 {
		switch {
		case state != "queued":
			return nil
		case busy != "":
			return "waiting: " + busy + "; starts when it finishes"
		}
		return "waiting: the daemon has not looked at it yet - it does on its next tick"
	}
	since, ok := recInt(rec["since"])
	if !ok {
		since = time.Now().Unix()
	}
	secs := max(time.Now().Unix()-since, 0)
	return fmt.Sprintf("waiting: %s (for %s)", pyStr(rec["why"]), ago(secs))
}

// ago is a duration in the largest unit that still says something, as infra/timefmt.ago has it.
func ago(secs int64) string {
	switch {
	case secs < 120:
		return fmt.Sprintf("%ds", secs)
	case secs < 5400:
		return fmt.Sprintf("%dm", secs/60)
	case secs < 86400:
		return fmt.Sprintf("%dh", secs/3600)
	}
	return span(secs)
}

// span is a long time as days and hours, as infra/timefmt.span has it from a day up.
func span(secs int64) string { return fmt.Sprintf("%dd%dh", secs/86400, secs%86400/3600) }

// Tasks is the spec's Task of each of the tasks by id, as the board shows them now, without their bodies (the shape of tasks.list and
// task.get); a task that is gone is left out. The board is read once for all of them, which is what a change event carries. Its
// spent_usd is null: the calls it sums change every tick, and that is no change to the task.
func (e *Env) Tasks(ids []int64) (map[int64]map[string]any, error) {
	out := map[int64]map[string]any{}
	if len(ids) == 0 {
		return out, nil
	}
	b, err := e.loadBoard()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		t, err := queryOne(e.DB, "SELECT *, "+taskActing+" FROM tasks WHERE id = ?", id)
		if err != nil {
			return nil, err
		}
		if t == nil {
			continue
		}
		if out[id], err = e.taskOf(t, b.state(t), b); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (e *Env) taskGet(c *rpc.Conn, params map[string]any) (any, error) {
	tid, err := optInt(params, "task")
	if err != nil || tid == nil {
		return nil, badParam("task is a task number", "task")
	}
	t, err := queryOne(e.DB, "SELECT *, "+taskActing+" FROM tasks WHERE id = ?", *tid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		nf := rpc.NotFoundError(fmt.Sprintf("no task T%d", *tid))
		nf.Data = map[string]any{"what": "task", "id": *tid}
		return nil, nf
	}
	b, err := e.loadBoard()
	if err != nil {
		return nil, err
	}
	task, err := e.taskOf(t, b.state(t), b)
	if err != nil {
		return nil, err
	}
	first, err := e.firstRequest(*tid)
	if err != nil {
		return nil, err
	}
	task["body"] = first.str("body")
	if task["turns"], err = e.turnsOf(*tid, true); err != nil {
		return nil, err
	}
	images, agent := attachments.New(e.Home), c != nil && c.Caller.Role == "agent"
	if agent {
		noUnread(task)
	}
	parts := func(text string) []any { return images.Parts(text, agent) }
	if p := parts(first.str("body")); p != nil {
		task["body_parts"] = p
	}
	if err := e.taskResultOf(*tid, task); err != nil {
		return nil, err
	}
	spent, err := e.usageEnv().TaskSpend(e.DB, []int64{*tid})
	if err != nil {
		return nil, err
	}
	task["spend"] = nil
	if s := spent[*tid]; s != nil {
		task["spent_usd"] = s.USD
		task["spend"] = map[string]any{"usd": s.USD, "minutes": s.Minutes, "by_agent": s.ByAgent, "by_model": s.ByModel}
	}
	reqs, err := query(e.DB, "SELECT r.* FROM requests r JOIN task_requests l ON l.request_id = r.id WHERE l.task_id = ? ORDER BY r.id", *tid)
	if err != nil {
		return nil, err
	}
	deliveries := []any{}
	for _, r := range reqs {
		at := r["delivered_at"]
		if r.i("delivered_at") == 0 {
			at = r["ts"]
		}
		how := r.str("kind")
		if how == "" {
			how = "request"
		}
		deliveries = append(deliveries, map[string]any{"at": ts(at), "how": how, "outcome": r.str("state")})
	}
	task["deliveries"] = deliveries
	events, err := query(e.DB, taskEventsSQL, *tid)
	if err != nil {
		return nil, err
	}
	evs := []any{}
	for _, ev := range events {
		kind := "agent"
		if actor.IsPerson(ev.str("agent")) {
			kind = "user"
		}
		evs = append(evs, map[string]any{
			"topic": "tasks", "type": ev.str("kind"), "at": ts(ev["ts"]),
			"actor": map[string]any{"kind": kind, "name": ev.str("agent")}, "agent": t.str("owner"), "title": nil,
			"target": map[string]any{"screen": "tasks", "id": fmt.Sprint(*tid)}, "data": decodeObject(ev.str("payload"))})
	}
	task["events"] = evs
	memos, err := query(e.DB, "SELECT * FROM task_memos WHERE task_id = ? ORDER BY id", *tid)
	if err != nil {
		return nil, err
	}
	ms := make([]any, len(memos))
	for i, m := range memos {
		if ms[i], err = memoJSON(e.DB, m, parts, images, agent); err != nil {
			return nil, err
		}
	}
	task["memos"] = ms
	if task["timeline"], err = e.taskTimeline(*tid, memos, reqs, parts); err != nil {
		return nil, err
	}
	blockers, err := e.blockers(*tid)
	if err != nil {
		return nil, err
	}
	bs := []any{}
	for _, bl := range blockers {
		item, err := e.lookOf(bl, b.state(bl), b)
		if err != nil {
			return nil, err
		}
		item["task"], item["owner"], item["title"] = bl.i("id"), bl.str("owner"), bl.str("title")
		item["reason"] = fmt.Sprintf("T%d (%s) %s", bl.i("id"), actingOf(bl), bl.str("title"))
		bs = append(bs, item)
	}
	task["blockers"] = bs
	kids, err := query(e.DB, "SELECT *, "+taskActing+" FROM tasks WHERE parent_task = ? ORDER BY id", *tid)
	if err != nil {
		return nil, err
	}
	cs, subs := []any{}, []any{}
	for _, k := range kids {
		cs = append(cs, k.i("id"))
		item, err := e.lookOf(k, b.state(k), b)
		if err != nil {
			return nil, err
		}
		item["id"], item["title"], item["owner"] = k.i("id"), k.str("title"), k.str("owner")
		subs = append(subs, item)
	}
	task["children"], task["subtasks"] = cs, subs
	task["merged_into"], task["parent_title"] = t.intOrNil("merged_into"), nil
	if t.set("parent_task") {
		if p, err := queryOne(e.DB, "SELECT title FROM tasks WHERE id = ?", t.i("parent_task")); err != nil {
			return nil, err
		} else if p != nil {
			task["parent_title"] = p.str("title")
		}
	}
	if task["people"], err = e.askable(t.str("owner")); err != nil {
		return nil, err
	}
	hands, err := query(e.DB, "SELECT ts, payload FROM events WHERE kind = 'task_handoff' AND "+eventTask+" = ? ORDER BY id", *tid)
	if err != nil {
		return nil, err
	}
	hs := []any{}
	for _, h := range hands {
		p := decodeObject(h.str("payload"))
		note := p["note"]
		if note == nil {
			note = ""
		}
		hs = append(hs, map[string]any{"at": ts(h["ts"]), "from": p["from"], "to": p["to"], "note": note})
	}
	task["handoffs"] = hs
	edits, err := query(e.DB, "SELECT ts, agent, payload FROM events WHERE kind = 'edit_task' AND "+eventTask+" = ? ORDER BY id", *tid)
	if err != nil {
		return nil, err
	}
	as := []any{} // every change of who works the task (the owner column) or who answers for it (the reporter), by an edit
	for _, ed := range edits {
		p := decodeObject(ed.str("payload"))
		for _, f := range [][2]string{{"owner", "assignee"}, {"reporter", "owner"}} {
			if c, ok := p[f[0]].(map[string]any); ok {
				as = append(as, map[string]any{"at": ts(ed["ts"]), "by": ed.str("agent"), "what": f[1], "from": c["old"], "to": c["new"]})
			}
		}
	}
	task["assignments"] = as
	return task, nil
}

// taskResultOf adds what the task's end kept (its commit and screenshots) and whether it was opened as a ui task.
func (e *Env) taskResultOf(tid int64, task map[string]any) error {
	res, err := taskresult.Get(e.DB, tid)
	if err != nil {
		return err
	}
	if !res.Empty() {
		out := map[string]any{}
		if res.Commit != "" {
			out["commit"], out["on_main"] = res.Commit, res.OnMain
		}
		shots := make([]any, len(res.Shots))
		for i, s := range res.Shots {
			shots[i] = map[string]any{"n": s.N, "bytes": s.Bytes}
		}
		out["shots"] = shots
		task["result"] = out
	}
	ui, err := taskresult.IsUI(e.DB, tid)
	if err != nil {
		return err
	}
	if ui {
		task["ui"] = true
	}
	return nil
}

func memoJSON(q db.Q, m row, parts func(string) []any, store *attachments.Store, withPath bool) (map[string]any, error) {
	var edited any
	if m.set("edited_at") {
		edited = ts(m["edited_at"])
	}
	memo := map[string]any{"id": m.i("id"), "task": m.i("task_id"), "author": m.str("author"),
		"at": ts(m["created_at"]), "text": m.str("body"), "edited_at": edited, "reply_to": nil}
	if m.set("reply_to") {
		memo["reply_to"] = m.i("reply_to")
	}
	if p := parts(m.str("body")); p != nil {
		memo["parts"] = p
	}
	if actor.IsPerson(m.str("author")) {
		readers, err := memoReaders(q, m.i("id"))
		if err != nil {
			return nil, err
		}
		if len(readers) > 0 {
			memo["readers"] = readers
		}
	}
	infos, err := attachments.MemoFiles(q, m.i("id"))
	if err != nil {
		return nil, err
	}
	if len(infos) > 0 {
		files := make([]any, len(infos))
		for i, in := range infos {
			files[i] = store.FileMap(in, withPath)
		}
		memo["files"] = files
	}
	return memo, nil
}

// memoReaders is where the tell of memo mid stands for each reader it reached or may yet: read (with when), held for the reader's next digest
// (since when), or sent and not yet read (when it was sent). A tell typed at once counts as read when it landed in the agent's session;
// a held one when the agent opens it with hl read (MarkReadBy).
func memoReaders(q db.Q, mid int64) ([]any, error) {
	rows, err := query(q, "SELECT t.reader, t.request_id, r.read_at, r.delivered_at, r.typed_at, r.landed_at, r.send_now, r.notice "+
		"FROM memo_tells t JOIN requests r ON r.id = t.request_id WHERE t.memo_id = ? AND NOT "+db.NeverReached("r.")+" ORDER BY t.rowid", mid)
	if err != nil {
		return nil, err
	}
	var out []any
	for _, r := range rows {
		one := map[string]any{"agent": r.str("reader"), "request": r.i("request_id"), "at": nil}
		switch {
		case r.set("read_at"):
			one["state"], one["at"] = "read", ts(r["read_at"])
		case r.i("notice") == 2 && !r.set("delivered_at") && !r.set("send_now"):
			one["state"], one["at"] = "held", ts(r["landed_at"])
			// The digest goes a window after the agent's oldest waiting notice, which may be older than this one.
			var first sql.NullInt64
			if err := q.QueryRow("SELECT MIN(landed_at) FROM requests WHERE target = ? AND "+db.Held(""), r.str("reader")).Scan(&first); err != nil {
				return nil, err
			}
			if first.Valid {
				one["minutes"] = max(1, (first.Int64+settings.NoticeDigestS-time.Now().Unix()+59)/60)
			}
		case r.i("notice") != 2 && r.set("landed_at"): // a tell typed at once: its text is in the agent's session
			one["state"], one["at"] = "read", ts(r["landed_at"])
		default:
			one["state"] = "sent"
			for _, k := range []string{"delivered_at", "typed_at", "landed_at"} {
				if r.set(k) {
					one["at"] = ts(r[k])
					break
				}
			}
		}
		out = append(out, one)
	}
	return out, nil
}

// task.memo.list: a task's memos, oldest first. With limit, the newest limit memos, or the limit just older than before.
func (e *Env) taskMemoList(c *rpc.Conn, params map[string]any) (any, error) {
	tid, err := optInt(params, "task")
	if err != nil || tid == nil {
		return nil, badParam("task is a task number", "task")
	}
	before, err := rangedInt(params, "before", 1, 0)
	if err != nil {
		return nil, err
	}
	limit, err := rangedInt(params, "limit", 1, maxLimit)
	if err != nil {
		return nil, err
	}
	if before != nil && limit == nil {
		return nil, badParam("before needs limit", "limit")
	}
	t, err := queryOne(e.DB, "SELECT id FROM tasks WHERE id = ?", *tid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		nf := rpc.NotFoundError(fmt.Sprintf("no task T%d", *tid))
		nf.Data = map[string]any{"what": "task", "id": *tid}
		return nil, nf
	}
	var b, n any
	if before != nil {
		b = *before
	}
	n = -1
	if limit != nil {
		n = *limit + 1
	}
	rows, err := query(e.DB, "SELECT * FROM (SELECT * FROM task_memos WHERE task_id = ? AND (? IS NULL OR id < ?) ORDER BY id DESC LIMIT ?) ORDER BY id",
		*tid, b, b, n)
	if err != nil {
		return nil, err
	}
	more := limit != nil && int64(len(rows)) > *limit
	if more {
		rows = rows[1:]
	}
	images, agent := attachments.New(e.Home), c != nil && c.Caller.Role == "agent"
	parts := func(text string) []any { return images.Parts(text, agent) }
	memos := make([]any, len(rows))
	for i, m := range rows {
		if memos[i], err = memoJSON(e.DB, m, parts, images, agent); err != nil {
			return nil, err
		}
	}
	return map[string]any{"memos": memos, "more": more}, nil
}

// taskActing and taskActingT are the acting column (db.ActingSQL) of a tasks row read whole, unaliased or as t.
var (
	taskActing  = actingSQL + " AS acting"
	taskActingT = db.ActingSQL("t") + " AS acting"
	actingSQL   = db.ActingSQL("tasks") // who acts on a tasks row, as an expression
	actingSQLc  = db.ActingSQL("c")
)

// actingOf is who acts on task row t now: its acting column, else its owner for a row read without one.
func actingOf(t row) string {
	if a := t.str("acting"); a != "" {
		return a
	}
	return t.str("owner")
}

// turnsOf is Task.turns: the turns on task tid, bottom first. Each says when its ask was last rewritten (edited); with history, also the
// texts it replaced (earlier), which only task.get carries.
func (e *Env) turnsOf(tid int64, history bool) ([]any, error) {
	turns, err := db.Turns(e.DB, tid)
	if err != nil || len(turns) == 0 {
		return []any{}, err
	}
	// a turn's asks are its push, then each rewrite, in event order: the events since that agent's last push on the task
	evs, err := query(e.DB, "SELECT id, ts, kind, payload FROM events WHERE kind IN ('task_turn_push', 'task_turn_ask') AND "+eventTask+" = ? ORDER BY id", tid)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(turns))
	for i, u := range turns {
		var onPass, evidence, edited any
		if u.OnPass != "" {
			onPass, evidence = u.OnPass, u.Evidence
		}
		earlier, at := []any{}, ts(u.Since)
		for _, ev := range evs {
			p := decodeObject(ev.str("payload"))
			switch {
			case ev.str("kind") == "task_turn_push" && p["to"] == u.Agent:
				earlier, at, edited = []any{}, ts(ev["ts"]), nil
			case ev.str("kind") == "task_turn_ask" && p["agent"] == u.Agent:
				earlier = append(earlier, map[string]any{"ask": p["from"], "at": at})
				at, edited = ts(ev["ts"]), ts(ev["ts"])
			}
		}
		var memo any
		if u.Memo != 0 {
			memo = u.Memo
		}
		turn := map[string]any{"agent": u.Agent, "ask": u.Ask, "by": u.By, "since": ts(u.Since), "on_pass": onPass, "evidence": evidence, "edited": edited,
			"memo": memo}
		if history {
			turn["earlier"] = earlier
		}
		out[i] = turn
	}
	return out, nil
}
