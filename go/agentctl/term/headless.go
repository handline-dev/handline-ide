package term

import "errors"

// Chat is what the terminal layer knows of sessions run headless (Chat mode) under a holder in their terminal: which terminals they are,
// and the two things the screen and keys did for them. The holder's window shows a status line and takes no keys.
type Chat interface {
	Headless(handle string) bool
	// Interrupt stops the running turn on the session's stream; EndInput ends its input, so it finishes its turn and exits.
	Interrupt(handle string) error
	EndInput(handle string) error
}

// ErrHeadless is the answer to a key sent to a headless session.
var ErrHeadless = errors.New("the session runs in Chat mode: no key reaches it")

func (t *Term) headless(handle string) bool { return t.Chat != nil && t.Chat.Headless(handle) }

// sendHeadless is Send at a headless session: Esc alone interrupts its turn on the stream, as it would at the screen; anything else is
// refused.
func (t *Term) sendHeadless(handle, text string, enter bool) (bool, string) {
	if text != Escape || enter {
		return false, ErrHeadless.Error()
	}
	if err := t.Chat.Interrupt(handle); err != nil {
		return false, err.Error()
	}
	return true, ""
}
