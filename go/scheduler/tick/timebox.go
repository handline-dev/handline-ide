package tick

import (
	"fmt"
	"strings"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/timefmt"
)

// Four notices about work that has run long, all read from the db every tick: a task past its time box (SweepTaskTimeouts), background
// work about to outlive the agent's prompt cache (its supervisor is told it may be stopped after a veto window), and an agent IDLE with
// its wait never recorded or nudged about a forgotten head as often as the schedule allows (the task's opener is told once, else the
// agent's supervisor). A notice to an agent is a line sent to its open session, so one that cannot be sent now is tried again; a notice
// to the human has no session to type at and is an event.
const (
	TitleShown = 60
	// NoticeRetryS: a notice that could not be sent (mid-turn, awaiting a permission, no session) is not tried again for this long, per agent.
	NoticeRetryS = 60
	// HandOffRetryS: a handoff the daemon was refused is not tried again for this long, per task.
	HandOffRetryS = 600
)

// Forgotten is delivery.forgotten_resolve with the questions timebox asks about its head: an idle agent whose current head request nobody
// resolved. Task is the head's task when it is one.
type Forgotten struct {
	HeadID    int64
	Task      *db.Task
	IdleS     int64
	Behind    int
	Blocked   bool // delivery.head_task_blocked
	Exhausted bool // delivery.nudge_exhausted: the nudge schedule is used up
}

// BgJob is one running background job as bgjobs reports it.
type BgJob struct{ Line string }

// TimeboxPorts are the foreign calls the notices make: the board's actions, delivery's background bookkeeping, and the idle-close settings.
type TimeboxPorts interface {
	// TaskDoing is actions.task_state(row) == "doing".
	TaskDoing(q db.Q, t *db.Task) bool
	// SupervisorOf is actions.supervisor_of(routing(), agent), "" when it has none.
	SupervisorOf(agent string) string
	// SendNotice is delivery.type_notice, sent through the session: true when the line was sent.
	SendNotice(q db.Q, agent, text string) bool
	// SendBatch writes text as a notice for the agent's next digest (delivery.ClassBatch).
	SendBatch(q db.Q, agent, text string) error
	// SayNow is a say to the agent that wakes its closed session, as a request delivered at once.
	SayNow(q db.Q, agent, text string) error
	// HandOffHeld hands task tid to `to` as the daemon, with the note.
	HandOffHeld(q db.Q, tid int64, to, note string) error
	BgStale(q db.Q, agent string, now int64) bool
	BgOwesNothing(q db.Q, agent string) bool
	BgSilentS(q db.Q, agent string, now int64) (int64, bool)
	BgSince(q db.Q, agent string) (int64, bool)
	BgPromptedAt(q db.Q, agent string) int64
	StaleBgVetoS() int64
	ForgottenResolve(q db.Q, agent string, row *db.AgentState, now int64) *Forgotten
	NudgeScheduleLen() int
	// NeverCloses is idle_close.close_timeout_s(agent) == NEVER.
	NeverCloses(agent string) bool
	BgNoticeS(agent string) int64
	BgGraceS(agent string) int64
	BgCheckS(agent string) int64
	// BlockerAlarm is the blocker_alarm setting and its holder and lead thresholds.
	BlockerAlarm() (on bool, holderS, leadS int64)
	// BgJobs names what still runs, one line per job, the last count of them when count is set.
	BgJobs(q db.Q, agent string, count int, now int64) []string
}

// Timebox is the four notices.
type Timebox struct {
	Ports TimeboxPorts
}

func (b *Timebox) tell(t *Tick, who, text string, now int64) bool {
	if actor.IsPerson(who) {
		return true
	}
	key := "notice_try:" + who
	if now-must(db.Cursor(t.Q, key)) < NoticeRetryS {
		return false
	}
	check(db.SetCursor(t.Q, key, now))
	return b.Ports.SendNotice(t.Q, who, text)
}

