package taskwrites

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
)

// A seat at work asks its agent, not a person: the agent answers what follows from the task or the person's own words, and puts the person on
// only what it cannot.

// seatTurn is where a turn `by` puts on task tid goes, and what it is typed: to and ask unchanged, but a seat at work asking a person asks
// its agent instead, with the line that tells it how to answer. drop is the agent's own turn to take out once the person's is on: the
// agent passing its seat's question to the person, so the person's answer goes back to the seat.
func seatTurn(tx *sql.Tx, tid int64, by, to, ask string) (newTo, newAsk, drop string, err error) {
	if !actor.IsPerson(to) || actor.IsPerson(by) {
		return to, ask, "", nil
	}
	turns, err := db.Turns(tx, tid)
	if err != nil {
		return "", "", "", err
	}
	t, err := getTask(tx, tid)
	if err != nil || t == nil || len(turns) > 0 && actor.IsPerson(turns[len(turns)-1].Agent) {
		return to, ask, "", err // the person's turn is on already: asking again rewrites it
	}
	origin, err := paths.StandsInFor(tx, by)
	if err != nil {
		return "", "", "", err
	}
	if origin != "" {
		return origin, SeatAskLine(tid, ask), "", nil
	}
	if len(turns) == 0 {
		return to, ask, "", nil
	}
	top, below := turns[len(turns)-1], Below(turns, t.Owner)
	if top.Agent != by || top.By != below || top.OnPass != "" { // only the turn its seat's question put on it
		return to, ask, "", nil
	}
	if stands, err := paths.StandsInFor(tx, below); err != nil || stands != by {
		return to, ask, "", err
	}
	return to, ask, by, nil
}

// SeatAskLine is what an agent is typed when its seat on task tid asks a person: the question, and how to answer it.
func SeatAskLine(tid int64, question string) string {
	return fmt.Sprintf("Your temp on T%d asks: %s\nAnswer on T%d if it follows from the task or the founder's own words: `hl task pass T%d \"<answer>\"` gives it back with your answer. "+
		"Else put the founder on it with one question: `hl task comment T%d add \"<question>\" --turn human`; their answer goes to your temp.",
		tid, firstLines(question), tid, tid, tid)
}

// firstLines is text's first two non-empty lines on one line, cut at 400 runes.
func firstLines(text string) string {
	var keep []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			keep = append(keep, l)
		}
		if len(keep) == 2 {
			break
		}
	}
	t := textsafe.Oneline(strings.Join(keep, " "))
	if r := []rune(t); len(r) > 400 {
		t = string(r[:400]) + "…"
	}
	return t
}

// dropRelay takes agent's turn out from under the person's it just put on task tid, so the person's answer goes back to its seat below.
func dropRelay(tx *sql.Tx, tid int64, agent, to string) error {
	return db.DropTurn(tx, tid, agent, db.TurnPass, "asked "+addr.Ref(to)+" for its temp")
}
