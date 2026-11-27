// Package daemon runs the listeners: the local socket, and the optional network listener, one contract on both.
package daemon

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/accounts/weblogin"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	_ "github.com/handline/handline/go/agentctl/agent/vendors"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/agenthost"
	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/adminwrites"
	"github.com/handline/handline/go/internal/boardcopy"
	"github.com/handline/handline/go/internal/chatwrites"
	"github.com/handline/handline/go/internal/deskrpc"
	"github.com/handline/handline/go/internal/filerw"
	"github.com/handline/handline/go/internal/hooks"
	"github.com/handline/handline/go/internal/inputhold"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/pausewrites"
	"github.com/handline/handline/go/internal/placement"
	"github.com/handline/handline/go/internal/prefs"
	"github.com/handline/handline/go/internal/projects"
	"github.com/handline/handline/go/internal/prompts"
	"github.com/handline/handline/go/internal/proposalwrites"
	"github.com/handline/handline/go/internal/push"
	"github.com/handline/handline/go/internal/reads"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/internal/screen"
	"github.com/handline/handline/go/internal/sessionrpc"
	"github.com/handline/handline/go/internal/sim"
	"github.com/handline/handline/go/internal/termchan"
	"github.com/handline/handline/go/internal/transcript"
	"github.com/handline/handline/go/internal/watcher"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/lock"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/peer"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/version"
	"github.com/handline/handline/go/protocol/authority"
	"github.com/handline/handline/go/protocol/devices"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/access"
	"github.com/handline/handline/go/scheduler/agentload"
	"github.com/handline/handline/go/scheduler/leases"
	"github.com/handline/handline/go/scheduler/leasestore"
	"github.com/handline/handline/go/scheduler/loadgate"
	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/runs"
	"github.com/handline/handline/go/scheduler/schedules"
	"github.com/handline/handline/go/scheduler/session"
	"github.com/handline/handline/go/scheduler/settings"
	"github.com/handline/handline/go/scheduler/sysres"
	"github.com/handline/handline/go/scheduler/worktreepool"
	"github.com/handline/handline/go/taskboard/attachments"
	"github.com/handline/handline/go/taskboard/handoff"
	"github.com/handline/handline/go/taskboard/supervisor"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// daemonLock is the name of the singleton the Python daemon also holds (handline/infra/daemonlock.py DAEMON_LOCK).
const daemonLock = "daemon"

type Config struct {
	Home       string
	DBFile     string
	SocketFile string
	// Claude is the claude binary agents are run with, for daemon.status missing_tools; empty: the daemon runs no agent side.
	Claude string
	// Listen is the address the network listener binds; empty leaves it off. With CertFile and KeyFile it is a TLS
	// WebSocket. With neither it is plain: Listen is ":PORT" (the VPN interface address is found) or a VPN
	// interface or loopback address, and it serves plain HTTP/ws.
	Listen            string
	CertFile, KeyFile string
	Log               func(format string, args ...any)
	Auth              authority.Authority // nil: the protected store of Home

	WebDir    string // the built web UI the listener serves on its own origin; empty serves none
	Advertise string // the host:port a pairing code names, when it differs from where the listener binds
	// AddressesHome is the home whose network_addresses the doors serve, "" for Home: a cluster engine serves those of its Mac's own
	// daemon, a fact about the machine, read live so a change in that home's Settings reaches the engine at its next relisten.
	AddressesHome string

	Terminal term.Backend // the host the daemon reads panes from; nil is the configured one (tests give a fake)
	// Turns, when set, says whether a terminal agent is mid-turn from its hooks and transcript; nil reads its screen (-turns-from-hooks).
	Turns *turns.Tracker
	// BoardCopy keeps a copy of the board for the owner's runners (boardcopy): a master's daemon runs with it.
	BoardCopy bool

	// EnsureFounderKey makes the founder key when there is none and reports whether the store could be written (the daemon main sets it
	// to the home's tokens: the store is outside the home, so a test leaves it nil).
	EnsureFounderKey func() bool

	// Teller follows the task writes with what Python did after their commit (the typed says, the deliveries): the supervisor's telling
	// layer. Nil is the Env's own telling once its ports are all wired (OnOpen sets them), and tells nothing before that.
	Teller taskwrites.Teller

	// Env is the telling layer the chat, mailbox and say writes run on (supervisor.Env). Nil is an Env of this database and home alone: its
	// writes are made, and what needs a port that has not landed is left to the tick.
	Env *supervisor.Env

	// Transcripts has hello say transcript, which clients read as chat.send taking parent_item (a reply to a chat.transcript item).
	Transcripts bool
	// Watch runs the detectors that turn the board's changes into rows of the stream.
	Watch bool
	// HealthEvery is how often DaemonStatus.health is looked at for a change to tell; 0 is healthEvery.
	HealthEvery time.Duration
	// Resources, when set, is the sampler shared with the load gate, run whether or not Watch is; nil makes one for Watch.
	Resources *sysres.Sampler
	// Load, when set, is the process tracker shared with the agent views (its servers); the daemon gives it the database and the panes. Nil makes one for Watch.
	Load *procload.Tracker
	// Context is the model id and context figures of an agent on an account (agent.strip). Nil leaves them null.
	Context func(agent, account string) (string, *int, *int64, int)
	// Delivery is what the supervisor knows of why a queued line waits (a chat line's sending_reason). Nil leaves out the reasons that need it.
	Delivery reads.Delivery
	// KeepAwake is whether the sleep lock is held now, and since when (DaemonStatus.keep_awake.held). Nil reads as not held.
	KeepAwake func() (bool, time.Time)
	// Session makes the env of the session methods (session.start, wake, stop, restart, park, interrupt) once the lifecycle is built; nil
	// serves none of them.
	Session func() *sessionrpc.Env
	// PromptChanged is told an agent whose prompt row opened or went, so its stored view follows the card at once. Nil leaves it to the tick.
	PromptChanged func(agent string)
	// OnOpen is given the database once it is open, for the parts built before it was (the supervisor's ports, the tick), and the
	// runners linked to this master.
	OnOpen func(db *sql.DB, env *supervisor.Env, runners *nodes.Registry)
	// BeforeClose is called by Close once the connections have ended, before the database closes, to wait for work the parts run off
	// the caller (the heal steps).
	BeforeClose func()
	// StopWork, when set, stops the tick and returns once it has: the master hands over, or a newer one outranked it.
	StopWork func()
}

