package supervisor

import (
	"fmt"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
)

// TellAbout tells to, chosen by db.NoticeTo or db.AgentNoticeTo, text about agent and its task tid (0 for none). An agent gets a notice,
// and with a task the founder hears once if that task has not moved an hour later (SweepNoticeWatches). The founder, or an agent that is
// not registered, gets it as a warning.
func (e *Env) TellAbout(q db.Q, to, agent string, tid int64, text, reason string, now int64) (err error) {
	defer Catch(&err)
	if actor.IsPerson(to) || !e.registered(to) {
		check(db.NoteFounderNotice(q, db.FounderNotice{Agent: agent, Task: tid, Reason: reason, Text: text, At: now}))
		return nil
	}
	rid := must(e.Notice(q, to, text, reason))
	if tid != 0 {
		check(db.LinkRequest(q, tid, rid))
		check(db.WatchNotice(q, tid, to, text, now))
	}
	return nil
}

// SweepNoticeWatches tells the founder, once, of each task notice whose receiver has not moved the task in an hour, and drops the founder
// notices that are over.
func (e *Env) SweepNoticeWatches(now int64) (err error) {
	defer Catch(&err)
	q := db.Q(e.DB)
	check(db.ClearOverFounderNotices(q))
	for _, w := range must(db.DueNoticeWatches(q, now)) {
		check(db.NoteFounderNotice(q, db.FounderNotice{Agent: w.To, Task: w.Task, Reason: "unmoved", At: now,
			Text: fmt.Sprintf("T%d: told %s, no movement in 1h: %s", w.Task, addr.Ref(w.To), w.Text)}))
	}
	return nil
}
