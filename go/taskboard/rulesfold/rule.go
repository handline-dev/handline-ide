// Package rulesfold turns what a person who reviews work writes (task comments and chat) into that person's standing rules, kept as
// lessons about the person, in two areas: screen and work. It is a fold: a model reads the person's new comments past a cursor with the
// rules kept so far and returns the whole rule set, which is written, and the cursor moved, only once it validates.
package rulesfold

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
)

// Rule is one of a person's standing rules: an instruction to agents, how many times the person said it, and the person's own words once.
type Rule struct {
	ID    int64  `json:"id"` // the lesson it is, 0 for a new rule
	Area  string `json:"area"`
	Text  string `json:"text"`
	Count int    `json:"count"`
	Quote string `json:"quote"`
	// QuoteRef names where the quote was said: a comment ("comment 9053") or a chat message ("msg_1e3b758").
	QuoteRef string `json:"quote_ref"`
	Task     int64  `json:"-"` // the task the quote was said on, 0 for none

	from   []int64 // the kept rules it replaces
	stored string  // its lesson's text when it comes back as it was kept
}

// ruleLine is a rule's lesson text: `Screen: <text> (said 3 times; "<quote>", comment 9053)`, with no quote for a rule a person wrote.
var ruleLine = regexp.MustCompile(`^(Screen|Work): (.+?) \(said (once|(\d+) times)(?:; "(.*)", ([^"]+))?\)$`)

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
		return db.AreaText(r.Area, fmt.Sprintf("%s (said %s)", r.Text, said))
	}
	return db.AreaText(r.Area, fmt.Sprintf("%s (said %s; %q, %s)", r.Text, said, r.Quote, r.QuoteRef))
}

// FromLesson is the rule a lesson keeps. A lesson written another way (by hand, or from a Redo note) is a work or screen rule, by its
// start, said once, its whole text the rule's.
func FromLesson(l db.Lesson) Rule {
	m := ruleLine.FindStringSubmatch(l.Text)
	if m == nil {
		text := strings.TrimPrefix(strings.TrimPrefix(l.Text, "Screen: "), "Work: ")
		return Rule{ID: l.ID, Area: db.LessonArea(l.Text), Text: text, Count: 1, Task: l.Task}
	}
	n := 1
	if m[4] != "" {
		n, _ = strconv.Atoi(m[4])
	}
	quote, err := strconv.Unquote(`"` + m[5] + `"`)
	if err != nil {
		quote = m[5]
	}
	return Rule{ID: l.ID, Area: strings.ToLower(m[1]), Text: m[2], Count: n, Quote: quote, QuoteRef: m[6], Task: l.Task}
}
