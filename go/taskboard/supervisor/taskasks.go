package supervisor

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// SweepTaskAsks asks for plans and sends the asks for a plan or for core logic. With auto (the plans setting), every task opened as a draft
// that an agent is to work and that needs a plan (none written since its brief or assignee last changed, and none asked since) gets an
// ask. An ask waits until its agent is no longer doing the task it was doing when asked, and while a plan it was sent in the last
// PlanWaitS is unanswered; then it goes out as a say, one per agent per sweep. An ask that no longer applies is dropped unsent: its task
// ended or changed hands, a plan ask whose task is no longer a draft or that a plan has answered, a core ask whose task the agent is doing.
func (e *Env) SweepTaskAsks(auto bool) (err error) {
	defer Catch(&err)
	q := db.Q(e.DB)
	if auto {
		for _, t := range must(db.OpenTasks(q)) {
			if actor.IsPerson(t.Owner) || t.Owner == db.Gate || !must(db.OpenedAsDraft(q, t.ID)) || must(db.TaskState(q, t)) != "draft" ||
				!must(db.PlanDue(q, t.ID)) {
				continue
			}
			check(taskwrites.AskPlan(q, t.ID, db.NoticeRequester))
			e.log("  %s: draft T%d has no plan - asked for one", t.Owner, t.ID)
		}
	}
	held := map[string]bool{}
	for _, a := range must(db.UnsentAsks(q)) {
		if held[a.To] {
			continue
		}
		t := must(db.GetTask(q, a.Task))
		doing := must(db.DoingTask(q, a.To))
		if t == nil || t.Ended() || t.Owner != a.To || a.Kind == db.PlanAskKind && (must(db.TaskState(q, t)) != "draft" || answered(q, a)) ||
			a.Kind == db.CoreAskKind && doing == a.Task {
			check(db.MarkAskSent(q, a, 0))
			continue
		}
		held[a.To] = true // one ask per agent per sweep, and none while this one waits
		if a.Behind != 0 && doing == a.Behind || must(db.WritingPlan(q, a.To, e.now()-PlanWaitS)) {
			continue
		}
		e.sendAsk(a, t)
		e.log("  %s: sent the %s ask of T%d", a.To, strings.TrimSuffix(a.Kind, "_ask"), a.Task)
	}
	return nil
}

// PlanWaitS: a plan ask sent this long ago with no plan since no longer holds back the agent's next ask.
const PlanWaitS = 3600

// sendAsk writes the ask's say, with the notices waiting for its agent, and marks the ask sent in one transaction, then types it.
func (e *Env) sendAsk(a *db.TaskAsk, t *db.Task) {
	tx, err := e.DB.Begin()
	check(err)
	var rid int64
	func() {
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
				panic(r)
			}
		}()
		body := askText(a, t)
		if rows := must(db.PendingNotices(tx, a.To)); len(rows) > 0 {
			body += "\n\n" + DigestText(tx, rows)
			check(db.MarkNoticesDigested(tx, requestIDs(rows)))
		}
		rid = e.queueSay(tx, a.To, body, a.Kind)
		check(db.MarkAskSent(tx, a, rid))
		check(tx.Commit())
	}()
	e.Push(e.DB, a.To, rid)
}

// answered: a plan was written after the ask.
func answered(q db.Q, a *db.TaskAsk) bool {
	p := must(db.TaskPlan(q, a.Task))
	return p != nil && p.Current().ID > a.ID
}

func shortTitle(title string) string {
	title = textsafe.Oneline(title)
	if utf8.RuneCountInString(title) > TitleShown {
		return strings.TrimRight(string([]rune(title)[:TitleShown]), " ") + "…"
	}
	return title
}

// askText is what the agent is typed for an ask.
func askText(a *db.TaskAsk, t *db.Task) string {
	n, title := t.ID, shortTitle(t.Title)
	if a.Kind == db.PlanAskKind {
		return fmt.Sprintf("Write the plan for T%d (%s). Read its brief (`hl task show T%d`) and the code it touches, and change nothing: no edits, "+
			"no commits, no branch. The plan says what you will change, in which files, how you will show it done, what you leave out, and what it "+
			"starts after. Keep it with `hl task plan T%d --file <path>` (write the file outside the repo). A question only the founder can answer "+
			"goes on the task: `hl task comment T%d add \"<question>\" --turn human`. Then go back to what you were doing; T%d itself comes to "+
			"you when it is run.", n, title, n, n, n, n)
	}
	note := ""
	if a.Note != "" {
		note = " Which part: " + a.Note
	}
	return fmt.Sprintf("Write the core logic of T%d (%s): only its key function and that function's test, nothing around them.%s Read "+
		"`hl task show T%d` first. Commit them in your worktree on a branch of T%d's own, made from origin/main; do not merge or push it, "+
		"and leave the worktree on that branch, which is where its diff is read. Then say on the task what you wrote: `hl task comment T%d "+
		"add \"<what, and the branch>\"`. When T%d is run, carry on from that branch.", n, title, note, n, n, n, n)
}
