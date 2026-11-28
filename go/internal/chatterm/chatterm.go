// Package chatterm is what the terminal layer is told of Chat sessions: which terminals run one, and how Esc and the end of a session
// reach its stream instead of its screen.
package chatterm

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/chatport"
	"github.com/handline/handline/go/agentctl/holder"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/kit/paths"
)

// Running is whether agent's session runs under a live Chat holder now. The setting says how its next session runs.
func Running(home, agent string) bool { return holder.Alive(paths.ChatSocket(home, agent)) }

// New is term.Chat for t: a terminal is headless while the agent stamped on it runs under a Chat holder. port is what reads the
// holders' streams; nil, or a port not built yet, knows nothing of them. A runner's terminal is stamped there, not here: onRunner is
// the agent whose terminal it is (a relay serves its holder here, package chatrelay).
func New(home string, t *term.Term, port func() *chatport.Port, onRunner func(handle string) string) term.Chat {
	return chatTerm{home: home, t: t, port: port, onRunner: onRunner}
}

type chatTerm struct {
	home     string
	t        *term.Term
	port     func() *chatport.Port
	onRunner func(handle string) string
}

func (c chatTerm) stream() *chatport.Port {
	if c.port == nil {
		return nil
	}
	return c.port()
}

func (c chatTerm) Idle(handle string) (bool, bool) {
	if p := c.stream(); p != nil {
		return p.Idle(c.agent(handle))
	}
	return false, false
}

func (c chatTerm) Background(handle string) (string, bool) {
	p := c.stream()
	if p == nil {
		return "", false
	}
	tasks, known := p.Background(c.agent(handle))
	names := make([]string, 0, len(tasks))
	for _, t := range tasks {
		names = append(names, cmp.Or(t.Description, t.ID))
	}
	return strings.Join(names, ", "), known
}

// AtLimit holds until the window the stream named resets, as the screen's limit dialog does until then.
func (c chatTerm) AtLimit(handle string) bool {
	p := c.stream()
	if p == nil {
		return false
	}
	return atLimit(p.Limit(c.agent(handle)), time.Now().Unix())
}

func atLimit(l agent.Limit, now int64) bool {
	return l.Blocked && (l.ResetsAt == 0 || now < l.ResetsAt)
}

func (c chatTerm) StopBackground(handle string, ids []string) (int, error) {
	p := c.stream()
	if p == nil {
		return 0, errors.New("the session's stream is not read yet")
	}
	return p.StopBackground(c.agent(handle), ids)
}

func (c chatTerm) agent(handle string) string {
	if _, _, ok := term.OnRunner(handle); ok {
		if c.onRunner == nil {
			return ""
		}
		return c.onRunner(handle)
	}
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
