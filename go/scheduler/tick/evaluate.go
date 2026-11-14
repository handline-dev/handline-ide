package tick

import (
	"cmp"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/actor"
)

// Result is one tick: when it ran and what it did.
type Result struct {
	Now     int64
	Actions []string
}

// Tick is daemon/evaluate.py: what one tick decides for each agent and the passes after them. Its fields are the ports; the unexported
// ones are the state Python kept in module variables, which a port must keep per process or its log lines differ after the first tick.
type Tick struct {
	Q   db.Q
	Log func(string)
	Now func() int64

	Host      Host
	Usage     Usage
	Delivery  Delivery
	Messaging Messaging
	Lifecycle Lifecycle
	Handoff   Handoff
	Login     Login
	Reconcile Reconciler
	Sweeps    Sweeps
	Passes    []NamedSweep // the guarded passes after the per-agent turns, in order
	Observe   Observe
	Config    Config
	Cost      *CostPilot

	said, saidClose map[string]string
	rolloverDay     string
	spans           []span // this tick's stages and what each took (timed)
}

// NamedSweep is one of the tick's guarded passes: its name is how a failure is logged.
type NamedSweep struct {
	Name string
	Run  func(q db.Q, now int64) error
}

func (t *Tick) logf(format string, a ...any) { t.Log(fmt.Sprintf(format, a...)) }

// check and must turn a database error inside one agent's turn into that agent's failure, caught in Run.
type failure struct{ err error }

func check(err error) {
	if err != nil {
		panic(failure{err})
	}
}

func must[T any](v T, err error) T {
	check(err)
	return v
}

func must2[A, B any](a A, b B, err error) (A, B) {
	check(err)
	return a, b
}

func must3[A, B, C any](a A, b B, c C, err error) (A, B, C) {
	check(err)
	return a, b, c
}

// guard runs f and returns the error it panicked with through check, nil otherwise.
func guard(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if fl, ok := r.(failure); ok {
				err = fl.err
			} else if e, ok := r.(error); ok {
				err = e // a port that fails by panicking is this agent's failure, as any exception was in Python
			} else {
				err = fmt.Errorf("panic: %v", r)
			}
		}
	}()
	f()
	return nil
}

func (t *Tick) init() {
	if t.said == nil {
		t.said, t.saidClose = map[string]string{}, map[string]string{}
	}
}

// QueuedOrIdle writes QUEUED when something has not reached the agent yet, IDLE otherwise - and returns which. Shared by every writer of a
// due wake_at, so QUEUED stays a claim about the queue everywhere.
func QueuedOrIdle(q db.Q, agent, when string) (string, error) {
	n, err := db.UnreachedCount(q, agent)
	if err != nil {
		return "", err
	}
	if n > 0 {
		return states.Queued, db.SetAgentState(q, agent, states.Queued, 0, when)
	}
	return states.Idle, db.SetAgentState(q, agent, states.Idle, 0, when+"; no requests waiting")
}

// confirmedReading is the reading, unless the running process is on another account: the column names where a move sent the agent, the
// process is where it is, so the process wins and the column is set right.
func (t *Tick) confirmedReading(agent string, reading *AccountReading) *AccountReading {
	actual := t.Host.RunningAccount(agent)
	if reading == nil || actual == "" || actual == reading.Account {
		return reading
	}
	t.logf("  %s: recorded on %s but its process is on %s - judging it there", agent, reading.Account, actual)
	must(db.AddEvent(t.Q, agent, "account_mismatch", db.KV{K: "recorded", V: reading.Account}, db.KV{K: "running", V: actual}))
	check(db.SetAccount(t.Q, agent, actual, "recorded"))
	return &AccountReading{actual, "recorded"}
}

func (t *Tick) accountReading(agent string) *AccountReading {
	r, ok := t.Usage.AccountReading(t.Q, agent)
	if !ok {
		return nil
	}
	return &r
}

func closedAt(row *db.AgentState) bool {
	return row != nil && row.ClosedAt.Valid && row.ClosedAt.Int64 != 0
}

