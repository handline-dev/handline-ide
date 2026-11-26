package taskwrites

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/handline/handline/go/kit/addr"

	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/requestref"
	"github.com/handline/handline/go/taskboard/taskresult"
)

var shaLike = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)

// doneMethod is task.done (actions.mark_task_done): the assignee's, the owner's (who it answers to, the opener unless named) or the founder's call, with exactly one of a merge sha on the repo's default branch, a
// request already answered, or a one-line fact. Done closes the task: a review is the task handed to its reviewer before the done.
func doneMethod(home string, tell Teller, decide CardDecider) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		tid, err := taskID(p)
		if err != nil {
			return nil, err
		}
		var evidence *string
		if v := p["evidence"]; v != nil {
			s, ok := v.(string)
			if ok && strings.TrimSpace(s) == "" && c.Caller.Role == "owner" && p["request"] == nil {
				s = "Done" // the founder needs no evidence
			} else if !ok || strings.TrimSpace(s) == "" {
				return nil, badParam("evidence is text and not empty", "evidence")
			}
			evidence = &s
		} else if p["request"] == nil && c.Caller.Role == "owner" {
			evidence = new(string)
			*evidence = "Done"
		}
		var request *int64
		if p["request"] != nil {
			request = new(int64) // resolved in the write's transaction
		}
		if (evidence == nil) == (request == nil) {
			return nil, badParam("done takes exactly one of evidence or request", "evidence")
		}
		forceWords, err := optionalBool(p, "reason")
		if err != nil {
			return nil, err
		}
		in := DoneIn{Task: tid}
		if in.Commit, in.Shots, err = resultParams(p); err != nil {
			return nil, err
		}
		by := c.Caller.Actor()
		var sha, words string
		if evidence != nil {
			if !forceWords && shaLike.MatchString(*evidence) {
				sha = *evidence
			} else {
				words = *evidence
			}
		}
		repo := ""
		if v := p["repo"]; v != nil {
			s, ok := v.(string)
			if !ok {
				return nil, badParam("repo is text", "repo")
			}
			repo = s
		}
		if repo == "" && (sha != "" || in.Commit != "") {
			repo = team.NewRoster(home).RepoOf(by)
		}
		if (sha != "" || in.Commit != "") && repo == "" {
			return nil, badParam("a sha is checked against a repo: name one", "repo")
		}
		// git is asked about the sha and the commit here, before the write: the transaction below holds no git process.
		in.Sha, in.Words, in.Request, in.Repo = sha, words, request, repo
		in = in.Looked()
		var done *DoneResult
		var decided func()
		out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			if request != nil {
				var err error
				*request, err = requestref.Resolve(tx, p["request"], requestref.Viewer(c), "request")
				if name, missing := requestref.Missing(err); missing {
					return nil, refusal("no request #" + name)
				}
				if err != nil {
					return nil, err
				}
			}
			if decide != nil && by == actor.OwnerKey && words != "" && in.Commit == "" && len(in.Shots) == 0 {
				handled, said, then, err := decide(tx, tid, words)
				if err != nil {
					return nil, err
				}
				if handled {
					decided = then
					return accepted(said, nil), nil
				}
			}
			res, err := MarkDone(tx, home, by, in)
			if err != nil {
				return nil, err
			}
			done = res
			return accepted(res.Said, nil), nil
		})
		if err == nil && done != nil && done.Keep != nil {
			if kerr := done.Keep(); kerr != nil {
				return nil, fmt.Errorf("task T%d is done, but its screenshots could not be put in place: %w", tid, kerr)
			}
		}
		if err == nil && decided != nil {
			decided()
		}
		if err == nil && done != nil && tell != nil {
			tell.Done(tid, by, done.Owner, done.Told, done.Finished)
		}
		return out, err
	}
}

