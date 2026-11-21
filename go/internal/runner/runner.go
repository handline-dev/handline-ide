// Package runner is a daemon in runner mode (docs/story4-design.md, slice 4): a joined machine's daemon, which keeps no board. It
// keeps one link open to the master, with its device token, and serves the calls the master drives it with over that link; its local
// socket forwards its agents' calls to the master, each proven the agent's by process descent from a session it opened.
package runner

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/internal/boardcopy"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/homefiles"
	"github.com/handline/handline/go/internal/hooks"
	"github.com/handline/handline/go/internal/termchan"
	"github.com/handline/handline/go/internal/transcript"
	"github.com/handline/handline/go/kit/atomicfile"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/peer"
	"github.com/handline/handline/go/protocol/authority"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/protocol/sessiontoken"
)

// Terminals is what the runner asks of its terminal host.
type Terminals interface {
	CreateTerminal(command, cwd, title, agent string) (handle, why string)
	CloseTerminal(handle string, tab bool) (bool, string)
	RespawnTerminal(handle string) (bool, string)
	ListTerminals() map[string]bool
	Send(handle, text string, enter bool) (bool, string)
	ReadScreen(handle string, limit int) *term.Screen
	PanePid(handle string) int
}

// Tokens is how the runner records and forgets an agent's session: the proof its local socket checks.
type Tokens interface {
	Record(agent string, pid int) bool
	Forget(agent string, pid int) bool
}

// Config is what a runner runs with. Term, Auth and Tokens default to the home's tmux, protected store and session tokens; Control,
// the control-mode client the bytes format streams from, to Term's (term.ControlSpawner).
type Config struct {
	Home, Socket, Sha string
	Master            joined.Master
	Term              Terminals
	Control           func() (termchan.Proc, error)
	Auth              authority.Authority
	Tokens            Tokens
	HL                string // the hl its hooks call; paths.HlExe when empty
	Log               func(format string, args ...any)
	// Accounts is this machine's Claude accounts, whose transcripts' calls the master counts (node.tokens); nil is the home's account
	// folders logged in to one (a runner keeps no accounts table).
	Accounts func() []string
	// Battery is whether the machine runs on its battery now, which each heartbeat says; nil reads the Mac's power source.
	Battery func() bool
	// Meter reads this machine's accounts' usage, which each heartbeat carries; nil reads none.
	Meter Meter
}

// Runner is one runner daemon.
type Runner struct {
	cfg    Config
	local  *rpc.Server
	meters meters // what this machine read of its accounts' usage
	ln     net.Listener
	// turns watches the sessions here, from their hooks and processes, and tells the master each turn (node.turn); hooks records
	// each hook event: the tracker hears it, then the master, or the spool while the master is away.
	turns *turns.Tracker
	hooks *hooks.Recorder
	// changed is the agents whose turn the master has not been sent since it changed; turnSent wakes the one loop that sends them.
	changed  map[string]bool
	turnSent chan struct{}
	beatNow  chan struct{} // the link came up: a heartbeat goes at once
	check    []string      // the programs the master asks whether this machine has
	hasKept  hasKept

	mu       sync.Mutex
	link     *rpc.Link                             // nil while the master is not reached
	sent     map[string]map[string]homefiles.Stamp // by agent: its files as last sent up
	sendMu   sync.Mutex                            // one send up at a time, so two never cross
	stopped  bool                                  // Close ran: no link is kept from here
	sessions map[string]session
	repos    map[string]*sync.Mutex // a clone's lock, held while it is synced
	pushers  map[string]*pusher     // a worktree's push running now
	streams  *termchan.Streams      // the master's bytes attaches, ended with the link they go up
	// rescuing is the agents whose session's rescue runs now; rescueFails how many times each one's push failed.
	rescuing    map[string]bool
	rescueFails map[string]int
	stop        chan struct{}
	once        sync.Once
	pulling     sync.Mutex // held by Pull
	wg          sync.WaitGroup
}

// session is an agent's terminal the runner opened, kept in runner.json. Agent is the agent's folder under the home once a launch there
// named it: its memory and handoff note go up to the master as they change.
type session struct {
	Handle string `json:"handle"`
	Pid    int    `json:"pid"`
	Agent  string `json:"agent,omitempty"`
	Folder string `json:"folder,omitempty"` // the folder it was opened in: the agent's worktree, whose commits are pushed
	// Rescued is that what its worktree held was pushed to a rescue branch: the master no longer places it here.
	Rescued bool `json:"rescued,omitempty"`
}