// Supervise is one agent's turn in a tick, its own function so that all of it sits inside the single guard in Run that treats a failure
// as this agent's failure.
func (t *Tick) Supervise(agent string, now int64, actions *[]string) {
	t.supervise(agent, now, actions, false)
}

// supervise is Supervise; afterHandoff is the second pass, run right after this tick handed the agent off: it wakes the agent and judges
// no trigger, as its reading is the old session's.
func (t *Tick) supervise(agent string, now int64, actions *[]string, afterHandoff bool) {
	t.init()
	q := t.Q
	row := must(db.GetAgentState(q, agent))
	state := states.Idle
	if row != nil {
		state = row.State
	}
	if state == states.Stalled {
		return // 8.6: only a human clears this
	}
	if !states.Evaluated[state] {
		return
	}
	// An agent mid-handoff is excluded from trigger evaluation (8.9).
	if h := must(db.InFlightFor(q, agent)); h != nil {
		return
	}
	// A clear held off by background work is settled before anything else is done for the agent.
	if t.Handoff.SettleOwedClear(q, agent) {
		return
	}
	// A person's move kept for the turn's end goes before the work that would start another turn.
	if t.Handoff.RunPendingMove(q, agent) {
		return
	}
	// Whatever a message could not type at once gets one more try every tick, RUNNING included.
	t.Messaging.LandingSweep(q, agent)
	t.Messaging.TellUndelivered(q, agent)
	t.Messaging.DeliverPending(q, agent)

	// The live reading this tick judges the agent against, not the account a resume would use.
	reading := t.confirmedReading(agent, t.accountReading(agent))
	acct := ""
	if reading != nil {
		acct = reading.Account
	}
	if t.loginExpired(agent, row, acct, actions) {
		return
	}
	st := t.Usage.ReadForAgent(agent, acct)
	ctxLimit := t.Usage.ContextLimit(agent, st.ModelID)

	// An agent whose closed session was restored in this tick is judged on the next one: st was read above, before the wake, and a closed
	// agent's snapshot is stale by construction.
	wokeNow := false

	// Wake anything whose deadline has come due, whatever its state.
	if row != nil && row.WakeAt.Valid && row.WakeAt.Int64 != 0 && row.WakeAt.Int64 <= now &&
		(state == states.Parked || state == states.Throttled || state == states.Degraded) {
		// The streak is NOT cleared here: waking is the normal end of every park. It is cleared by a handoff that actually succeeds.
		restored := closedAt(row)
		if restored {
			// A closed agent's edges are enforced by nothing else.
			check(t.Delivery.EnforceEdges(q, agent))
		}
		var woke bool
		var why string
		if restored && !((t.Host.SessionAlive(agent) || t.Lifecycle.ResumableWithoutTerminal(row)) && t.Lifecycle.WaitingFor(q, agent)) {
			// The deadline is spent and nothing came for it: a closed session is not resumed to be told nothing. It stays closed, and
			// what arrives next is what wakes it.
			woke, restored = false, false
		} else {
			woke, why = t.Lifecycle.WakeAgent(q, agent)
			if restored && woke {
				t.Delivery.NoteFailure(agent, "")
			}
		}
		if restored && !woke {
			t.Delivery.NoteFailure(agent, "could not be woken - "+why)
			// For an agent whose session was closed, the deadline is the only thing that makes the wake due again: clearing it on a wake
			// that did not work is how a park slept through every later tick.
			t.logf("  %s: stays %s, still due - %s", agent, state, why)
			return
		}
		// The prompt has been sent by now, so the agent may already have answered it: only write QUEUED if nothing has claimed it since.
		if fresh := must(db.GetAgentState(q, agent)); fresh != nil && fresh.State != state {
			return
		}
		newState := must(QueuedOrIdle(q, agent, "wake_at reached"))
		whyWord := "nothing queued"
		if newState == states.Queued {
			whyWord = "window rolled over"
		}
		*actions = append(*actions, fmt.Sprintf("%s: %s -> %s (%s)", agent, state, newState, whyWord))
		t.logf("  %s: %s -> %s (%s)", agent, state, newState, whyWord)
		state = newState
		row = must(db.GetAgentState(q, agent))
		wokeNow = wokeNow || restored
	}

	// An agent with nowhere to run cannot be given work by anyone but a human, which is what sends an agent looking for a subagent that
	// dies with its session. The one fact the daemon needs is which repository: its workspace's folder. An answer counts as work here.
	need := t.Lifecycle.StartOrWakeNeeded(q, agent, row)
	if need == nil && !t.Lifecycle.WaitingFor(q, agent) {
		t.Lifecycle.ForgetLoadHold(agent) // the work a load hold was timing went away
	}
	if need != nil && need.Kind == "start" {
		said := t.Lifecycle.StartForWork(q, agent)
		*actions = append(*actions, said)
		row = must(db.GetAgentState(q, agent))
		if t.Host.SessionAlive(agent) {
			t.Delivery.NoteFailure(agent, "")
			delete(t.said, agent)
		} else {
			t.Delivery.NoteFailure(agent, "could not be started - "+said)
			// Once per reason: the tick comes round every few seconds and a refusal can last for hours.
			if t.said[agent] != said {
				t.said[agent] = said
				t.logf("  %s: %s and could not be started - %s", agent, need.What, said)
			}
		}
	}

	// A closed session cannot read its own inbox, so the arrival of work is what has to bring it back. Not one started a moment ago: that
	// terminal is a fresh session booting.
	if need != nil && need.Kind == "wake" {
		what := need.What
		ok, why := t.Lifecycle.WakeAgent(q, agent)
		if ok {
			t.Delivery.NoteFailure(agent, "")
			*actions = append(*actions, fmt.Sprintf("%s: woken for %s", agent, what))
		} else {
			t.Delivery.NoteFailure(agent, "could not be woken - "+why)
			*actions = append(*actions, fmt.Sprintf("%s: could not wake - %s", agent, why))
			// Said out loud, because this is the tick doing nothing about something that is waiting - and once, because two of these
			// reasons last for hours.
			if t.said[agent] != why {
				t.said[agent] = why
				t.logf("  %s: %s and could not be woken - %s", agent, what, why)
			}
		}
		if ok {
			delete(t.said, agent)
			row = must(db.GetAgentState(q, agent))
			wokeNow = true
		}
	}

	// Work that has arrived is delivered before anything else is decided. An agent woken in this tick is told on the next one. An agent
	// whose turn ended on the usage limit cannot run one: sending work to it would start a turn that fails at once.
	t.Delivery.TrackBackgroundWork(q, agent, row)
	row = must(db.GetAgentState(q, agent))
	atWall := t.Usage.AtLimit(q, agent)
	openIdle := state != states.Running && state != states.Handoff && !wokeNow && atWall == nil && !closedAt(row)
	// Short of the wall, an idle agent with work waiting is handed off to an account with room before that work starts a turn; while
	// that handoff waits for its note the work waits too, and the trigger below carries it on.
	if openIdle {
		if kind, _, kept := must3(db.NoteWaitKept(q, agent, now)); kept && kind == states.EarlyUsageTrigger {
			openIdle = false
		} else if early, dest := t.earlyTrigger(agent, reading, st); early.Fired {
			t.handOff(agent, early, dest, actions)
			// A handoff that did nothing (too soon after the last) leaves the agent as it was, and its work is delivered.
			_, _, waits := must3(db.NoteWaitKept(q, agent, now))
			after := states.Idle
			if fresh := must(db.GetAgentState(q, agent)); fresh != nil {
				after = fresh.State
			}
			if waits || after != state {
				return
			}
		}
	}
	if openIdle {
		closed := t.Lifecycle.ColdBeforeSend(q, agent, row, now, st)
		if closed != nil && *closed {
			*actions = append(*actions, fmt.Sprintf("%s: cache expired; session closed, the wake delivers", agent))
			row = must(db.GetAgentState(q, agent))
		}
		if closed == nil {
			// A task told past an idle head starts a turn: the nudge about that head waits.
			if !t.Delivery.DeliverQueued(q, agent) {
				t.Delivery.NudgeForgottenResolve(q, agent, row, now)
			}
		}
	}

	// An open session that has sat idle until its prompt cache is nearly spent is asked once to write its handoff note; the cost pilot
	// takes this over for an agent that has turned it on.
	if openIdle {
		if t.Cost.RenewalOn(t, agent) {
			t.Cost.Renew(t, agent, row, now, st)
		} else {
			t.Handoff.NoteAtCacheEdge(q, agent, now, st)
		}
	}

	// Pilot: an idle session over its fold size, between tasks, is closed fresh, after the note.
	if openIdle && t.Cost.FoldDue(t, agent, row, now, st) && !t.Handoff.NoteHold(q, agent, now) {
		ok, why := t.Cost.Fold(t, agent, now, st)
		if ok {
			*actions = append(*actions, fmt.Sprintf("%s: folded while idle", agent))
			row = must(db.GetAgentState(q, agent))
		} else if t.saidClose[agent] != why {
			t.saidClose[agent] = why
			t.logf("  %s: idle over its fold size but not folded - %s", agent, why)
		}
	}

	// Memory is the scarce resource on one laptop running a team, and an idle session spends it while earning nothing. Closing it clears
	// nothing, and waking it resumes the same session.
	if atWall == nil && t.Lifecycle.ShouldClose(q, agent, state, row, now, st) && !t.Handoff.NoteHold(q, agent, now) &&
		compactAsked(q, agent) == "" {
		ok, why := t.Lifecycle.CloseAgent(q, agent, st)
		if ok {
			*actions = append(*actions, fmt.Sprintf("%s: session closed while idle", agent))
			row = must(db.GetAgentState(q, agent))
		} else if t.saidClose[agent] != why {
			// Once per reason: a refusal that leaves no close_failed behind comes round every tick.
			t.saidClose[agent] = why
			t.logf("  %s: idle but not closed - %s", agent, why)
		}
	}

	t.Usage.Settle(q, agent)

	// Un-parks a usage park - PARKED only, and only when the handoff that put it there named a usage trigger - the moment that same
	// reading, evaluated the way the park itself was, no longer fires. Never on an assumed or unknown account: a guess is not evidence
	// enough to wake this agent into.
	if !wokeNow && !closedAt(row) && state == states.Parked &&
		reading != nil && reading.Source != "assumed" &&
		t.Host.SessionAlive(agent) {
		last := must(db.LastHandoffTrigger(q, agent))
		verdict := Evaluate(st.ShortTrusted, st.TrustedCtxUsed, st.LongTrusted, ctxLimit)
		movable := last != "" && t.Handoff.Movable(last)
		// Still at the wall here, but another account may have rolled over or freed a slot since the park: if the work is worth a cold
		// start, move now rather than wait out this account's own window.
		if movable && verdict.Fired && t.Handoff.Movable(verdict.Kind) {
			if dest, _ := t.Handoff.MovePlan(q, agent, verdict.Kind); dest != "" {
				moved := t.Handoff.MoveToRoom(q, agent, verdict)
				if moved {
					*actions = append(*actions, fmt.Sprintf("%s: moved off a parked account", agent))
					return
				}
			}
		}
		if movable && !verdict.Fired {
			newState := must(QueuedOrIdle(q, agent, "usage recovered"))
			*actions = append(*actions, fmt.Sprintf("%s: %s -> %s (usage recovered)", agent, state, newState))
			t.logf("  %s: %s -> %s (usage recovered)", agent, state, newState)
			row = must(db.GetAgentState(q, agent))
			state = newState
			wokeNow = true
		}
	}

	if state == states.Parked || state == states.Throttled || state == states.Degraded {
		return // waiting on wake_at, already handled above
	}

	// What sits at the other end of a closed agent is a shell prompt, and a handoff document is full of backticks. An agent woken in
	// this tick is judged on the next one: st was read before the wake and a closed agent's snapshot is stale by construction.
	if wokeNow || afterHandoff || !t.Lifecycle.TriggersApply(row) {
		return
	}

	// trusted_* is nil only when we genuinely do not know. Unknown is not a wall, so it does not fire; a quiet window reads as 0.
	trig := Evaluate(st.ShortTrusted, st.TrustedCtxUsed, st.LongTrusted, ctxLimit)
	in := triggerInput{Eval: trig, AtWall: atWall}
	in.EvalMovable = atWall != nil && trig.Fired && t.Handoff.Movable(trig.Kind)
	if _, _, kept := must3(db.NoteWaitKept(q, agent, now)); !kept {
		in.CompactBy = compactAsked(q, agent) // a note already awaited goes on as its own kind, and its handoff spends the ask
	}
	in.FreshDue = atWall == nil && !trig.Fired && t.Cost.FreshDue(t, agent, row)
	trig = chooseTrigger(in)
	// Asking whether the note's turn is over asks the agent for the note when it was not asked yet: only when nothing else fired.
	if kind, _, kept := must3(db.NoteWaitKept(q, agent, now)); kept && !trig.Fired {
		in.NoteKind, in.NoteKept = kind, true
		in.NoteHeld = t.Handoff.NoteHold(q, agent, now)
		trig = chooseTrigger(in)
	}
	if !trig.Fired && !in.NoteKept {
		check(db.ClearSavingSwitch(q, agent)) // the wall it was saving for is gone
	}
	if !trig.Fired && must(db.Cursor(q, "handoff_deferred:"+agent)) != 0 {
		check(db.ClearDeferred(q, agent)) // a trigger deferred behind a turn that no longer fires holds nothing
	}
	dest := ""
	if trig.Fired && trig.Kind == states.EarlyUsageTrigger {
		// The note is in: the account it moves to is picked again, still under the setting, or it stays and its work is delivered.
		var early Trigger
		if early, dest = t.earlyTrigger(agent, reading, st); !early.Fired {
			check(db.ClearNoteWait(q, agent))
			t.logf("  %s: early move no longer due - staying", agent)
			return
		}
	}
	if trig.Fired {
		t.handOff(agent, trig, dest, actions)
	}
}

