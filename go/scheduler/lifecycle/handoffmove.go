package lifecycle

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/timefmt"
	"github.com/handline/handline/go/scheduler/staymove"
)

// The triggers a move answers: the two walls, a person's order, a login that expired, a subscription that ended, and
// a pause.
const (
	LoginTrigger  = "login"
	EndedTrigger  = "ended"
	PausedTrigger = "paused"
	ClearWaitS    = 300
)

// MovableTrigger is whether a trigger of this kind may be answered by a move to another account: a usage wall, a person's order,
// a login that expired, a subscription that ended or a pause.
func MovableTrigger(kind string) bool {
	return kind == "usage" || kind == "usage_7d" || kind == states.EarlyUsageTrigger || kind == "forced" || kind == LoginTrigger ||
		kind == EndedTrigger || kind == PausedTrigger
}

// outTrigger is a move off an account that cannot be used: no stay-or-move weighing and no minimum interval between handoffs.
func outTrigger(kind string) bool {
	return kind == LoginTrigger || kind == EndedTrigger || kind == PausedTrigger
}

func usageTrigger(kind string) bool { return kind == "usage" || kind == "usage_7d" }

// wallTrigger is a usage wall or the early move ahead of one.
func wallTrigger(kind string) bool { return usageTrigger(kind) || kind == states.EarlyUsageTrigger }

// parkReason is why a handoff of kind parks its agent: by hand first, then a short retry is the move waiting, then a usage wall.
func parkReason(kind string, shortRetry bool) verdict.ParkReason {
	switch {
	case kind == states.ByHandMark:
		return verdict.ParkByHand
	case shortRetry:
		return verdict.ParkMoving
	case usageTrigger(kind):
		return verdict.ParkWall
	}
	return verdict.ParkNone
}

// HandoffTrigger is why a handoff happens: its kind and the sentence for it. NoteWritten is whether the agent's note was in when it began.
type HandoffTrigger struct {
	Kind, Reason string
	NoteWritten  bool
}

// Windows are an account's two windows as the move reads them.
type Windows struct {
	ShortReset, LongReset int64
	ShortS                float64 // the shortest window's length in seconds
}

// MoveEnv is what a handoff and a move read beyond the lifecycle: usage, the note, and the policy's pure functions.
type MoveEnv struct {
	// Reading is which account the agent's usage is landing on and how that is known ("recorded", "reported" or "assumed").
	Reading func(q db.Q, agent string) (account, source string, ok bool)
	Windows func(acct string) Windows
	// ColdStartPct is the median share of the short window a fresh session's first turn has cost lately, nil when too few were measured.
	ColdStartPct func(q db.Q) *float64
	// AtLimit is when the agent's last turn ended on a usage limit, ok false when it did not.
	AtLimit func(q db.Q, agent string) (at int64, ok bool)
	// NoteHold is whether the handoff note holds a move of this trigger kind: the ask is out and the note not yet in.
	NoteHold func(q db.Q, agent, kind string, now int64) bool
	// NoteWritten is whether the agent wrote its handoff note for this stretch; nil for never.
	NoteWritten func(agent string) bool
	// LimitDialogOpen is whether the usage-limit dialog is on the agent's screen: the turn behind it is over, whatever the session says.
	LimitDialogOpen func(agent string) bool
	// BackgroundWork is what the agent's session still runs behind its prompt.
	BackgroundWork func(q db.Q, agent string) Background
	// ContextUse is the context used and its limit, used < 0 when unknown.
	ContextUse    func(q db.Q, agent string) (used, limit int)
	MeasureForget func(q db.Q, agent string)
	// StayOrMove is staymove.Decide, and WakeAfter tick.WakeAfter: the policy's pure functions, passed in so a test can fix them.
	StayOrMove func(s staymove.Situation) (move bool, why string)
	WakeAfter  func(kind string, shortReset, longReset, now int64) int64
}

// Background is the work a session still runs behind its prompt: Words names each job as a person reads it, Lines as it was started.
// Words "" is none.
type Background struct{ Words, Lines string }

// BackgroundWaits begins a saving switch's waits when its move waits on the agent's background work.
const BackgroundWaits = "background: "

// MovedPastBackgroundKind is the event of a wall move that stopped waiting on background work: the jobs and how long it waited.
const MovedPastBackgroundKind = "move_past_background"

// Mover is the handoff, the move to another account and the clear that is owed. When each happens is its caller's; how is
// not: nothing is typed, a close is proven by the process group (a terminal's by its pane and the session's pid), and a cleared context is
// a closed session whose next start is fresh from the record.
type Mover struct {
	M *Manager
	E MoveEnv
}

