package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Browser is one agent's running Chrome: its folder (the profile and the state), its process, its DevTools port and the page it works in.
type Browser struct {
	Dir    string `json:"-"`
	PID    int    `json:"pid"`
	Port   int    `json:"port"`
	Show   bool   `json:"show"`
	PageID string `json:"page,omitempty"`
}

// startWithin is how long a starting Chrome has to open its DevTools port.
const startWithin = 30 * time.Second

func profile(dir string) string   { return filepath.Join(dir, "profile") }
func stateFile(dir string) string { return filepath.Join(dir, "state.json") }

// kept is the browser recorded in dir while its process is still that browser, answering or not; nil otherwise.
func kept(dir string) *Browser {
	b, err := os.ReadFile(stateFile(dir))
	if err != nil {
		return nil
	}
	br := &Browser{Dir: dir}
	if json.Unmarshal(b, br) != nil || !br.ours() {
		return nil
	}
	return br
}

// Running is the browser in dir when its process runs and answers on its port; nil otherwise.
func Running(dir string) *Browser {
	if b := kept(dir); b != nil && b.answers() {
		return b
	}
	return nil
}

// ours is whether the process is still this browser: it runs and its command line names this profile (a pid can be reused).
func (b *Browser) ours() bool {
	if b.PID <= 0 || syscall.Kill(b.PID, 0) != nil {
		return false
	}
	out, err := exec.Command("ps", "-ww", "-o", "command=", "-p", strconv.Itoa(b.PID)).Output()
	return err == nil && strings.Contains(string(out), "--user-data-dir="+profile(b.Dir))
}

func (b *Browser) answers() bool {
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", b.Port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// held runs f under dir's lock, so one agent's browser is started, restarted or closed by one command at a time.
func held(dir string, f func() error) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	unlock, err := lockFile(filepath.Join(dir, "lock"))
	if err != nil {
		return err
	}
	defer unlock()
	return f()
}

// Ensure is the browser in dir, started with engine when none runs, or started again when it runs shown and show is false or the other
// way round. One that runs but no longer answers is stopped first.
func Ensure(dir, engine string, show bool) (*Browser, error) {
	var b *Browser
	err := held(dir, func() error {
		if k := kept(dir); k != nil {
			if k.Show == show && k.answers() {
				b = k
				return nil
			}
			k.stop()
		}
		var err error
		b, err = start(dir, engine, show)
		return err
	})
	return b, err
}

// Opened is the browser in dir when one runs; it starts none.
func Opened(dir string) (*Browser, error) {
	if b := Running(dir); b != nil {
		return b, nil
	}
	return nil, errors.New("no browser is open: start one with hl browser open <url>")
}

func start(dir, engine string, show bool) (*Browser, error) {
	prof := profile(dir)
	if err := os.MkdirAll(prof, 0o700); err != nil { // its cookies and logins
		return nil, err
	}
	portFile := filepath.Join(prof, "DevToolsActivePort")
	_ = os.Remove(portFile)
	args := []string{"--user-data-dir=" + prof, "--remote-debugging-port=0", "--no-first-run", "--no-default-browser-check",
		"--window-size=1280,800"}
	if !show {
		args = append(args, "--headless=new")
	}
	args = append(args, "about:blank")
	log, err := os.Create(filepath.Join(dir, "chrome.log"))
	if err != nil {
		return nil, err
	}
	defer log.Close()
	cmd := exec.Command(engine, args...)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // outlives this hl, in a group of its own; the session's end closes it
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", engine, err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	b := &Browser{Dir: dir, PID: cmd.Process.Pid, Show: show}
	// Chrome writes the port it took into the profile once it listens. Its stderr would say so too, but a pipe to this hl breaks when hl
	// exits, so its output goes to the log and the file is looked at.
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(startWithin)
	for b.Port == 0 {
		select {
		case err := <-exited:
			return nil, fmt.Errorf("the browser exited as it started (%v); see %s", err, log.Name())
		case <-deadline:
			_ = syscall.Kill(-b.PID, syscall.SIGKILL)
			return nil, fmt.Errorf("the browser did not open its DevTools port within %s; see %s", startWithin, log.Name())
		case <-tick.C:
			if s, err := os.ReadFile(portFile); err == nil {
				first, _, _ := strings.Cut(string(s), "\n")
				b.Port, _ = strconv.Atoi(strings.TrimSpace(first))
			}
		}
	}
	if err := b.save(); err != nil {
		b.stop()
		return nil, err
	}
	return b, nil
}

func (b *Browser) save() error {
	state, _ := json.Marshal(b)
	return os.WriteFile(stateFile(b.Dir), state, 0o600)
}

// Close closes the browser in dir, if one runs, and says whether one did.
func Close(dir string) bool {
	closed := false
	_ = held(dir, func() error {
		if b := kept(dir); b != nil {
			b.stop()
			closed = true
		}
		_ = os.Remove(stateFile(dir))
		return nil
	})
	return closed
}

// stopWithin is how long a browser asked to close has before its processes are ended.
const stopWithin = 3 * time.Second

// stop asks Chrome to close, and ends its process group if it has not within stopWithin.
func (b *Browser) stop() {
	defer os.Remove(stateFile(b.Dir))
	if c, err := b.dialBrowser(); err == nil {
		c.within = stopWithin
		_, _ = c.call("Browser.close", nil)
		c.close()
	}
	for end := time.Now().Add(stopWithin); time.Now().Before(end); time.Sleep(50 * time.Millisecond) { // not our child: no wait to take
		if !b.ours() {
			return
		}
	}
	_ = syscall.Kill(-b.PID, syscall.SIGKILL)
}
