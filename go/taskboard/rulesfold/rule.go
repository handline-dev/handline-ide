// Package rulesfold turns what a person who reviews work writes (task comments and chat) into lessons: a standard any agent's work
// should follow becomes one of the person's rules (lessons about the person, one list every agent reads), and a correction of one
// agent's work a lesson of that agent's own. It is a fold: a model reads the person's new comments past a cursor with what is kept so far
// and returns the whole rule set and the agents' new lessons, which are written, and the cursor moved, only once they validate.
package rulesfold

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/handline/handline/go/data/db"
)

// Rule is one of a person's standing rules: an instruction to agents, how many times the person said it, and the person's own words once.
type Rule struct {
	ID    int64  `json:"id"` // the lesson it is, 0 for a new rule
	Text  string `json:"text"`
	Count int    `json:"count"`
	Quote string `json:"quote"`
	// QuoteRef names where the quote was said: a comment ("comment 9053") or a chat message ("msg_1e3b758").
	QuoteRef string `json:"quote_ref"`
	Task     int64  `json:"-"` // the task the quote was said on, 0 for none

	from   []int64 // the kept rules it replaces
	stored string  // its lesson's text when it comes back as it was kept
}

// ruleLine is a rule's lesson text: `<text> (said 3 times; "<quote>", comment 9053)`, with no quote for a rule a person wrote.
var ruleLine = regexp.MustCompile(`^(.+?) \(said (once|(\d+) times)(?:; "(.*)", ([^"]+))?\)$`)

// Line is the rule as its lesson's text: as stored when it came back unchanged.
func (r Rule) Line() string {
	if r.stored != "" {
		return r.stored
	}
	said := "once"
	if r.Count > 1 {
		said = strconv.Itoa(r.Count) + " times"
	}
	if r.QuoteRef == "" {
		return fmt.Sprintf("%s (said %s)", r.Text, said)
	}
	return fmt.Sprintf("%s (said %s; %q, %s)", r.Text, said, r.Quote, r.QuoteRef)
}

// FromLesson is the rule a lesson keeps. A lesson written another way (by hand, or from a Redo note) is a rule said once, its whole text
// the rule's.
func FromLesson(l db.Lesson) Rule {
	m := ruleLine.FindStringSubmatch(l.Text)
	if m == nil {
		return Rule{ID: l.ID, Text: l.Text, Count: 1, Task: l.Task}
	}
	n := 1
	if m[3] != "" {
		n, _ = strconv.Atoi(m[3])
	}
	quote, err := strconv.Unquote(`"` + m[4] + `"`)
	if err != nil {
		quote = m[4]
	}
	return Rule{ID: l.ID, Text: m[1], Count: n, Quote: quote, QuoteRef: m[5], Task: l.Task}
}