// Daemon is a running daemon. Close stops the listeners and waits for the connections to end.
type Daemon struct {
	cfg    Config
	db     *sql.DB
	lent   func() // takes db back from the shared handles (store.Lend)
	copier *boardcopy.Copier
	hooks  *hooks.Recorder
	files  *filerw.Env        // file.read, file.write and the shared docs
	pause  pauseClock         // the founder's pause of new work: its until tells the status topic
	health healthWatch        // the health the status topic was last told
	hclock *reads.HealthClock // since when health has had its level, for every server
	logins *weblogin.Manager  // the web logins

	acctMu    sync.Mutex        // one account.changed is made and published at a time, so the topic gets a login's moves in order
	acctSeq   map[string]uint64 // the number of the last move published for each account
	prompts   *prompts.Env
	usageSeen map[string]string // the usage loop's own: each account's row as last told the accounts topic (usageKey); nil before the first look
	tchan     *termchan.Env     // the terminal channel and the input hold
	res       *sysres.Sampler   // what the machine and each agent use; nil when the daemon does not watch
	placer    *placement.Placer // the machine each session opens on
	load      *procload.Tracker // every process and whose it is; nil when the daemon does not watch
	leases    *leases.Manager   // the port leases and the shared runs' turns
	runs      *runs.Book        // the shared runs, as the leases and the hook read them
	holds     *inputhold.Store  // who holds each terminal's input
	single    *lock.Hold        // the daemon singleton, held while this daemon runs
	stop      chan struct{}     // closed by Close: the spool loop ends
	started   time.Time
	ln        net.Listener
	local     *rpc.Server
	net       *netDoors
	dev       *devices.Manager // nil unless the network listener is on
	nodes     *nodes.Registry  // the runners linked to this master
	master    *mastership      // whether this master holds its writes, and must start again
	projects  *projects.Syncer // the workspaces' repos on those runners
	push      *push.Sender     // nil when push is off: no credentials in config.yaml
	hub       *stream.Hub
	wg        sync.WaitGroup
	envOnce   sync.Once
	envMade   *supervisor.Env
	mu        sync.Mutex
	conns     map[net.Conn]struct{}
	closing   bool
	closeOnce sync.Once
}

