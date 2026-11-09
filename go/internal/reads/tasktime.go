package reads

import (
	"fmt"
	"sort"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
)

// The waiting reasons a segment names, in the words of docs/story1-research.md ("One picture of what waits and why"). They live here alone so
// the shared state and reason list can replace them in one place.
const (
	waitFounder = "needs you"
	waitTask    = "blocked by T%d"
	waitChild   = "waiting on T%d"
	waitAgent   = "waiting on %s"
	waitWall    = "wall"
	waitQueued  = "queued"
	startText   = "agent starting"
)

// A segment's kind, and its rank where two overlap: the higher one is what the stretch shows.
const (
	segWorking = iota
	segStarting
	segAccount
	segQueued
	segAgent
	segTask
	segFounder
)

var segKinds = map[int]struct{ kind, on string }{
	segWorking: {"working", ""}, segStarting: {"starting", ""}, segAccount: {"waiting", "account"}, segAgent: {"waiting", "agent"},
	segTask: {"waiting", "task"}, segFounder: {"waiting", "founder"}, segQueued: {"queued", ""},
}

type stretch struct {
	from, to int64
	rank     int
	text     string
	task     int64
	who      string // the agent acting on it, for working and starting
}

// taskTime is task.time: where a task's time went, from its opening to its end (or now), as segments that cover it with no gap or overlap.
func (e *Env) taskTime(c *rpc.Conn, params map[string]any) (any, error) {
	return atNow(params, e.taskTimeAt)
}

// atNow runs a read of the task the params name at the present moment.
func atNow(params map[string]any, read func(tid, now int64) (any, error)) (any, error) {
	tid, err := optInt(params, "task")
	if err != nil || tid == nil {
		return nil, badParam("task is a task number", "task")
	}
	return read(*tid, time.Now().Unix())
}

// taskRowSQL is the task row task.time and the expectation read.
const taskRowSQL = "SELECT id, owner, created_at, estimate, timeout_s, COALESCE(done_at, discarded_at, closed_at) AS ended FROM tasks WHERE id = ?"

func (e *Env) taskTimeAt(tid, now int64) (any, error) {
	t, err := queryOne(e.DB, taskRowSQL, tid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, noTask(tid)
	}
	start, end, segs, err := e.segments(t, now)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(segs))
	for i, s := range segs {
		k := segKinds[s.rank]
		seg := map[string]any{"kind": k.kind, "from": ts(s.from), "to": ts(s.to), "seconds": s.to - s.from, "text": s.text, "on": nil, "task": nil}
		if k.on != "" {
			seg["on"] = k.on
		}
		if s.task != 0 {
			seg["task"] = s.task
		}
		out[i] = seg
	}
	var endAt any
	if t.set("ended") {
		endAt = ts(end)
	}
	expected, err := (&expectation{e: e, now: now, worked: map[int64]int64{}}).of(t, segs)
	if err != nil {
		return nil, err
	}
	box, err := e.box(t, segs)
	if err != nil {
		return nil, err
	}
	return map[string]any{"task": tid, "start": ts(start), "end": endAt, "segments": out, "estimate": t.intOrNil("estimate"), "expected": expected,
		"box": box}, nil
}

