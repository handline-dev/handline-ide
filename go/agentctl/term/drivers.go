package term

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
	// FirstRunScreen is whether tail shows the vendor's first-run setup (its onboarding screens), which a program that started before
	// its config said setup was done stays on: no session runs under it yet.
	FirstRunScreen(tail []string) bool
}

// Blind is the driver of a screen whose wording nobody here knows: it finds no turn and no dialog, and presses nothing. Term
// uses it where its lookup gives no driver; the vendor's own wording is in the agent's package.
type Blind struct{}

func (Blind) Name() string                          { return "" }
func (Blind) TurnOnScreen([]string) bool            { return false }
func (Blind) WorkingOnScreen([]string) (bool, bool) { return false, false }
func (Blind) IdleWithBackground([]string) bool      { return false }
func (Blind) ScreenBlocker([]string) *Blocker       { return nil }
func (Blind) TrustScreen([]string) (string, bool)   { return "", false }
func (Blind) TrustAnswer() string                   { return "" }
func (Blind) FirstRunScreen([]string) bool          { return false }