// Client is the client a runner's hello names on its link: the master takes only that connection for the runner's link.
const Client = "handlined-runner"

// StateFile is where a runner keeps the sessions it opened, so a restart still knows them.
func StateFile(home string) string { return filepath.Join(home, "runner.json") }

// Start starts the runner: its local socket, and the link to the master, redialled until Close.
func Start(cfg Config) (*Runner, error) {
	if cfg.Log == nil {
		cfg.Log = func(string, ...any) {}
	}
	if cfg.Auth == nil {
		cfg.Auth = authority.New(cfg.Home)
	}
	if cfg.Tokens == nil {
		cfg.Tokens = sessiontoken.New(cfg.Home)
	}
	if cfg.Control == nil {
		if s, ok := cfg.Term.(term.ControlSpawner); ok {
			cfg.Control = termchan.SpawnCommand(s.ControlCommand())
		} else {
			cfg.Control = func() (termchan.Proc, error) { return nil, errors.New("the terminal host streams no bytes") }
		}
	}
	if cfg.HL == "" {
		cfg.HL = paths.HlExe()
	}
	if cfg.Accounts == nil {
		cfg.Accounts = func() []string { return loggedIn(cfg.Home) }
	}
	if cfg.Battery == nil {
		cfg.Battery = machineOnBattery
	}
	if err := writeHooks(cfg.Home, cfg.HL); err != nil {
		return nil, fmt.Errorf("hooks: %w", err)
	}
	r := &Runner{cfg: cfg, sessions: map[string]session{}, sent: map[string]map[string]homefiles.Stamp{}, repos: map[string]*sync.Mutex{},
		pushers: map[string]*pusher{}, rescuing: map[string]bool{}, rescueFails: map[string]int{}, stop: make(chan struct{}), changed: map[string]bool{}, turnSent: make(chan struct{}, 1),
		beatNow: make(chan struct{}, 1)}
	control := termchan.NewControlLink(cfg.Control, true)
	r.streams = termchan.NewStreams(func() *termchan.ControlLink { return control })
	if b, err := os.ReadFile(StateFile(cfg.Home)); err == nil {
		if err := json.Unmarshal(b, &r.sessions); err != nil {
			return nil, fmt.Errorf("%s: %w", StateFile(cfg.Home), err)
		}
	}
	r.local = rpc.NewServer(cfg.Auth, cfg.Sha, 0, "local", cfg.Log)
	r.local.Register("runner.committed", rpc.Method{Role: "agent", Handler: r.committed})
	r.local.Fallback = r.forward
	r.turns = &turns.Tracker{Log: cfg.Log, Lines: r.transcriptLines}
	r.turns.OnEnd(r.sessionPid, nil)
	r.turns.OnChange(r.tellTurn)
	r.hooks = &hooks.Recorder{Home: cfg.Home, Log: cfg.Log, Turns: r.turns, Forward: r.sendHook}
	if cfg.Socket != "" {
		os.Remove(cfg.Socket)
		old := syscall.Umask(0o177) // the socket is never open to others, not even before the chmod
		ln, err := net.Listen("unix", cfg.Socket)
		syscall.Umask(old)
		if err != nil {
			return nil, err
		}
		if err := os.Chmod(cfg.Socket, 0o600); err != nil {
			ln.Close()
			return nil, err
		}
		r.ln = ln
		r.wg.Add(1)
		go r.accept()
	}
	r.wg.Add(8)
	go r.heartbeats()
	go r.meter()
	go r.keepLinked()
	go r.mirror()
	go r.sendUp()
	go r.drainSpool()
	go r.sendTokens()
	go r.sendTurns()
	return r, nil
}

// Close stops the runner; the sessions it opened keep running.
func (r *Runner) Close() {
	r.once.Do(func() {
		close(r.stop)
		if r.ln != nil {
			r.ln.Close()
		}
		r.mu.Lock()
		r.stopped = true
		if r.link != nil {
			r.link.Close()
		}
		r.mu.Unlock()
	})
	r.wg.Wait()
}

// Local is the runner's local socket server, for a connection a test serves.
func (r *Runner) Local() *rpc.Server { return r.local }

func (r *Runner) accept() {
	defer r.wg.Done()
	for {
		c, err := r.ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			r.local.Serve(rpc.NewLineTransport(c), peer.PID(c))
		}()
	}
}