func (v *Mover) log(f string, a ...any) { v.M.Log(f, a...) }

// recordHandoff takes the record and commits the step. False when a fact it holds could not be read.
func (v *Mover) recordHandoff(agent, hid string) bool {
	m := v.M
	missing := must(m.Handoff.Record(m.Q, agent, ""))
	if len(missing) == 0 {
		check(db.AdvanceHandoff(m.Q, hid, "recorded", "in_flight", ""))
		return true
	}
	why := "could not read " + strings.Join(missing, ", ")
	check(db.AdvanceHandoff(m.Q, hid, "recorded", "degraded", clip300(why)))
	check(db.BumpCounter(m.Q, agent, "throttle_count", 1))
	v.log("  %s: handoff degraded (%s)", agent, why)
	return false
}

// lastLeft is the account the agent's last move took it off, "" when it never moved.
func lastLeft(q db.Q, agent string) string {
	var payload string
	if err := q.QueryRow("SELECT payload FROM events WHERE agent = ? AND kind = 'moved_account' ORDER BY id DESC LIMIT 1", agent).Scan(&payload); err != nil {
		return ""
	}
	var p struct {
		From string `json:"from"`
	}
	if json.Unmarshal([]byte(payload), &p) != nil {
		return ""
	}
	return p.From
}

// MovePlan is an agent at the wall: stay, or move and where to. (destination, why). Stay when nothing waits for it (a wake, starting, is
// itself the work) or its own account resets before a cold start would pay back and within the prompt cache's life, past which a resume
// pays for its context in full anyway; otherwise the room test the closed-agent wake already uses (Launcher.RoomElsewhere over the other accounts),
// never back to the account it just left before that resets, so one wall is one move.
func (v *Mover) MovePlan(agent, kind string, starting bool) (dest, why string, err error) {
	defer catch(&err)
	dest, why, _ = v.plan(agent, kind, starting)
	return dest, why, nil
}

// plan is MovePlan; roomless says the policy wanted a move and no account has room for it.
func (v *Mover) plan(agent, kind string, starting bool) (dest, why string, roomless bool) {
	m := v.M
	here, _, ok := v.E.Reading(m.Q, agent)
	if !ok || here == "" {
		return "", "no account is recorded for it", false
	}
	now := m.now()
	w := v.E.Windows(here)
	reset := w.ShortReset
	if kind == "usage_7d" {
		reset = w.LongReset
	}
	var resetIn *float64
	if reset != 0 {
		d := float64(max(0, reset-now))
		resetIn = &d
	}
	if !outTrigger(kind) && m.Launch.ManualSplit(agent) {
		return "", "account_split is manual - it waits on " + here + " for its reset", false
	}
	work := starting || len(must(db.QueuedFor(m.Q, agent))) > 0
	if !outTrigger(kind) {
		s := staymove.Situation{WorkWaiting: work, ResetInS: resetIn, ColdStartPct: v.E.ColdStartPct(m.Q), WindowS: w.ShortS,
			CacheLifeS: states.PromptCacheS}
		if end, ok := must2(db.LastEventOfTS(m.Q, agent, []string{"Stop", "StopFailure"})); ok {
			ago := float64(now - end)
			s.TurnEndedS = &ago
		}
		if move, why := v.E.StayOrMove(s); !move {
			return "", why, false
		}
	}
	// A usage move goes only where the agent fits; with none an early move stays and a walled agent waits for its reset, at most the
	// soonest 5-hour reset. A move off an account it cannot run on takes the most room.
	dest, why = m.Launch.RoomElsewhere(m.Q, agent, v.moveExclude(agent, here), !outTrigger(kind))
	if dest == "" {
		return "", why, true
	}
	return dest, "it has room", false
}

// moveExclude is the accounts a move never goes to: the one at the wall, and the one it just left until that resets.
func (v *Mover) moveExclude(agent, here string) map[string]string {
	m := v.M
	exclude := map[string]string{here: "the account it is at the wall on"}
	if left := lastLeft(m.Q, agent); left != "" && left != here {
		if r := v.E.Windows(left).ShortReset; r > m.now() {
			exclude[left] = "the account it just left, until it resets"
		}
	}
	return exclude
}

// closePlan is the account a close of kind moves the agent to, and why: dest held to the room test when one is given, else MovePlan's pick.
func (v *Mover) closePlan(agent, kind, dest string) (string, string) {
	if dest != "" {
		return v.forcedPlan(agent, dest)
	}
	there, why, _ := v.plan(agent, kind, false)
	return there, why
}