func (b *Timebox) boss(agent string) string {
	if s := b.Ports.SupervisorOf(agent); s != "" {
		return s
	}
	return actor.OwnerKey
}

// parked is whether agent is parked (at its account's limit, or until its next session), which no clock of its runs through.
func (b *Timebox) parked(t *Tick, agent string) bool {
	row := must(db.GetAgentState(t.Q, agent))
	return row != nil && row.State == states.Parked
}

// answerer is who hears about an agent stuck on task tid: the task's opener, else the agent's supervisor - never the agent itself, and
// the founder only when neither is an agent.
func (b *Timebox) answerer(t *Tick, agent string, tid int64) string {
	if tid != 0 {
		if task := must(db.GetTask(t.Q, tid)); task != nil && task.OpenedBy != agent && !actor.IsPerson(task.OpenedBy) && task.OpenedBy != "" {
			return task.OpenedBy
		}
	}
	return b.boss(agent)
}

// SweepTaskTimeouts runs each task's `hl task new -t/--timeout` clock: it counts only while the task is doing and nothing holds it (a
// blocker, a child, the founder's turn or ask, its agent's park), and goes on from what it had left when the hold ends. Past its length it
// runs on, over; its holder and lead are told then and each further length, until it is done, extended (`hl task edit --timeout`) or
// released. A task with no timeout_s takes the default where it becomes doing.
func (b *Timebox) SweepTaskTimeouts(t *Tick, now int64) {
	for _, row := range must(db.TasksToClock(t.Q)) {
		tid, holder := row.ID, db.HolderOf(row.Acting)
		if holder == verdict.GateHolds {
			continue // the gate sweep closes it at its deadline
		}
		waits := holder == verdict.AgentHolds && must(db.TaskWaits(t.Q, tid))
		if holder == verdict.FounderHolds || waits || must(db.FounderAsked(t.Q, tid)) || b.parked(t, row.Acting) || !b.Ports.TaskDoing(t.Q, row) {
			check(db.PauseTaskTimeout(t.Q, tid, now))
			continue
		}
		if !row.TimeoutAt.Valid {
			check(db.AdvanceTaskTimeout(t.Q, tid, now, db.DefaultTaskTimeoutS))
			continue
		}
		if !verdict.Overdue("doing", holder, waits, row.TimeoutAt.Int64, now) {
			continue
		}
		// told at its end, then each further length it runs
		box, over := row.TimeoutS.Int64, now-row.TimeoutAt.Int64
		if box <= 0 {
			box = db.DefaultTaskTimeoutS
		}
		if told := must(db.TimeoutNotices(t.Q, tid)); told.N > 0 && over < told.LastOver+box {
			continue
		}
		title := shortTitle(row.Title)
		text := fmt.Sprintf("T%d (%s), held by %s, has been doing past its %s timeout.%s Do it, hand it on (`hl task handoff T%d <agent> \"<note>\"`), "+
			"mark what blocks it (`hl task edit T%d --blocked-by T<n>`), or put the founder on it if only they can decide (`hl task comment T%d add \"<the decision, and what you recommend>\" --turn human`). "+
			"`hl task release T%d \"<reason>\"` takes it back.",
			tid, title, row.Acting, timefmt.Ago(box), behindLine(t, row.Acting, tid), tid, tid, tid, tid)
		// the holder can act on it, and its lead routes; the opener of a task another agent holds cannot
		var notified []string
		for _, who := range []string{row.Acting, b.boss(row.Acting)} {
			if !actor.IsPerson(who) { // never the founder; it waits in the next digest
				check(b.Ports.SendBatch(t.Q, who, text))
				notified = append(notified, who)
			}
		}
		// on the holder, whom it is about; the opener is not told. deadline is the box's end, over_s how far past it the clock has run.
		must(db.AddEvent(t.Q, row.Acting, "task_timeout_exceeded", db.KV{K: "task_id", V: tid}, db.KV{K: "timeout_s", V: box}, db.KV{K: "notified", V: notified},
			db.KV{K: "text", V: text}, db.KV{K: "deadline", V: row.TimeoutAt.Int64}, db.KV{K: "over_s", V: over}))
		t.logf("  T%d: doing, past its %s timeout - %s told", tid, timefmt.Ago(box), strings.Join(notified, ", "))
	}
}

