// Package hlclient is the hl command's way to the daemon (handline/infra/daemonclient.py and handline/cli/client.py): one
// JSON-RPC call on a fresh connection, hello first, as the founder when this terminal holds the founder key and as the
// agent when it runs in an agent's session (the daemon tells which from the process, not from anything said here).
package hlclient

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/protected"
	"github.com/handline/handline/go/protocol/pairsock"
)

const (
	Name = "hl"
	// Timeout is the deadline of a call that starts, wakes or stops a session: it can take up to ~450s, and shorter would read one still
	// running as a daemon that is down.
	Timeout = 480 * time.Second
	// Quick is the deadline of every other call: a read or a write that only touches the daemon's own store.
	Quick     = 60 * time.Second
	founderNS = "founder-keys"
)

// DaemonDown is a daemon that is not there. Nothing was done.
type DaemonDown struct{ Why, Log string }

func (e *DaemonDown) Error() string {
	return fmt.Sprintf("the daemon is not running: %s (log: %s); nothing was done", e.Why, e.Log)
}

// RPCError is the daemon's typed refusal.
type RPCError struct {
	Code    string
	Message string
	Data    map[string]any
}

func (e *RPCError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// Client reaches one home's daemon. Dial is the socket; tests give their own.
type Client struct {
	Home string
	Key  string
	Dial func() (net.Conn, error)
	// As is the name hello gives, Name when empty; Deadline a call's, DeadlineOf the method when zero.
	As       string
	Deadline time.Duration
	// note is what the next hello carries for the daemon to record (NoteVerbChain); it goes once.
	note map[string]any
}

// slow are the methods that start, wake or stop a session (or the daemon) before they answer; the rest are Quick.
var slow = map[string]bool{
	"session.start": true, "session.wake": true, "session.restart": true, "session.stop": true, "session.park": true,
	"agent.create": true, "agent.retire": true, "agent.move": true, "agent.switch_now": true, "agent.unstall": true, "agent.force_start": true,
	"workspace.create": true, "workspace.delete": true,
	"account.wake": true, "say": true, "chat.send": true, "chat.send_now": true,
}

// DeadlineOf is how long a call to method may take: Timeout for the ones that start, wake or stop a session, Quick for the rest.
func DeadlineOf(method string) time.Duration {
	if slow[method] {
		return Timeout
	}
	return Quick
}

// New is the client for this process's home, holding the founder key when this terminal may read it.
func New() *Client {
	home := paths.Home()
	key, _ := protected.New(home).Get(founderNS, "key")
	return &Client{
		Home: home,
		Key:  strings.TrimSpace(key),
		Dial: Dialer(home, 5*time.Second),
	}
}

// Dialer connects to home's daemon socket within timeout. Under a test root, HANDLINE_DAEMON_FD names an inherited
// unix socket (pairsock.Listener.Control) that each dial sends its connection over instead, so the rig binds no file.
func Dialer(home string, timeout time.Duration) func() (net.Conn, error) {
	if over := rigConn(); over != nil {
		return pairsock.DialOver(over)
	}
	sock := paths.SocketFile(home)
	return func() (net.Conn, error) { return net.DialTimeout("unix", sock, timeout) }
}

// rigConn is the inherited HANDLINE_DAEMON_FD socket, taken once a process: a second os.File on the fd would close it
// when collected.
var rigConn = sync.OnceValue(takeRigConn)

func takeRigConn() *net.UnixConn {
	root, _ := paths.Getenv("TEST_ROOT")
	v, _ := paths.Getenv("DAEMON_FD")
	fd, err := strconv.Atoi(v)
	if root == "" || err != nil {
		return nil
	}
	f := os.NewFile(uintptr(fd), "daemon-fd")
	defer f.Close()
	c, err := net.FileConn(f)
	uc, _ := c.(*net.UnixConn)
	if err != nil {
		return nil
	}
	return uc
}

// Connect is a connected socket. Nothing here starts a daemon: the Handline app is the only thing that runs the live home's.
func (c *Client) Connect() (net.Conn, error) {
	conn, err := c.Dial()
	if err == nil {
		return conn, nil
	}
	return nil, &DaemonDown{"open the Handline app to start it", filepath.Join(c.Home, "logs", "daemon.err.log")}
}

// The ids of a call's two messages, and of an error the daemon sends about the connection itself (a line too long to read, say).
const (
	connID  = 0
	helloID = 1
	callID  = 2
)

type reply struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	} `json:"error"`
}

