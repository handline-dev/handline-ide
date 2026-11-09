// Package term is how the daemon talks to the terminal an agent runs in (handline/session/term.py and backends/): a host that
// lists terminals, reads the rendered screen and types into one, and the readers above it that turn a screen into a dialog,
// a permission prompt or a question, and press keys on it only for what a fresh read just showed.
package term

// Screen is a rendered frame: the bottom rows, top first, and where they came from. Anything but "screen" is the
// accumulated stream, whose history is the conversation and not what is drawn now.
type Screen struct {
	Tail   []string
	Source string
}

// Backend is everything the daemon asks of the program that hosts a terminal. Every method answers rather than panics, and
// "could not ask" is never "nothing there": ListTerminals and ReadScreen return nil when they could not answer.
type Backend interface {
	Name() string
	Available() bool
	ListTerminals() map[string]bool
	// ReadScreen is the bottom limit rows of the rendered frame.
	ReadScreen(handle string, limit int) *Screen
	// Send types text (raw bytes pass through), then Enter if asked.
	Send(handle, text string, enter bool) (bool, string)
	// WaitIdle is whether the terminal's program is quiet within timeoutMs; a timeout is false. turn is the vendor's reading of
	// whether a screen shows a turn in progress, for a host that must read the screen to tell.
	WaitIdle(handle string, timeoutMs int, turn func(tail []string) bool) bool
}

// Place is where a program keeps its terminals: Key names its set (an in-process host keys its panes by it), Socket is the tmux server
// (tmux -L), and Env the variables every new terminal gets, KEY=VALUE.
type Place struct {
	Key, Socket string
	Env         []string
}

// ---- which host

type unknown struct{ name string }

func (u unknown) Name() string                 { return u.name }
func (unknown) Available() bool                { return false }
func (unknown) ListTerminals() map[string]bool { return nil }
func (unknown) ReadScreen(string, int) *Screen { return nil }
func (u unknown) Send(string, string, bool) (bool, string) {
	return false, "unknown terminal backend " + PyRepr(u.name)
}
func (unknown) WaitIdle(string, int, func([]string) bool) bool { return false }

// backends are the hosts this build has, by name: package hosts registers the ones of the OS it is built for.
var backends = map[string]func(Place) Backend{}

// Register makes a host known by name; package hosts calls it for each host the build has.
func Register(name string, open func(Place) Backend) { backends[name] = open }

// ForName is the host called name, at p. A name nobody knows gives a backend that is never available, so a typo stops the team with a
// message instead of quietly running on the other host.
func ForName(name string, p Place) Backend {
	if b, ok := backends[name]; ok {
		return b(p)
	}
	return unknown{name}
}

// dynamic is the backend in force, asked again on every call: the program's settings can change while it runs.
type dynamic struct{ current func() Backend }

// Dynamic is a Backend that is whatever current says each time it is used.
func Dynamic(current func() Backend) Backend { return dynamic{current} }

func (d dynamic) cur() Backend                                { return d.current() }
func (d dynamic) Name() string                                { return d.cur().Name() }
func (d dynamic) Available() bool                             { return d.cur().Available() }
func (d dynamic) ListTerminals() map[string]bool              { return d.cur().ListTerminals() }
func (d dynamic) ReadScreen(h string, n int) *Screen          { return d.cur().ReadScreen(h, n) }
func (d dynamic) Send(h, t string, enter bool) (bool, string) { return d.cur().Send(h, t, enter) }
func (d dynamic) WaitIdle(h string, ms int, turn func([]string) bool) bool {
	return d.cur().WaitIdle(h, ms, turn)
}