// SweepQueuedBlockers: a queued task that other open tasks wait on (a block or a parent) has no clock running, so nothing tells anyone it
// holds them up. With blocker_alarm on, once the first of them has waited blocker_alarm_s its holder is told, and at blocker_alarm_lead_s
// its holder's lead; each once per wait, which ends when no task waits on it or the task is closed. A reopened task's wait counts from the
// reopen. A task that itself waits is left to the head of its chain. Never the founder: it waits in the digest.
func (b *Timebox) SweepQueuedBlockers(t *Tick, now int64) {
	on, holderS, leadS := b.Ports.BlockerAlarm()
	if !on {
		return
	}
	for _, prefix := range []string{"blocker_alarm:", "blocker_alarm_lead:"} {
		check(db.ClearCursorsOfEndedTasks(t.Q, prefix))
	}
	for _, row := range must(db.TasksToClock(t.Q)) {
		tid := row.ID
		held, lead := fmt.Sprintf("blocker_alarm:%d", tid), fmt.Sprintf("blocker_alarm_lead:%d", tid)
		waiters := must(db.WaitersOn(t.Q, tid))
		if len(waiters) == 0 {
			check(db.ClearCursor(t.Q, held))
			check(db.ClearCursor(t.Q, lead))
			continue
		}
		if db.HolderOf(row.Acting) != verdict.AgentHolds || must(db.TaskState(t.Q, row)) != "queued" || must(db.TaskWaits(t.Q, tid)) {
			continue
		}
		since := waiters[0].Since
		for _, w := range waiters {
			since = min(since, w.Since)
		}
		reopened := must(db.ReopenedAt(t.Q, tid))
		since = max(since, reopened)
		text := fmt.Sprintf("T%d (%s), held by %s, is queued, and %s %s waited on it for %s. Start it, close it if its work is done "+
			"(`hl task done T%d \"<evidence>\"`), or hand it on (`hl task handoff T%d <agent> \"<note>\"`).",
			tid, shortTitle(row.Title), row.Acting, waitNames(waiters), waitVerb(waiters, "has", "have"), timefmt.Ago(now-since), tid, tid)
		for _, level := range []struct {
			key, who string
			afterS   int64
		}{{held, row.Acting, holderS}, {lead, b.boss(row.Acting), leadS}} {
			if told := must(db.Cursor(t.Q, level.key)); now-since < level.afterS || actor.IsPerson(level.who) || told != 0 && told >= reopened {
				continue
			}
			check(b.Ports.SendBatch(t.Q, level.who, text))
			check(db.SetCursor(t.Q, level.key, now))
			must(db.AddEvent(t.Q, row.Acting, "queued_blocker_alarm", db.KV{K: "task_id", V: tid}, db.KV{K: "told", V: level.who},
				db.KV{K: "since", V: since}, db.KV{K: "text", V: text}))
			t.logf("  T%d: queued, waited on by %s for %s - %s told", tid, waitNames(waiters), timefmt.Ago(now-since), level.who)
		}
	}
}

// BehindShown is how many tasks a Behind it sentence names before "+N more".
const BehindShown = 3

