package taskwrites

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// The founder's review of a story's sub-task: db.Review.

// readyFor is the land_by agent an agent's handoff of task tid to `to` is held back from, as a Ready for the founder; "" for a handoff
// that goes as asked. The land_by is the one of the task's owner's workspace, whoever hands it on, and only the agent working the task
// finishes it so. An agent's handoff of a task that is Ready now is refused: only the founder's verdict moves it on.
func readyFor(tx *sql.Tx, tid int64, by, to string, reviewFlow bool) (string, error) {
	t, err := getTask(tx, tid)
	if err != nil || t == nil || actor.IsPerson(by) {
		return "", err // no task: handOff refuses it
	}
	if r, err := db.TaskReview(tx, tid); err != nil {
		return "", err
	} else if r != nil && r.Ready {
		return "", refusal(fmt.Sprintf("T%d is Ready for the founder: it waits for their OK, merge or Redo", tid))
	}
	if actor.IsPerson(to) {
		return "", nil
	}
	rules, _, err := team.AgentRules(tx, t.Owner)
	if err != nil {
		return "", err
	}
	landBy := false
	for _, r := range rules {
		landBy = landBy || r.Key == team.RuleLandBy && r.Value == to
	}
	if !landBy {
		return "", nil
	}
	if due, err := db.ReadyDue(tx, tid, reviewFlow); err != nil || !due {
		return "", err
	}
	if t.Acting != by {
		return "", refusal(fmt.Sprintf("T%d goes to the founder before %s: %s, who works it, finishes it",
			tid, addr.Ref(to), addr.Ref(t.Acting)))
	}
	return to, nil
}

// readyNow is whether task tid is open and Ready for the founder now.
func readyNow(tx *sql.Tx, tid int64) (bool, error) {
	if t, err := getTask(tx, tid); err != nil || t == nil || t.done() || t.DiscardedBy.String != "" {
		return false, err
	}
	r, err := db.TaskReview(tx, tid)
	return r != nil && r.Ready, err
}

// readyRefusal is the founder's no to any answer of a Ready task but OK, merge or Redo.
func readyRefusal(tid int64) string {
	return fmt.Sprintf("T%d is Ready for you: answer it with OK, merge or Redo", tid)
}

// readyTask is task tid's review, refused unless the task is Ready now and by may answer the founder's turn on it.
func readyTask(tx *sql.Tx, tid int64, by string) (*db.Review, error) {
	t, err := openTask(tx, tid)
	if err != nil {
		return nil, err
	}
	r, err := db.TaskReview(tx, tid)
	if err != nil {
		return nil, err
	}
	if r == nil || !r.Ready {
		return nil, refusal(fmt.Sprintf("T%d is not Ready for you: only a story's sub-task its assignee finished waits for OK or Redo", tid))
	}
	if why := turnRefusal(t, tid, by); why != "" {
		return nil, refusal(why)
	}
	return r, nil
}

// okMethod is task.ok, "OK, merge": the founder's turn on a Ready task comes off, and the task goes to the land_by agent it waited for,
// with the assignee's result as the note, as the assignee's own handoff would have.
func okMethod(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		me := c.Caller.Actor()
		var (
			h     Handover
			asked *Asked
		)
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			r, err := readyTask(tx, tid, me)
			if err != nil {
				return nil, err
			}
			_, result, err := memoOf(tx, tid, r.Memo)
			if err != nil {
				return nil, err
			}
			if _, _, _, err := db.PopTurn(tx, tid, me, db.TurnPass, "", 0, 0); err != nil {
				return nil, err
			}
			if err := db.AddVerdict(tx, tid, me, db.VerdictOK, 0); err != nil {
				return nil, err
			}
			if h, asked, err = handOff(tx, tell, tid, me, r.LandBy, "OK, merge (the founder). "+result, paths.ListAgents(home), false); err != nil {
				return nil, err
			}
			if !asked.OK { // all of it rolls back: the task stays Ready
				return nil, refusal(asked.Msg)
			}
			return accepted(fmt.Sprintf("T%d is OK: handed to %s to merge", tid, addr.Ref(r.LandBy)), nil), nil
		})
		if err != nil {
			return nil, err
		}
		h.After = asked.After
		tell.HandedOver(h)
		return out, nil
	}
}

