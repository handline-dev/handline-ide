// Package claudecli runs a Claude CLI in stream-json mode (the Agent SDK's wire protocol: one JSON object per line on stdin and stdout)
// for the usage poll: it asks the account's plan limits with the get_usage control request and, to renew a token, takes one turn.
package claudecli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/handline/handline/go/kit/paths"
)

// Handlers are the session's callbacks.
type Handlers struct {
	// OnEvent is told every frame the CLI writes.
	OnEvent func(ev map[string]any)
}

// Launch is how the CLI is started. ConfigDir is the account's CLAUDE_CONFIG_DIR; the child sees nothing of the daemon's
// environment beyond PATH, HOME, USER, LOGNAME (the CLI finds the account's login by user name), TMPDIR and LANG.
type Launch struct {
	Path      string
	Args      []string // extra arguments, after the protocol's own
	Cwd       string
	ConfigDir string
	Model     string
	Env       []string // NAME=value pairs the daemon sets on purpose, after the allowlisted ones
}

func (l Launch) args() []string {
	a := []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-prompt-tool", "stdio"}
	if l.Model != "" {
		a = append(a, "--model", l.Model)
	}
	return append(a, l.Args...)
}

func (l Launch) env() []string {
	var env []string
	for _, k := range []string{"PATH", "HOME", "USER", "LOGNAME", "TMPDIR", "LANG"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	if l.ConfigDir != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+l.ConfigDir)
	}
	return append(env, l.Env...)
}

// Session is one running CLI.
type Session struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	h   Handlers

	wmu sync.Mutex // one writer at a time

	mu      sync.Mutex
	nextReq int
	waiting map[string]chan map[string]any // request_id -> the control response
	closed  bool
	closeMu sync.Mutex // one Close at a time; a repeat proves the group again
	exitErr error
	done    chan struct{}
}

// Open starts the CLI and completes the initialize handshake; it fails closed when the CLI does not answer within timeout.
func Open(ctx context.Context, l Launch, h Handlers, timeout time.Duration) (*Session, error) {
	cmd := exec.Command(l.Path, l.args()...)
	cmd.Dir, cmd.Env = l.Cwd, l.env()
	if cmd.Dir == "" { // never the daemon's own folder
		cmd.Dir = paths.QuietDir()
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // the CLI and what it starts are one group, so close can prove they are gone
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}
	s := &Session{cmd: cmd, in: in, h: h, waiting: map[string]chan map[string]any{}, done: make(chan struct{})}
	go s.read(out)
	ictx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := s.control(ictx, map[string]any{"subtype": "initialize"}); err != nil {
		s.Close()
		return nil, fmt.Errorf("claude did not complete initialize: %w", err)
	}
	return s, nil
}

func (s *Session) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, err = s.in.Write(append(b, '\n'))
	return err
}

// control sends a control request and waits for its response.
func (s *Session) control(ctx context.Context, req map[string]any) (map[string]any, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("session closed")
	}
	s.nextReq++
	id := fmt.Sprintf("req_%d", s.nextReq)
	ch := make(chan map[string]any, 1)
	s.waiting[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.waiting, id); s.mu.Unlock() }()
	if err := s.write(map[string]any{"type": "control_request", "request_id": id, "request": req}); err != nil {
		return nil, err
	}
	select {
	case resp := <-ch:
		if resp["subtype"] == "error" {
			return nil, fmt.Errorf("claude refused %v: %v", req["subtype"], resp["error"])
		}
		r, _ := resp["response"].(map[string]any)
		return r, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, s.exitError()
	}
}

func (s *Session) exitError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exitErr != nil {
		return s.exitErr
	}
	return errors.New("claude exited")
}

// GetUsage asks the CLI for the account's plan rate limits (control request get_usage): the CLI makes the request with its own login,
// so the daemon reads no credential. It returns the response as the CLI gave it; no model turn is taken.
func (s *Session) GetUsage(ctx context.Context) (map[string]any, error) {
	r, err := s.control(ctx, map[string]any{"subtype": "get_usage"})
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("claude answered get_usage with nothing")
	}
	return r, nil
}

// Send writes text as a user message, which starts a turn; the turn's frames come to OnEvent.
func (s *Session) Send(text string) error {
	return s.write(map[string]any{"type": "user", "parent_tool_use_id": nil, "message": map[string]any{"role": "user", "content": text}})
}

// CloseGrace is how long Close waits for the CLI to go after its stdin ends, before it kills the process group.
var CloseGrace = 3 * time.Second

// Close ends stdin, which ends the CLI, kills the CLI's process group if it does not go, and returns only once no process of the
// group is left. An error means one is still alive.
func (s *Session) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.in.Close()
	pgid := s.cmd.Process.Pid // Setpgid made the CLI the leader of its own group
	select {
	case <-s.done:
	case <-time.After(CloseGrace):
		syscall.Kill(-pgid, syscall.SIGKILL)
		<-s.done
	}
	// Every call proves the group again, so a Close that failed once is not reported as done by the next.
	return proveGone(pgid)
}

// proveGone is groupGone; a test replaces it.
var proveGone = groupGone

// groupGone proves the process group is empty, killing what is left of it: the CLI's children outlive it when it dies without
// reaping them.
func groupGone(pgid int) error {
	for i := 0; i < 50; i++ {
		if err := syscall.Kill(-pgid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		syscall.Kill(-pgid, syscall.SIGKILL)
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("processes of the claude group %d are still running", pgid)
}

func (s *Session) read(out io.Reader) {
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		s.handle(m)
	}
	err := s.cmd.Wait()
	s.mu.Lock()
	if err == nil {
		err = errors.New("claude exited")
	}
	s.exitErr = err
	s.mu.Unlock()
	close(s.done)
}

func (s *Session) handle(m map[string]any) {
	switch m["type"] {
	case "control_response":
		r, _ := m["response"].(map[string]any)
		id, _ := r["request_id"].(string)
		s.mu.Lock()
		ch := s.waiting[id]
		s.mu.Unlock()
		if ch != nil {
			ch <- r
		}
	case "control_request":
		// Nothing here answers a permission prompt or any other request the CLI makes: it is refused, so the CLI never waits.
		id, _ := m["request_id"].(string)
		s.write(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": id, "error": "not served"}})
		return
	}
	if s.h.OnEvent != nil {
		s.h.OnEvent(m)
	}
}