// behindLine is " Behind it: ..." for task tid that holder holds, or "": the open tasks that wait on it, then holder's queued tasks that
// cannot start while it holds tid, each with the open tasks that wait on it; those that others wait on first.
func behindLine(t *Tick, holder string, tid int64) string {
	var waited, rest []string
	onIt := map[int64]bool{}
	if ws := must(db.WaitersOn(t.Q, tid)); len(ws) > 0 {
		waited = append(waited, waitNames(ws)+" "+waitVerb(ws, "waits", "wait")+" on it")
		for _, w := range ws {
			onIt[w.ID] = true
		}
	}
	for _, o := range must(db.OpenTasksOf(t.Q, holder)) {
		if o.ID == tid || onIt[o.ID] || must(db.TaskState(t.Q, o)) != "queued" {
			continue
		}
		if ws := must(db.WaitersOn(t.Q, o.ID)); len(ws) > 0 {
			waited = append(waited, fmt.Sprintf("T%d (queued), which %s %s on", o.ID, waitNames(ws), waitVerb(ws, "waits", "wait")))
		} else {
			rest = append(rest, fmt.Sprintf("T%d (queued)", o.ID))
		}
	}
	all := append(waited, rest...)
	if len(all) == 0 {
		return ""
	}
	if len(all) > BehindShown {
		all = append(all[:BehindShown], fmt.Sprintf("+%d more", len(all)-BehindShown))
	}
	return " Behind it: " + strings.Join(all, "; ") + "."
}

// waitNames is the waiters' numbers, BehindShown of them and then how many more.
func waitNames(ws []db.Waiter) string {
	var names []string
	for _, w := range ws[:min(len(ws), BehindShown)] {
		names = append(names, fmt.Sprintf("T%d", w.ID))
	}
	if len(ws) > BehindShown {
		names = append(names, fmt.Sprintf("%d more", len(ws)-BehindShown))
	}
	return joinAnd(names)
}

func waitVerb(ws []db.Waiter, one, many string) string {
	if len(ws) > 1 {
		return many
	}
	return one
}

