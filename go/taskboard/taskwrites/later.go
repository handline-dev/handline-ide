package taskwrites

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// heldDone is an agent's done held behind its task's later steps, kept on the person's turn and run again as that agent on its pass.
// Again is a done the pass cannot run in its place (one that links a pull request or carries screenshots): the task goes back to the
// agent to finish.
type heldDone struct {
	Sha, Words, Repo, Commit string
	Request                  *int64 `json:",omitempty"`
	Again                    bool   `json:",omitempty"`
	// what git said of Sha and Commit when the done was asked, so the pass runs no git with the database held
	Full, Refusal, CommitFull, CommitWhy, Branch string
	CommitOnBase                                bool
}

func (d heldDone) in(tid int64) DoneIn {
	in := DoneIn{Task: tid, Sha: d.Sha, Words: d.Words, Request: d.Request, Repo: d.Repo, Commit: d.Commit, TurnPassed: true}
	in.looked = &shaLook{sha: d.Sha, commit: d.Commit, repo: d.Repo, full: d.Full, refusal: d.Refusal, commitFull: d.CommitFull,
		commitWhy: d.CommitWhy, branch: d.Branch, commitOnBase: d.CommitOnBase}
	return in
}

// laterHold is a done or handoff held behind the person its task's open later comments are for.
type laterHold struct {
	h    Handover
	memo int64
	text string
	said string
	// note is a review's pass note held with it (0 for none), told as a comment of its own
	note     int64
	noteText string
	noteBy   string
}

