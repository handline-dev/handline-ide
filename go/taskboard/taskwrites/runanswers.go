package taskwrites

import (
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
)

var taskRef = regexp.MustCompile(`\bT([0-9]+)\b`)

// taskRefs is the task numbers text names, each once.
func taskRefs(text string) []int64 {
	var out []int64
	seen := map[int64]bool{}
	for _, m := range taskRef.FindAllStringSubmatch(text, -1) {
		if n, err := strconv.ParseInt(m[1], 10, 64); err == nil && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func isDraft(tx *sql.Tx, tid int64) (bool, error) {
	t, err := getTask(tx, tid)
	if err != nil || t == nil {
		return false, err
	}
	st, err := state(tx, t)
	return st == "draft", err
}

// answerDrafts is a person's Run (verb "Ran") or discard ("Discarded") of drafts taken as the answer to the questions that asked for it.
// On each open task whose top turn is by's and whose question names one of drafts, the turn passes once none of the tasks it names is a
// draft, and the one below is told what was done. A question naming no task is answered on a parent of drafts once that parent has no
// draft sub-task left, quietly: the parent then waits on its children. A Ready task waits for OK or Redo either way.
func answerDrafts(tx *sql.Tx, home string, tell Teller, by, verb string, drafts []int64) ([]Popped, error) {
	if !actor.IsPerson(by) || len(drafts) == 0 {
		return nil, nil
	}
	acted := map[int64]bool{}
	parents := map[int64]bool{}
	for _, d := range drafts {
		acted[d] = true
		if t, err := getTask(tx, d); err != nil {
			return nil, err
		} else if t != nil && t.Parent.Valid {
			parents[t.Parent.Int64] = true
		}
	}
	ids, err := db.IDs(tx, "SELECT t.id FROM tasks t WHERE "+openSQL+" AND EXISTS (SELECT 1 FROM task_turns WHERE task_id = t.id) AND "+
		db.ActingSQL("t")+" = ? ORDER BY t.id", by)
	if err != nil {
		return nil, err
	}
	var out []Popped
	for _, tid := range ids {
		t, err := getTask(tx, tid)
		if err != nil {
			return nil, err
		}
		if t == nil || t.Acting != by { // a person acts on a task only by its top turn
			continue
		}
		turns, err := db.Turns(tx, tid)
		if err != nil {
			return out, err
		}
		if len(turns) == 0 || turns[len(turns)-1].OnPass == db.TurnOnPassDone { // a review's pass closes the task: a Run is not that
			continue
		}
		refs := slices.DeleteFunc(taskRefs(turns[len(turns)-1].Ask), func(r int64) bool { return r == tid })
		var named []string
		answered := false
		if len(refs) == 0 {
			if parents[tid] {
				if answered, err = noDraftChild(tx, tid); err != nil {
					return out, err
				}
			}
		} else {
			answered = true
			for _, r := range refs {
				if acted[r] {
					named = append(named, fmt.Sprintf("T%d", r))
				}
				if draft, err := isDraft(tx, r); err != nil {
					return out, err
				} else if draft {
					answered = false
				}
			}
			answered = answered && len(named) > 0
		}
		if !answered {
			continue
		}
		if ready, err := readyNow(tx, tid); err != nil {
			return out, err
		} else if ready {
			continue
		}
		if len(refs) == 0 { // the parent waits on its children: it goes back with no delivery and no message
			if err := db.EndHeld(tx, tid, by); err != nil {
				return out, err
			}
			if _, _, _, err := db.PopTurn(tx, tid, by, db.TurnPass, "", 0, 0); err != nil {
				return out, err
			}
			continue
		}
		did := verb + " " + strings.Join(named, ", ")
		told := fmt.Sprintf("T%d is your turn again: %s answered your question on it: %s", tid, addr.Ref(by), did)
		p, err := popTurnTold(tx, home, tell, tid, by, db.TurnPass, "", 0, told)
		if err != nil {
			return out, err
		}
		if p.Asked != nil && !p.Asked.OK { // the return was refused: the turn stays
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func noDraftChild(tx *sql.Tx, tid int64) (bool, error) {
	kids, err := openChildren(tx, tid)
	if err != nil {
		return false, err
	}
	for _, k := range kids {
		if draft, err := isDraft(tx, k); err != nil || draft {
			return false, err
		}
	}
	return true, nil
}

// draftsOf is which of tids are drafts now, before a write changes them.
func draftsOf(tx *sql.Tx, tids []int64) ([]int64, error) {
	var out []int64
	for _, tid := range tids {
		if draft, err := isDraft(tx, tid); err != nil {
			return nil, err
		} else if draft {
			out = append(out, tid)
		}
	}
	return out, nil
}

// saidWith is a write's reply with each turn it answered.
func saidWith(said string, answered []Popped) string {
	for _, p := range answered {
		said += "\n" + p.said()
	}
	return said
}
