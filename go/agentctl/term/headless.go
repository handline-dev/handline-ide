package term

import (
	"errors"
	"time"
)

// Chat is what the terminal layer knows of sessions run headless (Chat mode) under a holder in their terminal: which terminals they are,
// and the two things the screen and keys did for them. The holder's window shows a status line and takes no keys.
type Chat interface {
	Headless(handle string) bool
	// Interrupt stops the running turn on the session's stream; EndInput ends its input, so it finishes its turn and exits.
	Interrupt(handle string) error
	EndInput(handle string) error
	// What its stream last said: whether it is between turns and the work it runs behind them (known false before it said), and whether
	// its account is at its usage limit. StopBackground asks it to stop that work; n is how many pieces were asked.
	Idle(handle string) (idle, known bool)
	Background(handle string) (running string, known bool)
	AtLimit(handle string) bool
	// ids are the pieces the hooks listed, asked too while the stream has not listed its own (after a daemon restart).
	StopBackground(handle string, ids []string) (n int, err error)
}

// ErrHeadless is the answer to a key sent to a headless session.
var ErrHeadless = errors.New("the session runs in Chat mode: no key reaches it")

func (t *Term) headless(handle string) bool { return t.Chat != nil && t.Chat.Headless(handle) }

// Headless is whether handle's session runs in Chat mode now.
func (t *Term) Headless(handle string) bool { return t.headless(handle) }

// StopBackground stops the background work of a headless session on its stream; ids are the pieces its hooks listed.
func (t *Term) StopBackground(handle string, ids []string) (int, error) {
	if !t.headless(handle) {
		return 0, errors.New("the session runs in a terminal")
	}
	return t.Chat.StopBackground(handle, ids)
}

// streamIdle is a headless session's turn as its stream says, polled until idle or timeout.
func (t *Term) streamIdle(handle string, timeout time.Duration) (idle, known bool) {
	end := t.now().Add(timeout)
	for {
		idle, known = t.Chat.Idle(handle)
		if !known || idle || !t.now().Before(end) {
			return idle, known
		}
		t.sleep(200 * time.Millisecond)
	}
}

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
