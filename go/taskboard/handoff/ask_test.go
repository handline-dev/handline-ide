package handoff

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
)

func TestTheBuiltInAskListsEveryNoteItemInOrder(t *testing.T) {
	text := AskText("", "/n/a.md")
	at := 0
	for i, item := range NoteItems {
		n := strings.Index(text[at:], "("+strconv.Itoa(i+1)+") "+item)
		if n < 0 {
			t.Fatalf("item %d missing or out of order: %q\n%s", i+1, item, text)
		}
		at += n
	}
	for _, want := range []string{"/n/a.md", strconv.Itoa(NoteMax) + " characters", "verbatim, each with its time", "not committed",
		"'next: none'", "Do not invent work", "T<n>/#<n>", "'Slowed:'", "'Slowed: none'", "cite its T or # number", "`hl lesson add`",
		"the single word: nothing"} {
		if !strings.Contains(text, want) {
			t.Fatalf("ask lacks %q:\n%s", want, text)
		}
	}
	for _, gone := range []string{"preferences and the pitfalls", "memory first", "{max}", "{path}"} {
		if strings.Contains(text, gone) {
			t.Fatalf("ask still has %q:\n%s", gone, text)
		}
	}
}

func TestACustomPromptIsLoggedAtStartAndNoneLogsNothing(t *testing.T) {
	home := t.TempDir()
	var logged []string
	log := func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) }
	LogCustomPrompts(home, "", OwnPrompts(home), log)
	if len(logged) != 0 {
		t.Fatalf("nothing set, yet logged %v", logged)
	}
	teamtest.Set(t, home, "ann", PromptKey, "Mine: {path}")
	teamtest.Set(t, home, "bob", PromptKey, "  ")
	own := OwnPrompts(home)
	if ann := teamtest.Key(t, home, "ann"); len(own) != 1 || own[0] != ann {
		t.Fatalf("own prompts: %v, want only ann", own)
	}
	LogCustomPrompts(home, "Global: {path}", own, log)
	if len(logged) != 2 || !strings.Contains(logged[0], "config.yaml") || !strings.HasPrefix(logged[1], "handoff: "+paths.AgentHandle(home, own[0])+" set") {
		t.Fatalf("want the global, then ann by name (never the key), got %q", logged)
	}
}