func Start(cfg Config) (*Daemon, error) {
	if cfg.Log == nil {
		cfg.Log = func(string, ...any) {}
	}
	if cfg.Auth == nil {
		cfg.Auth = authority.New(cfg.Home)
	}
	if cfg.plain() && cfg.Listen != "" {
		addr, err := plainAddr(cfg.Listen)
		if err != nil {
			return nil, err
		}
		cfg.Listen = addr
	}
	if _, _, err := net.SplitHostPort(cfg.Advertise); cfg.Advertise != "" && err != nil {
		return nil, fmt.Errorf("-advertise %q is host:port: %w", cfg.Advertise, err)
	}
	if (cfg.CertFile == "") != (cfg.KeyFile == "") {
		return nil, errors.New("the network listener takes -cert and -key together, or neither (plain HTTP, no certificate)")
	}
	single := lock.TakeSingleton(cfg.Home, daemonLock)
	if single == nil {
		return nil, errors.New("another daemon holds this home: not starting a second")
	}
	held := false
	defer func() {
		if !held {
			single.Release()
		}
	}()
	if err := SetBoardAside(cfg.Home, cfg.DBFile, time.Now()); err != nil {
		return nil, fmt.Errorf("set the board aside for the master: %w", err)
	}
	db, err := store.Open(cfg.DBFile)
	if err != nil {
		return nil, err
	}
	if err := markJoined(cfg.Home, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureOwner(db, ownerName); err != nil {
		db.Close()
		return nil, err
	}
	if err := keepOwnNode(db, cfg.Home); err != nil {
		db.Close()
		return nil, err
	}
	if err := applyPromotion(db, cfg.Home, joined.NodeIDOf(cfg.Home), time.Now()); err != nil {
		db.Close()
		return nil, fmt.Errorf("take over as master: %w", err)
	}
	d := &Daemon{cfg: cfg, db: db, started: time.Now(), conns: map[net.Conn]struct{}{}, stop: make(chan struct{}), master: newMastership()}
	if cfg.Turns != nil {
		cfg.Turns.Agent = func(handle string) string { a, _ := dbpkg.AgentOfTerminal(db, handle); return a }
		cfg.Turns.Lines = func(agent, session string) func([]byte) {
			if !transcriptRows(cfg.Home) {
				return nil
			}
			return func(line []byte) {
				if err := transcript.Record(db, agent, session, line); err != nil {
					cfg.Log("  turns: %s: transcript row not stored: %v", agent, err)
				}
			}
		}
		cfg.Turns.Interrupted = func(agent string) {
			if err := hooks.MarkInterrupted(db, cfg.Home, agent); err != nil {
				cfg.Log("  turns: %s: %v", agent, err)
			}
		}
	}
	d.nodes = nodes.New(db, cfg.Home)
	d.nodes.Log = cfg.Log
	d.nodes.Self, d.nodes.Version = joined.NodeIDOf(cfg.Home), version.Sha
	d.nodes.Hold = d.master.refusal
	if err := d.readNumber(); err != nil {
		db.Close()
		return nil, err
	}
	d.nodes.BatterySwitch = func() bool { return (&settings.Settings{Home: cfg.Home}).Flag(nodes.BatteryKey, false) }
	d.nodes.BuildAuto = func() bool { return (&settings.Settings{Home: cfg.Home}).Flag(nodes.BuildAutoKey, true) }
	if exe, err := os.Executable(); err == nil { // a runner of this platform on another version is handed this build
		if d.nodes.Build, err = nodes.OpenBuild(paths.Resolved(exe)); err != nil {
			cfg.Log("the build this master runs is not handed to its runners: %v", err)
		}
	}
	d.nodes.AccountFor = func(email, name string, add bool) (string, error) {
		a := team.Accounts{Home: cfg.Home}
		if !add { // another person's runner: their emails make no account of the owner's
			return a.OfEmail(email), nil
		}
		return a.ByEmail(email, name)
	}
	d.nodes.Reading = func(acct string, b runner.BeatReading) {
		if (&usage.Env{Home: cfg.Home}).Read(acct).Unmetered {
			return
		}
		r := usage.Reading{Account: acct, At: time.Now().Unix() - max(0, b.AgeS)} // by this clock: the runner's may differ
		for _, w := range b.Windows {
			r.Windows = append(r.Windows, usage.ReadingWindow{Minutes: w.Minutes, Pct: w.Pct, Reset: w.Reset})
		}
		usage.RecordNewer(cfg.Home, r)
	}
	d.projects = &projects.Syncer{DB: db, Reg: d.nodes, Log: cfg.Log, Stop: d.stop}
	d.nodes.OnLink = d.projects.Node
	placer := &placement.Placer{DB: db, Home: cfg.Home, Reg: d.nodes, Holds: d.projects.Holds, Log: cfg.Log,
		Here: d.room, Limits: func() placement.Limits {
			c := usage.LoadConfig(cfg.Home)
			return placement.Limits{Sessions: loadgate.LimitsOf(c).Agents, AgentsRSS: worktreepool.GB(c, worktreepool.AgentsMemoryMaxKey)}
		}, Manual: func() bool { return usage.LoadConfig(cfg.Home)[placement.Key] == placement.Manual }}
	d.nodes.Placer, d.nodes.Check = placer, placer.Check
	d.placer = placer
	d.nodes.Turns = cfg.Turns
	d.nodes.Tokens = func(account string, raw json.RawMessage) error {
		var calls []usage.Call
		if err := json.Unmarshal(raw, &calls); err != nil {
			return rpc.BadRequestError("calls are the calls read: "+err.Error(), "calls", rpc.InvalidParams)
		}
		return (&usage.Env{Home: cfg.Home}).SaveCalls(db, account, calls)
	}
	d.nodes.Transcript = func(agent, session string, at int64, rows []transcript.Row) {
		if !transcriptRows(cfg.Home) {
			return
		}
		if err := transcript.Write(db, agent, session, at, rows); err != nil {
			cfg.Log("  nodes: %s: transcript rows not stored: %v", agent, err)
		}
	}
	if cfg.BoardCopy {
		id, err := boardID(db)
		var c *boardcopy.Copier
		if err == nil {
			c, err = boardcopy.Start(cfg.DBFile, boardcopy.Dir(cfg.Home), id)
		}
		if err != nil {
			cfg.Log("handlined: the board's copy did not start, so no runner keeps one and the WAL is checkpointed only past %d pages: %v",
				store.AutoCheckpointPages, err)
		} else {
			d.copier = c
			d.nodes.Copy = &nodes.Copy{Dir: boardcopy.Dir(cfg.Home)}
		}
	}
	if d.moves() {
		if err := d.ensureOwn(); err != nil {
			cfg.Log("master: no device of its own, so a newer master cannot take this one back as a runner: %v", err)
		}
	}
	if err := adminwrites.EnsureClusterName(db); err != nil {
		cfg.Log("cluster: no name given: %v", err)
	}
	if cfg.OnOpen != nil {
		cfg.OnOpen(db, d.env(), d.nodes)
	}
	global, _ := usage.LoadConfig(cfg.Home)[handoff.PromptKey].(string)
	handoff.LogCustomPrompts(cfg.Home, global, handoff.OwnPrompts(cfg.Home), cfg.Log)
	team.RemoveEmptyJournals(cfg.Home, cfg.Log)
	d.hooks = &hooks.Recorder{DB: db, Home: cfg.Home, Log: cfg.Log, Turns: cfg.Turns, Elsewhere: d.nodes.Elsewhere}
	hookEnv := &session.Env{DB: db, Home: cfg.Home}
	d.hooks.Nudge = hookEnv.FounderNudge
	d.files = &filerw.Env{DB: db, Log: cfg.Log}
	d.hooks.Background = func(agent string, raw map[string]any) {
		_ = hookEnv.NoteBackground(agent, raw)
		d.files.Touched(agent, raw) // names the agent on a shared doc its edit changed
	}
	d.hooks.Subagent = func(agent, session string, c hooks.SubagentCall) (string, error) {
		m := dbpkg.SubagentMeta{Description: c.Description, Type: c.Type, Background: c.Background}
		return (&usage.Env{Home: cfg.Home}).StartSubagent(db, agent, session, time.Now().Unix(), m)
	}
	ls := leasestore.Store{DB: db}
	d.leases = &leases.Manager{Log: cfg.Log, Store: ls, WorkspaceOf: ls.WorkspaceOf}
	d.runs = &runs.Book{DB: db, Log: cfg.Log}
	d.leases.Shared, d.leases.Named = d.runs.Shared, d.runs.Named
	d.leases.Tasks = leaseTasks{d}
	d.leases.Ports = func() (int, int, bool) { return leases.PortRange(usage.LoadConfig(cfg.Home)) }
	d.leases.MaxHold = func(kind string) int64 { return leases.MaxHold(usage.LoadConfig(cfg.Home), kind) }
	d.leases.Tell = d.tellLease
	d.hooks.TurnEnded = func(agent, kind string) {
		// A hold lasts the turn; a session's end also ends the commands' leases its hl use processes did not give back.
		if kind == "SessionEnd" {
			d.leases.ReleaseAgent(agent, "", false, "the session ended")
			go (&sim.Env{Home: cfg.Home}).Reap(agent) // the simulators and emulators it booted
		} else {
			d.leases.ReleaseAgent(agent, "", true, "the turn ended")
		}
		if kind == "Stop" {
			if err := agentload.KeepBackground(db, agent, time.Now().Unix()); err != nil {
				cfg.Log("load: %s's background shells: %v", agent, err)
			}
		}
	}
	d.logins = newLogins(cfg)
	d.prompts = prompts.New(db, cfg.Home, &reads.Env{DB: db, Home: cfg.Home, Started: d.started, Sha: version.Sha, Schema: store.LatestVersion()})
	d.prompts.Changed = cfg.PromptChanged
	d.prompts.Emit = func(topic, typ string, data map[string]any) error {
		_, err := d.hub.Append(stream.Entry{Topic: topic, Type: typ, Data: data})
		return err
	}
	runners := d.nodes.Terminals()
	d.tchan = termchan.New(db, cfg.Home, func() term.Backend {
		local := cfg.Terminal
		if local == nil {
			local = agenthost.Current(cfg.Home)
		}
		return &term.Machines{Local: local, Remote: runners} // a runner's terminal is read and typed into over its link
	})
	d.tchan.Relay = d.nodes
	d.tchan.Missing = func() map[string]string { e := d.readsEnv(); return e.Installable() }
	d.holds = &inputhold.Store{DB: db, Changed: d.tchan.HoldersChanged, IdleS: func() int64 {
		return (&settings.Settings{Home: cfg.Home}).ConfigSeconds("input_hold_idle_s", inputhold.DefaultIdleS)
	}}
	d.tchan.Holds = d.holds
	d.single, held = single, true
	// The stored leases come back before the spool, so a turn that ended while no daemon ran frees its holds.
	if err := d.leases.Adopt(); err != nil {
		cfg.Log("  leases: adopting: %v", err)
	}
	d.ingestSpool()                                                         // what hooks left while no daemon ran, before the first call is served
	if home := paths.DBFile(cfg.Home); filepath.Clean(cfg.DBFile) == home { // a -db elsewhere is not the file the home's readers open
		d.lent = store.Lend(home, db)
	}
	// Before the socket takes a call: without the founder key it would serve no founder call.
	if cfg.EnsureFounderKey != nil && !cfg.EnsureFounderKey() {
		cfg.Log("founder key: could not make one - the socket will serve no founder call")
	}
	sender, err := push.FromConfig(usage.LoadConfig(cfg.Home), devices.SQLStore{DB: db})
	if err != nil {
		cfg.Log("push: %v - push is off", err)
	}
	if sender != nil {
		sender.Log = cfg.Log
		d.push = sender
	}
	if cfg.Listen != "" {
		d.dev = devices.New(devices.SQLStore{DB: db})
		d.dev.Log, d.dev.Self = cfg.Log, d.nodes.Self
		d.dev.BeforeRevoke = func(device string) {
			var node string
			if db.QueryRow("SELECT id FROM nodes WHERE device = ?", device).Scan(&node) == nil {
				if _, err := d.nodes.Reclaim(node, "its device was revoked"); err != nil {
					cfg.Log("nodes: %s: its sessions were not taken back: %v", node, err)
				}
			}
		}
		d.dev.Update = func(device string) error {
			var node string
			if err := db.QueryRow("SELECT id FROM nodes WHERE device = ?", device).Scan(&node); errors.Is(err, sql.ErrNoRows) {
				return rpc.Refused("only a Mac or a server that joined takes the master's build", "kind", "")
			} else if err != nil {
				return err
			}
			return d.nodes.AskUpdate(node)
		}
		d.dev.MakeMaster = func(device string) error {
			var node string
			if err := db.QueryRow("SELECT id FROM nodes WHERE device = ?", device).Scan(&node); errors.Is(err, sql.ErrNoRows) {
				return rpc.Refused("only a Mac or a server that joined can be made the master", "kind", "")
			} else if err != nil {
				return fmt.Errorf("the node of device %s: %w", device, err)
			}
			if _, err := d.nodes.Runner(node); err != nil {
				return rpc.Refused("it is not linked to this master now", "node", "")
			}
			if _, _, _, err := d.canHandOver(node); err != nil { // the runner would only find it out after answering
				return err
			}
			_, err := d.nodes.Call(node, "runner.promote", nil)
			return err
		}
		d.dev.PushPlatforms = d.push.Platforms()
	}
	d.hub = stream.NewHub()
	d.hub.Log = &stream.Log{DB: db}
	d.hub.Render = (&reads.Env{DB: db, Home: cfg.Home}).TaskEventFor
	if d.logins != nil { // a login's state moving is an account.changed on the accounts topic
		d.logins.Changed = d.accountChanged
	}
	if cfg.Watch || cfg.Resources != nil { // made before the servers that read them; started below
		d.res = cfg.Resources
		if d.res == nil {
			d.res = &sysres.Sampler{Home: cfg.Home, Log: func(s string) { cfg.Log("%s", s) }}
		}
		d.res.Panes = d.agentPanes
		d.load = cfg.Load
		if d.load == nil {
			d.load = &procload.Tracker{Home: cfg.Home, Log: func(s string) { cfg.Log("%s", s) }}
		}
		d.load.DB, d.load.Panes = db, d.agentPanes
		if d.load.SimulatorOf == nil {
			d.load.SimulatorOf = func(start int64) string {
				a, err := agentload.SimulatorOf(db, start)
				if err != nil {
					cfg.Log("load: whose the simulator is: %v", err)
				}
				return a
			}
		}
	}
	d.hclock = &reads.HealthClock{}
	d.local = d.newServer("local")
	d.seedHealth()
	d.armPause() // a pause kept across a restart still ends at its until
	// The watcher's first look is a silent baseline, so it is taken before anything serves: else a client's write before it is in the
	// baseline and never sent.
	var w *watcher.Watcher
	var notes chan stream.Entry
	if cfg.Watch {
		proposalSignature := watcher.ProposalSignature(cfg.Home)
		emit := d.hub.Append
		if d.push != nil { // each for_you event is also a note
			notes = make(chan stream.Entry, 256)
			emit = d.push.Tee(emit, notes)
		}
		w = &watcher.Watcher{DB: db, Emit: emit, Log: cfg.Log, Signature: func() string { return proposalSignature() + "/" + watcher.ForYouSignature(db) },
			Detectors: []watcher.Detector{&watcher.Tasks{Reads: &reads.Env{DB: db, Home: cfg.Home}}, &watcher.Agents{Reads: &reads.Env{DB: db, Home: cfg.Home, Delivery: cfg.Delivery}}, watcher.People(), watcher.Workspaces(), watcher.Nodes(), &watcher.Notifications{Reads: &reads.Env{DB: db, Home: cfg.Home}}, &watcher.Inbox{Reads: &reads.Env{DB: db, Home: cfg.Home}},
				&watcher.Proposals{Reads: &reads.Env{DB: db, Home: cfg.Home}},
				&watcher.ForYou{Reads: &reads.Env{DB: db, Home: cfg.Home}}, &watcher.Prefs{}, &watcher.Requests{Reads: &reads.Env{DB: db, Home: cfg.Home}},
				&watcher.Chat{Reads: &reads.Env{DB: db, Home: cfg.Home, Delivery: cfg.Delivery}, Panes: d.paneRuns, Tail: d.paneTail, Log: cfg.Log}}}
		w.Scan()
	}
	if cfg.Listen != "" { // before the local socket serves, so no call reads d.net while it is set
		if _, err := listenNetwork(cfg, d); err != nil {
			d.Close()
			return nil, err
		}
		if d.moves() {
			if err := d.keepOwnDoor(); err != nil {
				cfg.Log("master: its door was not kept: %v", err)
			}
		}
	}
	if cfg.SocketFile != "" {
		ln, err := listenUnix(cfg.SocketFile)
		if err != nil {
			if d.lent != nil {
				d.lent()
			}
			db.Close()
			d.stopCopy()
			if d.single != nil {
				d.single.Release()
			}
			return nil, err
		}
		d.ln = ln
		d.wg.Add(1)
		go d.acceptLocal(d.local)
	}
	if d.hooks != nil {
		d.wg.Add(1)
		go d.spoolLoop()
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.hub.Run(d.stop, cfg.Log)
	}()
	d.wg.Add(1)
	go d.healthLoop()
	d.wg.Add(1)
	go func() { // the runners' copy of the shared skills, commands and subagents
		defer d.wg.Done()
		d.nodes.WatchShared(d.stop)
	}()
	d.wg.Add(1)
	go d.usageLoop()
	if w != nil {
		if notes != nil {
			d.wg.Add(1)
			go func() {
				defer d.wg.Done()
				d.push.Watch(d.stop, notes)
			}()
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			w.Run(d.stop)
		}()
	}
	// The sampler only reads: it runs for the load gate (Resources) even where the watcher does not.
	if cfg.Watch || cfg.Resources != nil {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.res.Run(d.stop)
		}()
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.load.Run(d.stop)
		}()
	}
	d.wg.Add(1)
	go func() { // a command's lease whose processes are gone is freed even where the processes are not sampled
		defer d.wg.Done()
		sysres.Loop(d.stop, procload.Every, func(int64) {
			var procs []procload.Proc
			if d.load != nil {
				if v, ok := d.load.View(); ok {
					procs = v.Processes
				}
			}
			d.leases.Check(procs)
		})
	}()
	if d.moves() {
		d.wg.Add(2)
		go func() {
			defer d.wg.Done()
			d.startCheck()
		}()
		go d.tellOldMaster()
	} else {
		close(d.master.checked)
	}
	cfg.Log("handlined %s serving %q", version.Sha, cfg.SocketFile)
	return d, nil
}

