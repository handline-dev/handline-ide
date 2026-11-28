package tick

import (
	"database/sql"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

// The cost pilot spends fewer cache tokens on an idle agent; each half is off until the agent's own setting turns it on. Renewal
// (cache_renewal) asks an idle session with an hour-long cache for its note just before the cache lapses, at most MaxRenewals per idle
// stretch and never past RenewCtxMax tokens; it replaces the 90% ask. Fold (idle_fold) closes fresh an idle session that holds no task,
// has nothing queued and a context of idle_fold_tokens or more, after idle_fold_after_s of silence; the model fold is not run here.
// Every renewal and fold is an event with the figures it was decided on, and decisions are levels read back from the record.
const (
	RenewalKey = "cache_renewal"
	FoldKey    = states.FoldKind
	// FreshKey (fresh_after_task: true) starts a new session between unrelated tasks (FreshDue): the context handoff, before the next
	// task is delivered. The agent's own setting, else config.yaml's; off by default.
	FreshKey = states.FreshAfterTaskKey
	// FreshKind is the kind a handoff for it carries: not a wall and not a full context, so it parks for no time and wakes at once.
	FreshKind = "task_done"
	// RenewLeadS: a renewal is asked in the last stretch of the cache's life, wide enough that one 30-second tick cannot step over it.
	RenewLeadS   = 150
	MaxRenewals  = 2
	RenewCtxMax  = 150_000
	FoldCtxMin   = 150_000
	FoldIdleS    = 600
	longTTLS     = 3600 // cachettl.LONG_TTL_S
	renewalsRows = 20
)

// CacheClock is when the prompt cache lapses, read from the transcript. TTL is nil when unknown; Left is nil when the clock cannot say.
type CacheClock struct {
	TTL  *int64
	Left func(now int64) *float64
}

// CostPorts are the foreign calls the cost pilot makes.
type CostPorts interface {
	// ReadCache is the cache clock of the status line's transcript; nil when unreadable.
	ReadCache(transcriptPath string) *CacheClock
}

// CostPilot reads an agent's own switches, and config.yaml's for FreshKey; absent means off.
type CostPilot struct{ Ports CostPorts }

func (c *CostPilot) own(t *Tick, agent, key string) any { return t.Config.AgentConfig(agent)[key] }

func (c *CostPilot) flag(t *Tick, agent, key string) bool {
	b, ok := c.own(t, agent, key).(bool)
	return ok && b
}

// RenewalOn is the agent's switch.
func (c *CostPilot) RenewalOn(t *Tick, agent string) bool { return c.flag(t, agent, RenewalKey) }

func (c *CostPilot) ownInt(t *Tick, agent, key string, def int64) int64 {
	switch v := c.own(t, agent, key).(type) {
	case int:
		if v > 0 {
			return int64(v)
		}
	case int64:
		if v > 0 {
			return v
		}
	}
	return def
}

// RenewalsInStretch is how many renewals this idle stretch has had. rows are the agent's UserPromptSubmit, SessionStart and cache_renewal
// events' kinds, newest first. The stretch reaches back to the last message someone typed or the last session start; a prompt straight
// after a renewal is that renewal's own turn and does not end it.
func RenewalsInStretch(kinds []string) int {
	n := 0
loop:
	for i, kind := range kinds {
		switch kind {
		case "cache_renewal":
			n++
		case "SessionStart":
			break loop
		case "UserPromptSubmit":
			older := ""
			if i+1 < len(kinds) {
				older = kinds[i+1]
			}
			if older != "cache_renewal" {
				break loop
			}
		}
	}
	return n
}

func (c *CostPilot) renewals(t *Tick, agent string) int {
	rows, err := t.Q.Query("SELECT kind FROM events WHERE agent = ? AND kind IN ('UserPromptSubmit', 'SessionStart', 'cache_renewal') ORDER BY id DESC LIMIT ?", agent, renewalsRows)
	check(err)
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var k string
		check(rows.Scan(&k))
		kinds = append(kinds, k)
	}
	check(rows.Err())
	return RenewalsInStretch(kinds)
}

func (c *CostPilot) openIdle(t *Tick, agent string, row *db.AgentState) bool {
	in := openIdleInput{HasRow: row != nil}
	if row != nil {
		in.Closed, in.State = closedAt(row), row.State
		in.AtLimit = t.Usage.AtLimit(t.Q, agent) != nil
		in.BackgroundWork = t.Delivery.HasBackgroundWork(t.Q, agent)
	}
	return openIdle(in)
}

