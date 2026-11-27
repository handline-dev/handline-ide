// Package chatterm is what the terminal layer is told of Chat sessions: which terminals run one, and how Esc and the end of a session
// reach its stream instead of its screen.
package chatterm

import (
	"fmt"
	"time"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/holder"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/kit/paths"
)

// Running is whether agent's session runs under a live Chat holder now. The setting says how its next session runs.
func Running(home, agent string) bool { return holder.Alive(paths.ChatSocket(home, agent)) }

// New is term.Chat for t: a terminal is headless while the agent stamped on it runs under a Chat holder.
func New(home string, t *term.Term) term.Chat { return chatTerm{home: home, t: t} }

type chatTerm struct {
	home string
	t    *term.Term
}

func (c chatTerm) agent(handle string) string {
	a, _ := c.t.PaneAgent(handle)
	return a
}

func (c chatTerm) Headless(handle string) bool {
	a := c.agent(handle)
	return a != "" && Running(c.home, a)
}

func (c chatTerm) Interrupt(handle string) error {
	line := claudecode.Driver{}.InterruptLine(fmt.Sprintf("hl_interrupt_%d", time.Now().UnixNano()))
	return holder.Write(paths.ChatSocket(c.home, c.agent(handle)), line)
}

func (c chatTerm) EndInput(handle string) error {
	return holder.CloseInput(paths.ChatSocket(c.home, c.agent(handle)))
}
