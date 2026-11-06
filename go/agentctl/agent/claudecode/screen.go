package claudecode

import (
	"regexp"
	"strconv"
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

// The line the prompt names its folder under. Wrapped at terminal width with no separator, so its continuation lines are joined bare.
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
		if s == "" {
			break
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

// Verified against a live prompt: one Down off the default cursor ("No, exit") lands on "Yes, I trust this folder"; a bare Enter
// then confirms it. A bare Enter with no Down confirms "No, exit" instead.
const trustPromptDownArrow = "\x1b[B"

// Confirmation is Claude Code's one-time permission prompt, reconstructed from its wording, not measured on a live screen: the
// match is as narrow as that wording allows, so a layout that differs is not recognized and stays for the terminal.
func (Screen) Confirmation(d *term.Dialog) *term.Confirmation {
	if !strings.Contains(d.Footer, "Esc to cancel") {
		return nil
	}
	var prose []string
	for _, ln := range d.Body {
		if !term.OptionLabelRE.MatchString(ln) {
			prose = append(prose, ln)
		}
	}
	var asked []int
	for i, ln := range prose {
		if strings.ToLower(strings.TrimSpace(ln)) == "do you want to proceed?" {
			asked = append(asked, i)
		}
	}
	if len(asked) != 1 || asked[0] < 2 {
		return nil // no question, or nothing above it saying what is asked
	}
	var yes, no []int
	for _, o := range d.Options {
		l := term.NormLabel(o.Label)
		if l == "yes" {
			yes = append(yes, o.Number)
		}
		if strings.SplitN(l, ",", 2)[0] == "no" {
			no = append(no, o.Number)
		}
	}
	if len(yes) != 1 || len(no) != 1 {
		return nil
	}
	return &term.Confirmation{Title: prose[0], Lines: append([]string{}, prose[1:asked[0]]...), Yes: yes[0], No: no[0]}
}

func (Screen) AskQuestion(tail []string) *term.Question { return recognizeAsk(tail) }

func (Screen) AnswerQuestionKeys(q *term.Question, choices []int, text *string) []string {
	return questionKeys(q, choices, text)
}

func (Screen) QuestionToggleKeys(q *term.Question, choices []int) []string {
	return toggleKeysForOther(q, choices)
}

func (Screen) QuestionOtherAfterToggleKeys(q *term.Question, text string) []string {
	return finishOtherKeys(q, text)
}

// ---- claude's AskUserQuestion picker

// parse_dialog cannot read this screen: it takes the rule closest to the footer as the panel's own top, which here is the second
// rule, above only "Chat about this". This walks tail from the footer upward through the fixed shape: footer, blank,
// "N. Chat about this", blank, a rule, the numbered options (each optionally an indented description line below it), the
// question, the header, a rule. A call with more than one question, or a multiSelect question's own "Review your answers" screen,
// is not read: recognizeAsk returns nil rather than guess at either.
const askFooterMarker = "Esc to cancel"

var (
	chatAboutThisRE = regexp.MustCompile(`(?i)^(\d+)\.\s+chat about this$`)
	singleHeaderRE  = regexp.MustCompile(`^☐\s+(.+)$`)
	tabHeaderRE     = regexp.MustCompile(`^←\s+(.+)→$`)
	tabIconRE       = regexp.MustCompile(`^[☐☒✅✔]\s*`)
	askOptionRE     = regexp.MustCompile(`^(❯)?\s*(\d+)\.\s+(?:\[(.)\]\s*)?(\S.*)$`)
	twoSpacesRE     = regexp.MustCompile(`\s{2,}`)
)

const (
	typeSomething = "type something"
	right         = "\x1b[C"
	down          = "\x1b[B"
)

// questionKeys is the keys that answer q, sent in order, or nil when this combination is not sent here. One path per shape:
// single-select a choice: its own number submits outright; single-select Other: its number, the text, Enter; multiSelect
// choices: toggle only what differs from what is checked, then Right onto "Submit", then "1"; multiSelect Other: Down from the
// cursor to Other, the text, one more Down onto "Submit", Enter, then "1" for the Review screen. Boxes and Other together are
// refused: toggling a box moves the cursor too.
func questionKeys(q *term.Question, choices []int, text *string) []string {
	for _, c := range choices {
		if c < 0 || c >= len(q.Options) {
			return nil
		}
	}
	if text != nil {
		if q.OtherNumber == 0 || len(choices) > 0 {
			return nil
		}
		if !q.MultiSelect {
			return []string{strconv.Itoa(q.OtherNumber), *text, term.Enter}
		}
		downs := q.OtherNumber - q.Cursor
		if downs < 0 {
			return nil
		}
		return append(repeat(down, downs), *text, down, term.Enter, "1")
	}
	if !q.MultiSelect {
		if len(choices) == 1 {
			return []string{strconv.Itoa(choices[0] + 1)}
		}
		return nil
	}
	// Empty choices is a real answer on a multiSelect question: nothing picked, so it still submits.
	return append(toggleKeys(q, choices), right, "1")
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func toggleKeys(q *term.Question, choices []int) []string {
	desired := map[int]bool{}
	for _, c := range choices {
		desired[c] = true
	}
	var keys []string
	for i, o := range q.Options {
		if desired[i] != o.Checked {
			keys = append(keys, strconv.Itoa(i+1))
		}
	}
	return keys
}

// toggleKeysForOther is stage one of a multiSelect question's boxes-and-Other combo.
func toggleKeysForOther(q *term.Question, choices []int) []string {
	if !q.MultiSelect {
		return nil
	}
	for _, c := range choices {
		if c < 0 || c >= len(q.Options) {
			return nil
		}
	}
	return toggleKeys(q, choices)
}

// finishOtherKeys is stage two, given q freshly re-read after the toggles landed.
func finishOtherKeys(q *term.Question, text string) []string {
	if q.OtherNumber == 0 {
		return nil
	}
	downs := q.OtherNumber - q.Cursor
	if downs < 0 {
		return nil
	}
	return append(repeat(down, downs), text, down, term.Enter, "1")
}

func recognizeAsk(tail []string) *term.Question {
	footer := -1
	for i := len(tail) - 1; i >= 0; i-- {
		if strings.Contains(tail[i], askFooterMarker) {
			footer = i
			break
		}
	}
	if footer < 0 {
		return nil
	}
	lines := make([]string, footer)
	for i := range lines {
		lines[i] = strings.TrimRight(tail[i], " \t\r\n\f\v")
	}
	i := lastNonblank(lines, len(lines)-1)
	if i < 0 {
		return nil
	}
	chat := chatAboutThisRE.FindStringSubmatch(strings.TrimSpace(lines[i]))
	if chat == nil {
		return nil
	}
	chatNumber, _ := strconv.Atoi(chat[1])
	midRule := lastNonblank(lines, i-1)
	if midRule < 0 || !term.ComposerRuleRE.MatchString(strings.TrimSpace(lines[midRule])) {
		return nil
	}
	topRule := -1
	for k := midRule - 1; k >= 0; k-- {
		if term.ComposerRuleRE.MatchString(strings.TrimSpace(lines[k])) {
			topRule = k
			break
		}
	}
	if topRule < 0 {
		return nil
	}
	var body []string
	for _, ln := range lines[topRule+1 : midRule] {
		if strings.TrimSpace(ln) != "" {
			body = append(body, ln)
		}
	}
	if len(body) < 3 {
		return nil
	}
	header, multi, ok := askHeader(strings.TrimSpace(body[0]))
	if !ok {
		return nil
	}
	opts, other, otherText, hasOther, cursor := askOptions(body[2:], multi)
	if other == 0 || other+1 != chatNumber {
		return nil
	}
	return &term.Question{Header: header, Question: strings.TrimSpace(body[1]), Options: opts, MultiSelect: multi, OtherNumber: other,
		OtherText: otherText, HasOtherText: hasOther, Lines: body, Cursor: cursor}
}

func lastNonblank(lines []string, start int) int {
	i := start
	for i >= 0 && strings.TrimSpace(lines[i]) == "" {
		i--
	}
	return i
}

// askHeader is the header line of a lone question: " ☐ <header>" (single-select) or "←  ☒ <header>  ✔ Submit  →" (multiSelect,
// which always has its own "Submit" step even with one question).
func askHeader(line string) (header string, multi, ok bool) {
	if m := singleHeaderRE.FindStringSubmatch(line); m != nil {
		return strings.TrimSpace(m[1]), false, true
	}
	m := tabHeaderRE.FindStringSubmatch(line)
	if m == nil {
		return "", false, false
	}
	var parts []string
	for _, p := range twoSpacesRE.Split(strings.TrimSpace(m[1]), -1) {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, strings.TrimSpace(tabIconRE.ReplaceAllString(p, "")))
		}
	}
	// A real second question adds a third tab before "Submit": not this scan's to read.
	if len(parts) != 2 || strings.ToLower(parts[1]) != "submit" {
		return "", false, false
	}
	return parts[0], true, true
}

func askOptions(lines []string, multi bool) (opts []term.QuestionOption, other int, otherText string, hasOther bool, cursor int) {
	cursor = 1
	i, n := 0, 1
	for i < len(lines) {
		m := askOptionRE.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			return nil, 0, "", false, 1
		}
		if num, _ := strconv.Atoi(m[2]); num != n {
			return nil, 0, "", false, 1
		}
		if m[1] != "" {
			cursor = n
		}
		box, label := m[3], strings.TrimSpace(m[4])
		i++
		var desc []string
		for i < len(lines) && askOptionRE.FindStringSubmatch(strings.TrimSpace(lines[i])) == nil {
			text := strings.TrimSpace(lines[i])
			// A multiSelect header's own wide tab bar sometimes wraps its "Submit" tab onto the row under the last option.
			if !(multi && i == len(lines)-1 && text == "Submit") {
				desc = append(desc, text)
			}
			i++
		}
		isLast := i == len(lines)
		switch {
		case strings.ToLower(strings.TrimRight(label, ".")) == typeSomething:
			other = n
		case isLast && other == 0:
			other, otherText, hasOther = n, label, true
		default:
			opts = append(opts, term.QuestionOption{Label: label, Description: strings.Join(desc, " "), Checked: box == "✔" || box == "x" || box == "X"})
		}
		n++
	}
	return opts, other, otherText, hasOther, cursor
}
