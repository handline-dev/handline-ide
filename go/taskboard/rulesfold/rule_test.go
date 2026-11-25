package rulesfold

import (
	"testing"

	"github.com/handline/handline/go/data/db"
)

// A rule's lesson text reads back as the rule; a lesson written another way is a rule said once with its whole text.
func TestARuleRoundTripsThroughItsLessonText(t *testing.T) {
	for _, r := range []Rule{
		{Area: db.AreaScreen, Text: "No cards (ever).", Count: 3, Quote: `카드 "쓰지" 말자`, QuoteRef: "comment 8982"},
		{Area: db.AreaWork, Text: "Fix the cause.", Count: 1, Quote: "근본수정", QuoteRef: "msg_1e3b758"},
		{Area: db.AreaWork, Text: "Write in Korean.", Count: 2},
	} {
		got := FromLesson(db.Lesson{Text: r.Line()})
		if got.Area != r.Area || got.Text != r.Text || got.Count != r.Count || got.Quote != r.Quote || got.QuoteRef != r.QuoteRef {
			t.Errorf("%q read back as %+v", r.Line(), got)
		}
	}
	for text, area := range map[string]string{"Lists are flat rows (the founder, comment 7208)": db.AreaWork, "Screen: no hints": db.AreaScreen} {
		if got := FromLesson(db.Lesson{ID: 4, Text: text}); got.Area != area || got.Count != 1 || got.QuoteRef != "" || got.ID != 4 {
			t.Errorf("%q: %+v", text, got)
		}
	}
}