// agentPanes is each agent's terminal pane process, from one listing of the host's panes; none when the host cannot list them.
func (d *Daemon) agentPanes() map[string]int {
	host := d.cfg.Terminal
	if host == nil {
		host = agenthost.Current(d.cfg.Home)
	}
	lister, ok := host.(term.PaneLister)
	if !ok {
		return nil
	}
	panes, ok := lister.ListPanes()
	if !ok {
		return nil
	}
	pid := map[string]int{}
	for _, p := range panes {
		pid[p.Handle] = p.PID
	}
	rows, err := d.db.Query("SELECT agent, terminal FROM agent_state WHERE COALESCE(terminal, '') != ''")
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var agent, handle string
		if rows.Scan(&agent, &handle) == nil && pid[handle] != 0 {
			out[agent] = pid[handle]
		}
	}
	return out
}

// env is the telling layer the writes run on.
func (d *Daemon) env() *supervisor.Env {
	d.envOnce.Do(func() {
		d.envMade = d.cfg.Env
		if d.envMade == nil {
			d.envMade = &supervisor.Env{DB: d.db, Home: d.cfg.Home, Log: d.cfg.Log}
		}
	})
	return d.envMade
}

// teller is what follows the task and chat writes: the configured one, else the supervisor Env once every port has its owner, since a
// missing port is a nil call. Nil tells nothing; the tick does it later.
func (d *Daemon) teller() taskwrites.Teller {
	if d.cfg.Teller != nil {
		return d.cfg.Teller
	}
	if e := d.env(); e.Ports.Wired() {
		return e
	}
	return nil
}