// forward is every local call but hello: an agent's call goes to the master as that agent; anyone else is refused.
func (r *Runner) forward(c *rpc.Conn, name string, params map[string]any) (any, error) {
	if err := r.cfg.Auth.SessionsReadable(); err != nil {
		return nil, rpc.Refused("this runner cannot read its agents' sessions, so it forwards nothing", "agent", "")
	}
	if c.Caller.Kind != "agent" {
		return nil, rpc.Refused("this daemon is a runner: it forwards only the calls of the agents it runs", "agent", "")
	}
	if strings.HasPrefix(name, "runner.") {
		return nil, rpc.Refused(name+" is the master's call, on the runner's link", "link", "")
	}
	if name == "hook.record" {
		return r.hooks.Record(c, params)
	}
	r.mu.Lock()
	l := r.link
	r.mu.Unlock()
	if l == nil {
		return nil, rpc.Refused("the master cannot be reached now", "master", "")
	}
	if name == "session.launch_prepare" {
		return r.launch(l, c.Caller.Key, params)
	}
	return l.Call("node.forward", map[string]any{"agent": c.Caller.Key, "method": name, "params": params, "client": c.HelloClient(),
		"cwd": c.Cwd()}, hlclient.DeadlineOf(name))
}

// keepLinked keeps the link to the master open, dialling again after a wait that grows to a minute while it cannot.
func (r *Runner) keepLinked() {
	defer r.wg.Done()
	wait := time.Second
	for {
		up, err := r.linkOnce()
		select {
		case <-r.stop:
			return
		default:
		}
		if up {
			wait = time.Second
		}
		r.cfg.Log("runner: the link to the master at %s ended: %v; again in %s", r.cfg.Master.Addr, err, wait)
		select {
		case <-r.stop:
			return
		case <-time.After(wait):
		}
		wait = min(2*wait, time.Minute)
	}
}

// linkIdle is how long the link waits for the master's next call before it dials again; the master drops an idle one after an hour.
const linkIdle = rpc.IdleTimeout + time.Minute

// linkOnce dials the master, says hello with the device token and serves the master's calls until the link ends; up is whether hello
// was answered.
func (r *Runner) linkOnce() (up bool, err error) {
	ws, err := hlclient.DialMaster(r.cfg.Master.Addr, r.cfg.Master.Fingerprint)
	if err != nil {
		return false, err
	}
	var l *rpc.Link
	l = rpc.NewLink(rpc.NewWSTransport(ws), func(method string, p map[string]any) (any, error) {
		r.mu.Lock() // Close waits for the calls under way, so none writes in the home after it
		if r.stopped {
			r.mu.Unlock()
			return nil, rpc.Refused("the runner is stopping", "state", "")
		}
		r.wg.Add(1)
		r.mu.Unlock()
		defer r.wg.Done()
		if method == "runner.attach" { // its stream's notices go up this link, and end with it
			return r.attach(p, l.Notify)
		}
		return r.serve(method, p)
	})
	ended := make(chan error, 1)
	go func() { ended <- l.Run(linkIdle) }()
	if _, err := l.Call("hello", map[string]any{"client": Client, "protocol": rpc.Protocol, "token": r.cfg.Master.Token},
		hlclient.Quick); err != nil {
		l.Close()
		<-ended
		return false, fmt.Errorf("hello: %w", err)
	}
	r.mu.Lock()
	if r.stopped { // Close ran while this dialled: it saw no link to close
		r.mu.Unlock()
		l.Close()
		<-ended
		return true, errors.New("the runner is stopping")
	}
	r.link = l
	agents := make([]string, 0, len(r.sessions))
	for a := range r.sessions {
		agents = append(agents, a)
	}
	r.mu.Unlock()
	r.cfg.Log("runner: linked to the master at %s", r.cfg.Master.Addr)
	r.beatSoon()
	for _, a := range agents { // what changed while the link was down
		r.tellTurn(a)
	}
	err = <-ended
	r.mu.Lock()
	r.link = nil
	r.mu.Unlock()
	r.streams.DetachAll()
	return true, err
}