// RenewalDue is whether this session is due a renewal now: the switch is on, it is open and idle, its cache is an hour's and in its last
// stretch, its context is known and not over RenewCtxMax, no ask of its is still waiting to be taken, and the stretch has had fewer than
// MaxRenewals.
func (c *CostPilot) RenewalDue(t *Tick, agent string, row *db.AgentState, now int64, st Status) bool {
	if !c.RenewalOn(t, agent) || !t.Handoff.NoteEnabled(agent) || !c.openIdle(t, agent, row) {
		return false
	}
	if st.TrustedCtxUsed == nil || *st.TrustedCtxUsed > RenewCtxMax {
		return false // unreadable is not small: fail closed
	}
	clock := c.Ports.ReadCache(st.TranscriptPath)
	if clock == nil || clock.TTL == nil || *clock.TTL < longTTLS {
		return false
	}
	left := clock.Left(now)
	if left == nil || !(float64(t.Handoff.AskMarginS()) <= *left && *left < RenewLeadS) {
		return false
	}
	if turns, ok := t.Handoff.NoteAsked(t.Q, agent); ok && turns == 0 {
		return false // asked, and the turn has not begun
	}
	return c.renewals(t, agent) < MaxRenewals
}

// Renew asks for the note if a renewal is due, and records it. True when the ask was sent.
func (c *CostPilot) Renew(t *Tick, agent string, row *db.AgentState, now int64, st Status) bool {
	if !c.RenewalDue(t, agent, row, now, st) {
		return false
	}
	count := c.renewals(t, agent) + 1
	if !t.Handoff.NoteAsk(t.Q, agent, "cache") {
		return false
	}
	var left any
	if clock := c.Ports.ReadCache(st.TranscriptPath); clock != nil {
		if l := clock.Left(now); l != nil {
			left = int64(*l)
		}
	}
	var ctx any
	if st.TrustedCtxUsed != nil {
		ctx = *st.TrustedCtxUsed
	}
	check(eventErr(db.AddEvent(t.Q, agent, RenewalKey, db.KV{K: "n", V: count}, db.KV{K: "of", V: MaxRenewals}, db.KV{K: "ctx", V: ctx}, db.KV{K: "cache_left_s", V: left})))
	t.logf("  %s: cache renewal %d of %d", agent, count, MaxRenewals)
	return true
}

func eventErr(_ int64, err error) error { return err }

// FoldDue is whether this session is due a fold now: the switch is on, it is open and idle past its fold wait, it holds no task and has
// nothing queued or waiting to be told, its context is known and at the fold size, there is a session to take the record from, and no
// close of it failed lately.
func (c *CostPilot) FoldDue(t *Tick, agent string, row *db.AgentState, now int64, st Status) bool {
	if !c.flag(t, agent, FoldKey) || !c.openIdle(t, agent, row) || st.SessionID == "" {
		return false
	}
	if st.TrustedCtxUsed == nil || int64(*st.TrustedCtxUsed) < c.ownInt(t, agent, "idle_fold_tokens", FoldCtxMin) {
		return false
	}
	if _, has := must2(db.CurrentTask(t.Q, agent)); has || len(must(db.QueuedFor(t.Q, agent))) > 0 {
		return false // not at a task boundary
	}
	if t.Lifecycle.WaitingFor(t.Q, agent) {
		return false
	}
	wait := c.ownInt(t, agent, "idle_fold_after_s", FoldIdleS)
	last, have := t.Delivery.IdleSince(t.Q, agent)
	// The note's own turn restarts the idle clock; it is the answer this fold was held for.
	if !t.Handoff.NoteAnsweredTurn(t.Q, agent) && (!have || now-last < wait) {
		return false
	}
	failed, ok := must2(db.LastEventTS(t.Q, agent, "close_failed"))
	return !ok || now-failed >= wait
}

// Fold closes the session fresh and records it. The caller has held the close for the note and checked FoldDue.
func (c *CostPilot) Fold(t *Tick, agent string, now int64, st Status) (bool, string) {
	_, hadNote := t.Handoff.NoteText(t.Q, agent)
	since, have := t.Delivery.IdleSince(t.Q, agent)
	if !have {
		since = now
	}
	idle := now - since
	if idle < 0 {
		idle = 0
	}
	ok, why := t.Lifecycle.CloseFresh(t.Q, agent, st)
	if ok {
		var ctx any
		ctxText := "None"
		if st.TrustedCtxUsed != nil {
			ctx = *st.TrustedCtxUsed
			ctxText = commas(*st.TrustedCtxUsed)
		}
		check(eventErr(db.AddEvent(t.Q, agent, FoldKey, db.KV{K: "ctx", V: ctx}, db.KV{K: "idle_s", V: idle}, db.KV{K: "note", V: hadNote})))
		noNote := ""
		if !hadNote {
			noNote = " (no note)"
		}
		t.logf("  %s: folded - idle %dm at %s tokens, closed fresh%s", agent, idle/60, ctxText, noNote)
	}
	return ok, why
}