// proposalEnv is what proposal.decide runs on: the telling layer for the proposer, and the process table when the Env has an observer.
func (d *Daemon) proposalEnv() *proposalwrites.Env {
	env := d.env()
	h := &proposalwrites.Env{DB: d.db, Home: d.cfg.Home, Say: env, Log: d.cfg.Log, Ask: d.askFounder, ProposalView: (&reads.Env{DB: d.db, Home: d.cfg.Home}).ProposalView}
	if d.tchan != nil {
		h.Runs = d.tchan
	}
	if env.Observer != nil {
		h.Live = env.Observer.LiveSessions
	}
	return h
}

// askFounder opens a task for the founder, as proposal.decide's retire needs to put them a question; it answers the task's number.
func (d *Daemon) askFounder(title, body string, tags ...string) (int64, error) {
	return taskwrites.AskFounder(d.db, d.teller(), title, body, tags...)
}

// accountChanged says an account changed (its web login moved) on the accounts topic, as accounts.list shows the account with the login in
// the state the move made, not the one it has by now. Announcements arrive in any order; one numbered before the last published for the
// account is dropped, so the topic never says an earlier state after a later one.
func (d *Daemon) accountChanged(name string, login weblogin.Login) {
	d.acctMu.Lock()
	defer d.acctMu.Unlock()
	if login.Seq <= d.acctSeq[name] {
		return
	}
	acct, ok := (&reads.Env{DB: d.db, Home: d.cfg.Home, Login: func(string) *weblogin.Login { return &login }}).Account(name)
	if !ok {
		return
	}
	if err := d.hub.AccountChanged(acct); err != nil {
		d.cfg.Log("  stream: account.changed %s: %v", name, err)
		return
	}
	if d.acctSeq == nil {
		d.acctSeq = map[string]uint64{}
	}
	d.acctSeq[name] = login.Seq
}

// newLogins is the daemon's web logins, logging each end to the daemon's log.
func newLogins(cfg Config) *weblogin.Manager { return &weblogin.Manager{Home: cfg.Home, Log: cfg.Log} }

