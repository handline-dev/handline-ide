package sessionrpc

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/term"
)

var (
	composer = []string{"", "────────────────────────────────", "❯ ", "────────────────────────────────", "  ? for shortcuts"}
	addDirUI = []string{"earlier output", strings.Repeat("▔", 20), " Add directory to workspace", "",
		" ❯ 1. Yes, for this session", "   2. Yes, and remember this directory", "   3. No", "", " Enter to confirm · Esc to cancel"}
)

// dialogHost shows the /add-dir dialog once /add-dir is typed, and the composer again once a digit or Esc is pressed.
type dialogHost struct {
	*host
	dialog []string
}

func (h *dialogHost) Send(handle, text string, enter bool) (bool, string) {
	h.host.Send(handle, text, enter)
	switch {
	case strings.HasPrefix(text, "/add-dir "):
		h.rows = h.dialog
	case text == term.Escape || (len(text) == 1 && text >= "1" && text <= "9"):
		h.rows = composer
	}
	return true, ""
}

// AddDir types /add-dir at an idle composer and picks "Yes, for this session"; it types nothing at a screen that waits on something,
// in a turn, nor while someone holds the input, nor a path with a space; it cancels a dialog that is not the one expected and deletes a line
// left unsent.
func TestAddDirPicksForThisSessionAtAnIdleComposerOnly(t *testing.T) {
	r, h, _ := typingRig(t)
	addDirWait = 3 * addDirProbe
	dh := &dialogHost{host: h, dialog: addDirUI}
	r.e.Term = term.New(dh, claudecode.ScreenFor)
	h.rows = composer
	if !r.e.AddDir("ann", "/data") || strings.Join(h.sent, "|") != "/add-dir /data<enter>|1" {
		t.Fatalf("sent %q", h.sent)
	}
	h.sent, h.rows = nil, []string{"Do you want to proceed?", "Enter to confirm · Esc to cancel"}
	if r.e.AddDir("ann", "/data") || len(h.sent) != 0 {
		t.Fatalf("at a dialog: sent %q", h.sent)
	}
	h.rows = []string{"", "────────────────────────────────", "❯ ", "────────────────────────────────", "  esc to interrupt"}
	if r.e.AddDir("ann", "/data") || len(h.sent) != 0 {
		t.Fatalf("in a turn: sent %q", h.sent)
	}
	h.rows = composer
	r.e.Holds.Take("ann", "person", "sam")
	if r.e.AddDir("ann", "/data") || len(h.sent) != 0 {
		t.Fatalf("held: sent %q", h.sent)
	}
	r.e.Holds.Release("ann")
	dh.dialog = []string{"earlier output", strings.Repeat("▔", 20), " Something else", "", " ❯ 1. Yes", "   2. No", "", " Enter to confirm · Esc to cancel"}
	if r.e.AddDir("ann", "/data") || strings.Join(h.sent, "|") != "/add-dir /data<enter>|"+term.Escape {
		t.Fatalf("another dialog: sent %q", h.sent)
	}
	// No dialog comes and the line stays in the composer: it is deleted, and no Esc is pressed.
	h.sent = nil
	dh.dialog = []string{"", "────────────────────────────────", "❯ /add-dir /data", "────────────────────────────────", "  ? for shortcuts"}
	if r.e.AddDir("ann", "/data") || strings.Join(h.sent, "|") != "/add-dir /data<enter>|"+ctrlU {
		t.Fatalf("no dialog: sent %q", h.sent)
	}
	h.sent, h.rows = nil, composer
	if r.e.AddDir("ann", "/my data") || len(h.sent) != 0 {
		t.Fatalf("a path with a space: sent %q", h.sent)
	}
}
