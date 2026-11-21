// Package runner is a daemon in runner mode (docs/story4-design.md, slice 4): a joined machine's daemon, which keeps no board. It
// keeps one link open to the master, with its device token, and serves the calls the master drives it with over that link; its local
// socket forwards its agents' calls to the master, each proven the agent's by process descent from a session it opened.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/atomicfile"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/peer"
	"github.com/handline/handline/go/protocol/authority"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/protocol/sessiontoken"
)

// Terminals is what the runner asks of its terminal host.
type Terminals interface {
	CreateTerminal(command, cwd, title, agent string) (handle, why string)
	CloseTerminal(handle string, tab bool) (bool, string)
	Send(handle, text string, enter bool) (bool, string)
	ReadScreen(handle string, limit int) *term.Screen
	PanePid(handle string) int
}

// Tokens is how the runner records and forgets an agent's session: the proof its local socket checks.
type Tokens interface {
	Record(agent string, pid int) bool
	Forget(agent string, pid int) bool
}

// Config is what a runner runs with. Term, Auth and Tokens default to the home's tmux, protected store and session tokens.
type Config struct {
	Home, Socket, Sha string
	Master            joined.Master
	Term              Terminals
	Auth              authority.Authority
	Tokens            Tokens
	Log               func(format string, args ...any)
}

// Runner is one runner daemon.
type Runner struct {
	cfg   Config
	local *rpc.Server
	ln    net.Listener

	mu       sync.Mutex
	link     *rpc.Link // nil while the master is not reached
	stopped  bool      // Close ran: no link is kept from here
	sessions map[string]session
	stop     chan struct{}
	once     sync.Once
	wg       sync.WaitGroup
}

// session is an agent's terminal the runner opened, kept in runner.json.
type session struct {
	Handle string `json:"handle"`
	Pid    int    `json:"pid"`
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
	r := &Runner{cfg: cfg, sessions: map[string]session{}, stop: make(chan struct{})}
	if b, err := os.ReadFile(StateFile(cfg.Home)); err == nil {
		if err := json.Unmarshal(b, &r.sessions); err != nil {
			return nil, fmt.Errorf("%s: %w", StateFile(cfg.Home), err)
		}
	}
	r.local = rpc.NewServer(cfg.Auth, cfg.Sha, 0, "local", cfg.Log)
	r.local.Fallback = r.forward
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
	r.wg.Add(1)
	go r.keepLinked()
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
	r.mu.Lock()
	l := r.link
	r.mu.Unlock()
	if l == nil {
		return nil, rpc.Refused("the master cannot be reached now", "master", "")
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
	l := rpc.NewLink(rpc.NewWSTransport(ws), r.serve)
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
	r.mu.Unlock()
	r.cfg.Log("runner: linked to the master at %s", r.cfg.Master.Addr)
	err = <-ended
	r.mu.Lock()
	r.link = nil
	r.mu.Unlock()
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
		s := r.cfg.Term.ReadScreen(str("handle"), rows)
		if s == nil {
			return nil, rpc.NotFoundError("no screen for " + str("handle"))
		}
		return map[string]any{"rows": append([]string{}, s.Tail...)}, nil
	case "runner.stop":
		return r.close(str("agent"), str("handle"))
	case "runner.worktree.make":
		return gitWorktree(r.cfg.Home, str("remote"), str("path"), str("branch"), str("base"))
	case "runner.push":
		if err := plain(str("path"), str("branch")); err != nil {
			return nil, err
		}
		if out, err := runGit("-C", str("path"), "push", "origin", "--", str("branch")); err != nil {
			return nil, fmt.Errorf("git push: %v: %s", err, out)
		}
		return map[string]any{"accepted": true}, nil
	}
	return nil, rpc.BadRequestError("no such method: "+method, "method", rpc.NoSuchMethod)
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
	r.sessions[agent] = session{Handle: handle, Pid: pid}
	err := r.save()
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
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
	r.cfg.Tokens.Forget(agent, s.Pid)
	r.mu.Lock()
	delete(r.sessions, agent)
	err := r.save()
	r.mu.Unlock()
	return map[string]any{"accepted": true}, err
}

// save writes the sessions to runner.json; called with r.mu held.
func (r *Runner) save() error {
	b, err := json.Marshal(r.sessions)
	if err != nil {
		return err
	}
	return atomicfile.Write(StateFile(r.cfg.Home), string(b))
}

// gitWorktree makes a worktree of remote at path on branch: the remote is cloned once under the home's runner-repos, fetched each
// time, and the branch made from base when the remote has no such branch.
func gitWorktree(home, remote, path, branch, base string) (any, error) {
	if base == "" {
		base = "main"
	}
	if remote == "" {
		return nil, rpc.BadRequestError("remote is a string", "remote", rpc.InvalidParams)
	}
	if err := plain(remote, path, branch, base); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(remote))
	repo := filepath.Join(home, "runner-repos", hex.EncodeToString(sum[:8]))
	git := func(args ...string) error {
		if out, err := runGit(args...); err != nil {
			return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return nil
	}
	if _, err := os.Stat(repo); errors.Is(err, os.ErrNotExist) {
		if err := git("clone", "--no-checkout", "--", remote, repo); err != nil {
			return nil, err
		}
	} else if err := git("-C", repo, "fetch", "origin"); err != nil {
		return nil, err
	}
	from := "origin/" + branch
	if git("-C", repo, "rev-parse", "--verify", "--quiet", "--end-of-options", from) != nil {
		from = "origin/" + base
	}
	if err := git("-C", repo, "worktree", "add", "-B", branch, "--", path, from); err != nil {
		return nil, err
	}
	return map[string]any{"accepted": true, "path": path}, nil
}

// gitTimeout bounds one git command a master's call runs.
const gitTimeout = 10 * time.Minute

func runGit(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "git", args...).CombinedOutput()
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
