package session

import (
	"strings"

	"github.com/handline/handline/go/agentctl/term"
)

// What is in the way of giving an agent its work, in words: the obstacles' names and how a row of them reads.

// DecisionAfterS: a dialog stands this long before the founder is told. Long enough that someone at the bridge has seen it and
// acted, short enough that nobody learns of it from an agent that has been silent for an hour.
const DecisionAfterS = 600

// The obstacles that are not a screen's own.
const (
	NoTerminal = "no terminal"
	NoSession  = "no session"
	StalledOb  = "stalled"
	HostDown   = "host down"
	// TableMiss: the process table lists no session for an agent whose terminal draws one: a fault in the table, not an obstacle.
	TableMiss = "table miss"
)

// Label is each obstacle in a few words.
var Label = map[string]string{
	term.Modal: "dialog open", term.Backgrounded: "conversation in the background", term.Unreadable: "screen unreadable",
	HostDown: "host unreachable", NoTerminal: "no terminal", NoSession: "no session", StalledOb: "stalled",
	TableMiss: "process table misses a live session",
}

func label(kind string) string {
	if l, ok := Label[kind]; ok {
		return l
	}
	return kind
}

// Reason is what is in the way, as a sentence.
func Reason(kind, detail string) string {
	switch kind {
	case term.Modal, term.Backgrounded, term.Unreadable:
		return term.Blocker{Kind: kind, Detail: detail}.Reason()
	}
	if detail != "" {
		return label(kind) + ": " + detail
	}
	return label(kind)
}

// Headline is the short form for a row with little room: the label and, for a dialog, only its title.
func Headline(kind, detail string) string {
	head := detail
	if kind == term.Modal {
		head = strings.SplitN(detail, " - ", 2)[0]
	}
	if head != "" {
		return label(kind) + ": " + head
	}
	return label(kind)
}