// redoMethod is task.redo: the founder sends a Ready task back to its assignee with a note, kept as their comment.
func redoMethod(home string, tell Teller) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		note, err := optText(p, "note")
		if err != nil {
			return nil, err
		}
		if note == "" {
			return nil, badParam("a redo says what to change in note", "note")
		}
		me := c.Caller.Actor()
		var (
			popped Popped
			memo   int64
			// skipped: the task went on without the founder (an automatic OK), and is handed back from where it is now
			skipped bool
			h       Handover
			asked   *Asked
			to      string // whom the Redo goes to
		)
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			r, err := skippedTask(tx, tid, me)
			if err != nil {
				return nil, err
			}
			if skipped = r != nil; !skipped {
				if r, err = readyTask(tx, tid, me); err != nil {
					return nil, err
				}
			}
			if err := claimImages(tx, home, me, note, "", "note"); err != nil {
				return nil, err
			}
			if memo, err = db.AddMemo(tx, tid, me, note); err != nil {
				return nil, err
			}
			if err := db.AddVerdict(tx, tid, me, db.VerdictRedo, memo); err != nil {
				return nil, err
			}
			if err := redoToEvery(tx, home, r.By, tid, me); err != nil {
				return nil, err
			}
			told := fmt.Sprintf("T%d 대표님 Redo %d (댓글 %d): %s\n\nFirst keep one lesson from this note, what to do differently from now on: "+
				"`hl lesson add \"<the lesson>\" --task T%d`. Then work it again on your branch, and hand it to %s as before: it comes to the founder "+
				"first again.", tid, r.Redo+1, memo, note, tid, addr.Ref(r.LandBy))
			if skipped {
				if h, asked, err = handOff(tx, tell, tid, me, r.By, told, paths.ListAgents(home), false); err != nil {
					return nil, err
				}
				if !asked.OK {
					return nil, refusal(asked.Msg)
				}
				to = r.By
				return accepted(fmt.Sprintf("T%d sent back to %s: Redo %d", tid, addr.Ref(r.By), r.Redo+1), map[string]any{"comment": memo}), nil
			}
			if popped, err = popTurnTold(tx, home, tell, tid, me, db.TurnSendBack, note, memo, told); err != nil {
				return nil, err
			}
			if popped.Asked != nil && !popped.Asked.OK { // all of it rolls back: the task stays Ready
				return nil, refusal(popped.Asked.Msg)
			}
			return accepted(fmt.Sprintf("T%d sent back to %s: Redo %d", tid, addr.Ref(popped.To), r.Redo+1), map[string]any{"comment": memo}), nil
		})
		if err != nil {
			return nil, err
		}
		if skipped {
			h.After = asked.After
			tell.HandedOver(h)
		} else {
			popped.tell(tell)
			to = popped.To
		}
		tell.MemoChanged(tid, memo, me, "added", note, false, []string{to}) // the task's return carries the note
		return out, nil
	}
}

// skippedTask is task tid's review when it went on without the founder (its newest verdict an automatic OK) and by is a person: the
// founder may Redo it while its land_by agent still holds it. nil when it never skipped them.
func skippedTask(tx *sql.Tx, tid int64, by string) (*db.Review, error) {
	if !actor.IsPerson(by) {
		return nil, nil
	}
	r, err := db.TaskReview(tx, tid)
	if err != nil || r == nil || r.Ready || !r.OK || !r.Auto {
		return nil, err
	}
	t, err := getTask(tx, tid)
	switch {
	case err != nil || t == nil:
		return nil, err
	case t.done() || t.DiscardedBy.String != "":
		return nil, refusal(fmt.Sprintf("T%d went on without you and is closed now: open a new task for what to change", tid))
	case t.Acting != r.LandBy:
		return nil, refusal(fmt.Sprintf("T%d went on without you and %s holds it now: answer that first, or say what to change in a comment", tid, addr.Ref(t.Acting)))
	}
	return r, nil
}