// accountAdded says an account was added on the accounts topic, as accounts.list shows it: registered, not logged in.
func (d *Daemon) accountAdded(name string) {
	acct, ok := (&reads.Env{DB: d.db, Home: d.cfg.Home, Login: func(string) *weblogin.Login { return nil }}).Account(name)
	if !ok {
		return
	}
	if err := d.hub.AccountChanged(acct); err != nil {
		d.cfg.Log("  stream: account.changed %s: %v", name, err)
	}
}

// nodesChanged sends nodes.changed: node.status reads anew.
func (d *Daemon) nodesChanged() {
	if _, err := d.hub.Append(stream.Entry{Topic: "nodes", Type: "nodes.changed", Data: map[string]any{}}); err != nil {
		d.cfg.Log("  stream: nodes.changed: %v", err)
	}
}

// writeRegistrars are the packages that register methods once the hooks are wired, one per line in alphabetical order of the package: a port
// adds its line at its own place, so two ports do not edit the same spot and git merges them.
func (d *Daemon) writeRegistrars() []func(*rpc.Server) {
	return []func(*rpc.Server){
		func(s *rpc.Server) {
			access.Register(s, d.cfg.Home, func(topic, typ string, data map[string]any) error {
				_, err := d.hub.Append(stream.Entry{Topic: topic, Type: typ, Data: data})
				return err
			}, d.typing().AddDir)
		},
		func(s *rpc.Server) {
			adminwrites.Register(s, &adminwrites.Env{DB: d.db, Home: d.cfg.Home, Logins: d.logins, Changed: d.accountAdded,
				NodesChanged: d.nodesChanged, LabelKey: (&reads.Env{DB: d.db, Home: d.cfg.Home}).LabelKey, Network: d.networkSetting()})
		},
		func(s *rpc.Server) {
			attachments.Register(s, attachments.New(d.cfg.Home), func(topic, typ string, data map[string]any) error {
				_, err := d.hub.Append(stream.Entry{Topic: topic, Type: typ, Data: data})
				return err
			})
		},
		func(s *rpc.Server) { chatwrites.Register(s, d.env(), d.teller()) },
		func(s *rpc.Server) { filerw.Register(s, d.files) },
		func(s *rpc.Server) { d.hooks.Register(s) },
		func(s *rpc.Server) { pausewrites.Register(s, nil, d.pauseChanged) },
		func(s *rpc.Server) { (&screen.Env{DB: d.db, Home: d.cfg.Home}).Register(s) },
		func(s *rpc.Server) { proposalwrites.Register(s, d.proposalEnv()) },
		func(s *rpc.Server) { prompts.Register(s, d.prompts) },
		func(s *rpc.Server) {
			prefs.Register(s, d.cfg.Home)
			prefs.RegisterDismiss(s, (&reads.Env{DB: d.db, Home: d.cfg.Home}).WarningSinces)
		},
		func(s *rpc.Server) {
			if d.cfg.Session != nil {
				env := d.cfg.Session()
				env.Emit = d.operationEvent
				env.WorkspaceAdded = d.projects.Workspace
				env.InterruptTerminal = d.typing().PressEsc
				sessionrpc.Register(s, env)
			}
		},
		func(s *rpc.Server) {
			if d.holds != nil {
				sessionrpc.RegisterTyping(s, d.typing())
			}
		},
		func(s *rpc.Server) {
			taskwrites.Register(s, d.cfg.Home, d.teller(), chatwrites.DecideCard(d.env(), d.teller()))
			taskwrites.RegisterLessons(s)
			taskwrites.RegisterReviewLevels(s, d.cfg.Home)
			taskwrites.RegisterAgentContext(s, d.cfg.Home)
			deskrpc.Register(s, d.cfg.Home)
		},
		func(s *rpc.Server) { termchan.Register(s, d.tchan) },
	}
}

// typing is the env of input.take and release and the session typing methods: who holds each terminal, and the host to type at.
func (d *Daemon) typing() *sessionrpc.Env {
	screens := team.ScreenFor(d.cfg.Home, d.db, claudecode.ScreenFor)
	t := term.New(&term.Machines{Local: agenthost.Dynamic(d.cfg.Home), Remote: d.nodes.Terminals()}, screens)
	if d.cfg.Terminal != nil {
		t = term.New(d.cfg.Terminal, screens)
	}
	env := &sessionrpc.Env{DB: d.db, Home: d.cfg.Home, Holds: d.holds, Term: t, Marker: d.tchan.Marker,
		Wall: func(agent string) string { why, _ := d.env().WallReason(d.db, agent, nil); return why }}
	if d.cfg.Turns != nil {
		t.Turns = d.cfg.Turns
		env.Turns = d.cfg.Turns
		env.Running = func(agent string) []sessionrpc.RunningJob {
			var out []sessionrpc.RunningJob
			for _, j := range d.cfg.Turns.Running(agent) {
				out = append(out, sessionrpc.RunningJob{ID: j.ID, Type: j.Type, Name: j.Name})
			}
			return out
		}
	}
	return env
}

// operationEvent sends an operation's progress to the operations topic.
func (d *Daemon) operationEvent(topic, typ string, data any, agent string) {
	if _, err := d.hub.Append(stream.Entry{Topic: topic, Type: typ, Data: data, Agent: agent}); err != nil {
		d.cfg.Log("  stream: %s: %v", typ, err)
	}
}