// serve is a call the master sends on the link.
func (r *Runner) serve(method string, p map[string]any) (any, error) {
	str := func(k string) string { s, _ := p[k].(string); return s }
	switch method {
	case "runner.terminal.open":
		return r.open(str("agent"), str("folder"), str("command"))
	case "runner.keys":
		enter, _ := p["enter"].(bool)
		if ok, why := r.cfg.Term.Send(str("handle"), str("keys"), enter); !ok {
			return nil, rpc.Refused("could not type into "+str("handle")+": "+why, "terminal", "")
		}
		return map[string]any{"accepted": true}, nil
	case "runner.screen":
		rows := 50
		if n, ok := rpc.AsInt(p["rows"]); ok && n > 0 {
			rows = int(n)
		}
		if styled, _ := p["styled"].(bool); styled {
			sr, ok := r.cfg.Term.(term.StyledReader)
			var got []string
			if ok {
				got = sr.ReadScreenANSI(str("handle"), rows)
			}
			if got == nil {
				return nil, rpc.NotFoundError("no styled screen for " + str("handle"))
			}
			return map[string]any{"rows": got}, nil
		}
		s := r.cfg.Term.ReadScreen(str("handle"), rows)
		if s == nil {
			return nil, rpc.NotFoundError("no screen for " + str("handle"))
		}
		return map[string]any{"rows": append([]string{}, s.Tail...)}, nil
	case "runner.input":
		data, err := base64.StdEncoding.DecodeString(str("data"))
		if err != nil || len(data) == 0 {
			return nil, rpc.BadRequestError("data is base64", "data", rpc.InvalidParams)
		}
		bs, ok := r.cfg.Term.(term.BytesSender)
		if !ok {
			return nil, rpc.Refused("this runner's terminal host cannot send raw bytes", "unsupported", "")
		}
		if ok, why := bs.SendBytes(str("handle"), data); !ok {
			return nil, rpc.Refused("could not type into "+str("handle")+": "+why, "terminal", "")
		}
		return map[string]any{"accepted": true}, nil
	case "runner.size", "runner.resize":
		sz, ok := r.cfg.Term.(term.Sizer)
		if !ok {
			return nil, rpc.Refused("this runner's terminal host has no size", "unsupported", "")
		}
		if method == "runner.size" {
			cols, rows, ok := sz.Size(str("handle"))
			if !ok {
				return nil, rpc.NotFoundError("no size for " + str("handle"))
			}
			return map[string]any{"cols": cols, "rows": rows}, nil
		}
		cols, ok1 := rpc.AsInt(p["cols"])
		rows, ok2 := rpc.AsInt(p["rows"])
		if !ok1 || !ok2 {
			return nil, rpc.BadRequestError("cols and rows are integers", "cols", rpc.InvalidParams)
		}
		if ok, why := sz.Resize(str("handle"), int(cols), int(rows)); !ok {
			return nil, rpc.Refused("could not resize "+str("handle")+": "+why, "unsupported", "")
		}
		return map[string]any{"accepted": true}, nil
	case "runner.detach":
		r.streams.Detach(str("stream"))
		return map[string]any{"accepted": true}, nil
	case "runner.resync":
		if err := r.streams.Resync(str("stream"), str("reason")); err != nil {
			return nil, err
		}
		return map[string]any{"accepted": true}, nil
	case "runner.history":
		before, ok1 := rpc.AsInt(p["before"])
		limit, ok2 := rpc.AsInt(p["limit"])
		if !ok1 || !ok2 {
			return nil, rpc.BadRequestError("before and limit are integers", "before", rpc.InvalidParams)
		}
		return r.streams.History(str("handle"), int(before), int(limit))
	case "runner.stop":
		return r.close(str("agent"), str("handle"))
	case "runner.respawn":
		return r.respawn(str("handle"))
	case "runner.terminals":
		set := r.cfg.Term.ListTerminals()
		if set == nil {
			return nil, errors.New("the terminal host could not list its terminals")
		}
		handles := []string{}
		for h := range set {
			handles = append(handles, h)
		}
		return map[string]any{"handles": handles}, nil
	case "runner.home":
		return map[string]any{"home": r.cfg.Home}, nil
	case "runner.files":
		files, err := homefiles.FromWire(p["files"])
		if err != nil {
			return nil, rpc.BadRequestError(err.Error(), "files", rpc.InvalidParams)
		}
		if err := homefiles.Apply(r.cfg.Home, files, homefiles.Strings(p["deleted"])); err != nil {
			return nil, rpc.Refused(err.Error(), "path", "")
		}
		return map[string]any{"accepted": true}, nil
	case "runner.flush":
		if err := r.sendAgent(str("agent")); err != nil {
			return nil, err
		}
		return map[string]any{"accepted": true}, nil
	case "runner.project.sync":
		if err := r.syncProject(str("remote")); err != nil {
			return nil, err
		}
		return map[string]any{"state": "cloned"}, nil
	case "runner.worktree.make":
		return r.makeWorktree(str("remote"), str("path"), str("branch"), str("base"))
	case "runner.push":
		if err := plain(str("path"), str("branch")); err != nil {
			return nil, err
		}
		if err := r.git("-C", str("path"), "push", "origin", "--", str("branch")); err != nil {
			return nil, err
		}
		return map[string]any{"accepted": true}, nil
	}
	return nil, rpc.BadRequestError("no such method: "+method, "method", rpc.NoSuchMethod)
}