// compactAsked is who asked this agent to compact (session.compact), "" when no one did or a handoff or a fresh session the daemon
// started came after the ask (a resume, or Claude Code's own compaction, does not count). A level read, as FreshDue is: a turn under way, a
// note not written yet or a handoff too soon after the last only put it off to a later tick.
func compactAsked(q db.Q, agent string) string {
	_, payload, ok := must3(db.LastEvent(q, agent, states.CompactAskedKind))
	if !ok {
		return ""
	}
	at, _ := must2(db.LastEventTS(q, agent, states.CompactAskedKind))
	var fresh int
	check(q.QueryRow("SELECT COUNT(*) FROM session_starts WHERE agent = ? AND resumed = 0 AND ts >= ?", agent, at).Scan(&fresh))
	if fresh > 0 {
		return ""
	}
	if h := must(db.LatestHandoff(q, agent)); h != nil && h.Updated > at {
		return ""
	}
	by, _ := payload["by"].(string)
	return cmp.Or(by, actor.OwnerKey)
}

// handOff performs the handoff trig fired, to dest when it is set, stalls an agent whose recovery keeps failing, and starts the next
// session at once when the park's wake is already due.
func (t *Tick) handOff(agent string, trig Trigger, dest string, actions *[]string) {
	q := t.Q
	ok := t.Handoff.PerformHandoff(q, agent, trig, dest)
	suffix := ""
	if !ok {
		suffix = " -> degraded"
	}
	*actions = append(*actions, fmt.Sprintf("%s: handoff (%s)%s", agent, trig.Kind, suffix))

	fresh := must(db.GetAgentState(q, agent))
	switch {
	case fresh != nil && fresh.ThrottleCount >= states.MaxThrottleStreak:
		// 8.6: raise to the founder only after repeated automatic recovery failed.
		check(db.SetAgentState(q, agent, states.Stalled, 0,
			fmt.Sprintf("%d consecutive degraded handoffs", fresh.ThrottleCount)))
		*actions = append(*actions, fmt.Sprintf("%s: STALLED after %d degraded handoffs", agent, fresh.ThrottleCount))
		t.logf("  %s: STALLED - needs a human", agent)
	case fresh != nil && fresh.FailedAttempts >= states.MaxFailedAttempts:
		check(db.SetAgentState(q, agent, states.Stalled, 0, fmt.Sprintf("%d failed attempts", fresh.FailedAttempts)))
		*actions = append(*actions, fmt.Sprintf("%s: STALLED after %d failed attempts", agent, fresh.FailedAttempts))
	}
	// A handoff parks the agent with its wake already due: start the new session now, not at the next poll.
	if fresh = must(db.GetAgentState(q, agent)); fresh != nil && fresh.State == states.Parked && fresh.WakeAt.Valid &&
		fresh.WakeAt.Int64 != 0 && fresh.WakeAt.Int64 <= t.Now() {
		t.supervise(agent, t.Now(), actions, true)
	}
}

