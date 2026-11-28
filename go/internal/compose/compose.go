// Package compose builds the daemon's agent side from its parts, in one place the daemon main wires: the lifecycle over the terminal
// host, the supervisor's ports, the policy tick and the one set of agent locks the lifecycle and the spawns share. It holds no behaviour
// of its own beyond adapting one interface to another.
package compose

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/handline/handline/go/accounts/subscriptions"
	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	"github.com/handline/handline/go/agentctl/chatport"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/agentview"
	"github.com/handline/handline/go/internal/heal"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/prompts"
	"github.com/handline/handline/go/scheduler/keepawake"
	"github.com/handline/handline/go/scheduler/lifecycle"
	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/sysres"
	"github.com/handline/handline/go/scheduler/tick"
)

// Config is what the daemon main knows and the parts need.
type Config struct {
	DB   *sql.DB
	Home string
	Log  func(format string, args ...any)
	// Now is the clock the parts decide by; nil is the wall clock. Rows are stamped by db.Now, which a test sets to the same clock.
	Now func() time.Time
	// CLI is the claude binary the usage poll and the plan reads run.
	CLI string
	// Env are NAME=value pairs set on every claude the daemon runs.
	Env []string
	// Peer says whether a terminal agent's messages go through its session's peer inbox; nil reads the agent row's peer_inbox (PeerFromRows).
	Peer func(agent string) bool
	// Term is the terminal host agents run in; nil is the one config.yaml names (tmux by default). Tests give a fake one.
	Term *term.Term
	// Nodes is the runners linked to this master; nil runs every agent here.
	Nodes *nodes.Registry
	// Turns, when set, is asked before the screen whether a terminal agent is mid-turn (-turns-from-hooks).
	Turns *turns.Tracker
	// DiskFree is the free and total bytes of the volume holding a path, for the disk watch; nil is statfs. Tests give a low one.
	DiskFree func(path string) (free, total int64, err error)
	// Machine is the machine's latest measure for the load gate (sysres.Sampler.LatestMachine); nil checks only the agent cap.
	Machine func() *sysres.Machine
	// AgentsMemory is when the agents' memory was last read and each agent's resident bytes (sysres.Sampler.AgentsMemory), for the
	// founder's cap on it (agents_memory_max_gb); nil leaves the cap unchecked.
	AgentsMemory func() (at int64, rss map[string]int64)
	// Servers is the TCP servers an agent's processes listen on (procload.Tracker.ServersOf); nil is none.
	Servers func(agent string) []procload.Server
	// Screens is an agent's open screen sources (screenpipe.Hub.Sources); nil is none.
	Screens func(agent string) []any
	// NotifiedNeedsYou has an unanswered prompt that Claude Code's Notification hook reports raise the needs-you card.
	NotifiedNeedsYou bool
	// FounderTyped has the founder's own messages typed at a terminal agent as its person's prompt (supervisor.Env.FounderTyped); needs
	// Turns.
	FounderTyped bool
}

// Parts is what New builds. Locks goes to lifecycle.Manager.Locks and Spawns.Locks alike.
type Parts struct {
	chatOnce  sync.Once
	chat      *chatport.Port
	chatBuilt atomic.Pointer[chatport.Port] // chat, for the readers that may come before it is built
	prompts   atomic.Pointer[prompts.Env]   // the daemon's prompt code, which a Chat agent's asks are raised through

	Locks *lifecycle.AgentLocks
	Peer  func(agent string) bool
	// LC is the lifecycle, once Lifecycle has built it.
	LC *Lifecycle

	db       *sql.DB
	home     string
	notified bool // Config.NotifiedNeedsYou
	turns    *turns.Tracker
	tell     Telling
	log      func(format string, args ...any)
	healer   *heal.Healer                  // the tick's, once Tick has built it
	view     atomic.Pointer[agentview.Env] // the tick's too; a move reached over RPC may ask for it first
	awake    *keepawake.Holder
	plans    []*subscriptions.Env // each tick's subscriptions pass, whose plan question runs off it
	acts     *tick.Acts           // agents the tick loop supervises at once between ticks (ActNow)
}

// Wait returns once the heal steps still running off the tick have ended: the daemon closes the database only after.
func (p *Parts) Wait() {
	if p.healer != nil {
		p.healer.Wait()
	}
}

// WaitForPlanAsks returns once the plan questions the ticks started have ended (tests: one may run for as long as the vendor's CLI).
func (p *Parts) WaitForPlanAsks() {
	for _, s := range p.plans {
		s.WaitForPlanAsk()
	}
}

// Telling is what the supervisor's Env does with a session's end of a send: the operation is delivered once the session showed it, and
// what the last daemon wrote and did not see shown goes back to a watch, or is unknown (supervisor.Env).
type Telling interface {
	SessionWritten(q db.Q, agent, op string) error
	SessionConfirmed(q db.Q, agent, op string) error
	ResumeSends(q db.Q, agent string, rewatch func(op string, written time.Time) bool) error
}

// New builds the parts. The daemon fills in what only it knows when it starts (the database and the telling).
func New(cfg Config) *Parts {
	peer := cfg.Peer
	if peer == nil {
		peer = PeerFromRows(cfg.Home)
	}
	p := &Parts{Locks: &lifecycle.AgentLocks{}, Peer: peer, log: cfg.Log, home: cfg.Home, notified: cfg.NotifiedNeedsYou, turns: cfg.Turns,
		awake: &keepawake.Holder{Log: func(s string) { cfg.Log("%s", s) }}, acts: tick.NewActs()}
	if cfg.DB != nil {
		p.Bind(cfg.DB, nil)
	}
	return p
}

// ActNow is supervisor.Env.ActNow: each agent's view is stored now and the tick loop supervises it between ticks.
func (p *Parts) ActNow(agents ...string) {
	for _, a := range agents {
		p.RefreshView(a)
	}
	p.acts.Add(agents...)
}

// RefreshView stores agent's view now, not at the next tick: its prompt opened or went. Nothing before the tick is built.
func (p *Parts) RefreshView(agent string) {
	if v := p.view.Load(); v != nil {
		v.RefreshOne(agent)
	}
}

// KeepAwake is whether the tick holds the sleep lock now, and since when (daemon.Config.KeepAwake).
func (p *Parts) KeepAwake() (bool, time.Time) { return p.awake.State() }

// Bind gives the parts the database when it is open and the telling that follows a session's deliveries and its end; a nil tell tells
// nothing.
func (p *Parts) Bind(db *sql.DB, tell Telling) {
	p.db, p.tell = db, tell
}
