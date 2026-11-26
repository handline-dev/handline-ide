package rulesfold

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/textsafe"
)

// maxLesson is the most bytes an agent's lesson the fold writes takes (the prompt asks for 160 characters).
const maxLesson = 300

// Lesson is a correction of one agent's work, written as a lesson of that agent's own on the task it was said on: new, or replacing
// lessons it has that say the same thing.
type Lesson struct {
	Agent string  `json:"agent"`
	Text  string  `json:"text"`
	Task  int64   `json:"task,omitempty"`
	From  []int64 `json:"from,omitempty"` // the agent's lessons it replaces
	Ref   string  `json:"ref"`            // the item it comes from
	Why   string  `json:"why,omitempty"`  // why it was left out
}

type outLesson struct {
	Agent  string   `json:"agent"`
	Text   string   `json:"text"`
	Merges []int64  `json:"merges"`
	Refs   []string `json:"refs"`
}

// agentLessons are the live lessons of each agent the items name.
func agentLessons(q db.Q, items []Item) (map[string][]db.Lesson, error) {
	out := map[string][]db.Lesson{}
	for _, it := range items {
		for _, a := range it.Agents {
			if _, ok := out[a]; ok {
				continue
			}
			ls, err := db.LiveLessons(q, a)
			if err != nil {
				return nil, err
			}
			out[a] = ls
		}
	}
	return out, nil
}

// checkLessons validates the model's agent lessons and gives those to write and those left out: each comes from items read that name its
// agent, is one plain line of at most maxLesson bytes, and merges only that agent's live lessons, each into one lesson at most. Its task
// is its first such item's.
func checkLessons(out []outLesson, items []Item, have map[string][]db.Lesson) (ok, skipped []Lesson) {
	byRef := map[string]Item{}
	for _, it := range items {
		byRef[it.Ref] = it
	}
	claimed := map[int64]bool{}
	for _, o := range out {
		l := Lesson{Agent: o.Agent, Text: strings.TrimSpace(o.Text)}
		for _, ref := range o.Refs {
			if it, found := byRef[ref]; found && slices.Contains(it.Agents, o.Agent) {
				l.Ref, l.Task = ref, it.task
				break
			}
		}
		l.Why = lessonFault(l, o.Merges, have[o.Agent], claimed)
		if l.Why != "" {
			skipped = append(skipped, l)
			continue
		}
		for _, id := range o.Merges {
			claimed[id] = true
		}
		l.From = o.Merges
		ok = append(ok, l)
	}
	return ok, skipped
}

// lessonFault is why l cannot be written, "" when it can.
func lessonFault(l Lesson, merges []int64, have []db.Lesson, claimed map[int64]bool) string {
	switch {
	case l.Text == "" || len(l.Text) > maxLesson || textsafe.HasUnsafe(l.Text):
		return fmt.Sprintf("empty, over %d bytes or not one plain line", maxLesson)
	case l.Ref == "":
		return "no item read is about that agent's work"
	}
	for i, id := range merges {
		if !slices.ContainsFunc(have, func(x db.Lesson) bool { return x.ID == id }) {
			return fmt.Sprintf("merges %d, not a live lesson of that agent", id)
		}
		if claimed[id] || slices.Contains(merges[:i], id) {
			return fmt.Sprintf("merges %d twice", id)
		}
	}
	return ""
}

// applyLessons writes each lesson to its agent, and gives those written and those left out: past the agent's lesson cap, or merging a
// lesson no longer live.
func applyLessons(q db.Q, lessons []Lesson) (written, skipped []Lesson, err error) {
	for _, l := range lessons {
		err := db.ReviseLessons(q, l.Agent, []db.Revision{{IDs: l.From, Text: l.Text, Task: l.Task}}, By)
		_, gone := err.(db.NotLiveLesson) // as returned before any write; joined with another error it fails the step
		switch {
		case errors.Is(err, db.ErrLessonsFull):
			l.Why = "past the agent's lesson cap"
		case gone:
			l.Why = err.Error()
		case err != nil:
			return nil, nil, err
		default:
			written = append(written, l)
			continue
		}
		skipped = append(skipped, l)
	}
	return written, skipped, nil
}