func (r *reply) err() error {
	if r.Error == nil {
		return nil
	}
	code, _ := r.Error.Data["code"].(string)
	if code == "" {
		code = "internal"
	}
	return &RPCError{Code: code, Message: r.Error.Message, Data: r.Error.Data}
}

// Call is the result of method; Call("hello", nil) is the hello's own. A *DaemonDown when the daemon is not there, an *RPCError when it
// refuses.
func (c *Client) Call(method string, params map[string]any) (map[string]any, error) {
	conn, err := c.Connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, as := c.Deadline, c.As
	if deadline == 0 {
		deadline = DeadlineOf(method)
	}
	if as == "" {
		as = Name
	}
	_ = conn.SetDeadline(time.Now().Add(deadline))
	hello := map[string]any{"client": as, "protocol": 1}
	if c.Key != "" {
		hello["key"] = c.Key
	}
	if wd, err := os.Getwd(); err == nil {
		hello["cwd"] = wd // where a person's agent names resolve
	}
	if c.note != nil {
		hello["note"], c.note = c.note, nil
	}
	if params == nil {
		params = map[string]any{}
	}
	msgs, want := []map[string]any{{"jsonrpc": "2.0", "id": helloID, "method": "hello", "params": hello}}, helloID
	if method != "hello" {
		msgs, want = append(msgs, map[string]any{"jsonrpc": "2.0", "id": callID, "method": method, "params": params}), callID
	}
	var out []byte
	for i, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", i+1, err)
		}
		out = append(append(out, b...), '\n')
	}
	// Written beside the reading: the daemon answers the hello before it reads the request, and a request and a hello reply each
	// bigger than the socket's buffer would otherwise leave both sides writing until the deadline.
	wrote := make(chan error, 1)
	go func() { _, err := conn.Write(out); wrote <- err }()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64<<10), 64<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r reply
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("the daemon sent what is not JSON: %w", err)
		}
		// An error on the connection, the hello or the call ends the call: no answer follows it.
		if err := r.err(); err != nil && (r.ID == connID || r.ID == helloID || r.ID == callID) {
			return nil, err
		}
		if r.ID == want {
			var res map[string]any
			if err := json.Unmarshal(r.Result, &res); err != nil {
				return nil, fmt.Errorf("the daemon's result is not an object: %w", err)
			}
			return res, nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if err := <-wrote; err != nil {
		return nil, err
	}
	return nil, errors.New("the daemon closed the connection before it replied")
}

// Write is a write under a fresh request_id: (true, what it said) or (false, why it was refused or not made).
func (c *Client) Write(method string, params map[string]any) (bool, string) {
	ok, said, _ := c.Written(method, params)
	return ok, said
}

// Written is Write, and the whole result beside the words (nil when refused or not made).
func (c *Client) Written(method string, params map[string]any) (bool, string, map[string]any) {
	p := map[string]any{"request_id": newID()}
	for k, v := range params {
		p[k] = v
	}
	got, err := c.Call(method, p)
	var down *DaemonDown
	var refused *RPCError
	switch {
	case errors.As(err, &down):
		return false, "daemon not running, nothing done: " + down.Error(), nil
	case errors.As(err, &refused):
		return false, refused.Error(), nil
	case err != nil:
		return false, "the daemon did not answer, so it is not known whether this was done: " + err.Error(), nil
	}
	said, _ := got["said"].(string)
	return true, said, got
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