// readsEnv is the env of the reads every server serves, and of the status the daemon tells the status topic.
func (d *Daemon) readsEnv() reads.Env {
	renv := reads.Env{DB: d.db, Home: d.cfg.Home, Started: d.started, Sha: version.Sha, Schema: store.LatestVersion(), Delivery: d.cfg.Delivery,
		Context: d.cfg.Context, Claude: d.cfg.Claude, HealthClock: d.hclock, KeepAwake: d.cfg.KeepAwake}
	if d.projects != nil {
		renv.NodeProjects = d.projects.Of
	}
	if d.nodes != nil {
		renv.NodeOnline = d.nodes.Online
		renv.NodeHeard = func(node string) (map[string]any, bool, string, bool) {
			b, _, ok := d.nodes.Heard(node)
			l := b.Load
			return map[string]any{"cores": l.Cores, "load1": l.Load1, "ram_total": l.RAMTotal, "ram_free": l.RAMFree, "agents_rss": l.AgentsRSS},
				b.Battery, b.Version, ok
		}
		renv.NodeApp = func(node string) any {
			if b, _, ok := d.nodes.Heard(node); ok && b.App != nil {
				return map[string]any{"version": b.App.Version, "ready": b.App.Ready}
			}
			return nil
		}
		renv.NodeSetAside = func(node string) any {
			if b, _, ok := d.nodes.Heard(node); ok && b.SetAside != nil {
				return map[string]any{"since": time.Unix(b.SetAside.Since, 0).UTC().Format("2006-01-02T15:04:05Z"), "master": b.SetAside.Master}
			}
			return nil
		}
	}
	if d.nodes != nil && d.nodes.Copy != nil {
		renv.NodeCopy = d.nodes.CopyOf
	}
	if d.placer != nil {
		renv.NodeLacks = d.placer.Lacks
		renv.NodePlacement = func(agent string) (any, error) { return d.placer.Status(agent) }
		renv.NodeManual = d.placer.Manual
	}
	if d.res != nil {
		renv.Resources = d.res.Latest
		renv.Machine = d.res.LatestMachine
	}
	if d.load != nil {
		renv.Load, renv.LoadSeries = d.load.View, d.load.Series
	}
	if d.logins != nil {
		renv.Login = d.logins.Of
	}
	if d.tchan != nil {
		renv.Installs = d.tchan.Installs
	}
	return renv
}

// newServer is a connection server on one transport, with the methods this daemon serves.
func (d *Daemon) newServer(transport string) *rpc.Server {
	s := rpc.NewServer(d.cfg.Auth, version.Sha, store.LatestVersion(), transport, d.cfg.Log)
	s.DB = d.db
	s.Names = d.db
	s.Note = func(agent, kind string, argv []string, chain string) {
		_, _ = dbpkg.AddEvent(d.db, agent, kind, dbpkg.KV{K: "argv", V: argv}, dbpkg.KV{K: "chain", V: chain})
	}
	if d.hooks != nil {
		for _, register := range d.writeRegistrars() {
			register(s)
		}
	}
	if transport == "network" {
		s.Net = d.dev
		s.Wrap("hello", d.linkRunner)
	}
	s.Register("node.handover", rpc.Method{Waits: true, Handler: d.handover}) // a runner's link, or another master's ask: refused elsewhere
	s.Register("node.outranked", rpc.Method{Handler: d.outranked})
	s.Hold = d.master.refusal
	s.DeclareCapabilities(func() map[string]any {
		machines, err := reads.Machines(d.db)
		if err != nil {
			d.cfg.Log("hello: the machines could not be counted: %v", err)
		}
		return map[string]any{"master": map[string]any{"node": d.nodes.Self, "number": d.number()},
			"cluster": map[string]any{"name": reads.ClusterName(d.db), "machines": machines}}
	})
	nodes.Register(s, func() *rpc.Server { return d.local }, d.nodes)
	devices.Register(s, d.dev, d.hub.Emit)
	devices.RegisterPeople(s, d.dev)
	stream.Register(s, d.hub)
	if d.cfg.Transcripts {
		s.DeclareCapabilities(func() map[string]any { return map[string]any{"transcript": true} })
	}
	s.DeclareCapabilities(func() map[string]any { return map[string]any{"founder_typed": d.env().FounderTyped != nil} })
	reads.Register(s, d.readsEnv())
	leases.Register(s, d.leases)
	runs.Register(s, d.runs, d.leases.View)
	schedules.Register(s, d.cfg.Home)
	s.Register("connection.addresses", rpc.Method{Handler: d.connectionAddresses, Read: true})
	stream.StampCursor(s, d.hub.Log)
	return s
}

func (d *Daemon) ingestSpool() {
	if _, err := d.hooks.Ingest(); err != nil {
		d.cfg.Log("  spool: %v", err)
	}
}

// spoolLoop reads the hook spool every SpoolEvery, the work the Python daemon does each tick.
func (d *Daemon) spoolLoop() {
	defer d.wg.Done()
	t := time.NewTicker(hooks.SpoolEvery)
	defer t.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-t.C:
			d.ingestSpool()
		}
	}
}

// listenUnix binds the socket, 0600, replacing a stale file but never one a live daemon answers on.
func listenUnix(file string) (net.Listener, error) {
	if c, err := net.DialTimeout("unix", file, time.Second); err == nil {
		c.Close()
		return nil, fmt.Errorf("a daemon already answers on %s", file)
	}
	os.Remove(file)
	old := umaskSet(0o177)
	defer umaskSet(old)
	ln, err := net.Listen("unix", file)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(file, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func (d *Daemon) track(c net.Conn) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closing {
		return false
	}
	d.conns[c] = struct{}{}
	d.wg.Add(1)
	return true
}

func (d *Daemon) untrack(c net.Conn) {
	d.mu.Lock()
	delete(d.conns, c)
	d.mu.Unlock()
	c.Close()
	d.wg.Done()
}

func (d *Daemon) acceptLocal(srv *rpc.Server) {
	defer d.wg.Done()
	for {
		c, err := d.ln.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				d.cfg.Log("accept: %v", err)
			}
			return
		}
		if !d.track(c) {
			c.Close()
			return
		}
		go func() {
			defer d.untrack(c)
			srv.Serve(rpc.NewLineTransport(c), peer.PID(c))
		}()
	}
}

// ServeFile serves one connection that already exists, a unix stream socket made by the caller (a socketpair):
// for a run where no socket can be bound. The caller on it is told apart as on the local socket.
func (d *Daemon) ServeFile(f *os.File) error {
	c, err := net.FileConn(f)
	f.Close()
	if err != nil {
		return err
	}
	if !d.track(c) {
		c.Close()
		return errors.New("daemon is closing")
	}
	go func() {
		defer d.untrack(c)
		d.local.Serve(rpc.NewLineTransport(&inherited{Conn: c}), peer.PID(c))
	}()
	return nil
}

