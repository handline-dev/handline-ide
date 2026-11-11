package supervisor

import (
	"fmt"
	"github.com/handline/handline/go/kit/actor"
	"sort"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/textsafe"
)

// A queued task nobody has been sent is delivered as soon as its owner is free (daemon/taskqueue.py).
//
// Delivering the next task is done at the edges (AdvanceCurrentTask after a done, a close, an unblock, a queue); an edge that finds the owner
// busy, or that never fires, leaves the task waiting for good. This reads the same picker as a level every tick, so a waiting task reaches its
// owner whatever happened to the edge.

// QueueRetryS: a delivery that landed nothing (every candidate refused) is tried again after this long. One that landed is not throttled.
const QueueRetryS = 60

// SweepTaskQueue is taskqueue.sweep.
func (e *Env) SweepTaskQueue(now int64) (err error) {
	defer Catch(&err)
	q := db.Q(e.DB)
	if _, err := e.ReleaseEndedBlockers(); err != nil {
		return err
	}
	set := map[string]bool{}
	for _, t := range must(db.OpenTasks(q)) {
		if t.QueuedBy.Valid && t.QueuedBy.String != "" && t.DoneEvidence.String == "" && !actor.IsPerson(t.Acting) {
			set[t.Acting] = true
		}
	}
	owners := make([]string, 0, len(set))
	for o := range set {
		owners = append(owners, o)
	}
	sort.Strings(owners)
	for _, agent := range owners {
		if must(db.CurrentTaskStillAsking(q, agent)) {
			if must(e.AdvanceToDormantBlocker(q, agent)) {
				e.Push(q, agent, 0)
				e.log("  %s: its current task waits on a task of its own behind it - delivered that one first", agent)
			} else {
				e.moveOffWaiting(q, agent, now)
			}
			continue
		}
		if len(must(db.WaitingTasks(q, agent))) == 0 {
			continue
		}
		key := "taskqueue:" + agent
		if now-must(db.Cursor(q, key)) < QueueRetryS {
			continue
		}
		changed, _, aerr := e.AdvanceCurrentTask(q, agent)
		if aerr != nil {
			return aerr
		}
		if changed {
			e.Push(q, agent, 0)
			e.log("  %s: a queued task was waiting and it is free - delivered", agent)
		} else {
			check(db.SetCursor(q, key, now))
		}
	}
	return nil
}

// moveOffWaiting gives an idle agent with an overdue, unanswered current task its next one, or pings the holder once per overrun notice.
func (e *Env) moveOffWaiting(q db.Q, agent string, now int64) {
	tid, ok := ok2(db.CurrentTask(q, agent))
	cur := must(db.GetTask(q, tid))
	if !ok || cur == nil || cur.Ended() {
		return
	}
	if row := stateOf(q, agent); row == nil || (row.State != states.Idle && row.State != states.Queued) {
		return
	}
	if !must(db.Overdue(q, cur, now)) {
		return
	}
	why := "past its timeout, unanswered"
	for _, w := range must(db.WaitingTasks(q, agent)) {
		if w.Task.ID != cur.ID && e.makeCurrent(q, agent, w, nil) {
			e.typeSay(q, agent, fmt.Sprintf("T%d is %s, so T%d (%s) is yours now: start it.", cur.ID, why, w.Task.ID, textsafe.Oneline(w.Task.Title)), "next task while current waits")
			e.Push(q, agent, 0)
			e.log("  %s: T%d is %s - delivered T%d", agent, cur.ID, why, w.Task.ID)
			return
		}
	}
	if key, told := fmt.Sprintf("overdue_ping:%d", cur.ID), must(db.TimeoutNotices(q, cur.ID)).LastID; must(db.Cursor(q, key)) != told {
		check(db.SetCursor(q, key, told))
		e.typeSay(q, agent, fmt.Sprintf("T%d (%s) is %s. Do it, hand it on (`hl task handoff T%d <agent> \"<note>\"`), mark what blocks it (`hl task edit T%d --blocked-by T<n>`), "+
			"or put the founder on it if only they can decide (`hl task comment T%d add \"<the decision, and what you recommend>\" --turn human`).", cur.ID, textsafe.Oneline(cur.Title), why, cur.ID, cur.ID, cur.ID), "overdue task, idle holder")
	}
}