// TaskWorks are the kinds of work agent's commits naming task tid are, in the repo it works in: none when they cannot be read, which is
// looked at every time.
func TaskWorks(home, agent string, tid int64) []string {
	cfg, err := team.NewRoster(home).Read(agent)
	repo, _ := cfg["repo"].(string)
	if err != nil || repo == "" {
		return nil
	}
	files, err := gitrun.TaskFiles(repo, tid)
	if err != nil {
		return nil
	}
	return db.WorksOf(files)
}

// redoToEvery puts the agent that finished the task back to every at each kind of work the task is, at once, with the task on the row.
func redoToEvery(tx *sql.Tx, home, agent string, tid int64, by string) error {
	model, err := team.NewRosterOn(tx, home).Model(agent)
	if err != nil {
		return err
	}
	works, err := db.ReadyWorks(tx, tid)
	if err != nil {
		return err
	}
	for _, w := range works {
		l, err := db.ReviewLevel(tx, agent, w, model)
		if err != nil {
			return err
		}
		if l.Level != db.LevelEvery {
			if err := db.SetReviewLevel(tx, agent, w, model, db.LevelEvery, by, "redo", tid); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkReport is what the assignee's handoff of a finished sub-task says of its own check: the lessons it checked, what the fresh look
// missed, and whether the tests passed.
type checkReport struct {
	checked, misses, tests string
}

func reportOf(p map[string]any) (checkReport, error) {
	var r checkReport
	for _, f := range []struct {
		key string
		to  *string
	}{{"checked", &r.checked}, {"misses", &r.misses}, {"tests", &r.tests}} {
		v, err := optText(p, f.key)
		if err != nil {
			return r, err
		}
		*f.to = strings.TrimSpace(v)
	}
	if r.tests != "" && r.tests != "pass" && r.tests != "fail" {
		return r, badParam("tests is pass or fail", "tests")
	}
	return r, nil
}

// fields are the report as the Ready event keeps it, with what the record and the sampling count by.
func (r checkReport) fields(maySkip bool, works []string, model, judgedBy string) []db.KV {
	out := []db.KV{{K: "may_skip", V: maySkip}, {K: "works", V: works}, {K: "model", V: model}, {K: "judged_by", V: judgedBy}}
	for _, f := range [][2]string{{"checked", r.checked}, {"misses", r.misses}, {"tests", r.tests}} {
		if f[1] != "" {
			out = append(out, db.KV{K: f[0], V: f[1]})
		}
	}
	return out
}

// skipsFounder is whether agent's finish of task tid goes on without the founder, by the strictest of its levels at works (judged, its
// kind). Only a clean check, passing tests and no Redo may (maySkip); at 1 in n the first since the level and every n-th after still go.
func skipsFounder(tx *sql.Tx, tid int64, agent string, rep checkReport, works []string, model string) (maySkip, skip bool, level, judged string, err error) {
	r, err := db.TaskReview(tx, tid)
	if err != nil {
		return false, false, "", "", err
	}
	maySkip = rep.checked != "" && rep.misses == "" && rep.tests == "pass" && (r == nil || r.Redo == 0)
	levels := make([]db.LevelAt, len(works))
	for i, w := range works {
		if levels[i], err = db.ReviewLevel(tx, agent, w, model); err != nil {
			return false, false, "", "", err
		}
	}
	l := db.Strictest(levels)
	if !maySkip {
		return false, false, l.Level, l.Work, nil
	}
	n := db.LevelEach(l.Level)
	switch n {
	case 0:
		return true, true, l.Level, l.Work, nil
	case 1:
		return true, false, l.Level, l.Work, nil
	}
	done, err := db.MaySkipCount(tx, agent, l.Work, model, l.After)
	return true, done%n != 0, l.Level, l.Work, err
}