// earlyTrigger fires once the agent's account is at usage_move_before_pct of its 5-hour window or states.MoveBeforeWeekPct of its week, and
// MovePlan picks an account under both, which is dest. Off at 0, and never on an assumed account.
func (t *Tick) earlyTrigger(agent string, reading *AccountReading, st Status) (Trigger, string) {
	pct := t.Config.MoveBeforePct()
	if pct <= 0 || reading == nil || reading.Source == "assumed" {
		return Trigger{}, ""
	}
	at := func(v *int, line int) bool { return v != nil && *v >= line }
	over := func(s Status) bool { return at(s.ShortTrusted, pct) || at(s.LongTrusted, states.MoveBeforeWeekPct) }
	if !over(st) || !t.Handoff.Movable(states.EarlyUsageTrigger) {
		return Trigger{}, ""
	}
	dest, _ := t.Handoff.MovePlan(t.Q, agent, states.EarlyUsageTrigger)
	if dest == "" || over(t.Usage.ReadForAgent(agent, dest)) {
		return Trigger{}, ""
	}
	why := fmt.Sprintf("5h window %d%% >= usage_move_before_pct %d", *st.ShortTrusted, pct)
	if !at(st.ShortTrusted, pct) {
		why = fmt.Sprintf("weekly window %d%% >= %d", *st.LongTrusted, states.MoveBeforeWeekPct)
	}
	return Trigger{true, states.EarlyUsageTrigger, why}, dest
}