// CardDecider ends the founder's own decision task through its card while that card is still queued, so what waits on the answer (a trust
// proposal) runs. handled is false for any other task, which MarkDone then ends; then runs once the call has committed.
type CardDecider func(tx *sql.Tx, tid int64, words string) (handled bool, said string, then func(), err error)

// DoneIn is what MarkDone is given: exactly one of Sha (checked against Repo's default branch), Request (already answered) or Words.
type DoneIn struct {
	Task       int64
	Sha, Words string
	Request    *int64
	Repo       string
	// Commit is the sha the work was done at (it need not have landed); Shots are absolute paths of PNG files. Both are optional.
	Commit string
	Shots  []string
	// TurnPassed is a review turn's pass closing the task (task.turn.pop): its turn's event says pass, not cleared.
	TurnPassed bool
	// looked is what git said of Sha and Commit in Repo, when the caller asked before its transaction (Looked); nil has MarkDone ask.
	looked *shaLook
}

// shaLook is what git says of a done's Sha and Commit in its Repo: what the refusals and the kept result are made of. Asking takes
// several git processes, each with its own time limit, in a repo the caller names, so it is done with no transaction open.
type shaLook struct {
	sha, commit, repo string // what was asked: an answer is used only for the same question
	// full is the whole sha Sha names when the repo's default branch reaches it; else refusal says why not, as the caller is told.
	full, refusal string
	// commitFull is the whole sha Commit names, "" with commitWhy when it names none, and commitOnBase whether the default branch reaches it.
	commitFull, commitWhy string
	commitOnBase          bool
}

// Looked is in with its Sha and Commit looked up in Repo now. A caller runs it before it opens the transaction it hands MarkDone, which
// then runs no git while the database is held; for an in that was not looked up, MarkDone asks git itself, in the transaction.
func (in DoneIn) Looked() DoneIn {
	in.looked = lookUp(in)
	return in
}

// look is git's answers for in: the ones it carries when they answer the shas and the repo it has now, else asked here.
func (in DoneIn) look() *shaLook {
	if lk := in.looked; lk != nil && lk.sha == in.Sha && lk.commit == in.Commit && lk.repo == in.Repo {
		return lk
	}
	return lookUp(in)
}

func lookUp(in DoneIn) *shaLook {
	lk := &shaLook{sha: in.Sha, commit: in.Commit, repo: in.Repo}
	if in.Sha == "" && in.Commit == "" {
		return lk
	}
	base := gitrun.BaseOf(in.Repo)
	if in.Sha != "" {
		var why string
		if lk.full, why = shaOnBase(in.Repo, in.Sha, base); lk.full == "" {
			if why != "" {
				why = " (" + why + ")"
			}
			if gitrun.HasOrigin(in.Repo) {
				// The check reads what the clone holds and fetches nothing: a merge made on the remote shows after the caller's own fetch.
				why += "; if it was merged on origin, run `git fetch origin` and try again"
			}
			lk.refusal = fmt.Sprintf("%s is not an ancestor of %s%s", in.Sha, base.Branch, why)
		}
	}
	if in.Commit != "" {
		lk.commitFull, lk.commitOnBase, lk.commitWhy = commitIn(in.Repo, in.Commit, base)
	}
	return lk
}

// DoneResult is what a MarkDone leaves for the telling that follows the commit. Finished is false when only a leftover request of an
// already done task was settled.
type DoneResult struct {
	Said     string
	Owner    string
	Told     []db.Notice
	Finished bool
	// Keep puts the task's screenshots in place; the caller runs it once the end has committed. nil when there are none.
	Keep func() error
}

