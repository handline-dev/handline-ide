package compose

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/peerinbox"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/lock"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/worktreepaths"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// PeerKey is the top-level agent.yaml key that has a terminal agent's messages delivered through its session's peer inbox. On unless the
// agent's agent.yaml says `peer_inbox: false`: nothing types at a terminal any more, so the inbox is the only way in.
const PeerKey = "peer_inbox"

// PeerFromYAML reads PeerKey from an agent's agent.yaml.
func PeerFromYAML(home string) func(agent string) bool { return yamlFlag(home, PeerKey, true) }

// SettlePeerSends is the start of a daemon, run once the Env has its ports (telling a sender can start an agent): the peer port's queue is in memory, so what it had taken for a terminal agent and not seen in the
// transcript when the last daemon stopped is unknown now, as a message lost with its session is, and its sender is told.
func (p *Parts) SettlePeerSends() {
	if p.Peer == nil || p.db == nil || p.tell == nil {
		return
	}
	for _, agent := range paths.ListAgents(p.home) {
		if !p.Peer(agent) {
			continue
		}
		if err := p.tell.SessionLost(p.db, agent); err != nil && p.log != nil {
			p.log("  %s: settling the messages the last daemon left: %v", agent, err)
		}
	}
}

// peerDelivery is the Telling the peer port reports to.
type peerDelivery interface {
	SessionConfirmed(q db.Q, agent, op string) error
	SessionNotDelivered(agent, op, why string, written bool) error
	SessionLost(q db.Q, agent string) error
}

// NewPeerPort is the Agents port of the terminal agents that peer reports through, as ag delivers: the session is the live one in the
// agent's worktree under its account and in its own pane (panePid), a message is delivered once its transcript shows it, and the outcome
// goes to tell. typed is how the founder's own words are typed at the terminal.
func NewPeerPort(ag agent.Agent, home string, d *sql.DB, peer func(agent string) bool, panePid func(handle string) int, tell peerDelivery,
	typed agent.DeliverEnv, log func(string, ...any)) agent.Port {
	env := typed
	env.Peer = peer
	env.Where = func(a string) (agent.Place, error) { return agentPlace(home, d, a, panePid) }
	env.From = func(a, op string) string { return peerFrom(d)(a, op).Name }
	env.OnDelivered = func(a, op string) {
		if err := inTx(d, func(q db.Q) error { return tell.SessionConfirmed(q, a, op) }); err != nil && log != nil {
			log("  %s: telling the delivery of %s failed: %v", a, op, err)
		}
	}
	env.OnFailed = func(a, op, why string, written bool) {
		if log != nil {
			log("  %s: %s not delivered: %s", a, op, why)
		}
		if err := tell.SessionNotDelivered(a, op, why, written); err != nil && log != nil {
			log("  %s: telling that %s was not delivered failed: %v", a, op, err)
		}
	}
	return ag.Deliver(env)
}

// inTx runs fn in a transaction: the ledger move, the event and the sender's note of one outcome are one write.
func inTx(d *sql.DB, fn func(q db.Q) error) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func findSession(home string, d *sql.DB, a string, panePid func(handle string) int) (peerinbox.Session, error) {
	pl, err := agentPlace(home, d, a, panePid)
	if err != nil {
		if errors.Is(err, agent.ErrNoSession) {
			return peerinbox.Session{}, peerinbox.ErrNoSession
		}
		return peerinbox.Session{}, err
	}
	return peerinbox.ByCwd(pl.Config, pl.Cwd, pl.Alive)
}

// agentPlace is where the live session of an agent runs, found through its row's account and worktree. Agents can share a worktree and an
// account, so a session counts only when its process runs in the agent's own pane, and an agent with no terminal has none. When the
// host cannot name the pane's process, or the process table cannot be read, the worktree alone decides.
func agentPlace(home string, q *sql.DB, a string, panePid func(handle string) int) (agent.Place, error) {
	if q == nil { // the daemon's database is not open yet
		return agent.Place{}, agent.ErrNoSession
	}
	st, err := db.GetAgentState(q, a)
	if err != nil {
		return agent.Place{}, err
	}
	cwd := ""
	if st != nil {
		cwd = worktreepaths.WorktreePath(st.Worktree.String) // the column may hold `<repo>::<path>`
	}
	if cwd == "" || st.Account.String == "" || st.Terminal.String == "" {
		return agent.Place{}, agent.ErrNoSession
	}
	config, err := team.AccountConfigDir(home, st.Account.String)
	if err != nil {
		return agent.Place{}, errors.New("no config directory for the agent's account")
	}
	alive := lock.Alive
	if root := panePid(st.Terminal.String); root > 0 {
		if parent := parents(); parent != nil {
			alive = func(pid int) bool { return lock.Alive(pid) && under(parent, pid, root) }
		}
	}
	return agent.Place{Config: config, Cwd: cwd, Alive: alive}, nil
}

