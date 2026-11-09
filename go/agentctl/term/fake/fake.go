//go:build fakes

// Package fake is the terminal host of the fakes build.
package fake

import (
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/handline/handline/go/agentctl/term"
)

// Fake is the terminal host of the fakes build (HANDLINE_TERMINAL=fake): each pane is a real sh, so what runs in it has a pid the
// session checks can see, read from its pipes instead of a tmux screen. Its panes live in the program's process, keyed by Place.Key.
type Fake struct{ term.Place }

type fakePane struct {
	mu          sync.Mutex
	agent, cwd  string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	out         []byte
	exited      bool
	lastChanged time.Time
}

var (
	fakeMu    sync.Mutex
	fakePanes = map[string]map[string]*fakePane{} // key -> handle -> pane
	fakeNext  = 9000                              // handles look like tmux's (%N), so the code that checks a handle's form takes them
)

func (f Fake) panes() map[string]*fakePane {
	if fakePanes[f.Key] == nil {
		fakePanes[f.Key] = map[string]*fakePane{}
	}
	return fakePanes[f.Key]
}

func (f Fake) pane(handle string) *fakePane {
	fakeMu.Lock()
	defer fakeMu.Unlock()
	return f.panes()[handle]
}

func (Fake) Name() string    { return "fake" }
func (Fake) Available() bool { return true }

func (f Fake) ListTerminals() map[string]bool {
	fakeMu.Lock()
	defer fakeMu.Unlock()
	set := map[string]bool{}
	for h, p := range f.panes() {
		p.mu.Lock()
		if !p.exited {
			set[h] = true
		}
		p.mu.Unlock()
	}
	return set
}

// ReadScreen is the bottom limit lines of what the pane printed; a last line with no newline yet (a prompt) is one of them.
func (f Fake) ReadScreen(handle string, limit int) *term.Screen {
	p := f.pane(handle)
	if p == nil {
		return nil
	}
	p.mu.Lock()
	text := strings.TrimSuffix(string(p.out), "\n")
	p.mu.Unlock()
	lines := strings.Split(text, "\n")
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return &term.Screen{Tail: lines, Source: "screen"}
}

func (f Fake) Send(handle, text string, enter bool) (bool, string) {
	p := f.pane(handle)
	if p == nil {
		return false, "no fake pane " + term.PyRepr(handle)
	}
	if enter {
		text += "\n"
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.exited {
		return false, "the fake pane's shell has exited"
	}
	if _, err := io.WriteString(p.stdin, text); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// WaitIdle is whether the pane printed nothing for a moment and turn finds no turn on it, within timeoutMs.
func (f Fake) WaitIdle(handle string, timeoutMs int, turn func(tail []string) bool) bool {
	const settle = 200 * time.Millisecond
	for end := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		p := f.pane(handle)
		if p == nil {
			return false
		}
		p.mu.Lock()
		quiet := time.Since(p.lastChanged) >= settle
		p.mu.Unlock()
		if s := f.ReadScreen(handle, 50); quiet && s != nil && (turn == nil || !turn(s.Tail)) {
			return true
		}
	}
	return false
}

// CreateTerminal starts a shell in cwd with the daemon's home in its environment and types command at it, as a tmux window does.
func (f Fake) CreateTerminal(command, cwd, title, agent string) (string, string) {
	handle, why := f.start(cwd, agent)
	if handle == "" {
		return "", why
	}
	if ok, why := f.Send(handle, command, true); !ok {
		f.CloseTerminal(handle, true)
		return "", "could not type the command into the new fake pane: " + why
	}
	return handle, ""
}

func (f Fake) start(cwd, agent string) (string, string) {
	cmd := exec.Command("/bin/sh")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), f.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // a close ends what the shell started too
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err.Error()
	}
	p := &fakePane{agent: agent, cwd: cwd, cmd: cmd, stdin: stdin, lastChanged: time.Now()}
	w := fakeWriter{p}
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		return "", err.Error()
	}
	go func() {
		cmd.Wait()
		p.mu.Lock()
		p.exited = true
		p.mu.Unlock()
	}()
	fakeMu.Lock()
	fakeNext++
	handle := "%" + strconv.Itoa(fakeNext)
	f.panes()[handle] = p
	fakeMu.Unlock()
	return handle, ""
}

type fakeWriter struct{ p *fakePane }

func (w fakeWriter) Write(b []byte) (int, error) {
	w.p.mu.Lock()
	defer w.p.mu.Unlock()
	w.p.out = append(w.p.out, b...)
	if len(w.p.out) > 1<<20 { // a long run keeps its last MiB, as a pane keeps its scrollback
		w.p.out = w.p.out[len(w.p.out)-1<<20:]
	}
	w.p.lastChanged = time.Now()
	return len(b), nil
}

func (f Fake) CloseTerminal(handle string, _ bool) (bool, string) {
	fakeMu.Lock()
	p := f.panes()[handle]
	delete(f.panes(), handle)
	fakeMu.Unlock()
	if p == nil {
		return false, "no fake pane " + term.PyRepr(handle)
	}
	p.kill()
	return true, ""
}

func (p *fakePane) kill() {
	if p.cmd.Process != nil {
		syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// RespawnTerminal ends what runs in the pane and leaves a fresh shell under the same handle.
func (f Fake) RespawnTerminal(handle string) (bool, string) {
	p := f.pane(handle)
	if p == nil {
		return false, "no fake pane " + term.PyRepr(handle)
	}
	p.kill()
	fresh, why := f.start(p.cwd, p.agent)
	if fresh == "" {
		return false, why
	}
	fakeMu.Lock()
	f.panes()[handle] = f.panes()[fresh]
	delete(f.panes(), fresh)
	fakeMu.Unlock()
	return true, ""
}

func (f Fake) PaneAgent(handle string) (string, bool) {
	p := f.pane(handle)
	if p == nil {
		return "", false
	}
	return p.agent, true
}

func (f Fake) PanePid(handle string) int {
	if p := f.pane(handle); p != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}
