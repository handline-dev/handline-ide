// Package cluster runs a master and runners on one machine: separate handlined processes with their own homes, sockets and loopback
// ports, each runner joined to the master as `hl device join` joins one and set to Run, for the tests of the cluster slices
// (docs/story4-design.md, slice 3) and `hl dev cluster`. Loopback, plain HTTP only.
package cluster

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/protected"
)

// JoinWait is how long a start waits for a daemon's socket, and for the master to list a joined runner.
const JoinWait = 10 * time.Second

// Opts are a cluster's choices.
type Opts struct {
	Bin      string   // the folder holding the handlined and hl to run
	Root     string   // the homes are Root/master, Root/runner1, ...
	Runners  int      // how many runners; 0 is 1
	BasePort int      // the master listens on BasePort, runner i on BasePort+i; 0 takes free ports
	Flags    []string // more handlined flags for every node
	Env      []string // more environment (KEY=value) for every node's processes
	// Detach starts each daemon in a session of its own, so it outlives this process (hl dev cluster up). Without it each daemon
	// stops when this process ends, however it ends (-exit-with-stdin).
	Detach bool
}

// Cluster is a master and its runners.
type Cluster struct {
	Master  *Node
	Runners []*Node
}

// Nodes are the master and then the runners.
func (c *Cluster) Nodes() []*Node { return append([]*Node{c.Master}, c.Runners...) }

// Node is one daemon: its home, socket and listener address.
type Node struct {
	Name, Home, Socket, Addr string
	o                        Opts
	cmd                      *exec.Cmd
	exited                   chan struct{} // closed once the daemon this process started has been waited on
	lifeline                 io.Closer     // the write end of a daemon's stdin under -exit-with-stdin
}

// Start starts the master and the runners and joins each runner. On an error everything it started is stopped.
func Start(o Opts) (c *Cluster, err error) {
	if o.Runners == 0 {
		o.Runners = 1
	}
	c = &Cluster{}
	defer func() {
		if err != nil {
			c.Stop() // its own error is not the one that broke the start
			c = nil
		}
	}()
	for i := 0; i <= o.Runners; i++ {
		name := "master"
		if i > 0 {
			name = "runner" + strconv.Itoa(i)
		}
		n, err := newNode(o, name, i)
		if err != nil {
			return c, err
		}
		if i == 0 {
			c.Master = n
		} else {
			c.Runners = append(c.Runners, n)
		}
		if err := n.start(); err != nil {
			return c, err
		}
	}
	for _, r := range c.Runners {
		if err := c.join(r); err != nil {
			return c, fmt.Errorf("%s: %w", r.Name, err)
		}
	}
	return c, nil
}

func newNode(o Opts, name string, i int) (*Node, error) {
	home := filepath.Join(o.Root, name)
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, err
	}
	sock := paths.SocketFile(home) // where hl with HANDLINE_HOME set looks
	if len(sock) > 100 {           // a unix socket path holds 104 bytes
		return nil, fmt.Errorf("%s is too long for a socket: give a shorter root", sock)
	}
	port := 0
	if o.BasePort != 0 {
		port = o.BasePort + i
	} else {
		var err error
		if port, err = freePort(); err != nil {
			return nil, err
		}
	}
	return &Node{Name: name, Home: home, Socket: sock, Addr: "127.0.0.1:" + strconv.Itoa(port), o: o}, nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// PortFree is whether addr can be listened on now.
func PortFree(addr string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// env is the environment of a process of this node: none of the caller's HANDLINE_ variables (its agent, home, tmux server and
// hl), but this node's home, its own tmux server, the hl beside it, the protected stores this process reads, and Opts.Env.
func (n *Node) env() []string {
	var out []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HANDLINE_") {
			out = append(out, kv)
		}
	}
	out = append(out, "HANDLINE_HOME="+n.Home, "HANDLINE_TMUX_SOCKET="+n.tmux(), "HANDLINE_BIN="+filepath.Join(n.o.Bin, "hl"),
		"HANDLINE_TEST_ROOT="+n.UserHome())
	return append(out, n.o.Env...)
}

