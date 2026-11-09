package term

import "strings"

// Pane is one pane of the host's terminal server, as the debug view lists it.
type Pane struct {
	Session, Window, WindowName, Handle string
	PID                                 int    // the pane's own process (its shell); 0 when unknown
	Command                             string // what tmux says runs in the pane now
	Path                                string // the pane's working directory
	Activity                            int64  // the window's last output, unix seconds; 0 when unknown
	Dead                                bool
}

// shells are the commands a pane shows when only its shell runs in it.
var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true, "ksh": true, "tcsh": true, "csh": true}

// AtShell is whether only the pane's shell runs in it: a login shell shows as "-zsh".
func (p Pane) AtShell() bool { return shells[strings.TrimPrefix(p.Command, "-")] }

// PaneLister is a host that can list every pane it has with its details.
type PaneLister interface {
	ListPanes() ([]Pane, bool)
}

func (d dynamic) ListPanes() ([]Pane, bool) {
	if l, ok := d.cur().(PaneLister); ok {
		return l.ListPanes()
	}
	return nil, false
}
