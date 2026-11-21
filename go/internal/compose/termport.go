package compose

import (
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/scheduler/lifecycle"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// This file is the terminal an agent runs in, as the lifecycle and the telling layer see it: a tmux window on the daemon's own server, in
// which `hl launch` starts the vendor's session. The daemon launches (one typed line), ends a session, closes a terminal and looks at the
// screen; it types nothing at a running session: messages reach it by its inbox.

// termPort is lifecycle.Terminal and supervisor.Terminal over one Term; termHost is lifecycle.Host over the same one.
type termPort struct{ t *term.Term }

type termHost struct{ t *term.Term }

// newTerm is the Term over the machines agents run on: the host in force for home (tmux unless config.yaml says otherwise) and the
// runners, each terminal read in the wording of its agent's vendor.
func newTerm(home string, q db.Q, runners *nodes.Registry) *term.Term {
	m := &term.Machines{Local: agenthost.Dynamic(home)}
	if runners != nil {
		m.Remote = runners.Terminals()
		if runners.Placer != nil {
			m.Where, m.Placed = runners.Placer.Where, runners.Placer.Placed
		}
	}
	return term.New(m, team.ScreenFor(home, q, claudecode.ScreenFor))
}

var (
	_ lifecycle.Terminal  = termPort{}
	_ lifecycle.Host      = termHost{}
	_ supervisor.Terminal = termPort{}
)

func (p termPort) Available() bool { return p.t.Backend.Available() }
func (p termPort) Missing() string { return p.t.Missing() }

func (p termPort) WaitIdle(handle string) bool                 { return p.t.WaitIdle(handle, term.IdleTimeoutMs) }
func (p termPort) WaitDrawn(handle string) bool                { return p.t.WaitDrawn(handle, term.DrawnTimeoutMs) }
func (p termPort) TypeableNow(handle string) bool              { return p.t.TypeableNow(handle) }
func (p termPort) ScreenBlocker(handle string) *term.Blocker   { return p.t.ScreenBlocker(handle) }
func (p termPort) BackgroundWork(handle string) (string, bool) { return p.t.BackgroundWork(handle) }

// The folder-trust screen, read only: a launch marks its worktree trusted, so the screen showing is a fault the lifecycle reports.
func (p termPort) TrustPromptShowing(handle string) bool  { return p.t.TrustPromptShowing(handle) }
func (p termPort) TrustPromptFolder(handle string) string { return p.t.TrustPromptFolder(handle) }
func (p termPort) DriverScope() func()                    { return p.t.DriverScope() }

func (p termPort) CloseSession(handle string) (bool, string)  { return p.t.CloseSession(handle) }
func (p termPort) CloseTerminal(handle string) (bool, string) { return p.t.CloseTerminal(handle) }
func (p termPort) Relaunch(handle, cmd string, started func() bool) (bool, string) {
	return p.t.Relaunch(handle, cmd, started)
}

// supervisor.Terminal
func (p termPort) SessionOnScreen(handle string) (bool, bool) { return p.t.SessionOnScreen(handle) }

// lifecycle.Host
func (h termHost) Available() bool { return h.t.Backend.Available() }
func (h termHost) Missing() string { return h.t.Missing() }
func (h termHost) OpenTerminal(cmd, cwd, title, agent string) (string, string) {
	return h.t.CreateTerminal(cmd, cwd, title, agent)
}

func (h termHost) WaitIdle(handle string, ms int) bool { return h.t.WaitIdle(handle, ms) }

// Submit would type the first prompt at the new session; it goes in through the session inbox, so none is typed here and the spawn reports
// that the prompt did not land.
func (termHost) Submit(string, string, int) {}

// The orchestration runs were Orca's: tmux has none.
func (termHost) HasRuns() bool                       { return false }
func (termHost) RunCurrent() string                  { return "" }
func (termHost) RunExists(string) bool               { return false }
func (termHost) RunUse(string)                       {}
func (termHost) RunCreate(string) string             { return "" }
func (termHost) TaskCreate(_, _, _, _ string) string { return "" }
func (termHost) Dispatch(_, _, _ string) string      { return "" }