// ServeNetFile serves one connection that already exists as a network connection: past TLS and the pinned
// fingerprint, so the gate and the device roles apply. It is how the contract's network fixtures reach a daemon where
// no socket can be bound.
func (d *Daemon) ServeNetFile(f *os.File) error {
	c, err := net.FileConn(f)
	f.Close()
	if err != nil {
		return err
	}
	if d.net == nil || !d.track(c) {
		c.Close()
		return errors.New("no network listener")
	}
	go func() {
		defer d.untrack(c)
		d.net.rs.ServeNet(rpc.NewLineTransport(&inherited{Conn: c}), "", func() { c.Close() })
	}()
	return nil
}

// inherited is a connection the parent opened before it was needed: the pre-hello limit starts at its first byte,
// not when the daemon began serving it, so a run that keeps spare connections for later fixtures does not lose them.
type inherited struct {
	net.Conn
	spoke bool
}

func (c *inherited) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.spoke = c.spoke || n > 0
	return n, err
}

func (c *inherited) SetReadDeadline(t time.Time) error {
	if !c.spoke {
		return nil
	}
	return c.Conn.SetReadDeadline(t)
}

// Close stops listening, drops every connection, waits for their goroutines and gives up the lock and the database.
// It is safe to call twice: a second call waits for the first to finish.
func (d *Daemon) Close() { d.closeOnce.Do(d.close) }

func (d *Daemon) close() {
	d.stopPause()
	d.mu.Lock()
	if !d.closing && d.stop != nil {
		close(d.stop)
	}
	d.closing = true
	for c := range d.conns {
		c.Close()
	}
	d.mu.Unlock()
	if d.ln != nil {
		d.ln.Close()
		os.Remove(d.cfg.SocketFile)
	}
	if d.net != nil {
		d.net.close()
	}
	if d.logins != nil {
		d.logins.Close()
	}
	d.wg.Wait()
	if d.envMade != nil {
		d.envMade.Wait()
	}
	if d.cfg.BeforeClose != nil {
		d.cfg.BeforeClose()
	}
	if d.db != nil {
		if d.lent != nil {
			d.lent()
		}
		d.db.Close()
	}
	d.stopCopy()
	if d.single != nil { // last: another daemon on this home may start its own copy only once this one's has stopped
		d.single.Release()
	}
}

// stopCopy stops the board's copy, after the board's own connections have closed (Litestream's handle on the file would drop their
// locks), with its last changes written.
func (d *Daemon) stopCopy() {
	if d.copier != nil {
		if err := d.copier.Stop(); err != nil {
			d.cfg.Log("  board copy: %v", err)
		}
		d.copier = nil
	}
}

// paneRuns is what the terminal host says of the windows that run commands (watcher.Chat.Panes); nil when it cannot say.
func (d *Daemon) paneRuns() map[string]term.PaneRun {
	if d.tchan == nil {
		return nil
	}
	return d.tchan.PaneRuns()
}

// paneTail is the last rows of a terminal as the host keeps them (watcher.Chat.Tail); nil when it cannot say.
func (d *Daemon) paneTail(handle string, rows int) []string {
	if d.tchan == nil {
		return nil
	}
	return d.tchan.Tail(handle, rows)
}

// ensureOwner is the first start's owner, and the check that it is actor.OwnerKey, the key phase 1 writes for the person: a home whose
// owner is another key would have writes go to no one, so the daemon does not start on it.
func ensureOwner(db *sql.DB, ownerName func() string) error {
	k, err := actor.EnsureOwner(db, ownerName)
	if err != nil {
		return err
	}
	if k != actor.OwnerKey {
		return fmt.Errorf("the owner of this home is %s, not %s: this daemon writes the person as %s, so it does not start here", k, actor.OwnerKey, actor.OwnerKey)
	}
	return nil
}

// keepOwnNode writes this machine's row in nodes at start, under its node id (made the first time).
func keepOwnNode(db *sql.DB, home string) error {
	id, err := dbpkg.NodeID(home, db)
	if err != nil {
		return fmt.Errorf("node id: %w", err)
	}
	return dbpkg.KeepOwnNode(db, id, sysres.MachineName(), time.Now().Unix())
}

// linkRunner is hello on the network door: a joined machine's device (one with a node) whose hello names runner.Client is that
// runner's link. Any other connection with its token (an app on that machine) is not.
func (d *Daemon) linkRunner(next func(*rpc.Conn, map[string]any) (any, error)) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		res, err := next(c, p)
		id, _ := c.Caller.Info["device"].(string)
		if err != nil || id == "" || d.dev == nil || p["client"] != runner.Client {
			return res, err
		}
		ds, derr := d.dev.Devices()
		if derr != nil {
			d.cfg.Log("  device %s: not linked as a runner: %v", id, derr)
			return res, nil
		}
		for _, dev := range ds {
			if dev.ID == id && dev.Node != "" {
				d.nodes.Link(dev.Node, c)
			}
		}
		return res, nil
	}
}

// transcriptRows is whether chat keeps the rows of agents' session transcripts (chat_transcript_rows).
func transcriptRows(home string) bool {
	return (&settings.Settings{Home: home}).Flag(settings.TranscriptRowsKey, false)
}

// boardID is this board's own id (meta board_id), made the first time: the board's copy names it, so a board made anew is not copied
// on top of another's.
func boardID(db *sql.DB) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	if _, err := db.Exec("INSERT OR IGNORE INTO meta(name, value, updated) VALUES('board_id', ?, ?)", hex.EncodeToString(b), time.Now().Unix()); err != nil {
		return "", err
	}
	var id string
	return id, db.QueryRow("SELECT value FROM meta WHERE name = 'board_id'").Scan(&id)
}

// room is this machine's room for a session (placement.Placer.Here): its agents' sessions here, their memory and its free memory.
func (d *Daemon) room() (placement.Room, error) {
	var r placement.Room
	if err := d.db.QueryRow("SELECT COUNT(*) FROM agent_state WHERE terminal IS NOT NULL AND terminal != '' AND instr(terminal, ':') = 0").
		Scan(&r.Sessions); err != nil {
		return r, err
	}
	if d.res != nil {
		_, rss := d.res.AgentsMemory()
		for _, n := range rss {
			r.AgentsRSS += n
		}
		if m := d.res.LatestMachine(); m != nil {
			r.RAMFree = m.RAMFree
		}
	}
	return r, nil
}
