package term

import (
	"strings"
)

// Remote is the master's hand on its runners' terminals (go/internal/nodes, over each runner's link). node is a linked runner's id
// and handle that runner's own handle for the terminal.
type Remote interface {
	// Nodes is the runners linked now.
	Nodes() []string
	Open(node, agent, cwd, command string) (handle string, err error)
	Keys(node, handle, keys string, enter bool) error
	Screen(node, handle string, rows int) ([]string, error)
	Close(node, handle string) error
	Respawn(node, handle string) error
	Terminals(node string) ([]string, error)
}

// Machines is the one way to the terminals agents run in, on this machine or on a runner: each call goes by its handle. This machine's
// handles are its host's own (tmux's %14); a runner's is "<node>:<its handle>" (RunnerHandle), and a tmux handle starts with %, so the
// two never meet. A terminal opens where Where says the agent runs (placement sets it; until then every agent runs here).
//
// A runner answers Backend and Host only: its pane's pid, stamped agent, shells and runs answer "cannot say".
type Machines struct {
	Local  Backend
	Remote Remote                    // nil: no runners
	Where  func(agent string) string // the runner agent's terminal opens on; "" (or a nil Where) is this machine
}

// RunnerHandle is node's handle for its terminal h, as Machines takes it.
func RunnerHandle(node, h string) string { return node + ":" + h }

// OnRunner is the runner and its own handle when handle is a runner's.
func OnRunner(handle string) (node, h string, ok bool) {
	node, h, ok = strings.Cut(handle, ":")
	if !ok || node == "" || h == "" || strings.HasPrefix(node, "%") {
		return "", "", false
	}
	return node, h, true
}

func (m *Machines) remote(handle string) (node, h string, ok bool) {
	if m.Remote == nil {
		return "", "", false
	}
	return OnRunner(handle)
}

func (m *Machines) Name() string    { return m.Local.Name() }
func (m *Machines) Available() bool { return m.Local.Available() }

// ListTerminals is this machine's terminals and each linked runner's; nil when one of them could not say.
func (m *Machines) ListTerminals() map[string]bool {
	local := m.Local.ListTerminals()
	if local == nil || m.Remote == nil {
		return local
	}
	set := make(map[string]bool, len(local))
	for h, v := range local {
		set[h] = v
	}
	for _, node := range m.Remote.Nodes() {
		hs, err := m.Remote.Terminals(node)
		if err != nil {
			return nil
		}
		for _, h := range hs {
			set[RunnerHandle(node, h)] = true
		}
	}
	return set
}

func (m *Machines) ReadScreen(handle string, limit int) *Screen {
	node, h, ok := m.remote(handle)
	if !ok {
		return m.Local.ReadScreen(handle, limit)
	}
	rows, err := m.Remote.Screen(node, h, limit)
	if err != nil {
		return nil
	}
	return &Screen{Tail: rows, Source: "screen"}
}

func (m *Machines) Send(handle, text string, enter bool) (bool, string) {
	node, h, ok := m.remote(handle)
	if !ok {
		return m.Local.Send(handle, text, enter)
	}
	if err := m.Remote.Keys(node, h, text, enter); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// WaitIdle on a runner's terminal reads its screen from here: turn, the vendor's reading, does not cross the link.
func (m *Machines) WaitIdle(handle string, timeoutMs int, turn func([]string) bool) bool {
	if _, _, ok := m.remote(handle); !ok {
		return m.Local.WaitIdle(handle, timeoutMs, turn)
	}
	return ScreenIdle(func() *Screen { return m.ReadScreen(handle, 40) }, timeoutMs, turn)
}

func (m *Machines) CreateTerminal(command, cwd, title, agent string) (string, string) {
	node := ""
	if m.Where != nil && m.Remote != nil {
		node = m.Where(agent)
	}
	if node == "" {
		if h, ok := m.Local.(Host); ok {
			return h.CreateTerminal(command, cwd, title, agent)
		}
		return "", "the " + m.Name() + " terminal host cannot start terminals"
	}
	h, err := m.Remote.Open(node, agent, cwd, command)
	if err != nil {
		return "", "could not open a terminal on " + node + ": " + err.Error()
	}
	return RunnerHandle(node, h), ""
}

func (m *Machines) CloseTerminal(handle string, tab bool) (bool, string) {
	node, h, ok := m.remote(handle)
	if !ok {
		if l, ok := m.Local.(Host); ok {
			return l.CloseTerminal(handle, tab)
		}
		return false, "the " + m.Name() + " terminal host cannot close terminals"
	}
	if err := m.Remote.Close(node, h); err != nil { // a runner closes the terminal's window whatever tab says
		return false, err.Error()
	}
	return true, ""
}

func (m *Machines) RespawnTerminal(handle string) (bool, string) {
	node, h, ok := m.remote(handle)
	if !ok {
		if l, ok := m.Local.(Host); ok {
			return l.RespawnTerminal(handle)
		}
		return false, "the " + m.Name() + " terminal host cannot restart a terminal's shell"
	}
	if err := m.Remote.Respawn(node, h); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// What only this machine's host says. A runner's handle is never passed to it.

func (m *Machines) PaneAgent(handle string) (string, bool) {
	if l, ok := m.Local.(PaneAgenter); ok && !m.isRemote(handle) {
		return l.PaneAgent(handle)
	}
	return "", false
}

func (m *Machines) PanePid(handle string) int {
	if l, ok := m.Local.(panePidder); ok && !m.isRemote(handle) {
		return l.PanePid(handle)
	}
	return 0
}

func (m *Machines) PaneShell(handle string) (string, bool) {
	if l, ok := m.Local.(Shells); ok && !m.isRemote(handle) {
		return l.PaneShell(handle)
	}
	return "", false
}

func (m *Machines) RunOutput(handle string, rows int) []string {
	if l, ok := m.Local.(Runs); ok && !m.isRemote(handle) {
		return l.RunOutput(handle, rows)
	}
	return nil
}

// Shells and runs open on this machine: they are the founder's, beside the daemon.

func (m *Machines) CreateShell(cwd, title, agent string) (string, string) {
	if l, ok := m.Local.(Shells); ok {
		return l.CreateShell(cwd, title, agent)
	}
	return "", "the terminal host (" + m.Name() + ") cannot open a shell"
}

func (m *Machines) CreateRun(cwd, title, agent, tag, command string) (string, string) {
	if l, ok := m.Local.(Runs); ok {
		return l.CreateRun(cwd, title, agent, tag, command)
	}
	return "", "the terminal host (" + m.Name() + ") cannot run a command"
}

func (m *Machines) PaneRuns() map[string]PaneRun {
	if l, ok := m.Local.(Runs); ok {
		return l.PaneRuns()
	}
	return nil
}

// ListPanes is this machine's panes: a runner does not list its own yet.
func (m *Machines) ListPanes() ([]Pane, bool) {
	if l, ok := m.Local.(PaneLister); ok {
		return l.ListPanes()
	}
	return nil, false
}

func (m *Machines) isRemote(handle string) bool { _, _, ok := m.remote(handle); return ok }

var (
	_ Backend     = (*Machines)(nil)
	_ Host        = (*Machines)(nil)
	_ Shells      = (*Machines)(nil)
	_ Runs        = (*Machines)(nil)
	_ PaneAgenter = (*Machines)(nil)
	_ panePidder  = (*Machines)(nil)
	_ PaneLister  = (*Machines)(nil)
)
