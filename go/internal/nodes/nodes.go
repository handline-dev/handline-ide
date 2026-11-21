// Package nodes is the master's side of its runners (docs/story4-design.md, slice 4): the live link of each runner, which the runner
// dials and keeps open, the calls the master drives a runner with over it, and node.forward, by which a runner's agents reach the master.
package nodes

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/protocol/rpc"
)

// CallWait is how long the master waits for a runner's answer; ReadWait for a read the tick and the waits on a screen make many
// times over; GitWait for a clone, a fetch or a push.
const (
	CallWait = 60 * time.Second
	ReadWait = 5 * time.Second
	GitWait  = 10 * time.Minute
)

// Registry is the runners linked to this master now, by node id. Home is the master's; Launch builds a launch a runner's hl launch asks
// for (nil starts none).
type Registry struct {
	DB     *sql.DB
	Home   string
	Launch LaunchFunc
	Log    func(format string, a ...any)
	// OnLink, when set, is told each time a node links (it joined, or came back), off the hello that linked it.
	OnLink func(node string)
	mu     sync.Mutex
	links  map[string]*rpc.Conn
	homes  map[*rpc.Conn]string // each link's runner's home, once asked

	refOnce sync.Once
	refRE   *regexp.Regexp // a path under the attachments folder
}

func New(db *sql.DB, home string) *Registry {
	return &Registry{DB: db, Home: home, links: map[string]*rpc.Conn{}, homes: map[*rpc.Conn]string{}}
}

// Link keeps c as node's link, in place of any before it, until c ends.
func (r *Registry) Link(node string, c *rpc.Conn) {
	r.mu.Lock()
	r.links[node] = c
	r.mu.Unlock()
	c.OnClose(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.links[node] == c {
			delete(r.links, node)
		}
		delete(r.homes, c)
	})
	if r.OnLink != nil {
		go r.OnLink(node)
	}
}

// Linked is the nodes linked now.
func (r *Registry) Linked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.links))
	for node := range r.links {
		out = append(out, node)
	}
	return out
}

// NodeOf is the node whose link c is; "" for any other connection.
func (r *Registry) NodeOf(c *rpc.Conn) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for node, l := range r.links {
		if l == c {
			return node
		}
	}
	return ""
}

// Runner is the master's hand on node: the calls that drive it. An error when node has no link now.
func (r *Registry) Runner(node string) (*Runner, error) {
	r.mu.Lock()
	c := r.links[node]
	r.mu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("node %s is not linked to this master", node)
	}
	return &Runner{reg: r, node: node, c: c}, nil
}

// Runner drives one runner over its link.
type Runner struct {
	reg  *Registry
	node string
	c    *rpc.Conn
}

// Open opens a terminal on the runner in folder, typing command at its shell, as agent's session, and keeps that the session is there.
func (r *Runner) Open(agent, folder, command string) (string, error) {
	got, err := r.c.Call("runner.terminal.open", map[string]any{"agent": agent, "folder": folder, "command": command}, CallWait)
	if err != nil {
		return "", err
	}
	handle, _ := got["handle"].(string)
	if handle == "" {
		return "", errors.New("the runner opened no terminal")
	}
	if _, err := r.reg.DB.Exec(`INSERT INTO node_sessions(agent, node, handle, opened_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(agent) DO UPDATE SET node = excluded.node, handle = excluded.handle, opened_at = excluded.opened_at`,
		agent, r.node, handle, time.Now().Unix()); err != nil {
		r.c.Call("runner.stop", map[string]any{"agent": agent, "handle": handle}, CallWait) // not kept, so not left open
		return "", err
	}
	return handle, nil
}

// Keys types keys into the terminal, then Enter when enter.
func (r *Runner) Keys(handle, keys string, enter bool) error {
	_, err := r.c.Call("runner.keys", map[string]any{"handle": handle, "keys": keys, "enter": enter}, CallWait)
	return err
}