// attach starts a stream of the terminal's bytes for the master: its answer is the snapshot, and each notice after it goes up the link
// with notify (termchan.Streams).
func (r *Runner) attach(p map[string]any, notify func(string, map[string]any) bool) (any, error) {
	id, _ := p["stream"].(string)
	handle, _ := p["handle"].(string)
	history, _ := rpc.AsInt(p["history"])
	return r.streams.Attach(id, handle, int(min(max(history, 0), termchan.MaxHistory)), notify)
}

// open opens agent's terminal in folder with command typed at its shell, and records its shell as the agent's session: what runs
// there descends from it, which is what the local socket proves.
func (r *Runner) open(agent, folder, command string) (any, error) {
	if agent == "" || folder == "" {
		return nil, rpc.BadRequestError("agent and folder are strings", "agent", rpc.InvalidParams)
	}
	r.mu.Lock()
	old, had := r.sessions[agent]
	r.mu.Unlock()
	if had { // one session an agent: the one before it no longer proves itself the agent's
		r.close(agent, old.Handle)
	}
	handle, why := r.cfg.Term.CreateTerminal(command, folder, agent, agent)
	if handle == "" {
		return nil, fmt.Errorf("could not open a terminal for %s: %s", agent, why)
	}
	pid := r.cfg.Term.PanePid(handle)
	if pid == 0 || !r.cfg.Tokens.Record(agent, pid) {
		r.cfg.Term.CloseTerminal(handle, true)
		return nil, fmt.Errorf("the session of %s could not be recorded, so it is not opened", agent)
	}
	r.mu.Lock()
	r.sessions[agent] = session{Handle: handle, Pid: pid, Folder: folder}
	err := r.save()
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	r.beatSoon() // the master hears of what it holds now
	return map[string]any{"handle": handle}, nil
}

func (r *Runner) close(agent, handle string) (any, error) {
	r.mu.Lock()
	s, ok := r.sessions[agent]
	r.mu.Unlock()
	if !ok || s.Handle != handle {
		return nil, rpc.NotFoundError(agent + " has no session " + handle + " here")
	}
	if ok, why := r.cfg.Term.CloseTerminal(handle, true); !ok {
		return nil, fmt.Errorf("could not close %s: %s", handle, why)
	}
	return map[string]any{"accepted": true}, r.forgetSession(agent, s)
}

// forgetSession drops agent's session s, which is no longer the agent's proof, and what was sent up of its files; a session opened
// for the agent since stays.
func (r *Runner) forgetSession(agent string, s session) error {
	r.cfg.Tokens.Forget(agent, s.Pid)
	r.mu.Lock()
	if r.sessions[agent] == s {
		delete(r.sessions, agent)
	}
	err := r.save()
	r.mu.Unlock()
	r.sendMu.Lock()
	delete(r.sent, agent)
	r.sendMu.Unlock()
	return err
}

