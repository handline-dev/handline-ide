package claudecode

import (
	"regexp"
	"strings"

	"github.com/handline/handline/go/agentctl/term"
)

// ---- claude

type Screen struct{}

func (Screen) Name() string { return "claude" }

// Text a working claude draws in its status row and an idle one does not.
const workingRows = 12

func (Screen) TurnOnScreen(tail []string) bool {
	rows := tail
	if len(rows) > workingRows {
		rows = rows[len(rows)-workingRows:]
	}
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r), "esc to interrupt") {
			return true
		}
	}
	return false
}

// claude's hooks report its working state; the screen carries no turn marker the daemon reads, so it is never the witness.
func (Screen) WorkingOnScreen([]string) (bool, bool) { return false, false }

var spinnerRowRE = regexp.MustCompile(`^\S\s+\S+…(\s|$)`)

const spinnerRows = 6

// IdleWithBackground is Claude Code's layout: the hint row counts background work, and neither the spinner nor "esc to
// interrupt" is drawn.
func (Screen) IdleWithBackground(tail []string) bool {
	top, bottom, ok := term.ComposerBox(tail)
	if !ok {
		return false
	}
	if bg, _ := term.FooterBackground(tail); bg == "" {
		return false
	}
	for _, r := range tail[bottom+1:] {
		if strings.Contains(r, "esc to interrupt") {
			return false
		}
	}
	for _, r := range tail[max(0, top-spinnerRows):top] {
		if spinnerRowRE.MatchString(strings.TrimSpace(r)) {
			return false
		}
	}
	return true
}

// Claude Code's folder-trust screen ("Quick safety check: Is this a project you created or one you trust?"), and the variant for
// a folder whose settings pre-approve tools (Claude Code 2.1.291): the same workspace line and options under a different question.
var trustMarkers = []string{"Is this a project you created or one you trust", "This folder pre-approves"}

// The line the prompt names its folder under, at once or after a blank line (Claude Code 2.1.322 draws it as the box's title). Wrapped
// at terminal width with no separator, so its continuation lines are joined bare.
const workspaceMarker = "Accessing workspace:"

func (Screen) TrustScreen(tail []string) (string, bool) {
	text := strings.Join(tail, "\n")
	shown := false
	for _, m := range trustMarkers {
		shown = shown || strings.Contains(text, m)
	}
	if !shown {
		return "", false
	}
	lines := strings.Split(text, "\n")
	start := -1
	for i, ln := range lines {
		if strings.Contains(ln, workspaceMarker) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", true
	}
	var parts []string
loop:
	for _, ln := range lines[start:] {
		s := strings.TrimSpace(ln)
		if s == "" && len(parts) == 0 {
			continue
		}
		if s == "" || len(parts) == 0 && !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "~") {
			break // the end of the path, or no path drawn
		}
		for _, m := range trustMarkers {
			if strings.Contains(s, m) {
				break loop
			}
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ""), true
}

// Claude Code's backgrounded-session picker: the conversation moved to the background and the screen is a list of sessions. Its
// new-session box has the ordinary composer shape, so the shape alone cannot tell it from the agent's own prompt, and text typed
// there goes to a new session. Its hint line does ("enter to open" or "space to reply" at or below the box), but that is ordinary
// English an agent's own transcript quotes above the box, so the picker's banner is required above the box too.
var backgroundedHints = []string{"enter to open", "space to reply"}

const backgroundBanner = "moved to the background"

func hasPickerHint(row string) bool {
	for _, h := range backgroundedHints {
		if strings.Contains(row, h) {
			return true
		}
	}
	return false
}

// pickerBox is (banner top, hint) when the composer's hint carries a picker phrase and the picker's banner is above it.
func pickerBox(tail []string) (top int, ok bool) {
	top, bottom, has := term.ComposerBox(tail)
	if !has {
		return 0, false
	}
	hint := false
	for i := bottom; i < len(tail); i++ {
		hint = hint || hasPickerHint(tail[i])
	}
	if !hint {
		return 0, false
	}
	for _, x := range tail[:top] {
		if strings.Contains(x, backgroundBanner) {
			return top, true
		}
	}
	return 0, false
}

// hintBelowBox is whether the composer's hint says a picker phrase, banner or not: the unconfirmed match fails closed.
func hintBelowBox(tail []string) bool {
	_, bottom, ok := term.ComposerBox(tail)
	if !ok {
		return false
	}
	for i := bottom; i < len(tail); i++ {
		if hasPickerHint(tail[i]) {
			return true
		}
	}
	return false
}

// describeBackgrounded is the picker's banner and session list above its own box, quoted with the care of DescribeModal.
func describeBackgrounded(tail []string, top int) string {
	var shown []string
	for _, x := range tail[:top] {
		ln := term.Oneline(x)
		if ln != "" && !term.ComposerRuleRE.MatchString(ln) && ln != "❯" {
			shown = append(shown, ln)
		}
	}
	if len(shown) > 3 {
		shown = shown[:3]
	}
	return term.Cut(strings.Join(shown, " / "), term.DetailChars)
}

func (Screen) ScreenBlocker(tail []string) *term.Blocker {
	if footer := term.FooterRow(tail); footer >= 0 {
		b := &term.Blocker{Kind: term.Modal, Detail: term.DescribeModal(tail, footer)}
		if d := term.ParseDialog(tail, footer, false); d != nil {
			b.Title = d.Title
		}
		return b
	}
	if top, ok := pickerBox(tail); ok {
		return &term.Blocker{Kind: term.Backgrounded, Detail: describeBackgrounded(tail, top)}
	}
	if hintBelowBox(tail) {
		return &term.Blocker{Kind: term.Unreadable} // shaped like a picker not measured: fail closed
	}
	if term.ComposerShape(tail) {
		return nil
	}
	return &term.Blocker{Kind: term.Unreadable}
}

func (Screen) TrustAnswer() string { return trustPromptDownArrow }

// firstRunScreens are Claude Code's first-run setup steps as drawn (Claude Code 2.1.331): two lines of each, and the row each ends the
// screen with. The theme picker opens with "Let's get started." only on a first run (/theme in a session does not); the security notes
// end on "Press Enter to continue". The step's own last row is required at the bottom, so the same words in scrollback or in a file
// a program shows are not read as the step. The config key the daemon writes (hasCompletedOnboarding) keeps a new start from them; a
// program already on one stays there.
var firstRunScreens = []struct{ head, body, bottom string }{
	{"Let's get started.", "Choose the text style that looks best with your terminal", "Syntax theme:"},
	{"Due to prompt injection risks, only use it with code you trust", "You're responsible for Claude's actions", "Press Enter to continue"},
}

func (Screen) FirstRunScreen(tail []string) bool {
	last := ""
	for i := len(tail) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(tail[i]); t != "" {
			last = t
			break
		}
	}
	text := strings.Join(tail, "\n")
	for _, s := range firstRunScreens {
		if strings.HasPrefix(last, s.bottom) && strings.Contains(text, s.head) && strings.Contains(text, s.body) {
			return true
		}
	}
	return false
}

// Verified against a live prompt: one Down off the default cursor ("No, exit") lands on "Yes, I trust this folder"; a bare Enter
// then confirms it. A bare Enter with no Down confirms "No, exit" instead.
const trustPromptDownArrow = "\x1b[B"
