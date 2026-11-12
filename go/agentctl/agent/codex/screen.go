package codex

import (
	"regexp"
	"strings"

	"github.com/handline/handline/go/agentctl/term"
)

// Screen is codex 0.157's TUI as captured in testdata: a composer row "› ..." near the bottom with the model and "? for shortcuts"
// under it, "• Working (... esc to interrupt)" above it while a turn thinks, and dialogs that replace the composer with options of
// which the chosen one is marked "› 1. ...".
type Screen struct{}

var _ term.Driver = Screen{}

func (Screen) Name() string { return Vendor }

const (
	bottomRows   = 16 // the composer, a draft of some lines, and the rows under it
	composerMark = "› "
	trustAsk     = "Trust this folder?"
)

var (
	optionRE  = regexp.MustCompile(`^\s*(?:[›>] )?(\d+)\.\s+\S`)
	workingRE = regexp.MustCompile(`^• Working \(.*esc to interrupt\)`)
	// A finished turn ends with the time it ended on a line of its own; from 0.167, after how long it worked ("Worked for 7s • 00:54").
	endedRE = regexp.MustCompile(`^\s*(?:Worked for .* • )?\d{1,2}:\d{2}(?:\s*[AP]M)?\s*$`)
)

// composer is the row of the composer among tail's bottom rows, -1 when none is drawn (a dialog, or not codex's screen). A dialog's
// chosen option is drawn "› 1. ..." too; it has another option or the dialog's footer under it, and a draft that starts "1. " has not.
func composer(tail []string) int {
	for i := len(tail) - 1; i >= max(0, len(tail)-bottomRows); i-- {
		if strings.HasPrefix(tail[i], composerMark) && !(optionRE.MatchString(tail[i]) && dialogBelow(tail[i+1:])) {
			return i
		}
	}
	return -1
}

// dialogBelow is whether rows, those under a chosen option, are a dialog's: another option or its footer.
func dialogBelow(rows []string) bool {
	for _, r := range rows {
		if optionRE.MatchString(r) || strings.Contains(r, "Press enter to") || strings.Contains(r, "enter continue") {
			return true
		}
	}
	return false
}

// lastBlock is the rows of the last run of non-blank rows above row end, skipping codex's "Tip:" rows.
func lastBlock(tail []string, end int) []string {
	i := end - 1
	for i >= 0 && (strings.TrimSpace(tail[i]) == "" || strings.HasPrefix(strings.TrimSpace(tail[i]), "Tip: ")) {
		i--
	}
	j := i
	for j >= 0 && strings.TrimSpace(tail[j]) != "" {
		j--
	}
	return tail[j+1 : i+1]
}

// WorkingOnScreen: the Working row is a turn; a finished turn's time row, an interrupted one's "■" or the bare header box is none.
// A reply still streaming draws neither, so the screen alone cannot tell it: known is false then, for the hooks (V5) to answer.
func (Screen) WorkingOnScreen(tail []string) (bool, bool) {
	c := composer(tail)
	if c < 0 {
		return false, false
	}
	block := lastBlock(tail, c)
	if len(block) == 0 {
		return false, true
	}
	if workingRE.MatchString(strings.TrimSpace(block[len(block)-1])) {
		return true, true
	}
	last := block[len(block)-1]
	if endedRE.MatchString(last) || strings.HasPrefix(block[0], "■ ") || strings.HasPrefix(strings.TrimSpace(last), "╰") {
		return false, true
	}
	return false, false
}

func (s Screen) TurnOnScreen(tail []string) bool { w, _ := s.WorkingOnScreen(tail); return w }

// codex's screen carries no background count.
func (Screen) IdleWithBackground([]string) bool { return false }

// ScreenBlocker: with the composer drawn a typed line lands. A dialog (an approval, the trust, login, update or hook-review screen) is
// quoted: its question, the command an approval asks about, and its options. Anything else cannot be typed into safely.
func (Screen) ScreenBlocker(tail []string) *term.Blocker {
	if composer(tail) >= 0 {
		return nil
	}
	first := -1
	var shown []string
	for i, ln := range tail {
		if m := optionRE.FindStringSubmatch(ln); m != nil {
			if m[1] == "1" {
				first, shown = i, nil
			}
			shown = append(shown, term.Oneline(strings.TrimLeft(strings.TrimSpace(ln), "›> ")))
		}
	}
	if first < 0 {
		return &term.Blocker{Kind: term.Unreadable}
	}
	// The question is the topmost row with a "?" above the options, up to the transcript's last "•" item (an approval also gives its
	// reason as a question).
	title, cmd := "", ""
	for i := first - 1; i >= 0; i-- {
		t := term.Oneline(tail[i])
		if strings.HasPrefix(t, "• ") {
			break
		}
		switch {
		case strings.HasPrefix(t, "$ ") && cmd == "":
			cmd = t
		case strings.Contains(t, "?"):
			title = t
		}
	}
	if title == "" { // no question asked: the screen's own first row names it
		for _, ln := range tail[:first] {
			if t := term.Oneline(ln); t != "" {
				title = t
				break
			}
		}
	}
	parts := []string{title}
	if cmd != "" {
		parts = append(parts, cmd)
	}
	return &term.Blocker{Kind: term.Modal, Title: title, Detail: term.Cut(strings.Join(append(parts, shown...), " / "), term.DetailChars)}
}

// TrustScreen is the folder codex's trust screen names: the rows under "Folder access", which wrap a long path with no break.
func (Screen) TrustScreen(tail []string) (string, bool) {
	ask, access := -1, -1
	for i, ln := range tail {
		switch t := strings.TrimSpace(ln); {
		case t == "Folder access":
			access = i
		case strings.HasPrefix(t, trustAsk):
			ask = i
		}
	}
	if ask < 0 {
		return "", false
	}
	if access < 0 || access > ask {
		return "", true
	}
	var folder strings.Builder
	for _, ln := range tail[access+1 : ask] {
		if t := strings.TrimSpace(ln); t != "" {
			folder.WriteString(t)
		}
	}
	return folder.String(), true
}

// TrustAnswer: "Trust and continue" is already chosen, so Enter alone takes it.
func (Screen) TrustAnswer() string { return "" }

// Answering an approval from Handline is not built: it stays for the person at the terminal, which ScreenBlocker shows.
func (Screen) Confirmation(*term.Dialog) *term.Confirmation                 { return nil }
func (Screen) AskQuestion([]string) *term.Question                          { return nil }
func (Screen) AnswerQuestionKeys(*term.Question, []int, *string) []string   { return nil }
func (Screen) QuestionToggleKeys(*term.Question, []int) []string            { return nil }
func (Screen) QuestionOtherAfterToggleKeys(*term.Question, string) []string { return nil }