// respawn leaves a fresh shell in a session's terminal and records that shell as the agent's session in place of the one it ended.
func (r *Runner) respawn(handle string) (any, error) {
	r.mu.Lock()
	agent, s := "", session{}
	for a, v := range r.sessions {
		if v.Handle == handle {
			agent, s = a, v
		}
	}
	r.mu.Unlock()
	if agent == "" {
		return nil, rpc.NotFoundError("no session " + handle + " here")
	}
	if ok, why := r.cfg.Term.RespawnTerminal(handle); !ok {
		return nil, fmt.Errorf("could not restart the shell of %s: %s", handle, why)
	}
	r.cfg.Tokens.Forget(agent, s.Pid)
	pid := r.cfg.Term.PanePid(handle)
	recorded := pid != 0 && r.cfg.Tokens.Record(agent, pid)
	if !recorded {
		r.cfg.Term.CloseTerminal(handle, true)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[agent] != s { // opened again or stopped meanwhile: that call's record stands
		return map[string]any{"accepted": true}, nil
	}
	if !recorded {
		delete(r.sessions, agent)
		return nil, errors.Join(fmt.Errorf("the new shell of %s could not be recorded, so its terminal is closed", agent), r.save())
	}
	r.sessions[agent] = session{Handle: handle, Pid: pid, Agent: s.Agent, Folder: s.Folder}
	return map[string]any{"accepted": true}, r.save()
}

// launch is hl launch's session.launch_prepare: the master builds the launch for this home and sends the files it starts with, which are
// written here before hl execs what it answered.
func (r *Runner) launch(l *rpc.Link, agent string, params map[string]any) (any, error) {
	got, err := l.Call("node.launch", map[string]any{"agent": agent, "params": params}, hlclient.DeadlineOf("session.launch_prepare"))
	if err != nil {
		return nil, err
	}
	files, err := homefiles.FromWire(got["files"])
	if err != nil {
		return nil, fmt.Errorf("the master's files for %s: %w", agent, err)
	}
	dir, _ := got["agent_dir"].(string)
	if !homefiles.Inside(dir) {
		return nil, fmt.Errorf("the master named %q as the agent's folder", dir)
	}
	// Its memory is the master's: a file left here by a session before, which the master no longer has, goes.
	shipped := map[string]bool{}
	for _, f := range files {
		shipped[f.Path] = true
	}
	var stale []string
	for p := range homefiles.Stamps(r.cfg.Home, filepath.Join(r.cfg.Home, dir, "memory")) {
		if !shipped[p] {
			stale = append(stale, p)
		}
	}
	if err := homefiles.Apply(r.cfg.Home, files, stale); err != nil {
		return nil, fmt.Errorf("the master's files for %s not written: %w", agent, err)
	}
	r.mu.Lock()
	s, ok := r.sessions[agent]
	if ok {
		s.Agent = dir
		r.sessions[agent] = s
		err = r.save()
	}
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s has no session here to launch in", agent)
	}
	if err != nil {
		return nil, err
	}
	r.sendMu.Lock()
	r.sent[agent] = homefiles.Stamps(r.cfg.Home, r.agentFiles(dir)...) // what came down is what the master has
	r.sendMu.Unlock()
	return map[string]any{"argv": got["argv"], "env": got["env"], "warnings": got["warnings"]}, nil
}

// SendEvery is how often the runner looks for an agent's memory and handoff note having changed.
const SendEvery = 2 * time.Second

// sendUp sends each agent's changed files up to the master until Close.
func (r *Runner) sendUp() {
	defer r.wg.Done()
	t := time.NewTicker(SendEvery)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
		}
		r.mu.Lock()
		agents := make([]string, 0, len(r.sessions))
		for a, s := range r.sessions {
			if s.Agent != "" && r.link != nil { // with no master to send to, they wait for the next look
				agents = append(agents, a)
			}
		}
		r.mu.Unlock()
		for _, a := range agents {
			if err := r.sendAgent(a); err != nil {
				r.cfg.Log("runner: %s's files not sent to the master: %v", a, err)
			}
		}
	}
}

// agentFiles are the files of an agent's folder that go up: its memory and its handoff note.
func (r *Runner) agentFiles(dir string) []string {
	return []string{filepath.Join(r.cfg.Home, dir, "memory"), filepath.Join(r.cfg.Home, dir, "handoff.md")}
}

// sendAgent sends the agent's files that changed since they were last sent: node.agent_files, by path under its folder.
func (r *Runner) sendAgent(agent string) error {
	r.sendMu.Lock()
	defer r.sendMu.Unlock()
	r.mu.Lock() // under sendMu, so a close that forgot the agent meanwhile is seen
	s, ok := r.sessions[agent]
	l := r.link
	r.mu.Unlock()
	if !ok || s.Agent == "" {
		return rpc.NotFoundError(agent + " has no session here")
	}
	if l == nil {
		return errors.New("the master cannot be reached now")
	}
	now := homefiles.Stamps(r.cfg.Home, r.agentFiles(s.Agent)...)
	changed, gone := homefiles.Changed(r.sent[agent], now)
	if len(changed) == 0 && len(gone) == 0 {
		return nil
	}
	var abs []string
	for _, p := range changed {
		abs = append(abs, filepath.Join(r.cfg.Home, p))
	}
	files, err := homefiles.Collect(r.cfg.Home, abs...)
	if err != nil {
		return err
	}
	for i := range files {
		if files[i].Path, err = filepath.Rel(s.Agent, files[i].Path); err != nil {
			return err
		}
	}
	deleted := make([]any, 0, len(gone))
	for _, p := range gone {
		rel, err := filepath.Rel(s.Agent, p)
		if err != nil {
			return err
		}
		deleted = append(deleted, rel)
	}
	if _, err := l.Call("node.agent_files", map[string]any{"agent": agent, "files": homefiles.Wire(files), "deleted": deleted},
		hlclient.Quick); err != nil {
		return err
	}
	r.sent[agent] = now
	return nil
}