// box is an open task's time box split as its clock ran since the box last started over (db.BoxStartedAt): each agent's working
// seconds, the waking, and what is left, adding up to the length.
func (e *Env) box(t row, segs []stretch) (any, error) {
	if t.set("ended") {
		return nil, nil
	}
	length := t.i("timeout_s")
	if !t.set("timeout_s") || length == 0 {
		length = db.DefaultTaskTimeoutS
	}
	since, err := db.BoxStartedAt(e.DB, t.i("id"))
	if err != nil {
		return nil, err
	}
	var first, waking, used int64
	worked := map[string]int64{}
	var order []string
	for _, s := range segs {
		from := max(s.from, since)
		if s.to <= from || s.who == "" || s.who == db.Human || (s.rank != segWorking && s.rank != segStarting) {
			continue
		}
		if first == 0 {
			first = from
		}
		used += s.to - from
		if s.rank == segStarting {
			waking += s.to - from
			continue
		}
		if _, ok := worked[s.who]; !ok {
			order = append(order, s.who)
		}
		worked[s.who] += s.to - from
	}
	sort.SliceStable(order, func(i, j int) bool { return worked[order[i]] > worked[order[j]] })
	parts := []map[string]any{}
	for _, who := range order {
		parts = append(parts, map[string]any{"agent": who, "seconds": worked[who]})
	}
	var began any
	if first != 0 {
		began = ts(first)
	}
	return map[string]any{"length_s": length, "start": began, "worked": parts, "waking_s": waking, "left_s": length - used}, nil
}

// segments is the task's time from its opening to its end (or now), covered with no gap or overlap.
func (e *Env) segments(t row, now int64) (start, end int64, segs []stretch, err error) {
	start, end = t.i("created_at"), now
	if t.set("ended") {
		end = t.i("ended")
	}
	spans, err := e.taskSpans(t, start, end)
	if err != nil {
		return 0, 0, nil, err
	}
	return start, end, cover(spans, start, end), nil
}