// holdForLater holds agent by's done (onPass finish, evidence its heldDone) or handoff (onPass handoff to passTo, evidence its note) of
// task tid while later comments are open on it and by acts on it: one comment by by lists the open ones of the person the oldest is for,
// and that person gets the turn hung on it. nil when nothing is held.
func holdForLater(tx *sql.Tx, tell Teller, tid int64, by, onPass, evidence, passTo string) (*laterHold, error) {
	t, err := getTask(tx, tid)
	if err != nil || t == nil || t.Acting != by {
		return nil, err // another closer, or a person's turn already on top: as before
	}
	person, err := db.LaterFor(tx, tid)
	if err != nil || person == "" {
		return nil, err
	}
	if in, err := db.InStack(tx, tid, person); err != nil || in {
		return nil, err // the person owns it or has a turn below: they are on it already
	}
	open, err := db.OpenLaterMemos(tx, tid, person)
	if err != nil {
		return nil, err
	}
	what := "closes it as done"
	if onPass == db.TurnOnPassHandoff {
		what = "hands it to " + addr.Ref(passTo)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Before T%d goes on, the steps only you can do:\n", tid)
	for _, m := range open {
		fmt.Fprintf(&b, "- comment %d: %s\n", m.ID, textsafe.Oneline(m.Body))
	}
	fmt.Fprintf(&b, "Pass %s; a comment sends it back to %s.", what, addr.Ref(by))
	text := b.String()
	memo, err := db.AddMemo(tx, tid, by, text)
	if err != nil {
		return nil, err
	}
	h, asked, err := pushTurnPassing(tx, tell, tid, by, person, text, memo, onPass, evidence, passTo)
	if err != nil {
		return nil, err
	}
	if !asked.OK {
		return nil, refusal(asked.Msg)
	}
	said := fmt.Sprintf("T%d waits on %s first (comment %d lists %d later steps): their pass %s, their comment sends it back to you",
		tid, addr.Ref(person), memo, len(open), what)
	return &laterHold{h: h, memo: memo, text: text, said: said}, nil
}

// holdDone holds agent by's done of task tid, in as asked, behind its later steps; again when the pass cannot run it in its place.
func holdDone(tx *sql.Tx, tell Teller, tid int64, by string, in DoneIn, again bool) (*laterHold, error) {
	d := heldDone{Sha: in.Sha, Words: in.Words, Repo: in.Repo, Commit: in.Commit, Request: in.Request, Again: again}
	if lk := in.look(); lk != nil {
		d.Full, d.Refusal, d.CommitFull, d.CommitWhy, d.Branch, d.CommitOnBase = lk.full, lk.refusal, lk.commitFull, lk.commitWhy, lk.branch, lk.commitOnBase
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	return holdForLater(tx, tell, tid, by, db.TurnOnPassFinish, string(raw), "")
}

// holdHandoff holds agent by's handoff of task tid to `to` behind its later steps, refused now as handOff would refuse it at the pass.
func holdHandoff(tx *sql.Tx, tell Teller, tid int64, by, to, note string, agents []string) (*laterHold, error) {
	t, err := getTask(tx, tid)
	if err != nil || t == nil {
		return nil, err
	}
	if to, err = paths.Reaches(tx, to); err != nil {
		return nil, err
	}
	if why := handoffRefusal(t, to, agents); why != "" {
		return nil, refusal(why)
	}
	if in, err := db.InStack(tx, tid, to); err != nil {
		return nil, err
	} else if in {
		return nil, refusal(fmt.Sprintf("%s already has a turn on T%d", addr.Ref(to), tid))
	}
	return holdForLater(tx, tell, tid, by, db.TurnOnPassHandoff, note, to)
}

// tell is what a committed hold leaves: the turn handed over and its comment told.
func (l *laterHold) tell(tell Teller, tid int64, by string) {
	if l == nil || tell == nil {
		return
	}
	tell.HandedOver(l.h)
	tell.MemoChanged(tid, l.memo, by, "added", l.text, false, nil)
	if l.note != 0 {
		tell.MemoChanged(tid, l.note, l.noteBy, "added", l.noteText, false, nil)
	}
}

// passHeld runs what top, a person's turn holding its agent's done or handoff, held, as its pass by `by` with note: the done again as that
// agent once the turn is off, or the handoff. done false is a pass that gives the task back to the agent below instead: a done the pass
// cannot run, or one a gate refuses now (note then says why).
func passHeld(tx *sql.Tx, home string, tell Teller, tid int64, by string, top db.Turn, note *string) (Popped, bool, error) {
	if top.OnPass == db.TurnOnPassHandoff {
		if err := passNote(tx, tid, by, *note); err != nil {
			return Popped{}, true, err
		}
		if _, _, _, err := db.PopTurn(tx, tid, by, db.TurnPass, "", 0, 0); err != nil {
			return Popped{}, true, err
		}
		h, asked, err := handOff(tx, tell, tid, top.By, top.PassTo, top.Evidence, paths.ListAgents(home), false)
		if err == nil && !asked.OK {
			return Popped{}, true, refusal(asked.Msg) // all of it rolls back: the turn stays, and a comment sends it back
		}
		return Popped{Handover: h, Asked: asked}, true, err
	}
	var d heldDone
	if err := json.Unmarshal([]byte(top.Evidence), &d); err != nil {
		return Popped{}, true, err
	}
	if d.Again {
		return Popped{}, false, nil
	}
	if _, err := tx.Exec("SAVEPOINT held_done"); err != nil {
		return Popped{}, true, err
	}
	res, err := func() (*DoneResult, error) {
		if err := passNote(tx, tid, by, *note); err != nil {
			return nil, err
		}
		if _, _, _, err := db.PopTurn(tx, tid, by, db.TurnPass, "", 0, 0); err != nil { // off first: its agent closes it as the one acting
			return nil, err
		}
		return MarkDone(tx, home, top.By, d.in(tid))
	}()
	var refused *rpc.Error
	switch {
	case err == nil:
		_, err = tx.Exec("RELEASE held_done")
		return Popped{Handover: Handover{Task: tid, By: by, From: top.Agent, To: top.By}, Done: res}, true, err
	case errors.As(err, &refused) && refused.Code == "refused":
		if _, err := tx.Exec("ROLLBACK TO held_done"); err != nil {
			return Popped{}, true, err
		}
		if _, err := tx.Exec("RELEASE held_done"); err != nil {
			return Popped{}, true, err
		}
		*note = strings.TrimSpace(*note + " (not closed: " + refused.Message + ")")
		return Popped{}, false, nil
	}
	return Popped{}, true, err
}

// passNote keeps a pass's note as by's comment, when it has one.
func passNote(tx *sql.Tx, tid int64, by, note string) error {
	if note == "" {
		return nil
	}
	_, err := db.AddMemo(tx, tid, by, note)
	return err
}

// holdReview holds a review turn's pass by `by` behind the task's later steps: the review comes off, and the done of the agent below with
// the review's evidence waits on their person as that agent's own would. memo is the comment the pass is, 0 for none (note then is
// written as by's). nil, with nothing changed, when nothing is held.
func holdReview(tx *sql.Tx, tell Teller, tid int64, by, below, note string, memo int64, top db.Turn) (*laterHold, error) {
	if actor.IsPerson(below) {
		return nil, nil
	}
	if person, err := db.LaterFor(tx, tid); err != nil || person == "" {
		return nil, err
	}
	if _, err := tx.Exec("SAVEPOINT held_review"); err != nil {
		return nil, err
	}
	hold, err := func() (*laterHold, error) {
		if _, _, _, err := db.PopTurn(tx, tid, by, db.TurnPass, "", 0, 0); err != nil {
			return nil, err
		}
		return holdDone(tx, tell, tid, below, DoneIn{Task: tid, Words: top.Evidence}, false)
	}()
	if err != nil {
		return nil, err
	}
	if hold == nil { // not held after all: the review stays on, for its pass as before
		if _, err := tx.Exec("ROLLBACK TO held_review"); err != nil {
			return nil, err
		}
		_, err := tx.Exec("RELEASE held_review")
		return nil, err
	}
	if _, err := tx.Exec("RELEASE held_review"); err != nil {
		return nil, err
	}
	if note != "" && memo == 0 {
		if hold.note, err = db.AddMemo(tx, tid, by, note); err != nil {
			return nil, err
		}
		hold.noteText, hold.noteBy = note, by
	}
	hold.said = fmt.Sprintf("T%d: the review passed; it waits on %s first for the later steps (comment %d), and their pass closes it",
		tid, addr.Ref(hold.h.To), hold.memo)
	return hold, nil
}

func (l *laterHold) handover() Handover {
	if l == nil {
		return Handover{}
	}
	return l.h
}
