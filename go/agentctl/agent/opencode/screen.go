package opencode

import (
	"regexp"
	"strings"

	"github.com/handline/handline/go/agentctl/internal/oneline"
	"github.com/handline/handline/go/agentctl/term"
)

// ---- opencode: the screen is its only witness

type Screen struct{}

func (Screen) Name() string { return "opencode" }

var boxBottomRE = regexp.MustCompile(`^\s*\x{2579}\x{2580}{8,}`)

const footerRows = 6

// footer is the rows under the prompt box; ok is false when no box is drawn.
func (Screen) footer(tail []string) ([]string, bool) {
	rows := tail
	if len(rows) > footerRows {
		rows = rows[len(rows)-footerRows:]
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if boxBottomRE.MatchString(rows[i]) {
			return rows[i+1:], true
		}
	}
	return nil, false
}

func (o Screen) WorkingOnScreen(tail []string) (bool, bool) {
	footer, ok := o.footer(tail)
	if !ok {
		return false, false
	}
	for _, r := range footer {
		if strings.Contains(r, "interrupt") {
			return true, true
		}
	}
	return false, true
}

func (o Screen) TurnOnScreen(tail []string) bool { w, _ := o.WorkingOnScreen(tail); return w }

// opencode's screen carries no background count.
func (Screen) IdleWithBackground([]string) bool { return false }

// ScreenBlocker: a permission request replaces the prompt box with its choices, the first already selected, so Enter answers it.
// Anything without the box cannot be typed into safely.
func (o Screen) ScreenBlocker(tail []string) *term.Blocker {
	low := max(0, len(tail)-footerRows)
	ask := -1
	for i := len(tail) - 1; i >= low; i-- {
		if strings.Contains(tail[i], "enter confirm") {
			ask = i
			break
		}
	}
	if ask >= 0 {
		lines := make([]string, ask)
		for i := 0; i < ask; i++ {
			lines[i] = oneline.Oneline(strings.Trim(tail[i], " ┃"))
		}
		title := -1
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.HasPrefix(lines[i], "△") {
				title = i
				break
			}
		}
		from := lines
		if title >= 0 {
			from = lines[title:]
		} else if len(lines) > 3 {
			from = lines[len(lines)-3:]
		}
		var shown []string
		for _, ln := range from {
			if ln != "" {
				shown = append(shown, ln)
			}
		}
		if len(shown) > 3 {
			shown = shown[:3]
		}
		return &term.Blocker{Kind: term.Modal, Detail: term.Cut(strings.Join(shown, " / "), term.DetailChars)}
	}
	if _, ok := o.footer(tail); !ok {
		return &term.Blocker{Kind: term.Unreadable}
	}
	return nil
}

// No folder-trust dialog, permission prompt or AskUserQuestion of its own to detect.
func (Screen) TrustScreen([]string) (string, bool)                          { return "", false }
func (Screen) TrustAnswer() string                                          { return "" }
func (Screen) Confirmation(*term.Dialog) *term.Confirmation                 { return nil }
func (Screen) AskQuestion([]string) *term.Question                          { return nil }
func (Screen) AnswerQuestionKeys(*term.Question, []int, *string) []string   { return nil }
func (Screen) QuestionToggleKeys(*term.Question, []int) []string            { return nil }
func (Screen) QuestionOtherAfterToggleKeys(*term.Question, string) []string { return nil }
