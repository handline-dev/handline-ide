package term_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/term"
)

var rule = strings.Repeat("─", 60)

// Screens captured off Claude Code: a question prompt, a permission prompt, and the prompt cleared.
var (
	singleDefault = []string{rule, " ☐ Library", "", "Which library should we use for dates?", "",
		"❯ 1. date-fns", "     small, tree-shakeable", "  2. dayjs", "     momentjs-compatible API",
		"  3. luxon", "     built-in timezone support", "  4. Type something.",
		rule, "  5. Chat about this", "", "Enter to select · ↑/↓ to navigate · Esc to cancel"}
	permissionPrompt = []string{"earlier output", rule, "   Bash command", "     rm -rf build",
		"   Do you want to proceed?", "   ❯ 1. Yes", "     2. No", "   Esc to cancel · Tab to amend"}
	cleared = []string{"back at the prompt"}
)

// fakeHost shows a screen until a key is sent; screenAt says what it shows once n keys have been sent.
type fakeHost struct {
	mu       sync.Mutex
	sent     []string
	screenAt func(n int) []string
}

func (h *fakeHost) Name() string                                   { return "fake" }
func (h *fakeHost) Available() bool                                { return true }
func (h *fakeHost) ListTerminals() map[string]bool                 { return nil }
func (h *fakeHost) WaitIdle(string, int, func([]string) bool) bool { return true }
func (h *fakeHost) ReadScreen(string, int) *term.Screen {
	h.mu.Lock()
	defer h.mu.Unlock()
	return &term.Screen{Tail: h.screenAt(len(h.sent)), Source: "screen"}
}
func (h *fakeHost) Send(_, text string, enter bool) (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent = append(h.sent, fmt.Sprintf("%s/%v", text, enter))
	return true, ""
}

func newTerm(h *fakeHost) *term.Term {
	t := term.New(h, claudecode.ScreenFor)
	t.Settle = 0
	return t
}

func TestTheDecliningKeyIsEscOnlyWhereTheScreenSaysItOnlyDeclines(t *testing.T) {
	dlg := func(options []term.Option, footer string, body ...string) *term.Dialog {
		return &term.Dialog{Title: "t", Options: options, Footer: footer, Body: append([]string{"t"}, body...)}
	}
	const both = "Enter to confirm · Esc to cancel"
	cases := []struct {
		name string
		d    *term.Dialog
		key  string
	}{
		{"an inert option and the footer: Esc", dlg([]term.Option{{1, "Yes"}, {2, "No"}}, both, "1. Yes", "2. No"), term.Escape},
		{"'No, exit' is a choice: no key", dlg([]term.Option{{1, "Yes, I trust this folder"}, {2, "No, exit"}}, both, "1. Yes, I trust this folder", "2. No, exit"), ""},
		{"an (esc) mark on an option that is not inert: no key", dlg([]term.Option{{1, "Yes"}, {2, "No, exit (esc)"}}, both, "1. Yes", "2. No, exit (esc)"), ""},
		{"an (esc) mark in free text: no key", dlg([]term.Option{{1, "Yes"}, {2, "No"}}, both, "Press Esc to exit (esc)", "1. Yes", "2. No"), ""},
		{"a footer that does not say Esc cancels: no key", dlg([]term.Option{{1, "Yes"}, {2, "No"}}, "Enter to confirm"), ""},
		{"no option that only declines: no key", dlg([]term.Option{{1, "Delete"}, {2, "Overwrite"}}, both), ""},
		{"the usage-limit dialog, whose Esc was measured", dlg([]term.Option{{1, "Stop and wait for limit to reset"}, {2, "Wait here, then continue automatically"}}, both), term.Escape},
	}
	for _, c := range cases {
		if key, why := term.DecliningKey(c.d); key != c.key {
			t.Errorf("%s: got %q (%s)", c.name, key, why)
		}
	}
}

func TestDialogParsingTakesTheRuleAboveTheFooterAndNeverGuesses(t *testing.T) {
	scr := []string{"output", "▔▔▔▔▔▔▔▔▔▔▔▔", "  Overwrite?", "  ❯ 1. Yes", "    2. No", "  Enter to confirm · Esc to cancel"}
	d := term.ParseDialog(scr, term.FooterRow(scr), false)
	if d == nil || d.Title != "Overwrite?" || len(d.Options) != 2 || d.Options[1].Label != "No" {
		t.Fatalf("%+v", d)
	}
	noRule := []string{"  1. Yes", "  2. No", "  Enter to confirm · Esc to cancel"}
	if term.ParseDialog(noRule, 2, false) != nil {
		t.Fatal("no rule opening the panel is no dialog")
	}
	if got := term.ParseDialog(noRule, 2, true); got == nil || got.Title != term.TruncatedTitle {
		t.Fatalf("a truncated read says so: %+v", got)
	}
	skipped := []string{"▔▔▔▔▔▔▔▔▔▔▔▔", "  T", "  1. Yes", "  3. No", "  Enter to confirm · Esc to cancel"}
	if term.ParseDialog(skipped, 4, false) != nil {
		t.Fatal("options not numbered 1, 2, ... in order are not read")
	}
}