// taskSpans is every stretch the db records for the task, overlapping as they happened: its state (queued, doing, reported), its blockers
// and open children, its asks of the founder and turns on them, and its acting agent's parks and starts while it was doing.
func (e *Env) taskSpans(t row, start, end int64) ([]stretch, error) {
	tid := t.i("id")
	events, err := query(e.DB, "SELECT ts, agent, kind, payload FROM events WHERE "+eventTask+" = ? AND kind IN "+
		"('queue_task', 'draft_task', 'task_begin', 'task_start', 'task_report', 'accept_task', 'reject_task', 'reopen_task', 'release_task', "+
		"'task_handoff', 'edit_task', 'block_task', 'unblock_task', 'task_turn_push', 'task_turn_pop') ORDER BY id", tid)
	if err != nil {
		return nil, err
	}
	// A delivery that landed in its holder's session counts as the work begun, like a task_start; it is read as one more event in time order.
	events, err = withDeliveries(e, tid, events)
	if err != nil {
		return nil, err
	}
	var spans []stretch
	open := func(from int64, rank int, text string, task int64, who string) int {
		spans = append(spans, stretch{from: from, to: end, rank: rank, text: text, task: task, who: who})
		return len(spans) - 1
	}
	// The state: queued until the holder has it, doing until it is reported, waiting on whoever accepts a report. Doing is worked by the
	// agent acting on it: the top of its turns, else its owner.
	owner := t.str("owner")
	var turns []string
	var founderTurn = -1
	acting := func() string {
		if n := len(turns); n > 0 {
			return turns[n-1]
		}
		return owner
	}
	state := open(start, segQueued, waitQueued, 0, "")
	var doing []stretch
	blocks := map[int64]int{}
	for _, ev := range events {
		at, p := ev.i("ts"), decodeObject(ev.str("payload"))
		cut := func() {
			if spans[state].rank == segWorking {
				doing = append(doing, stretch{from: spans[state].from, to: at, who: spans[state].who})
			}
			spans[state].to = at
		}
		// work goes on under whoever acts now
		rework := func() {
			if spans[state].rank == segWorking && spans[state].who != acting() {
				cut()
				state = open(at, segWorking, "", 0, acting())
			}
			switch top := acting(); {
			case top == db.Human && founderTurn < 0:
				founderTurn = open(at, segFounder, waitFounder, 0, "")
			case top != db.Human && founderTurn >= 0:
				spans[founderTurn].to, founderTurn = at, -1
			}
		}
		switch ev.str("kind") {
		case "queue_task", "draft_task", "reopen_task", "reject_task", "release_task":
			cut()
			state = open(at, segQueued, waitQueued, 0, "")
		case "delivered":
			if spans[state].rank == segQueued && ev.str("target") == owner {
				cut()
				state = open(at, segWorking, "", 0, acting())
			}
		case "task_begin", "task_start":
			if spans[state].rank != segWorking {
				cut()
				state = open(at, segWorking, "", 0, acting())
			}
		case "task_report":
			cut()
			if by := text(p["to"]); by != "" && by != db.Human {
				state = open(at, segAgent, fmt.Sprintf(waitAgent, by), 0, "")
			} else {
				state = open(at, segFounder, waitFounder, 0, "")
			}
		case "accept_task":
			cut()
		case "task_handoff":
			if to := text(p["to"]); to != "" {
				owner = to
				rework()
			}
		case "edit_task":
			if o, ok := p["owner"].(map[string]any); ok {
				owner = text(o["new"])
				rework()
			}
		case "task_turn_push":
			turns = append(turns, text(p["to"]))
			rework()
		case "task_turn_pop":
			who := text(p["agent"])
			for i := len(turns) - 1; i >= 0; i-- {
				if turns[i] == who {
					turns = append(turns[:i], turns[i+1:]...)
					break
				}
			}
			rework()
		case "block_task":
			on, _ := recInt(p["blocks_on"])
			if i, ok := blocks[on]; ok {
				spans[i].to = at // blocked again on the same task: the new span carries on from here
			}
			blocks[on] = open(at, segTask, fmt.Sprintf(waitTask, on), on, "")
		case "unblock_task":
			on, _ := recInt(p["blocks_on"])
			if i, ok := blocks[on]; ok {
				spans[i].to = at
				delete(blocks, on)
			}
		}
	}
	if spans[state].rank == segWorking {
		doing = append(doing, stretch{from: spans[state].from, to: spans[state].to, who: spans[state].who})
	}
	// A blocker ends when it is done or discarded, if no unblock said so first.
	for on, i := range blocks {
		b, err := queryOne(e.DB, "SELECT COALESCE(done_at, discarded_at) AS ended FROM tasks WHERE id = ?", on)
		if err != nil {
			return nil, err
		}
		if b.set("ended") && b.i("ended") < spans[i].to {
			spans[i].to = b.i("ended")
		}
	}
	// A child waits the task from its opening to its end, as a blocker does.
	kids, err := query(e.DB, "SELECT id, created_at, COALESCE(done_at, discarded_at) AS ended FROM tasks WHERE parent_task = ?", tid)
	if err != nil {
		return nil, err
	}
	for _, k := range kids {
		i := open(k.i("created_at"), segTask, fmt.Sprintf(waitChild, k.i("id")), k.i("id"), "")
		if k.set("ended") && k.i("ended") < spans[i].to {
			spans[i].to = k.i("ended")
		}
	}
	asks, err := query(e.DB, "SELECT r.ts, r.state, r.state_at FROM requests r JOIN task_requests l ON l.request_id = r.id "+
		"WHERE l.task_id = ? AND r.target = 'human' AND COALESCE(r.kind, '') != 'say'", tid)
	if err != nil {
		return nil, err
	}
	for _, a := range asks {
		i := open(a.i("ts"), segFounder, waitFounder, 0, "")
		switch {
		case a.str("state") == "queued":
		case a.set("state_at"):
			spans[i].to = a.i("state_at")
		default: // closed with no time on record: it ended by the task's next event at the latest
			for _, ev := range events {
				if ev.i("ts") > a.i("ts") {
					spans[i].to = ev.i("ts")
					break
				}
			}
		}
	}
	// The acting agent's parks and starts count only while it acted on the task and was doing it.
	for _, d := range doing {
		if d.who == db.Human || d.who == "" {
			continue
		}
		more, err := e.holderSpans(d.who, d.from, d.to)
		if err != nil {
			return nil, err
		}
		spans = append(spans, more...)
	}
	return spans, nil
}

