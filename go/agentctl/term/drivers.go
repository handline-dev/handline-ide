package term

// Confirmation is a permission prompt read off a vendor's screen: what the tool call is (Title, Lines verbatim from the panel)
// and the option numbers that say a plain one-time yes and no.
type Confirmation struct {
	Title   string
	Lines   []string
	Yes, No int
}

// QuestionOption is one choice a Question offers, in the vendor's own words. Checked is only ever true on a multiSelect question.
type QuestionOption struct {
	Label       string
	Description string
	Checked     bool
}

// Question is one AskUserQuestion question read off a vendor's screen. Options are the choices Claude itself proposed, never
// "Type something" or "Chat about this". OtherNumber is the "Type something" control's number; OtherText the words typed into it
// ("" and HasOtherText false while it still reads "Type something"). Lines is the panel's own text, quoted the way Dialog.Body
// is. Cursor is the row the "❯" marker is on now.
type Question struct {
	Header       string
	Question     string
	Options      []QuestionOption
	MultiSelect  bool
	OtherNumber  int
	OtherText    string
	HasOtherText bool
	Lines        []string
	Cursor       int
}

// What on the screen a typed line would answer or merge into, named.
const (
	Modal        = "modal"
	Backgrounded = "backgrounded"
	Unreadable   = "unreadable"
)

// Blocker is why a line typed now would not land. Detail is quoted from the screen, never classified; Title is the dialog's own
// title, raw from ParseDialog, for a Modal it could parse.
type Blocker struct {
	Kind   string
	Detail string
	Title  string
}

// Reason is the blocker in words.
func (b Blocker) Reason() string {
	var base string
	switch b.Kind {
	case Modal:
		base = "a dialog is open on its screen"
	case Backgrounded:
		base = "its conversation moved to the background"
	default:
		return "its screen could not be read"
	}
	if b.Detail != "" {
		return base + ": " + b.Detail
	}
	return base
}

// Driver is what a vendor's own screen wording gives the readers (the screen half of handline/vendors).
type Driver interface {
	Name() string
	TurnOnScreen(tail []string) bool
	WorkingOnScreen(tail []string) (working, known bool)
	IdleWithBackground(tail []string) bool
	ScreenBlocker(tail []string) *Blocker
	// TrustScreen is the folder the vendor's folder-trust dialog names when tail shows it; shown is false when it is not
	// showing, and folder "" when it is showing but the folder could not be parsed out.
	TrustScreen(tail []string) (folder string, shown bool)
	// TrustAnswer is the keys sent, then Enter, to accept the folder TrustScreen named.
	TrustAnswer() string
	Confirmation(d *Dialog) *Confirmation
	AskQuestion(tail []string) *Question
	// AnswerQuestionKeys is the keys that answer q with choices (0-based) or text, or nil when this is not sent this way.
	AnswerQuestionKeys(q *Question, choices []int, text *string) []string
	QuestionToggleKeys(q *Question, choices []int) []string
	QuestionOtherAfterToggleKeys(q *Question, text string) []string
}

// Blind is the driver of a screen whose wording nobody here knows: it finds no turn, no dialog and no question, and presses nothing. Term
// uses it where its lookup gives no driver; the vendor's own wording is in the agent's package.
type Blind struct{}

func (Blind) Name() string                                            { return "" }
func (Blind) TurnOnScreen([]string) bool                              { return false }
func (Blind) WorkingOnScreen([]string) (bool, bool)                   { return false, false }
func (Blind) IdleWithBackground([]string) bool                        { return false }
func (Blind) ScreenBlocker([]string) *Blocker                         { return nil }
func (Blind) TrustScreen([]string) (string, bool)                     { return "", false }
func (Blind) TrustAnswer() string                                     { return "" }
func (Blind) Confirmation(*Dialog) *Confirmation                      { return nil }
func (Blind) AskQuestion([]string) *Question                          { return nil }
func (Blind) AnswerQuestionKeys(*Question, []int, *string) []string   { return nil }
func (Blind) QuestionToggleKeys(*Question, []int) []string            { return nil }
func (Blind) QuestionOtherAfterToggleKeys(*Question, string) []string { return nil }