// save writes the sessions to runner.json; called with r.mu held.
func (r *Runner) save() error {
	b, err := json.Marshal(r.sessions)
	if err != nil {
		return err
	}
	return atomicfile.Write(StateFile(r.cfg.Home), string(b))
}

// plain refuses a value git could read as an option, and a branch that is not a branch name.
func plain(values ...string) error {
	for _, v := range values {
		if v == "" || strings.HasPrefix(v, "-") {
			return rpc.BadRequestError(fmt.Sprintf("%q is not a path, a remote or a branch", v), "", rpc.InvalidParams)
		}
	}
	return nil
}

// sessionPid is the process of agent's session here, 0 for none: its exit ends the session's turn.
func (r *Runner) sessionPid(agent string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[agent].Pid
}

// linked is the link to the master, nil while it is not reached.
func (r *Runner) linked() *rpc.Link {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.link
}

// tellTurn marks agent's turn for the master (node.turn), sent by sendTurns. While the master is away nothing is sent: the link's next
// start tells every session's.
func (r *Runner) tellTurn(agent string) {
	r.mu.Lock()
	r.changed[agent] = true
	r.mu.Unlock()
	select {
	case r.turnSent <- struct{}{}:
	default:
	}
}

// sendTurns sends each marked agent's turn as the tracker holds it when it is sent, one at a time: a newer view is never overtaken by
// an older one, and changes that came meanwhile go as one.
func (r *Runner) sendTurns() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stop:
			return
		case <-r.turnSent:
		}
		r.mu.Lock()
		agents := r.changed
		r.changed = map[string]bool{}
		l := r.link
		r.mu.Unlock()
		if l == nil {
			continue
		}
		for agent := range agents {
			if _, err := l.Call("node.turn", map[string]any{"agent": agent, "view": r.turns.ViewOf(agent)}, hlclient.Quick); err != nil {
				r.cfg.Log("runner: %s: its turn did not reach the master: %v", agent, err)
			}
		}
	}
}

// sendHook is hooks.Recorder's Forward: the event to the master as the agent's own hook.record, its nudge back.
func (r *Runner) sendHook(agent, kind string, data map[string]any, requestID string, at int64) (string, error) {
	l := r.linked()
	if l == nil {
		return "", hooks.ErrUnreached
	}
	got, err := l.Call("node.forward", map[string]any{"agent": agent, "method": "hook.record", "client": "hl",
		"params": map[string]any{"agent": agent, "kind": kind, "data": data, "at": at, "request_id": requestID}}, hlclient.Quick)
	var refused *rpc.Error
	if err != nil && !errors.As(err, &refused) {
		return "", fmt.Errorf("%w: %v", hooks.ErrUnreached, err)
	}
	if err != nil {
		return "", err
	}
	r.tellTurn(agent) // a hook that moves no state can still change the view (doing, settled)
	nudge, _ := got["nudge"].(string)
	return nudge, nil
}

// drainSpool sends what was spooled while the master was away, in order, once it is reached again.
func (r *Runner) drainSpool() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stop:
			return
		case <-time.After(hooks.SpoolEvery):
		}
		if r.linked() == nil {
			continue
		}
		if _, err := r.hooks.Ingest(); err != nil && !errors.Is(err, hooks.ErrUnreached) {
			r.cfg.Log("runner: the hook spool: %v", err)
		}
	}
}

// transcriptWait bounds a transcript line's send: the tracker's watcher waits on it, and its Esc and doing reads with it.
const transcriptWait = 3 * time.Second