// WakeCapS is the longest a start reads as starting: a session that shows no sign of being up by then is counted as working.
const WakeCapS = 120

// holderSpans is agent's parks (until a wake, launch or handoff ends them) and starts (from the daemon's launch, which every wake and
// relaunch writes, until the session's first hook, at most WakeCapS) within [from, to].
func (e *Env) holderSpans(agent string, from, to int64) ([]stretch, error) {
	evs, err := query(e.DB, "SELECT ts, kind FROM events WHERE agent = ? AND kind IN ('park', 'wake', 'launching', 'handoff') AND ts <= ? ORDER BY id", agent, to)
	if err != nil {
		return nil, err
	}
	var out []stretch
	parked := -1
	for _, ev := range evs {
		at := ev.i("ts")
		if ev.str("kind") == "park" {
			out = append(out, stretch{from: at, to: to, rank: segAccount, text: waitWall, who: agent})
			parked = len(out) - 1
			continue
		}
		if parked >= 0 {
			out[parked].to = at
			parked = -1
		}
		if ev.str("kind") != "launching" || at < from {
			continue
		}
		up := at + WakeCapS
		if over, ok, err := db.FirstEventOfTS(e.DB, agent, db.LaunchOver, at); err != nil {
			return nil, err
		} else if ok {
			up = min(up, over)
		}
		out = append(out, stretch{from: at, to: up, rank: segStarting, text: startText, who: agent})
	}
	for i := range out {
		out[i].from, out[i].to = max(out[i].from, from), min(out[i].to, to)
	}
	return out, nil
}

// cover lays spans on [start, end]: each stretch shows the highest-ranked span over it, and touching stretches that read the same are one.
func cover(spans []stretch, start, end int64) []stretch {
	cuts := []int64{start, end}
	for _, s := range spans {
		if s.from < s.to {
			cuts = append(cuts, max(start, min(end, s.from)), max(start, min(end, s.to)))
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i] < cuts[j] })
	var out []stretch
	for i := 0; i+1 < len(cuts); i++ {
		a, b := cuts[i], cuts[i+1]
		if a == b {
			continue
		}
		top := stretch{rank: -1}
		for _, s := range spans {
			if s.from <= a && s.to >= b && s.from < s.to && s.rank > top.rank {
				top = s
			}
		}
		if top.rank < 0 {
			continue
		}
		if n := len(out); n > 0 && out[n-1].to == a && out[n-1].rank == top.rank && out[n-1].text == top.text && out[n-1].who == top.who {
			out[n-1].to = b
			continue
		}
		out = append(out, stretch{from: a, to: b, rank: top.rank, text: top.text, task: top.task, who: top.who})
	}
	return out
}

// noTask is the not_found of a task that is not there.
func noTask(tid int64) error {
	nf := rpc.NotFoundError(fmt.Sprintf("no task T%d", tid))
	nf.Data = map[string]any{"what": "task", "id": tid}
	return nf
}

// withDeliveries puts the task's landed deliveries among its events in time order, as kind "delivered" with the target in "agent" and "target".
func withDeliveries(e *Env, tid int64, events []row) ([]row, error) {
	landed, err := query(e.DB, "SELECT r.landed_at AS ts, r.target, r.target AS agent FROM requests r JOIN task_requests l ON l.request_id = r.id "+
		"WHERE l.task_id = ? AND "+db.HasLanded("r.")+" AND r.target != 'human' ORDER BY r.landed_at, r.id", tid)
	if err != nil {
		return nil, err
	}
	for _, d := range landed {
		d["kind"] = "delivered"
		i := sort.Search(len(events), func(i int) bool { return events[i].i("ts") > d.i("ts") })
		events = append(events, nil)
		copy(events[i+1:], events[i:])
		events[i] = d
	}
	return events, nil
}
