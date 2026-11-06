package agent

import (
	"errors"

	"github.com/handline/handline/go/agentctl/delivery"
)

// ErrNoSession is the answer of a DeliverEnv.Where for an agent with no live session.
var ErrNoSession = errors.New("no live session")

// Port is how messages reach an agent's session: the supervisor's one send path asks only this.
type Port interface {
	// Structured says whether agent runs as a structured session: its kind, not whether the process is up.
	Structured(agent string) bool
	// Live says whether the agent's session is open and can be written to.
	Live(agent string) bool
	// Send delivers text under the operation id. The same op twice writes once and answers the first receipt. Now joins a running turn;
	// Queue is held until the turn ends. An error means nothing was written.
	Send(agent, op, text string, d delivery.Delivery) (delivery.Receipt, error)
	// Withdraw takes back a message still held behind a turn; ok is whether it was (also on a repeat). Promote writes a held message now.
	Withdraw(agent, op string) (delivery.Receipt, bool, error)
	Promote(agent, op string) (delivery.Receipt, error)
}

// Place is where an agent's session runs: the account's config dir and the worktree, and which processes may be the session's own.
type Place struct {
	Config, Cwd string
	Alive       func(pid int) bool
}

// DeliverEnv is what the daemon knows that delivering needs; the CLI turns it into its way of getting a message in.
type DeliverEnv struct {
	// Peer says whether agent is delivered to through its session's inbox.
	Peer func(agent string) bool
	// Where is the agent's session place, ErrNoSession when it has none.
	Where func(agent string) (Place, error)
	// From names who wrote the message op to agent; "" is the daemon.
	From func(agent, op string) string
	// OnDelivered is called once the session shows op; OnFailed when it did not (written says whether the message was written at all).
	OnDelivered func(agent, op string)
	OnFailed    func(agent, op, why string, written bool)
	// Typed says op is typed into the agent's terminal instead. Typeable is "" when it may be now, else why not; TypeIn types text
	// there and presses Enter.
	Typed    func(agent, op string) bool
	Typeable func(agent string) string
	TypeIn   func(agent, text string) error
}
