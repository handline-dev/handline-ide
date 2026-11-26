package rulesfold

import (
	"testing"

	"github.com/handline/handline/go/data/db"
)

// A rule's lesson text reads back as the rule; a lesson written another way is a rule said once with its whole text.
func TestARuleRoundTripsThroughItsLessonText(t *testing.T) {
	for _, r := range []Rule{
		{Text: "No cards (ever).", Count: 3, Quote: `카드 "쓰지" 말자`, QuoteRef: "comment 8982"},
		{Text: "Fix the cause.", Count: 1, Quote: "근본수정", QuoteRef: "msg_1e3b758"},
		{Text: "Write in Korean.", Count: 2},
	} {
		got := FromLesson(db.Lesson{Text: r.Line()})
		if got.Text != r.Text || got.Count != r.Count || got.Quote != r.Quote || got.QuoteRef != r.QuoteRef {
			t.Errorf("%q read back as %+v", r.Line(), got)
		}
	}
	text := "Lists are flat rows (the founder, comment 7208)"
	if got := FromLesson(db.Lesson{ID: 4, Text: text}); got.Text != text || got.Count != 1 || got.QuoteRef != "" || got.ID != 4 {
		t.Errorf("%q: %+v", text, got)
	}
}
