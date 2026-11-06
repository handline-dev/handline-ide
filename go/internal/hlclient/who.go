package hlclient

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/sessiontoken"
)

// Agent is $HANDLINE_AGENT as given, "" when unset.
func Agent() string {
	v, _ := paths.Getenv("AGENT")
	return v
}

// CurrentAgent is who is asking (ask.current_agent): $HANDLINE_AGENT, except under a Claude Code that itself runs under
// another, which inherited the name without being that agent and is nobody; that refusal is recorded.
func (c *Client) CurrentAgent() string {
	agent := Agent()
	if agent != "" && sessiontoken.New(c.Home).Nested(0) {
		c.noted(agent, "nested_claude_refused", os.Args[1:])
		quick := &Client{Home: c.Home, Dial: Dialer(c.Home, 2*time.Second), As: c.As, Deadline: 2 * time.Second, note: c.note}
		c.note = nil
		_, _ = quick.Call("hello", nil) // nothing else may follow a refusal to say who this is, so the note goes on its own
		return ""
	}
	return agent
}

// AskingAgent is CurrentAgent, reading a person at their own terminal as the founder (ask.asking_agent): an inherited
// name nobody vouches for is no one, and so is a script that lost the variable and has no terminal.
func (c *Client) AskingAgent() string {
	if Agent() != "" {
		return c.CurrentAgent()
	}
	if StdinIsTerminal() {
		return Human
	}
	return ""
}

// Human is the founder's name as a caller.
const Human = "human"

// WhoRefusal is why a write cannot be made as a caller that cannot be named (cli/ask._who_refusal).
func WhoRefusal(verb string) string {
	if a := Agent(); a != "" {
		return fmt.Sprintf("refused: this runs under a second Claude Code started inside an agent's session, which "+
			"inherited the name %s without being that agent; only the agent's own session can %s as it", a, verb)
	}
	return fmt.Sprintf("cannot tell who is %sing this: HANDLINE_AGENT is unset and this is not an interactive shell. "+
		"Agents are started by the daemon, which sets it.", verb)
}

// NoteVerbChain notes a state-changing command and the programs above it (ask.note_verb_chain), so a command whose caller the
// ancestry check could not tell apart can still be traced: they ride on the hello of the next call, and the daemon records them.
// A note no call carries is not made.
func (c *Client) NoteVerbChain(argv []string) {
	if a := Agent(); a != "" {
		c.noted(a, "verb_chain", argv)
	}
}

// noted sets the note the next hello carries.
func (c *Client) noted(agent, kind string, argv []string) {
	short := make([]string, 0, 3)
	for _, a := range argv {
		if len(short) == 3 {
			break
		}
		if r := []rune(a); len(r) > 40 {
			a = string(r[:40])
		}
		short = append(short, a)
	}
	var chain []string
	for i, a := range sessiontoken.New(c.Home).Chain(0) {
		if i == 8 {
			break
		}
		chain = append(chain, fmt.Sprintf("%s:%d", a.Program, a.PID))
	}
	c.note = map[string]any{"agent": agent, "kind": kind, "argv": short, "chain": strings.Join(chain, " > ")}
}