// Screen is the bottom rows of the terminal's screen.
func (r *Runner) Screen(handle string, rows int) ([]string, error) {
	got, err := r.c.Call("runner.screen", map[string]any{"handle": handle, "rows": rows}, ReadWait)
	if err != nil {
		return nil, err
	}
	var out []string
	list, _ := got["rows"].([]any)
	for _, v := range list {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out, nil
}

// Stop closes agent's session on the runner and forgets it.
func (r *Runner) Stop(agent string) error {
	var handle string
	if err := r.reg.DB.QueryRow("SELECT handle FROM node_sessions WHERE agent = ? AND node = ?", agent, r.node).Scan(&handle); err != nil {
		return fmt.Errorf("%s has no session on node %s: %w", agent, r.node, err)
	}
	var gone *rpc.Error
	if _, err := r.c.Call("runner.stop", map[string]any{"agent": agent, "handle": handle}, CallWait); err != nil &&
		!(errors.As(err, &gone) && gone.Code == "not_found") { // a session the runner no longer has is stopped
		return err
	}
	_, err := r.reg.DB.Exec("DELETE FROM node_sessions WHERE agent = ? AND node = ?", agent, r.node)
	return err
}

// MakeWorktree makes a worktree of remote at path on branch, from base when the branch is new.
func (r *Runner) MakeWorktree(remote, path, branch, base string) error {
	_, err := r.c.Call("runner.worktree.make", map[string]any{"remote": remote, "path": path, "branch": branch, "base": base}, GitWait)
	return err
}

// SyncProject has the runner clone remote, or fetch it when it has it.
func (r *Runner) SyncProject(remote string) error {
	_, err := r.c.Call("runner.project.sync", map[string]any{"remote": remote}, GitWait)
	return err
}

// Push pushes branch from the worktree at path.
func (r *Runner) Push(path, branch string) error {
	_, err := r.c.Call("runner.push", map[string]any{"path": path, "branch": branch}, GitWait)
	return err
}

// Terminals is the runners' terminals as term.Machines drives them, by node and the runner's own handle.
func (r *Registry) Terminals() term.Remote { return remote{r} }

type remote struct{ r *Registry }

func (m remote) Nodes() []string {
	m.r.mu.Lock()
	defer m.r.mu.Unlock()
	out := make([]string, 0, len(m.r.links))
	for node := range m.r.links {
		out = append(out, node)
	}
	return out
}

func (m remote) Open(node, agent, cwd, command string) (string, error) {
	run, err := m.r.Runner(node)
	if err != nil {
		return "", err
	}
	if command, err = m.r.forRunner(node, command); err != nil {
		return "", err
	}
	return run.Open(agent, cwd, command)
}

func (m remote) Keys(node, handle, keys string, enter bool) error {
	run, err := m.r.Runner(node)
	if err != nil {
		return err
	}
	if keys, err = m.r.forRunner(node, keys); err != nil {
		return err
	}
	return run.Keys(handle, keys, enter)
}

func (m remote) Screen(node, handle string, rows int) ([]string, error) {
	run, err := m.r.Runner(node)
	if err != nil {
		return nil, err
	}
	return run.Screen(handle, rows)
}

// Close stops the session the master opened at handle.
func (m remote) Close(node, handle string) error {
	run, err := m.r.Runner(node)
	if err != nil {
		return err
	}
	var agent string
	if err := m.r.DB.QueryRow("SELECT agent FROM node_sessions WHERE node = ? AND handle = ?", node, handle).Scan(&agent); err != nil {
		return fmt.Errorf("no session %s on node %s: %w", handle, node, err)
	}
	return run.Stop(agent)
}

func (m remote) Respawn(node, handle string) error {
	run, err := m.r.Runner(node)
	if err != nil {
		return err
	}
	return run.Respawn(handle)
}

func (m remote) Terminals(node string) ([]string, error) {
	run, err := m.r.Runner(node)
	if err != nil {
		return nil, err
	}
	return run.Terminals()
}

// Respawn ends what runs in the terminal and leaves a fresh shell there, which the runner records as the session in its place.
func (r *Runner) Respawn(handle string) error {
	_, err := r.c.Call("runner.respawn", map[string]any{"handle": handle}, CallWait)
	return err
}

// Terminals is the handles of the terminals the runner's host has open.
func (r *Runner) Terminals() ([]string, error) {
	got, err := r.c.Call("runner.terminals", nil, ReadWait)
	if err != nil {
		return nil, err
	}
	list, ok := got["handles"].([]any)
	if !ok {
		return nil, errors.New("the runner listed no handles")
	}
	out := []string{}
	for _, v := range list {
		if s, _ := v.(string); s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

// notForwarded are what a runner may not forward: its own handshake, the methods of the link itself, and a subscription, whose
// events have no connection to go to.
func notForwarded(method string) bool {
	return method == "hello" || strings.HasPrefix(method, "node.") || method == "subscribe" || method == "unsubscribe" || strings.HasPrefix(method, "runner.")
}

// Register adds node.forward to the network server: a runner's call for one of its agents, run on local as that agent (beside the
// link's loop, which must stay free to read the runner's answers), only when the connection is a runner's link and the agent's session
// is on that runner.
// local is the master's local server, asked at the call: on that server itself node.forward is refused, as no connection there is a link.
func Register(s *rpc.Server, local func() *rpc.Server, r *Registry) {
	s.Register("node.forward", rpc.Method{Waits: true, Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		node := r.NodeOf(c.Root())
		if node == "" {
			return nil, rpc.Refused("node.forward is a runner's own call, on its link", "node", "")
		}
		agent, _ := p["agent"].(string)
		method, _ := p["method"].(string)
		if agent == "" || method == "" {
			return nil, rpc.BadRequestError("agent and method are strings", "method", rpc.InvalidParams)
		}
		if notForwarded(method) {
			return nil, rpc.Refused(method+" is not forwarded", "method", "")
		}
		var on string
		if err := r.DB.QueryRow("SELECT node FROM node_sessions WHERE agent = ?", agent).Scan(&on); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if on != node {
			return nil, rpc.Refused(agent+" has no session on this runner", "not_on_node", "")
		}
		params, _ := p["params"].(map[string]any)
		client, _ := p["client"].(string)
		cwd, _ := p["cwd"].(string)
		home, err := r.runnerHome(node) // before the call has any effect
		if err != nil {
			return nil, err
		}
		l := local()
		res, err := l.Dispatch(l.AgentCaller(agent), client, cwd, method, params)
		if err != nil {
			return res, err
		}
		return r.answerFor(node, home, res), nil
	}})
	s.Register("node.launch", rpc.Method{Waits: true, Handler: r.launch})
	s.Register("node.agent_files", rpc.Method{Waits: true, Handler: r.agentFiles})
}