// mixedAgents is supervisor.Agents for peer-inbox agents and the others: each call goes to the one the agent is delivered through.
type mixedAgents struct {
	none   supervisor.Agents
	peer   supervisor.Agents
	isPeer func(agent string) bool
}

var _ supervisor.Agents = mixedAgents{}

func (m mixedAgents) of(agent string) supervisor.Agents {
	if m.isPeer(agent) {
		return m.peer
	}
	return m.none
}

func (m mixedAgents) Structured(agent string) bool { return m.of(agent).Structured(agent) }
func (m mixedAgents) Live(agent string) bool       { return m.of(agent).Live(agent) }
func (m mixedAgents) Send(agent, op, text string, d delivery.Delivery) (delivery.Receipt, error) {
	return m.of(agent).Send(agent, op, text, d)
}
func (m mixedAgents) Withdraw(agent, op string) (delivery.Receipt, bool, error) {
	return m.of(agent).Withdraw(agent, op)
}
func (m mixedAgents) Promote(agent, op string) (delivery.Receipt, error) {
	return m.of(agent).Promote(agent, op)
}

// vendorAgents is supervisor.Agents by the agent's vendor: the vendor's own port when it has one, else Claude Code's.
type vendorAgents struct {
	claude   supervisor.Agents
	others   map[string]supervisor.Agents
	vendorOf func(agent string) string
}

var _ supervisor.Agents = vendorAgents{}

func (v vendorAgents) of(agent string) supervisor.Agents {
	if port, ok := v.others[v.vendorOf(agent)]; ok {
		return port
	}
	return v.claude
}

func (v vendorAgents) Structured(agent string) bool { return v.of(agent).Structured(agent) }
func (v vendorAgents) Live(agent string) bool       { return v.of(agent).Live(agent) }
func (v vendorAgents) Send(agent, op, text string, d delivery.Delivery) (delivery.Receipt, error) {
	return v.of(agent).Send(agent, op, text, d)
}
func (v vendorAgents) Withdraw(agent, op string) (delivery.Receipt, bool, error) {
	return v.of(agent).Withdraw(agent, op)
}
func (v vendorAgents) Promote(agent, op string) (delivery.Receipt, error) {
	return v.of(agent).Promote(agent, op)
}

// noSessionPort is the Agents of an agent with no session port: nothing is delivered through it, and the supervisor's other paths reach it.
type noSessionPort struct{}

var errNoSessionPort = errors.New("no session port")

func (noSessionPort) Structured(string) bool { return false }
func (noSessionPort) Live(string) bool       { return false }
func (noSessionPort) Send(agent, _, _ string, _ delivery.Delivery) (delivery.Receipt, error) {
	return delivery.Receipt{}, fmt.Errorf("%s: %w", agent, errNoSessionPort)
}
func (noSessionPort) Withdraw(agent, _ string) (delivery.Receipt, bool, error) {
	return delivery.Receipt{}, false, fmt.Errorf("%s: %w", agent, errNoSessionPort)
}
func (noSessionPort) Promote(agent, _ string) (delivery.Receipt, error) {
	return delivery.Receipt{}, fmt.Errorf("%s: %w", agent, errNoSessionPort)
}

// peerFrom is who the message op to a session is from: whoever wrote it (the agents named in the operation, "founder" for the person), else
// the daemon. A message always says it is another's, never reads as the person's own typing.
func peerFrom(d *sql.DB) func(agent, op string) peerinbox.From {
	return func(agent, op string) peerinbox.From {
		authors, known := supervisor.AuthorsOfOp(d, op)
		if !known {
			return peerinbox.From{}
		}
		var names []string
		for _, a := range authors {
			if a == db.Human {
				a = "founder"
			}
			if a != agent && !slices.Contains(names, a) {
				names = append(names, a)
			}
		}
		return peerinbox.From{Name: strings.Join(names, ", ")}
	}
}