// joinAnd is "a", "a and b", "a, b and c".
func joinAnd(xs []string) string {
	if len(xs) < 2 {
		return strings.Join(xs, "")
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

// shortTitle is a task's title on one line, cut to TitleShown.
func shortTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	if r := []rune(title); len(r) > TitleShown {
		title = strings.TrimRight(string(r[:TitleShown]), " \t\n") + "…"
	}
	return title
}

// SweepStaleBackground tells the supervisor once that an agent's background work is stale, and that its session may be closed - which
// stops the work - after the veto window. The mark is cleared when the work is no longer stale, so a later stretch is told again.
func (b *Timebox) SweepStaleBackground(t *Tick, now int64) {
	for _, agent := range t.Host.Agents() {
		key := "stale_bg_told:" + agent
		if !b.Ports.BgStale(t.Q, agent, now) || b.Ports.BgOwesNothing(t.Q, agent) || b.Ports.NeverCloses(agent) {
			check(db.ClearCursor(t.Q, key))
			continue
		}
		if must(db.Cursor(t.Q, key)) != 0 {
			continue
		}
		boss := b.boss(agent)
		silent, _ := b.Ports.BgSilentS(t.Q, agent, now)
		since, have := b.Ports.BgSince(t.Q, agent)
		if !have {
			since = now
		}
		text := fmt.Sprintf("%s has had background work for %s and shown nothing for %s. In %s its session may be closed, which stops that work, "+
			"unless it shows activity or its idle timeout is set to never in the app's settings.",
			agent, timefmt.Ago(now-since), timefmt.Ago(silent), timefmt.Ago(b.Ports.StaleBgVetoS()))
		if b.tell(t, boss, text, now) {
			check(db.SetCursor(t.Q, key, now))
			must(db.AddEvent(t.Q, agent, "stale_background", db.KV{K: "told", V: boss}, db.KV{K: "silent_s", V: silent}))
			t.logf("  %s: background work stale (%s silent) - %s told", agent, timefmt.Ago(silent), boss)
		}
	}
}

// SweepForgottenBackground: background work nobody waits on settles itself, with nobody asked. When the agent's idle window is over it is
// told once to finish or stop the work; bg_grace_s later, with nothing shown, the idle close ends the session and its work. An agent set
// to never close is left alone. Both marks clear with the stretch: when the work ends, or a request, an ask or a task makes it owed.
func (b *Timebox) SweepForgottenBackground(t *Tick, now int64) {
	for _, agent := range t.Host.Agents() {
		key, noted := "bg_prompt:"+agent, "bg_stop_noted:"+agent
		if !t.Delivery.HasBackgroundWork(t.Q, agent) || !b.Ports.BgOwesNothing(t.Q, agent) || b.Ports.NeverCloses(agent) {
			check(db.ClearCursor(t.Q, key))
			check(db.ClearCursor(t.Q, noted))
			continue
		}
		silent, haveSilent := b.Ports.BgSilentS(t.Q, agent, now)
		if b.Ports.BgPromptedAt(t.Q, agent) == 0 {
			if !haveSilent || silent < b.Ports.BgNoticeS(agent) {
				continue
			}
			text := fmt.Sprintf("You have shown nothing for %s, with no request open, and background work is still running. Finish it or stop it now, "+
				"and say in one line what it was. If nothing more is shown in %s, it is stopped and this session closed.",
				timefmt.Ago(silent), timefmt.Ago(b.Ports.BgGraceS(agent)))
			if b.tell(t, agent, text, now) {
				check(db.SetCursor(t.Q, key, now))
				must(db.AddEvent(t.Q, agent, "background_prompted", db.KV{K: "silent_s", V: silent}))
				t.logf("  %s: background work with nothing owed, silent %s - told to finish or stop it", agent, timefmt.Ago(silent))
			}
		}
	}
}

// SweepBackgroundCheck: every bg_check_s while an agent has background work, at a turn's end, it is told what is running and asked
// to stop what it no longer needs - a shell nobody stops holds off the context rollover. Not the idle close: busy or idle, owing or not, a
// "never close" agent too. It waits out a notice already in flight.
func (b *Timebox) SweepBackgroundCheck(t *Tick, now int64) {
	for _, agent := range t.Host.Agents() {
		key := "bg_check:" + agent
		row := must(db.GetAgentState(t.Q, agent))
		if row == nil || !t.Delivery.HasBackgroundWork(t.Q, agent) {
			check(db.ClearCursor(t.Q, key))
			continue
		}
		if row.State != states.Idle || b.Ports.BgPromptedAt(t.Q, agent) != 0 {
			continue
		}
		every := b.Ports.BgCheckS(agent)
		since, have := b.Ports.BgSince(t.Q, agent)
		if !have {
			since = now
		}
		last := must(db.Cursor(t.Q, key))
		if last < since {
			last = since
		}
		if now-last < every {
			continue
		}
		count := int(must(db.Cursor(t.Q, "bgcount:"+agent)))
		jobs := b.Ports.BgJobs(t.Q, agent, count, now)
		named := count
		if named == 0 {
			named = len(jobs)
		}
		unnamed := named - len(jobs)
		if unnamed < 0 {
			unnamed = 0
		}
		var lines []string
		for i, j := range jobs {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, j))
		}
		listing := strings.Join(lines, "; ")
		if unnamed > 0 {
			if listing != "" {
				listing += "; "
			}
			listing += fmt.Sprintf("%d that Handline cannot name", unnamed)
		}
		if listing == "" {
			listing = "something Handline cannot name"
		}
		text := fmt.Sprintf("Background work has been running for %s: %s. Stop what you no longer need, and for each one you keep say in a line why. "+
			"If it watches for another task, stop it and set `--blocked-by T<n>` on your task instead. Only work this session started counts: "+
			"a process you detached (nohup, &, PPID 1) is not asked about. You are asked again in %s while any runs.",
			timefmt.Ago(now-since), listing, timefmt.Ago(every))
		// The event goes in before the send: its id is what separates the turn that ended before the check from the one that answers it.
		checked := must(db.AddEvent(t.Q, agent, "background_checked", db.KV{K: "jobs", V: len(jobs)}, db.KV{K: "unnamed", V: unnamed}))
		told := false
		func() {
			defer func() {
				if !told { // nothing sent, or the send raised: no check was asked
					_, _ = t.Q.Exec("DELETE FROM events WHERE id = ?", checked)
				}
			}()
			told = b.tell(t, agent, text, now)
		}()
		if told {
			check(db.SetCursor(t.Q, key, now))
			t.logf("  %s: background work for %s - asked which to stop", agent, timefmt.Ago(now-since))
		}
	}
}