// MarkDone is actions.mark_task_done in the caller's transaction: a refusal is the error, nothing is written then. The caller hands the
// result to Teller.Done once it has committed.
func MarkDone(tx *sql.Tx, home, by string, in DoneIn) (*DoneResult, error) {
	tid, sha, words, request := in.Task, in.Sha, in.Words, in.Request
	t, err := getTask(tx, tid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, refusal(fmt.Sprintf("no task T%d", tid))
	}
	if t.DiscardedBy.Valid && t.DiscardedBy.String != "" {
		return nil, refusal(fmt.Sprintf("task T%d is already discarded (%s)", tid, addr.Ref(t.DiscardedBy.String)))
	}
	if t.done() {
		// a delivery left queued when the task ended some other way is settled here
		reqs, err := taskRequests(tx, tid)
		if err != nil {
			return nil, err
		}
		for _, r := range reqs {
			if r.State == "queued" && (by == t.Owner || by == t.Acting || by == t.reporter() || by == actor.OwnerKey) {
				notes, err := endTask(tx, home, tid, by, t, doneText(tid, by, t.DoneEvidence.String), delivery.Failed(t.DoneEvidence.String), nil)
				if err != nil {
					return nil, err
				}
				return &DoneResult{Said: fmt.Sprintf("task T%d is already done (%s); its leftover request is settled", tid, t.DoneEvidence.String),
					Owner: t.Owner, Told: notes}, nil
			}
		}
		return nil, refusal(fmt.Sprintf("task T%d is already done (%s)", tid, t.DoneEvidence.String))
	}
	if ok, err := mayClose(tx, by, t); err != nil {
		return nil, err
	} else if !ok {
		return nil, refusal(fmt.Sprintf("task T%d can only be marked done by its assignee (%s), owner (%s), its parent's owner or the founder", tid, addr.Ref(t.Owner), addr.Ref(t.reporter())))
	}
	in.looked = in.look() // already asked by a caller that used Looked; else git is asked here, as it always was
	var mark, via string
	switch {
	case sha != "":
		if in.looked.full == "" {
			return nil, refusal(in.looked.refusal)
		}
		mark, via = in.looked.full, "sha"
	case request != nil:
		var state string
		switch err := tx.QueryRow("SELECT state FROM requests WHERE id = ?", *request).Scan(&state); {
		case err == sql.ErrNoRows:
			return nil, refusal("no such message")
		case err != nil:
			return nil, err
		}
		if state == "queued" {
			return nil, refusal(fmt.Sprintf("request %s is still queued - not answered yet", db.MsgName(tx, *request)))
		}
		mark, via = fmt.Sprintf("request %s", db.MsgName(tx, *request)), "request"
	default:
		mark, via = words, "reason"
	}
	kids, err := openChildren(tx, tid)
	if err != nil {
		return nil, err
	}
	if len(kids) > 0 {
		return nil, openChildrenRefusal(tx, tid, kids)
	}
	blockers, err := taskBlockers(tx, tid)
	if err != nil {
		return nil, err
	}
	if len(blockers) > 0 && by != actor.OwnerKey {
		var waits []int64
		for _, b := range blockers {
			waits = append(waits, b.ID)
		}
		return nil, refusal(fmt.Sprintf("task T%d is waiting on %s - it is done only once they are", tid, taskList(waits)))
	}
	if sha != "" || in.Commit != "" {
		if why, err := reviewMissing(tx, by, t); err != nil {
			return nil, err
		} else if why != "" {
			return nil, refusal(why)
		}
	}
	result, finalize, err := doneResult(tx, home, by, t, in, via, mark)
	if err != nil {
		return nil, err
	}
	from, err := state(tx, t)
	if err != nil {
		return nil, err
	}
	won, err := db.MarkTaskDone(tx, tid, mark, via)
	if err != nil {
		return nil, err
	}
	if !won {
		return nil, refusal(fmt.Sprintf("task T%d was marked done by someone else just now", tid))
	}
	if err := addEvent(tx, by, "mark_task_done", kv{"task_id", tid}, kv{"done", mark}, kv{"from", from}); err != nil {
		return nil, err
	}
	if !result.Empty() {
		if err := taskresult.Save(tx, tid, result); err != nil {
			return nil, err
		}
	}
	var also []string
	top := ""
	if in.TurnPassed {
		top = db.TurnPass
	}
	stack, err := clearStack(tx, tid, by, top)
	if err != nil {
		return nil, err
	}
	if len(stack) > 0 { // ended under turns: everyone on the stack hears it, the owner below them too
		also = append(append(also, stack...), t.Owner)
	}
	notes, err := endTask(tx, home, tid, by, t, doneText(tid, by, mark), delivery.Failed(mark), also)
	if err != nil {
		return nil, err
	}
	return &DoneResult{Said: fmt.Sprintf("task T%d done (%s)", tid, mark), Owner: t.Acting, Told: notes, Finished: true, Keep: finalize}, nil
}