// StartWaiting is, at daemon start, bringing up at once every agent that has work waiting and no session: the tick would, but only after
// its usage polls and each agent in turn, and a reboot leaves every agent in that state. It runs the sweep first, so a terminal the reboot
// killed is dropped and the agent started in its worktree.
func (t *Tick) StartWaiting() []string {
	t.init()
	var actions []string
	t.Reconcile.Sweep(t.Q, &actions)
	for _, agent := range t.Host.Agents() {
		err := guard(func() {
			row := must(db.GetAgentState(t.Q, agent))
			need := t.Lifecycle.StartOrWakeNeeded(t.Q, agent, row)
			switch {
			case need != nil && need.Kind == "start":
				actions = append(actions, t.Lifecycle.StartForWork(t.Q, agent))
			case need != nil:
				if ok, why := t.Lifecycle.WakeAgent(t.Q, agent); ok {
					actions = append(actions, agent+": woken at start")
				} else {
					actions = append(actions, fmt.Sprintf("%s: could not wake - %s", agent, why))
				}
			}
		})
		if err != nil { // one agent's failure is not the start's
			t.logf("  %s: start for waiting work failed: %v", agent, err)
		}
	}
	return actions
}

// Sweep is the reconcile pass alone, between ticks.
func (t *Tick) Sweep() {
	t.init()
	var actions []string
	t.Reconcile.Sweep(t.Q, &actions)
}