// SweepForgottenResolveCapped: the nudge about a forgotten head stops once its schedule is used up rather than
// nudging the agent forever - an agent that turns out to be genuinely stuck, not forgetful, still needs someone to look, so the head's
// task opener (else its supervisor) is told once instead. Cleared as soon as it no longer applies, so a later head that is also forgotten and also runs out the
// schedule is told again.
func (b *Timebox) SweepForgottenResolveCapped(t *Tick, now int64) {
	for _, agent := range t.Host.Agents() {
		toldKey := "forgotten_capped_told:" + agent
		found := b.Ports.ForgottenResolve(t.Q, agent, must(db.GetAgentState(t.Q, agent)), now)
		if found == nil || found.Blocked || !found.Exhausted {
			check(db.ClearCursor(t.Q, toldKey))
			continue
		}
		if must(db.Cursor(t.Q, toldKey)) == found.HeadID {
			continue
		}
		var tid int64
		if found.Task != nil {
			tid = found.Task.ID
		}
		who := b.answerer(t, agent, tid)
		// The founder's screens show no message id: told to them, the message is named by its task or not at all.
		name, read := db.MsgName(t.Q, found.HeadID), ""
		if !actor.IsPerson(who) {
			read = fmt.Sprintf("`hl read %s`; ", name)
		}
		on := name
		switch {
		case found.Task != nil && actor.IsPerson(who):
			on = fmt.Sprintf("T%d", found.Task.ID)
		case found.Task != nil:
			on = fmt.Sprintf("T%d (%s)", found.Task.ID, name)
		case actor.IsPerson(who):
			on = "a message"
		}
		after := fmt.Sprintf("after %d reminders", b.Ports.NudgeScheduleLen())
		more := ""
		if found.Behind > 0 {
			more = fmt.Sprintf(" with %d more waiting behind it", found.Behind)
		}
		var look string
		if found.Task != nil {
			look = fmt.Sprintf("Look at it: %stake it back with `hl task release T%d \"<reason>\"`, or ask %s by message (`hl say %s \"...\"`).",
				read, found.Task.ID, agent, agent)
		} else {
			look = fmt.Sprintf("Look at it: %smessage %s (`hl say %s \"...\"`).", read, agent, agent)
		}
		text := fmt.Sprintf("%s has been idle %s on %s %s%s. %s", agent, timefmt.Ago(found.IdleS), on, after, more, look)
		if b.tell(t, who, text, now) {
			check(db.SetCursor(t.Q, toldKey, found.HeadID))
			must(db.AddEvent(t.Q, agent, "forgotten_resolve_capped", db.KV{K: "request_id", V: found.HeadID}, db.KV{K: "idle_s", V: found.IdleS}, db.KV{K: "told", V: who}))
			t.logf("  %s: idle %s on request %d, nudge cap reached - %s told", agent, timefmt.Ago(found.IdleS), found.HeadID, who)
		}
	}
}