// FreshDue is whether this agent is owed a fresh session before its next task: its switch is on (its own, else config.yaml's), its
// session is open, it closed a task (done, or handed on) since its last session began or was handed off, it has no task in progress, and
// the task at the head of its queue is unrelated to the one it closed. Related is the same task, the same parent, one the other's
// parent, or one this session already began. A task that only waits (blocked, or on another's turn) is not in progress. A level read
// from the record, never an edge: due on every tick until a handoff or a new session comes after the close; the delivery holds the head
// while it is due (FreshHeld), so the task goes to the new session.
func FreshDue(q db.Q, cfg Config, agent string, row *db.AgentState) bool {
	if row == nil || closedAt(row) || !freshOn(cfg, agent) {
		return false
	}
	var last sql.NullInt64 // the task it closed last
	var closed int64       // and when
	err := q.QueryRow("SELECT id, at FROM (SELECT id, done_at AS at FROM tasks WHERE owner = ? AND done_evidence IS NOT NULL AND done_at IS NOT NULL "+
		"UNION ALL SELECT json_extract(payload, '$.task_id'), ts FROM events "+
		"WHERE kind = 'task_handoff' AND json_valid(payload) AND json_extract(payload, '$.from') = ?) "+
		"ORDER BY at DESC LIMIT 1", agent, agent).Scan(&last, &closed)
	if err == sql.ErrNoRows {
		return false
	}
	check(err)
	var began sql.NullInt64 // the later of the last handoff and the last session start
	check(q.QueryRow("SELECT MAX(at) FROM (SELECT MAX(updated) AS at FROM handoff WHERE agent = ? "+
		"UNION ALL SELECT MAX(ts) FROM events WHERE agent = ? AND kind = 'SessionStart')", agent, agent).Scan(&began))
	if began.Valid && closed <= began.Int64 {
		return false
	}
	if inProgress(q, agent) {
		return false
	}
	head := must(db.EffectiveTaskHead(q, agent))
	if head == nil {
		return false // nothing to start fresh for yet: decided when the next task comes
	}
	var prev *db.Task
	if last.Valid {
		prev = must(db.GetTask(q, last.Int64))
	}
	for _, next := range must(db.TasksForRequest(q, head.ID)) {
		if next.Acting == agent && !related(prev, next) && !must(db.BegunSince(q, next.ID, began.Int64)) {
			return true
		}
	}
	return false
}

// FreshHeld is whether the delivery holds agent's next task for a fresh session: FreshDue, only while the tick can hand the agent off
// (applies is the tick's TriggersApply, and the agent is not stalled) and no context compact is due, which goes first. A failure is the
// error.
func FreshHeld(q db.Q, cfg Config, agent string, applies func(*db.AgentState) bool) (held bool, err error) {
	err = guard(func() {
		row := must(db.GetAgentState(q, agent))
		if row == nil || !applies(row) || row.State == states.Stalled || must(db.CompactDueSince(q, agent)) != 0 {
			return
		}
		held = FreshDue(q, cfg, agent, row)
	})
	return held, err
}

func freshOn(cfg Config, agent string) bool {
	if b, ok := cfg.AgentConfig(agent)[FreshKey].(bool); ok {
		return b
	}
	return cfg.Flag(FreshKey, false)
}

// inProgress: the agent's current task is open, its own to act on, and waits on nothing.
func inProgress(q db.Q, agent string) bool {
	tid, ok := must2(db.CurrentTask(q, agent))
	if !ok {
		return false
	}
	t := must(db.GetTask(q, tid))
	return t != nil && !t.Ended() && t.Acting == agent && !must(db.TaskWaits(q, tid))
}

func related(a, b *db.Task) bool {
	if a == nil {
		return false
	}
	pa, pb := a.ParentTask, b.ParentTask
	return a.ID == b.ID || pa.Valid && pb.Valid && pa.Int64 == pb.Int64 || pa.Valid && pa.Int64 == b.ID || pb.Valid && pb.Int64 == a.ID
}