// transcriptLines sends what each line of agent's session transcript holds for chat to the master (node.transcript), which keeps it
// when chat_transcript_rows is on. A line read while the master is away is not kept: chat misses it, no count is wrong.
func (r *Runner) transcriptLines(agent, session string) func([]byte) {
	return func(line []byte) {
		at, rows := transcript.Rows(line)
		l := r.linked()
		if len(rows) == 0 || l == nil {
			return
		}
		if _, err := l.Call("node.transcript", map[string]any{"agent": agent, "session": session, "at": at, "rows": rows},
			transcriptWait); err != nil {
			r.cfg.Log("runner: %s: transcript rows did not reach the master: %v", agent, err)
		}
	}
}

// TokensEvery is how often a runner reads what its accounts' transcripts gained and sends it to the master.
const TokensEvery = 30 * time.Second

// TokensFile is where a runner keeps how far it has sent each account's transcripts.
func TokensFile(home string) string { return filepath.Join(home, "token-offsets.json") }

// sendTokens reads, every TokensEvery, the calls this machine's accounts' transcripts gained and sends them to the master
// (node.tokens), which counts them in its usage. A file's position moves only once the master has taken its calls, so a call is never
// lost; one sent twice counts once there (token_calls is keyed by account, session and request).
func (r *Runner) sendTokens() {
	defer r.wg.Done()
	u := &usage.Env{Home: r.cfg.Home}
	for {
		select {
		case <-r.stop:
			return
		case <-time.After(TokensEvery):
		}
		if err := r.SendTokens(u); err != nil {
			r.cfg.Log("runner: tokens: %v", err)
		}
	}
}

// SendTokens is one pass of sendTokens.
func (r *Runner) SendTokens(u *usage.Env) error {
	l := r.linked()
	if l == nil {
		return nil
	}
	kept := map[string]map[string]any{}
	if b, err := os.ReadFile(TokensFile(r.cfg.Home)); err == nil {
		json.Unmarshal(b, &kept)
	}
	var failed error
	for _, acct := range r.cfg.Accounts() {
		calls, moved, ok := u.NewCalls(acct, kept[acct], usage.BytesPerTick)
		if !ok {
			continue
		}
		if len(calls) > 0 {
			if _, err := l.Call("node.tokens", map[string]any{"account": acct, "calls": calls}, hlclient.Quick); err != nil {
				failed = errors.Join(failed, fmt.Errorf("%s: %w", acct, err)) // kept where it was, sent again next pass; the others go on
				continue
			}
		}
		kept[acct] = moved
		b, err := json.Marshal(kept)
		if err != nil {
			return err
		}
		if err := atomicfile.Write(TokensFile(r.cfg.Home), string(b)); err != nil {
			return err
		}
	}
	return failed
}

// MirrorEvery is how often a runner pulls what the board's copy on the master gained.
const MirrorEvery = time.Second

// mirror keeps this machine's copy of the master's board (boardcopy.Dir) up to date, if its person is the owner: each pull brings
// what it lacks and drops what the master dropped. Another person's runner is refused, and keeps no copy.
func (r *Runner) mirror() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stop:
			return
		case <-time.After(MirrorEvery):
		}
		if err := r.Pull(); err != nil {
			r.cfg.Log("runner: the board's copy: %v", err)
		}
	}
}

// Pull is one pass of mirror: it pulls until the copy here holds all of the master's. One pull at a time: two would write the same
// files' pieces.
func (r *Runner) Pull() error {
	r.pulling.Lock()
	defer r.pulling.Unlock()
	dir := boardcopy.Dir(r.cfg.Home)
	for {
		l := r.linked()
		if l == nil {
			return nil
		}
		have, err := boardcopy.List(dir)
		if err != nil {
			return err
		}
		parts, err := boardcopy.Parts(dir)
		if err != nil {
			return err
		}
		got, err := l.Call("node.copy.files", map[string]any{"have": have, "parts": parts}, hlclient.Quick)
		var refused *rpc.Error
		if errors.As(err, &refused) && refused.Data["gate"] == "owner" { // only the owner's machines keep one
			return os.RemoveAll(dir)
		}
		if err != nil {
			return err
		}
		b, err := json.Marshal(got)
		if err != nil {
			return err
		}
		var res struct {
			Send []boardcopy.Piece `json:"send"`
			Gone []string          `json:"gone"`
			More bool              `json:"more"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return err
		}
		if err := boardcopy.Apply(dir, res.Send, res.Gone); err != nil {
			return err
		}
		if !res.More { // nothing more is coming: a file still half here will not be
			return boardcopy.DropParts(dir, res.Send)
		}
	}
}