// forcedPlan holds a person's chosen destination to the same room test.
func (v *Mover) forcedPlan(agent, dest string) (string, string) {
	m := v.M
	row := must(db.GetAgentState(m.Q, agent))
	if why := m.Launch.RoomOn(m.Q, agent, row, dest); why != "" {
		return "", dest + " " + why
	}
	return dest, "it has room"
}

// closeFor ends the session for a handoff of kind. At a wall the conversation is kept while it is small enough to read again (carried):
// the next start resumes it, on this account after its reset or on the one it moves to (carrySession copies it there), and is told to
// carry on. Any other handoff closes it with no resume id: a cleared context is a session that is not resumed. Close errors while any
// process of the group lives, and that is the proof.
func (v *Mover) closeFor(agent, kind string) (bool, string) {
	m := v.M
	here := m.Agents.CurrentAccount(m.Q, agent)
	st := m.Status.ReadForAgent(agent, here)
	if !usageTrigger(kind) || st.SessionID == "" || m.carried(agent, st.SessionID) == "" {
		ok, why := m.endForMove(agent, st)
		if ok {
			m.afterFreshClose(agent)
		}
		return ok, why
	}
	ok, why := m.endTerminalByPane(agent, st.SessionID, st)
	if row := must(db.GetAgentState(m.Q, agent)); ok && isClosed(row) {
		check(db.SetCursor(m.Q, wallCutKey(agent), row.ClosedAt.Int64))
		m.Handoff.ClearNote(m.Q, agent)
	}
	return ok, why
}

// wallCutKey marks an agent whose session a wall closed with its conversation kept (closeFor): the mark is that close's closed_at.
func wallCutKey(agent string) string { return "wall_cut:" + agent }

// wallCut is whether the agent is still closed by the wall close that kept its conversation: closed then, with the session to resume.
// A later close, a session up again (closed_at cleared) or a resume id forgotten ends it, whichever path did it.
func (m *Manager) wallCut(agent string, row *db.AgentState) bool {
	at := must(db.Cursor(m.Q, wallCutKey(agent)))
	return at != 0 && row != nil && row.ResumeSession.String != "" && row.ClosedAt.Int64 == at
}

// moveToRoom puts the agent's next session on an account that has room. (moved, retrySoon); retrySoon is meaningful only when not moved:
// a destination existed, so what stopped the move is about this session now and not about the accounts. An account whose launch was refused
// is left out and the next tried at once. Any other failed start says nothing against the account, and may yet come up: another launch now
// could make a second session, so it waits for the retry. A person's chosen dest has no next.
func (v *Mover) moveToRoom(agent string, t HandoffTrigger, dest string) (moved, retrySoon bool) {
	m := v.M
	if !MovableTrigger(t.Kind) {
		return false, false
	}
	here := m.Agents.CurrentAccount(m.Q, agent)
	there, why := v.closePlan(agent, t.Kind, dest)
	if there == "" {
		v.log("  %s: staying on %s - %s", agent, here, why)
		return false, false
	}
	if !m.sessionAlive(agent) {
		return false, true
	}
	waited := must(db.GetSwitch(m.Q, agent))
	m.noteSwitch(agent, t.Kind, here, there, db.SwitchSwitching)
	running := v.E.BackgroundWork(m.Q, agent).Lines // the close stops it; the next session is told
	if ok, why := v.closeFor(agent, t.Kind); !ok {
		v.log("  %s: staying on %s - %s", agent, here, why)
		if waited != nil && waited.Phase == db.SwitchWaiting { // the tick's try off a parked account: the wait goes on
			check(db.SetSwitch(m.Q, *waited))
			m.refreshView(agent)
		}
		return false, true
	}
	failed := map[string]string{}
	for {
		refusedBefore, _ := must2(db.LastEventID(m.Q, agent, LaunchRefusedKind))
		ok, wakeWhy := v.land(agent, t, here, there, why, running, false)
		if ok {
			return true, false
		}
		v.log("  %s: closed on %s but could not start on %s: %s", agent, here, there, clip(wakeWhy, 120))
		refused, _ := must2(db.LastEventID(m.Q, agent, LaunchRefusedKind))
		if dest != "" || refused <= refusedBefore || must(db.GetAgentState(m.Q, agent)).State == states.Stalled {
			return false, true
		}
		failed[there] = wakeWhy
		exclude := v.moveExclude(agent, here)
		maps.Copy(exclude, failed)
		if there, _ = m.Launch.RoomElsewhere(m.Q, agent, exclude, !outTrigger(t.Kind)); there == "" {
			return false, true
		}
	}
}

