package session

import (
	"testing"

	"github.com/handline/handline/go/agentctl/term"
)

func TestHeadlineAndReasonAreTheWordsForARow(t *testing.T) {
	if got := Headline(term.Modal, "Title - body of it"); got != "dialog open: Title" {
		t.Errorf("got %q", got)
	}
	if got := Headline(NoSession, ""); got != "no session" {
		t.Errorf("got %q", got)
	}
	if got := Headline("odd", "x - y"); got != "odd: x - y" {
		t.Errorf("got %q", got)
	}
	if got := Reason(NoTerminal, "its terminal is gone"); got != "no terminal: its terminal is gone" {
		t.Errorf("got %q", got)
	}
	if got := Reason(term.Unreadable, "ignored"); got != "its screen could not be read" {
		t.Errorf("got %q", got)
	}
	if got := Reason(term.Backgrounded, "banner"); got != "its conversation moved to the background: banner" {
		t.Errorf("got %q", got)
	}
	if got := Reason("custom", ""); got != "custom" {
		t.Errorf("got %q", got)
	}
}