func TestPermissionFieldsAreOnlyWhatTheScreenStates(t *testing.T) {
	if got := term.PermissionFields("Edit file", []string{"src/a.py", "│ a reason"}); got != (term.Fields{}) {
		t.Fatalf("an edit approval's title was never captured: %+v", got)
	}
	if got := term.PermissionFields("Bash command", nil); got != (term.Fields{Tool: "Bash"}) {
		t.Fatalf("%+v", got)
	}
	if got := term.PermissionFields("Bash command", []string{"│ ls", "│ -la"}); got != (term.Fields{"Bash", "ls\n-la", ""}) {
		t.Fatalf("a boxed command alone has no reason: %+v", got)
	}
	if term.DialogKind([]string{"Yes", "No"}) != "dialog" || term.DialogKind([]string{"Exit now", "Stay"}) != "exit_confirm" {
		t.Fatal("a dialog is an exit confirmation by an option that exits")
	}
}

func TestComposerAndShellReadings(t *testing.T) {
	box := []string{rule, "❯ draft text here", rule, "  ⏵⏵ auto mode on · 2 monitors · ↓ to manage"}
	if txt, ok := term.ComposerText(box); !ok || txt != "drafttexthere" {
		t.Fatalf("%q %v", txt, ok)
	}
	if bg, ok := term.FooterBackground(box); !ok || bg != "2 monitors" {
		t.Fatalf("%q %v", bg, ok)
	}
	if term.ShellPromptIn(box) {
		t.Fatal("a box with a hint under it is a session")
	}
	if !term.ShellPromptIn(append(append([]string{}, box[:3]...), "me@host ~ %")) {
		t.Fatal("a prompt under the box is a shell")
	}
	if !term.IsTextTyped("hellothere", "hello there") || term.IsTextTyped("hellothere", "something else") {
		t.Fatal("the composer holds what was typed, whitespace aside")
	}
	if !(claudecode.Screen{}).IdleWithBackground(box) || (claudecode.Screen{}).IdleWithBackground(append(append([]string{}, box...), "esc to interrupt")) {
		t.Fatal("idle with background work is the count without a turn drawn")
	}
}

func TestTheFolderTrustScreenNamesItsFolderAndIsNotAPrompt(t *testing.T) {
	scr := []string{"Quick safety check: Is this a project you created or one you trust?", "", " Accessing workspace:", " /Users/me/pro", "ject", "",
		"❯ 1. Yes, I trust this folder", "  2. No, exit"}
	if folder, shown := (claudecode.Screen{}).TrustScreen(scr); !shown || folder != "/Users/me/project" {
		t.Fatalf("%q %v", folder, shown)
	}
	titled := []string{"────", " Accessing workspace:", "", " /Users/me/workspaces/handline.worktrees.noindex/desk-2", "",
		" Quick safety check: Is this a project you created or one you trust?", "", " ⚠ This folder pre-approves 42 tool permissions in .claude/settings.local.json:",
		"", " ❯ No, continue without these permissions", "   Yes, I trust this folder"}
	if folder, shown := (claudecode.Screen{}).TrustScreen(titled); !shown || folder != "/Users/me/workspaces/handline.worktrees.noindex/desk-2" {
		t.Fatalf("the folder under the box's title: %q %v", folder, shown)
	}
	if _, shown := (claudecode.Screen{}).TrustScreen(singleDefault); shown {
		t.Fatal("not the trust screen")
	}
}

// A vendor whose digit only moves the highlight: the dialog stays, the chosen option now carries the cursor and the footer says
// Enter confirms. Enter is then pressed once, and only on that screen.
func TestADigitThatOnlyMovesTheHighlightIsFollowedByOneEnterOnAFreshRead(t *testing.T) {
	before := []string{"earlier output", rule, "   Bash command", "     rm -rf build", "   Do you want to proceed?",
		"     1. Yes", "   ❯ 2. No", "   Enter to confirm · Esc to cancel"}
	moved := []string{"earlier output", rule, "   Bash command", "     rm -rf build", "   Do you want to proceed?",
		"   ❯ 1. Yes", "     2. No", "   Enter to confirm · Esc to cancel"}
	yes := func(string, *term.Dialog) int { return 1 }
	h := &fakeHost{screenAt: func(n int) []string {
		switch n {
		case 0:
			return before
		case 1:
			return moved
		}
		return cleared
	}}
	outcome, _, why := newTerm(h).AnswerDialog("%1", yes)
	if outcome != term.Cleared || len(h.sent) != 2 || h.sent[0] != "1/false" || h.sent[1] != "/true" {
		t.Fatalf("%s %q %q", outcome, why, h.sent)
	}

	// the footer does not say Enter confirms: the digit is not followed by Enter
	noFooter := append(append([]string{}, moved[:7]...), "   Esc to cancel")
	h = &fakeHost{screenAt: func(n int) []string {
		if n == 0 {
			return before
		}
		return noFooter
	}}
	if outcome, _, _ := newTerm(h).AnswerDialog("%1", yes); outcome == term.Cleared || len(h.sent) != 1 {
		t.Fatalf("%s %q", outcome, h.sent)
	}
}