// land starts the next session of an agent whose last one has closed on there, and records the move. False when nothing started: the row
// still says closed, and parking it with a deadline is what brings a closed agent back. stopped is the background work the close ended, ""
// for none. pastFit is a person's choice past the room test.
func (v *Mover) land(agent string, t HandoffTrigger, here, there, why, stopped string, pastFit bool) (bool, string) {
	m := v.M
	m.noteSwitch(agent, t.Kind, here, there, db.SwitchStarting)
	// HANDOFF protects the session that just ended; it comes off before the wake so the new session's own first turn can set RUNNING.
	check(db.SetParked(m.Q, agent, states.Queued, 0, string(verdict.ParkMoving), fmt.Sprintf("%s %s -> %s", states.MovingMark, here, there)))
	m.Agents.MeasureBegin(m.Q, agent, there)
	woke, wakeWhy := m.wakeAgent(agent, WakeOpts{MovedTo: there, PastFit: pastFit, Person: pastFit, Stopped: stopped})
	landed := must(db.GetAgentState(m.Q, agent))
	if !woke && landed != nil && landed.State != states.Running && isClosed(landed) {
		v.E.MeasureForget(m.Q, agent)
		return false, wakeWhy
	}
	check(db.SetAccount(m.Q, agent, there, ""))
	hookRunning := !must(db.SetStateUnlessRunning(m.Q, agent, states.Queued, fmt.Sprintf("moved %s -> %s (%s)", here, there, t.Kind)))
	if hookRunning {
		if fresh := must(db.GetAgentState(m.Q, agent)); fresh != nil && isClosed(fresh) {
			check(db.ClearClosed(m.Q, agent))
		}
		if !woke {
			check(db.ResetCounter(m.Q, agent, "wake_failures"))
			check(db.ClearHandoff(m.Q, agent))
		}
	}
	m.movedAccount(agent, here, there, t.Kind, m.moveReason(t.Kind, here), db.KV{K: "note_written", V: t.NoteWritten})
	v.log("  %s: moved %s -> %s (%s) - %s%s", agent, here, there, t.Kind, why, ifTrue(!woke && !hookRunning, " - but it was not handed its handoff: "+clip(wakeWhy, 80)))
	return true, ""
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// MoveToRoom is the move for a caller that has the agent's lock already decided (the tick's un-park of a walled agent).
func (v *Mover) MoveToRoom(agent string, t HandoffTrigger) (moved bool, err error) {
	defer catch(&err)
	defer v.M.lockAgent(agent)()
	if !wallTrigger(t.Kind) || v.M.wallBusy(agent) {
		return false, nil
	}
	moved, _ = v.moveToRoom(agent, t, "")
	return moved, nil
}

// PerformHandoff runs the pipeline, committing each step. True when the agent ended parked cleanly or the handoff was deferred (a defer
// counts nothing against it: the trigger is a condition and fires again), false when it degraded.
func (v *Mover) PerformHandoff(agent string, t HandoffTrigger, dest string) (ok bool, err error) {
	defer catch(&err)
	defer v.M.lockAgent(agent)()
	return v.performHandoff(agent, t, dest)
}

// turnOverAtLimit: a turn that ended on the usage limit and has stayed quiet is over, whatever else says; one that ended on an expired
// login is over at once. Sooner than that a person or the agent may be mid-turn.
func (v *Mover) turnOverAtLimit(agent, kind string, now int64) bool {
	stoppedAt, stopped := v.E.AtLimit(v.M.Q, agent)
	return kind == LoginTrigger || (stopped && now-stoppedAt >= states.LimitSettleS) ||
		((usageTrigger(kind) || kind == "forced") && v.E.LimitDialogOpen(agent))
}

// performHandoff is PerformHandoff for a caller that holds the agent's lock.
func (v *Mover) performHandoff(agent string, t HandoffTrigger, dest string) (bool, error) {
	m := v.M
	if existing := must(db.InFlightFor(m.Q, agent)); existing != nil {
		check(db.AdvanceHandoff(m.Q, existing.ID, existing.Step, existing.State, strings.Trim(existing.Detail.String+"; also "+t.Kind, "; ")))
		v.log("  %s: already handing off at step '%s', merged '%s' as an added reason", agent, existing.Step, t.Kind)
		return true, nil
	}
	now := m.now()
	var last *int64
	_ = m.Q.QueryRow("SELECT MAX(updated) FROM handoff WHERE agent = ?", agent).Scan(&last)
	if last != nil && t.Kind != "forced" && !outTrigger(t.Kind) && now-*last < states.MinHandoffIntervalS {
		v.log("  %s: %s fired again %ds after the last handoff - too soon, skipping", agent, t.Kind, now-*last)
		if usageTrigger(t.Kind) && m.sessionAlive(agent) {
			v.deferredSaving(agent, t.Kind, dest, "")
		}
		return true, nil
	}
	if !m.sessionAlive(agent) {
		v.log("  %s: %s fired but no session is running - not handing off", agent, t.Kind)
		return true, nil
	}
	settled := v.turnOverAtLimit(agent, t.Kind, now)
	busy := m.sessionBusy
	switch {
	case t.Kind == "context":
		busy = m.turnBusy // a context clear has its own wait for background work (reclaim)
	case wallTrigger(t.Kind):
		busy = m.wallBusy // the move stops background work nothing waits on
	}
	noteKind := t.Kind
	if !settled && busy(agent) {
		if t.Kind == "forced" {
			return false, fmt.Errorf("%s: the forced move did not happen: a terminal's turn cannot be interrupted without typing", agent)
		}
		waits, logged := "the turn to end", "the turn to end"
		bg := v.E.BackgroundWork(m.Q, agent)
		onBg := bg.Words != "" && m.TermRun.IdleWithBackground(agent)
		if onBg {
			waits, logged = BackgroundWaits+bg.Words, BackgroundWaits+bg.Lines
		}
		if !onBg || !v.backgroundWaitOver(agent, t.Kind, bg, now) {
			v.log("  %s: %s fired - deferring, waits for %s", agent, t.Kind, logged)
			check(db.MarkDeferred(m.Q, agent, now))
			v.deferredSaving(agent, t.Kind, dest, waits)
			return true, nil
		}
		noteKind = states.EarlyUsageTrigger // the note is asked as an early move asks it: with the work running
	}
	// The agent writes its own part of the record first, one turn while its cache is warm. A person's order is not held for it, and a
	// session at the wall cannot take a turn.
	if t.Kind != "forced" && t.Kind != LoginTrigger && !settled && v.E.NoteHold(m.Q, agent, noteKind, now) {
		v.log("  %s: %s fired - waiting for its handoff note", agent, t.Kind)
		check(db.MarkNoteWait(m.Q, agent, t.Kind, now))
		v.deferredSaving(agent, t.Kind, dest, "its handoff note")
		return true, nil
	}
	// A person's kept move names where any handoff takes the agent; one that cannot go there says so rather than vanish.
	kept := must(db.GetSwitch(m.Q, agent))
	if !kept.Pending() {
		return v.perform(agent, t, dest), nil
	}
	ok := v.perform(agent, t, kept.To)
	if m.Agents.CurrentAccount(m.Q, agent) != kept.To {
		m.moveFailed(agent, kept.To, fmt.Sprintf("a %s handoff came first and could not take it there", t.Kind))
	}
	return ok, nil
}

// backgroundWaitOver is whether a wall move has waited on the agent's background work as long as the setting lets it (none waits as long
// as the work runs). Past it the
// move goes on and its close ends the work; the first time is said once, in the log and a MovedPastBackgroundKind event.
func (v *Mover) backgroundWaitOver(agent, kind string, bg Background, now int64) bool {
	if !wallTrigger(kind) {
		return false
	}
	m := v.M
	since := v.backgroundWaitSince(agent, now)
	var limit int64
	if m.Settings != nil {
		limit = m.Settings.MoveBackgroundWaitS(agent)
	}
	if limit <= 0 || now-since < limit {
		return false
	}
	if said, ok := must2(db.LastEventOfTS(m.Q, agent, []string{MovedPastBackgroundKind})); !ok || said < since {
		must(db.AddEventAt(m.Q, now, agent, MovedPastBackgroundKind, db.KV{K: "kind", V: kind}, db.KV{K: "jobs", V: bg.Words},
			db.KV{K: "waited_s", V: now - since}))
		v.log("  %s: %s waited %s on background work (%s) - moving past it, which ends it: %s", agent, kind, timefmt.Ago(now-since),
			bg.Words, bg.Lines)
	}
	return true
}

// backgroundWaitSince is when the move began to wait on the background work running now: its first wait on background, or the start of
// the current stretch of work when that came later (a mark left by a session or work since ended counts for nothing).
func (v *Mover) backgroundWaitSince(agent string, now int64) int64 {
	m := v.M
	since := must(db.MarkBackgroundWait(m.Q, agent, now))
	if began, ok, err := m.Delivery.BgSince(m.Q, agent); err == nil && ok && began > since {
		since = min(began, now)
	}
	return since
}

func (v *Mover) perform(agent string, t HandoffTrigger, dest string) bool {
	m := v.M
	hid := newHandoffID()
	check(db.ClearNoteWait(m.Q, agent))
	check(db.ClearDeferred(m.Q, agent))
	v.saving(agent, t.Kind, "")
	acct := m.Agents.CurrentAccount(m.Q, agent)
	st := m.Status.ReadForAgent(agent, acct)
	check(db.StartHandoff(m.Q, hid, agent, st.SessionID, t.Kind, m.now()+states.HandoffDeadlineS))
	check(db.SetAgentState(m.Q, agent, states.Handoff, 0, t.Reason))
	v.log("  %s: HANDOFF (%s) - %s", agent, t.Kind, t.Reason)

	prior := must(db.GetHandoff(m.Q, agent))
	t.NoteWritten = v.E.NoteWritten != nil && v.E.NoteWritten(agent)
	ok := v.recordHandoff(agent, hid)
	check(db.AdvanceHandoff(m.Q, hid, "extracted", "in_flight", ""))
	released := func(detail string) {
		check(db.AdvanceHandoff(m.Q, hid, "released", ifTrue(ok, "done")+ifTrue(!ok, "degraded"), detail))
	}

	moved, retrySoon := v.moveToRoom(agent, t, dest)
	if moved {
		released("")
		check(db.BumpCounter(m.Q, agent, "continuations", 1))
		if ok {
			check(db.ResetCounter(m.Q, agent, "throttle_count"))
		}
		return ok
	}
	if landed := must(db.GetAgentState(m.Q, agent)); landed != nil && landed.State == states.Stalled {
		released("stalled on the move") // a park would overwrite what the founder has to see
		return false
	}
	if landed := must(db.GetAgentState(m.Q, agent)); t.Kind == "forced" && landed != nil && !isClosed(landed) {
		// Nothing was ended, so there is nothing to park. The record this attempt took would be injected at the next start as
		// "you were interrupted"; the row goes back as it was.
		if prior == "" {
			check(db.ClearHandoff(m.Q, agent))
		} else {
			check(db.SetHandoff(m.Q, agent, prior))
		}
		check(db.AdvanceHandoff(m.Q, hid, "released", "degraded", "forced move did not happen"))
		check(db.ClearSwitch(m.Q, agent))
		check(db.SetAgentState(m.Q, agent, states.Idle, 0, "forced move did not happen"))
		return false
	}
	cleared, why := v.reclaim(agent, t.Kind)
	if !cleared && v.clearOwed(agent) != 0 {
		// Background work holds the close off and the agent was told: a wait, not a failure.
		released(why)
		check(db.SetAgentState(m.Q, agent, states.Idle, 0, "clear owed: "+why))
		v.log("  %s: handoff recorded; %s", agent, why)
		return ok
	}
	check(db.AdvanceHandoff(m.Q, hid, "extracted", "in_flight", why))
	if ok && cleared {
		check(db.ResetCounter(m.Q, agent, "throttle_count"))
	}
	v.park(agent, t, hid, ok, cleared, retrySoon)
	return ok
}

func (v *Mover) clearOwed(agent string) int64 { return must(db.Cursor(v.M.Q, "clear_owed:"+agent)) }

// ClearNotice is what the agent is told when its clear waits for background work.
const ClearNotice = "handline: your context is full (%s), so this session will be cleared in %d minutes. No reply is needed. Before then: 1. Stop your background shells. Anything still running at that time is cut off. 2. If you were waiting on a result, write what was running and what is left in: hl progress \"done: ... left: ...\" - the next session starts from that note. 3. Do not start new work or new background jobs. When your background work has ended, the clear happens at once."

// reclaim returns the context: the session is closed (closeFor kind) and the park's wake starts the next one. Work the session still runs
// is cut off by the close, so it is owed: the agent is told once and the close comes when the work ends or its time is up.
func (v *Mover) reclaim(agent, kind string) (bool, string) {
	m := v.M
	if !m.sessionAlive(agent) {
		return false, "no session process; nothing to close"
	}
	running := v.E.BackgroundWork(m.Q, agent).Lines
	if running == "" {
		return v.closeNow(agent, kind)
	}
	now := m.now()
	since := v.clearOwed(agent)
	if last := must2opt(db.LastActivity(m.Q, agent)); last != 0 && now-last >= states.PromptCacheS {
		v.log("  %s: background work is running (%s) but its cache has lapsed - closing", agent, running)
		return v.closeNow(agent, kind)
	}
	owed := fmt.Sprintf("background work is running (%s); the close is owed", running)
	if since == 0 {
		used, limit := v.E.ContextUse(m.Q, agent)
		usedText := "?"
		if used >= 0 {
			usedText = fmt.Sprint(used)
		}
		err := m.tellSession(agent, "clear_notice:"+fmt.Sprint(now), fmt.Sprintf(ClearNotice, usedText+" of "+fmt.Sprint(limit), ClearWaitS/60))
		check(db.SetCursor(m.Q, "clear_owed:"+agent, now)) // owed even if the notice did not go
		v.log("  %s: background work is running (%s); told it will be cleared in %d min%s", agent, running, ClearWaitS/60, ifTrue(err != nil, " (the notice failed)"))
		return false, owed
	}
	if now-since < ClearWaitS {
		return false, owed
	}
	v.log("  %s: %d min are up with background work still running (%s) - closing", agent, ClearWaitS/60, running)
	ok, why := v.closeNow(agent, kind)
	if ok || must(db.Cursor(m.Q, orphanKey(agent))) != 0 { // a session that outlives its pane is cut off when it ends
		rec := must(db.GetHandoff(m.Q, agent))
		check(db.SetHandoff(m.Q, agent, fmt.Sprintf("%s\n\n## Cut off by the clear\nThis session was cleared with background work still running (%s); it was told %s earlier and asked to stop it. That work is gone - rerun it if it still matters.\n", rec, running, timefmt.Ago(now-since))))
	}
	return ok, why
}

func must2opt(v int64, ok bool, err error) int64 {
	if err != nil || !ok {
		return 0
	}
	return v
}

func (v *Mover) closeNow(agent, kind string) (bool, string) {
	ok, why := v.closeFor(agent, kind)
	if ok {
		check(db.ClearCursor(v.M.Q, "clear_owed:"+agent))
	}
	return ok, why
}

// SettleOwedClear is true while this agent's clear is owed: the close comes once its background work has ended or its time is up,
// and the agent is parked as the handoff would have. Past its time only a running turn holds it, whatever the trigger. False when nothing is owed or the session it was owed for is gone.
func (v *Mover) SettleOwedClear(agent string) (owed bool, err error) {
	defer catch(&err)
	m := v.M
	var ended bool
	func() {
		defer m.lockAgent(agent)()
		ended = m.SettleOrphanedClose(agent)
	}()
	if v.clearOwed(agent) == 0 {
		return false, nil
	}
	if ended {
		// The close this clear was owed ran past the pane; it ends like the one that went at once: parked, to wake now.
		check(db.ClearCursor(m.Q, "clear_owed:"+agent))
		defer m.lockAgent(agent)()
		v.parkOwed(agent)
		return true, nil
	}
	if must(db.Cursor(m.Q, orphanKey(agent))) != 0 {
		return true, nil // the close went and its session is still ending: nothing is closed twice
	}
	row := must(db.GetAgentState(m.Q, agent))
	if row == nil || isClosed(row) || !m.sessionAlive(agent) {
		check(db.ClearCursor(m.Q, "clear_owed:"+agent))
		return false, nil
	}
	defer m.lockAgent(agent)()
	kind, now := must(db.LastHandoffTrigger(m.Q, agent)), m.now()
	busy := m.sessionBusy
	switch {
	case now-v.clearOwed(agent) >= ClearWaitS && v.turnOverAtLimit(agent, kind, now):
		busy = func(string) bool { return false }
	case now-v.clearOwed(agent) >= ClearWaitS:
		busy = m.turnRunning
	case kind == "context":
		busy = m.turnBusy // only a context clear waits past background work
	}
	if busy(agent) {
		return true, nil
	}
	cleared, why := v.reclaim(agent, kind)
	if !cleared {
		v.log("  %s: its clear is owed and holds all it is told - %s", agent, ifTrue(why != "", why)+ifTrue(why == "", "the close did not go"))
		return true, nil
	}
	v.parkOwed(agent)
	return true, nil
}

// parkOwed parks an agent whose owed clear has closed its session.
func (v *Mover) parkOwed(agent string) {
	m := v.M
	kind, ok := "context", false
	if h := must(db.LastHandoffTrigger(m.Q, agent)); h != "" {
		kind = h
	}
	var state string
	if err := m.Q.QueryRow("SELECT state FROM handoff WHERE agent = ? ORDER BY updated DESC LIMIT 1", agent).Scan(&state); err == nil {
		ok = state == "done"
	}
	if ok {
		check(db.ResetCounter(m.Q, agent, "throttle_count"))
	}
	v.park(agent, HandoffTrigger{Kind: kind, Reason: "clear owed"}, "", ok, true, false)
}

// park is where a handoff that reached its close ends: parked until it may be woken. hid is "" for a close that was owed and came later.
func (v *Mover) park(agent string, t HandoffTrigger, hid string, ok, cleared, retrySoon bool) {
	m := v.M
	now := m.now()
	acct, source, have := v.E.Reading(m.Q, agent)
	shortRetry := !have || source == "assumed" || t.Kind == "forced" || retrySoon
	var wake int64
	switch {
	case shortRetry:
		wake = now + states.AssumedAccountRecheckS
	case usageTrigger(t.Kind) || t.Kind == states.ByHandMark:
		w := v.E.Windows(acct)
		wake = v.E.WakeAfter(t.Kind, w.ShortReset, w.LongReset, now)
	default:
		// Not a wall: the context is handled by the close, but the wake is still what hands the agent its record back.
		wake = now
		if !cleared {
			check(db.BumpCounter(m.Q, agent, "throttle_count", 1))
		}
	}
	label, reason := t.Kind, parkReason(t.Kind, shortRetry)
	if reason == verdict.ParkMoving {
		label = states.MovingMark
	}
	if fresh := must(db.GetAgentState(m.Q, agent)); fresh != nil && fresh.ThrottleCount > 0 {
		if back := now + int64(states.BackoffFor(int(fresh.ThrottleCount)).Seconds()); back > wake {
			wake = back
			v.log("  %s: backing off %ds after %d degraded handoff(s)", agent, back-now, fresh.ThrottleCount)
		}
	}
	if hid != "" {
		check(db.AdvanceHandoff(m.Q, hid, "released", ifTrue(ok, "done")+ifTrue(!ok, "degraded"), ""))
	}
	check(db.SetParked(m.Q, agent, states.Parked, wake, string(reason), fmt.Sprintf("%s; wakes at %s", label, timefmt.Stamp(wake, timefmt.Clock))))
	v.parkedSwitch(agent, t.Kind, acct, reason, wake)
	var acctV, srcV any
	if have {
		acctV, srcV = acct, source
	}
	must(db.AddEvent(m.Q, agent, "park", db.KV{K: "kind", V: t.Kind}, db.KV{K: "wake_at", V: wake}, db.KV{K: "account", V: acctV}, db.KV{K: "account_source", V: srcV}))
	check(db.BumpCounter(m.Q, agent, "continuations", 1))
	v.log("  %s: PARKED until %s", agent, timefmt.Stamp(wake, timefmt.DateTime))
}

// ResumeHandoff finishes a handoff from the step it reached, so a restart picks up where it landed instead of leaving the agent in
// HANDOFF. Steps run requested -> recorded -> extracted -> released, each committed. Nothing is ended here: a daemon that was restarted
// owns no session process, and the sweep records the agent closed.
func (v *Mover) ResumeHandoff(row db.HandoffRow) (err error) {
	defer catch(&err)
	m := v.M
	agent, step, hid := row.Agent, row.Step, row.ID
	doneOK := true
	if step == "requested" {
		doneOK = v.recordHandoff(agent, hid)
		if doneOK {
			check(db.ResetCounter(m.Q, agent, "throttle_count"))
		}
	}
	if step == "requested" || step == "recorded" || step == "folded" {
		check(db.AdvanceHandoff(m.Q, hid, "extracted", "in_flight", ""))
	}
	acct, source, have := v.E.Reading(m.Q, agent)
	kind := row.TriggerKind.String
	shortRetry := !have || source == "assumed" || kind == "forced"
	now := m.now()
	var wake int64
	switch {
	case shortRetry:
		wake = now + states.AssumedAccountRecheckS
	case usageTrigger(kind) || kind == states.ByHandMark:
		w := v.E.Windows(acct)
		wake = v.E.WakeAfter(kind, w.ShortReset, w.LongReset, now)
	default:
		wake = now
	}
	check(db.AdvanceHandoff(m.Q, hid, "released", ifTrue(doneOK, "done")+ifTrue(!doneOK, "degraded"), ""))
	label, reason := "resumed handoff", parkReason(kind, shortRetry)
	switch reason {
	case verdict.ParkByHand:
		label = states.ByHandMark
	case verdict.ParkMoving:
		label = states.MovingMark
	}
	check(db.SetParked(m.Q, agent, states.Parked, wake, string(reason), fmt.Sprintf("%s; wakes at %s", label, timefmt.Stamp(wake, timefmt.Clock))))
	v.parkedSwitch(agent, kind, acct, reason, wake)
	var acctV, srcV any
	if have {
		acctV, srcV = acct, source
	}
	must(db.AddEvent(m.Q, agent, "park", db.KV{K: "kind", V: kind}, db.KV{K: "wake_at", V: wake}, db.KV{K: "account", V: acctV}, db.KV{K: "account_source", V: srcV}))
	v.log("  %s: resumed handoff from '%s' -> PARKED", agent, step)
	return nil
}