// UserHome is the user home of the node's processes, under the cluster's root: their protected stores stay there, never in the
// real ~/.config/handline/protected another run's check compares.
func (n *Node) UserHome() string { return filepath.Join(n.o.Root, "user") }

// store is the node's protected store.
func (n *Node) store() *protected.Store {
	return &protected.Store{Root: protected.RootUnder(n.UserHome(), n.Home)}
}

// tmux is the tmux server (tmux -L) of the node's terminals.
func (n *Node) tmux() string {
	sum := sha256.Sum256([]byte(paths.Resolved(n.Home)))
	return fmt.Sprintf("handline-cluster-%x", sum[:4])
}

func (n *Node) pidFile() string { return filepath.Join(n.Home, "handlined.pid") }

// Log is the file the daemon writes its output to.
func (n *Node) Log() string { return filepath.Join(n.Home, "handlined.log") }

func (n *Node) start() error {
	if !PortFree(n.Addr) {
		return fmt.Errorf("%s is in use", n.Addr)
	}
	log, err := os.OpenFile(n.Log(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	args := append([]string{"-home", n.Home, "-socket", n.Socket, "-listen", n.Addr}, n.o.Flags...)
	cmd := exec.Command(filepath.Join(n.o.Bin, "handlined"), args...)
	cmd.Env, cmd.Stdout, cmd.Stderr = n.env(), log, log
	var lifeline io.Closer
	if n.o.Detach {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // a group of its own, which end signals
		cmd.Args = append(cmd.Args, "-exit-with-stdin")
		w, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		lifeline = w
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	n.cmd, n.lifeline, n.exited = cmd, lifeline, make(chan struct{})
	go func(done chan struct{}) { cmd.Wait(); close(done) }(n.exited) // reaped at once: a zombie still answers kill -0
	if err := os.WriteFile(n.pidFile(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		return err
	}
	return n.waitUp()
}

// waitUp waits for the daemon to answer on its socket.
func (n *Node) waitUp() error {
	end := time.Now().Add(JoinWait)
	for {
		if _, err := n.Call("hello", nil); err == nil {
			return nil
		} else if time.Now().After(end) {
			return fmt.Errorf("%s did not come up: %v (log %s)", n.Name, err, n.Log())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Client is a client of this node's daemon as its founder.
func (n *Node) Client() *hlclient.Client {
	key, _ := n.store().Get("founder-keys", "key")
	sock := n.Socket
	return &hlclient.Client{Home: n.Home, Key: strings.TrimSpace(key),
		Dial: func() (net.Conn, error) { return net.DialTimeout("unix", sock, 5*time.Second) }}
}

// Call calls method on this node's daemon as its founder.
func (n *Node) Call(method string, params map[string]any) (map[string]any, error) {
	return n.Client().Call(method, params)
}

// Hl runs the hl beside handlined on this node's home, as its founder at a terminal, and returns what it printed.
func (n *Node) Hl(args ...string) (string, error) {
	cmd := exec.Command(filepath.Join(n.o.Bin, "hl"), args...)
	cmd.Env = n.env()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// pid is the daemon's process id, 0 when none runs. One from the pid file counts only while that process is this node's daemon (its
// command line names the home), not another that took the pid since.
func (n *Node) pid() int {
	if n.cmd != nil {
		select {
		case <-n.exited:
			return 0
		default:
			return n.cmd.Process.Pid
		}
	}
	b, err := os.ReadFile(n.pidFile())
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid <= 0 || !n.isOurs(pid) {
		return 0
	}
	return pid
}

func (n *Node) isOurs(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	return err == nil && strings.Contains(string(out), "handlined -home "+n.Home+" ")
}

// Running is whether the daemon is up.
func (n *Node) Running() bool { return n.pid() != 0 }

// Kill ends the daemon at once (SIGKILL), as a machine that goes away does.
func (n *Node) Kill() error { return n.end(syscall.SIGKILL) }

// Stop ends the daemon as a quit does (SIGTERM) and waits for it.
func (n *Node) Stop() error { return n.end(syscall.SIGTERM) }

// end signals the daemon's process group (its own, so what it started goes too) and waits for it to go, killing it past JoinWait.
func (n *Node) end(sig syscall.Signal) error {
	pid := n.pid()
	if pid != 0 {
		if err := syscall.Kill(-pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		if !n.gone(pid) {
			syscall.Kill(-pid, syscall.SIGKILL)
			if !n.gone(pid) {
				return fmt.Errorf("%s (pid %d) did not stop", n.Name, pid)
			}
		}
	}
	if n.lifeline != nil {
		n.lifeline.Close()
	}
	n.cmd, n.lifeline = nil, nil
	os.Remove(n.pidFile())
	exec.Command("tmux", "-L", n.tmux(), "kill-server").Run() // none runs unless a terminal was opened
	return nil
}

// gone is whether pid ended within JoinWait.
func (n *Node) gone(pid int) bool {
	if n.cmd != nil {
		select {
		case <-n.exited:
			return true
		case <-time.After(JoinWait):
			return false
		}
	}
	for end := time.Now().Add(JoinWait); n.isOurs(pid); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(end) {
			return false
		}
	}
	return true
}

// Restart starts the daemon again on the same home, socket and port.
func (n *Node) Restart() error {
	if err := n.Stop(); err != nil {
		return err
	}
	return n.start()
}

// Stop stops every node it started; the homes stay.
func (c *Cluster) Stop() error {
	var errs []error
	for _, n := range c.Nodes() {
		if n != nil {
			errs = append(errs, n.Stop())
		}
	}
	return errors.Join(errs...)
}

// join joins r to the master: a pairing link from the master, `hl device join` on r, Run set by the device's person (the master's
// founder, who made the code), and r restarted, which sets its own board aside. It waits for the master to list r.
func (c *Cluster) join(r *Node) error {
	got, err := c.Master.Call("device.pair", map[string]any{"name": r.Name, "request_id": "cluster-pair-" + r.Name})
	if err != nil {
		return fmt.Errorf("device.pair: %w", err)
	}
	link, _ := got["uri"].(string)
	if out, err := r.Hl("device", "join", link); err != nil {
		return fmt.Errorf("hl device join: %v: %s", err, out)
	}
	m, ok := joined.OfStore(r.store())
	if !ok {
		return errors.New("hl device join kept no credential")
	}
	if _, err := c.Master.Call("device.set", map[string]any{"device": m.Device, "lending": "run",
		"request_id": "cluster-run-" + r.Name}); err != nil {
		return fmt.Errorf("device.set run: %w", err)
	}
	if err := r.Restart(); err != nil {
		return err
	}
	node := joined.NodeIDOf(r.Home)
	for end := time.Now().Add(JoinWait); ; time.Sleep(100 * time.Millisecond) {
		if c.Master.Lists(node) {
			return nil
		}
		if time.Now().After(end) {
			return fmt.Errorf("the master does not list node %s", node)
		}
	}
}

// Lists is whether node.status on this node lists the node id.
func (n *Node) Lists(node string) bool {
	got, err := n.Call("node.status", nil)
	if err != nil {
		return false
	}
	list, _ := got["nodes"].([]any)
	for _, v := range list {
		if row, _ := v.(map[string]any); row["id"] == node {
			return true
		}
	}
	return false
}

// Open is the cluster whose homes are under root, as Start left them: the nodes found there, running or not.
func Open(o Opts) *Cluster {
	c := &Cluster{}
	for i := 0; ; i++ {
		name := "master"
		if i > 0 {
			name = "runner" + strconv.Itoa(i)
		}
		home := filepath.Join(o.Root, name)
		if _, err := os.Stat(home); err != nil {
			break
		}
		n := &Node{Name: name, Home: home, Socket: paths.SocketFile(home), o: o}
		if o.BasePort != 0 {
			n.Addr = "127.0.0.1:" + strconv.Itoa(o.BasePort+i)
		}
		if i == 0 {
			c.Master = n
		} else {
			c.Runners = append(c.Runners, n)
		}
	}
	return c
}
