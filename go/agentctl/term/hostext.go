package term

// What the terminal channel asks of a host beyond Backend (handline/session/backends/tmux.py). A host that lacks one of these is
// refused as `unsupported` by the method that needs it; tmux has them all.

// Cursor is where the cursor is in the pane and how much of the pane's bottom is blank, from one read so the two agree.
type Cursor struct {
	X, Y       int
	Visible    bool
	Height     int
	BlankBelow int
}

// StyledReader reads rows with their SGR codes kept.
type StyledReader interface {
	ReadScreenANSI(handle string, limit int) []string
}

// HistoryReader reads the scrollback the host keeps itself; ok is false when it is unreadable. Row 0 is the oldest row held.
type HistoryReader interface {
	ReadHistory(handle string, before *int, limit int, styled bool) (first int, rows []string, ok bool)
}

// Sizer reads and changes a terminal's size.
type Sizer interface {
	Size(handle string) (cols, rows int, ok bool)
	Resize(handle string, cols, rows int) (bool, string)
}

// CursorReader says where a terminal's cursor is; nil when the host does not say.
type CursorReader interface {
	Cursor(handle string) *Cursor
}

// BytesSender types raw bytes as keystrokes.
type BytesSender interface {
	SendBytes(handle string, data []byte) (bool, string)
}

// ControlSpawner starts the control-mode client the bytes format streams from.
type ControlSpawner interface {
	ServesBytes() bool
	ControlCommand() []string
}

// Attacher is a host that has a command a person runs on the daemon's machine to see and answer its terminals.
type Attacher interface{ AttachCommand() []string }

// Focuser is a host that can bring a terminal to the front; false when it could not.
type Focuser interface{ Focus(handle string) bool }