// guarded runs one of the tick's passes: its failure is logged under its name and is not the tick's failure.
func (t *Tick) guarded(name string, f func() error) {
	var err error
	t.timed(name, func() {
		if p := guard(func() { err = f() }); p != nil {
			err = p
		}
	})
	if err != nil {
		t.logf("  %s: %v", name, err)
	}
}

// Run is one tick.
func (t *Tick) Run() (Result, error) {
	t.init()
	began := time.Now()
	defer func() { t.logSlow(time.Since(began)) }()
	now := t.Now()
	var actions []string
	q := t.Q

	// Before anything is decided on what we believe, check what is true.
	t.timed("reconcile", func() { t.Reconcile.Sweep(q, &actions) })

	// And what is true includes the two readings every decision below is made on. Both are pulls (D11), so they happen here.
	var snapErr error
	t.timed("snapshots", func() { snapErr = t.Sweeps.CollectSnapshot(q) })
	if snapErr != nil {
		return Result{now, actions}, fmt.Errorf("collect snapshots: %w", snapErr)
	}
	var pollErr error
	t.timed("usage poll", func() { pollErr = t.Sweeps.PollUsage(q) })
	if pollErr != nil { // a failure outside any one account is not the tick's failure either
		t.logf("  usage: poll failed: %v", pollErr)
	}

	// Link a file added to shared/.claude into accounts registered before it.
	if t.Sweeps.RelinkAccounts != nil {
		if relinked, err := t.Sweeps.RelinkAccounts(); err != nil {
			t.logf("  relink: %v", err)
		} else {
			for _, acct := range sortedKeys(relinked) {
				t.logf("  relinked %s: %s", acct, strings.Join(relinked[acct], ", "))
			}
		}
	}

	// A handoff past its deadline must fail here, not only at startup.
	actions = append(actions, t.Reconcile.ExpireHandoffs(q)...)

	// Midnight rollover (section 5). Without it yesterday's "in progress" stays in progress forever.
	day := t.Sweeps.Today()
	if t.rolloverDay != "" && t.rolloverDay != day {
		for _, agent := range t.Host.Agents() {
			if ok, msg := t.Sweeps.Rollover(agent); ok && !strings.Contains(msg, "already exists") {
				actions = append(actions, msg)
				t.logf("  rollover: %s", msg)
			}
		}
	}
	t.rolloverDay = day

	// The human is no agent, so no per-agent delivery checks the edges of what agents wrote to them.
	t.guarded(actor.OwnerKey, func() error {
		if err := t.Delivery.EnforceEdges(q, actor.OwnerKey); err != nil {
			return fmt.Errorf("edge check failed: %w", err)
		}
		return nil
	})

	for _, agent := range t.Host.Agents() {
		var err error
		t.timed("supervise", func() { err = guard(func() { t.Supervise(agent, now, &actions) }) })
		if err != nil {
			// A real failure, not a resource shortage: bad config, unreadable status, a host call that failed. Catching it per agent is
			// the point - supervising one agent is not allowed to end supervision of every agent after it in the list.
			t.Reconcile.StallAgent(q, agent, &actions, err, "error")
		}
	}

	t.timed("blocked sweep", func() { t.Sweeps.BlockedSweep(q, now) })
	for _, s := range t.Passes {
		s := s
		t.guarded(s.Name, func() error { return s.Run(q, now) })
	}
	t.timed("observe", t.observe)
	return Result{now, actions}, nil
}

// observe records each account's reading and learns from them and the token log: the usage samples, the dollar pace and the account
// factors. After the agents, so it sees the states this tick left.
func (t *Tick) observe() {
	q := t.Q
	accounts := t.Host.Accounts()
	for _, acct := range accounts {
		st := t.Observe.ReadAccount(acct)
		if err := t.Observe.RecordUsageSample(q, acct, st); err != nil {
			t.logf("  sample: %s: %v", acct, err)
		}
	}
	if err := t.Observe.PruneUsageSamples(q); err != nil {
		t.logf("  sample: prune failed: %v", err)
	}
	for _, acct := range accounts {
		if err := t.Observe.IngestTokens(q, acct); err != nil {
			t.logf("  tokens: %s: %v", acct, err)
		}
	}
	if err := t.Observe.PruneTokens(q); err != nil {
		t.logf("  tokens: prune failed: %v", err)
	}
	if err := t.Observe.RefreshDollarPace(q); err != nil {
		t.logf("  sample: dollar pace failed: %v", err)
	}
	if err := t.Observe.RefreshFactors(q); err != nil {
		t.logf("  sample: account factors failed: %v", err)
	}
}

func sortedKeys(m map[string][]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