// doneText is what a task's end says to whoever it tells: "T<n> done by <by>: <evidence on one line>".
func doneText(tid int64, by, evidence string) string {
	who := addr.Ref(by)
	if by == actor.OwnerKey {
		who = "the founder"
	}
	return strings.TrimRight(fmt.Sprintf("T%d done by %s: %s", tid, who, strings.Join(strings.Fields(evidence), " ")), ": ")
}

// mayClose is whether by may mark t done: its assignee, owner, the founder, or the owner of its parent task.
func mayClose(tx *sql.Tx, by string, t *task) (bool, error) {
	if by == t.Owner || by == t.Acting || by == t.reporter() || by == actor.OwnerKey {
		return true, nil
	}
	if t.Parent.Valid {
		p, err := getTask(tx, t.Parent.Int64)
		if err != nil {
			return false, err
		}
		if p != nil && (by == p.Owner || by == p.reporter()) {
			return true, nil
		}
	}
	return false, nil
}

// shaOnBase is the full sha `sha` names in repo when base, its default branch, reaches it (origin's copy of it when the clone has one,
// else the local one), else "" and why. A sha typed by hand that never merged must not mark a task done, so this is asked before
// anything is written, and what is stored is the full sha. It asks only what the clone holds, and fetches nothing: the repo is one the
// caller names, and a fetch there would run what that repo's config says.
func shaOnBase(repo, sha string, base gitrun.Base) (full, why string) {
	r := gitrun.Run(repo, "rev-parse", "--verify", "--quiet", sha+"^{commit}")
	if r.RC < 0 {
		return "", r.Stderr
	}
	if r.RC != 0 {
		return "", "no such commit in this repo"
	}
	full = strings.TrimSpace(r.Stdout)
	if ok, why := base.Reaches(repo, full); !ok {
		if ref := base.Ref(); why == "" && ref != base.Branch {
			why = "checked " + ref + ", which a push there moves" // origin's copy, not the clone's own branch
		}
		return "", textsafe.Oneline(why)
	}
	return full, ""
}

// openChildrenRefusal refuses a parent's done while children are open, naming each child (number, state, assignee, title) and the
// commands that close them, so the one refused can clear them from the refusal itself; data.children carries the same for the app.
func openChildrenRefusal(tx *sql.Tx, tid int64, kids []int64) error {
	lines := []string{fmt.Sprintf("task T%d still has open child tasks - it is done once they are closed:", tid)}
	var children []map[string]any
	for _, id := range kids {
		t, err := db.GetTask(tx, id)
		if err != nil {
			return err
		}
		state, err := db.TaskState(tx, t)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("  T%d  %s  %s  %s", id, state, addr.Ref(t.Owner), t.Title))
		children = append(children, map[string]any{"task": id, "state": state, "assignee": t.Owner, "title": t.Title})
	}
	hint := fmt.Sprintf("close each: `hl task discard T%d \"<reason>\"` for one not needed (a duplicate of T%d, say), or its assignee's `hl task done T%d` for one finished", kids[0], tid, kids[0])
	err := rpc.Refused(strings.Join(append(lines, hint), "\n"), "open_children", hint)
	err.Data["children"] = children
	return err
}