// SweepHeldTaskAfterClose: an agent whose session closed while it held a doing task that waits on nothing has no new delivery to wake it, so
// nothing would ever tell it to act. It is woken once per task with a say that asks it to do the task, hand it on, mark what blocks it or put
// the founder on it, and carries the notices waiting for it. If the session the ask woke closes with nothing done on the task (no memo, no
// turn or command of its own beyond the ask's), the task goes to whom it answers to, so the tasks queued behind it move. An agent that did
// work is asked anew at a later close.
func (b *Timebox) SweepHeldTaskAfterClose(t *Tick, now int64) {
	for _, agent := range t.Host.Agents() {
		row := must(db.GetAgentState(t.Q, agent))
		if row == nil || !row.ClosedAt.Valid || row.ClosedAt.Int64 == 0 {
			continue
		}
		tid, has := must2(db.CurrentTask(t.Q, agent))
		task := must(db.GetTask(t.Q, tid))
		if !has || task == nil || task.Acting != agent || !b.Ports.TaskDoing(t.Q, task) || must(db.TaskWaits(t.Q, tid)) {
			continue
		}
		key := fmt.Sprintf("held_asked:%d", tid)
		asked := must(db.Cursor(t.Q, key))
		// closed again since the ask: if the session the ask woke did something on the task, the ask is answered, and a later close asks anew
		if asked != 0 && row.ClosedAt.Int64 > asked && b.workedSince(t, agent, tid, asked) {
			check(db.ClearCursor(t.Q, key))
			asked = 0
		}
		if asked == 0 {
			text := fmt.Sprintf("You hold T%d (%s) and your session was closed.%s Do it, hand it on (`hl task handoff T%d <agent> \"<note>\"`), "+
				"mark what blocks it (`hl task edit T%d --blocked-by T<n>`), or put the founder on it if only they can decide (`hl task comment T%d add \"<the decision, and what you recommend>\" --turn human`).",
				tid, shortTitle(task.Title), behindLine(t, agent, tid), tid, tid, tid)
			check(b.Ports.SayNow(t.Q, agent, text))
			check(db.SetCursor(t.Q, key, now))
			t.logf("  %s: closed holding doing T%d - asked to act on it", agent, tid)
			continue
		}
		if row.ClosedAt.Int64 <= asked {
			continue
		}
		to := task.AnswersTo()
		if to == "" || to == agent {
			to = b.boss(agent)
		}
		if actor.IsPerson(to) {
			continue // a task is not handed to the founder
		}
		note := "held after being asked"
		var behind []string
		for _, o := range must(db.OpenTasksOf(t.Q, agent)) {
			if o.ID != tid {
				behind = append(behind, fmt.Sprintf("T%d", o.ID))
			}
		}
		if len(behind) > 0 {
			note += "; queue behind it: " + strings.Join(behind, ", ")
		}
		retry := fmt.Sprintf("held_handoff_try:%d", tid)
		if now-must(db.Cursor(t.Q, retry)) < HandOffRetryS {
			continue
		}
		check(db.SetCursor(t.Q, retry, now))
		if err := b.Ports.HandOffHeld(t.Q, tid, to, note); err != nil {
			t.logf("  %s: T%d held after being asked, handing it to %s failed: %v", agent, tid, to, err)
			continue
		}
		t.logf("  %s: still holding T%d after being asked - handed to %s", agent, tid, to)
	}
}

// workedSince: the agent did something on the task since the stamp - a memo, or beyond the ask's own turn (one prompt, one stop) any turn
// or command of its own.
func (b *Timebox) workedSince(t *Tick, agent string, tid, since int64) bool {
	if must(db.MemoCountBy(t.Q, tid, agent, float64(since))) > 0 {
		return true
	}
	return must(db.ActedSince(t.Q, agent, since))
}

// Sweep is timebox.sweep: the notices in order. Each one's failure stops the rest, as an exception did.
func (b *Timebox) Sweep(t *Tick, now int64) error {
	return guard(func() {
		b.SweepTaskTimeouts(t, now)
		b.SweepQueuedBlockers(t, now)
		b.SweepHeldTaskAfterClose(t, now)
		b.SweepStaleBackground(t, now)
		b.SweepForgottenBackground(t, now)
		b.SweepBackgroundCheck(t, now)
		b.SweepForgottenResolveCapped(t, now)
	})
}
