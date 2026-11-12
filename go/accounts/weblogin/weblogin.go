// Package weblogin runs an account's login for the web page: the vendor's login command as a plain child with no terminal. The
// daemon reads the authorize URL off its stdout and answers it; the person opens it elsewhere. The child's output is never kept or
// logged, only that URL is handed on, and no credential is read: the end of a login is told by the account's credential being there.
package weblogin

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
)

// The login states the accounts topic reports.
const (
	Waiting   = "waiting"
	CodeSent  = "code_sent"
	Done      = "done"
	Failed    = "failed"
	Cancelled = "cancelled"
)

const (
	// URLWait is how long the child has to print its URL.
	URLWait = 20 * time.Second
	// MaxRun is how long a login may run before it is killed.
	MaxRun = 10 * time.Minute
)

// Refusal is why a login was not started or went no further; the method turns it into a refusal with gate `state`.
type Refusal struct{ Reason string }

func (r *Refusal) Error() string { return r.Reason }

// Login is a login as the accounts topic reports it.
type Login struct {
	ID    string
	State string
	URL   string // while waiting
	// Reason is why the login failed, one plain line; empty otherwise.
	Reason string
	// Seq numbers the moves of all logins in the order they were made, on a login handed to Changed; it is 0 elsewhere.
	Seq uint64
}

// Manager starts, ends and remembers logins.
type Manager struct {
	Home string
	// Wait is a timer channel; nil is time.After. Tests give one they fire.
	Wait func(time.Duration) <-chan time.Time
	// Lookup finds the login command; nil is exec.LookPath.
	Lookup func(name string) (string, error)
	// Changed is told the account whose login state moved, after it moved, and the login as it was left by that move (no URL): what is
	// announced is the state the move made, not whatever the login is by the time the announcement is made; nil is none. Calls may overlap
	// and arrive out of order: Login.Seq says which move came first.
	Changed func(account string, login Login)
	// Signal sends sig to the process group of pid; nil is syscall.Kill(-pid, sig). Tests give one that records.
	Signal func(pid int, sig syscall.Signal) error
	// Log is told each login's end in one line, never its URL or code; nil logs nothing.
	Log func(format string, args ...any)

	mu      sync.Mutex
	byID    map[string]*run
	byAcct  map[string]*run
	closing bool
	seq     uint64 // the last move's number, taken under mu
}

type run struct {
	id, account string
	state, url  string
	reason      string   // why it failed, once it has
	out, errs   lastLine // the last line the child printed on stdout, on stderr
	cmd         *exec.Cmd
	stdin       io.Writer     // the child's, kept open for the code and closed when the child is reaped
	pid         int           // the child's, once started; it leads its own process group
	reaped      bool          // the child has been waited for: its number is free and is never signalled again
	ended       chan struct{} // closed once the child is reaped
}

func (m *Manager) wait(d time.Duration) <-chan time.Time {
	if m.Wait != nil {
		return m.Wait(d)
	}
	return time.After(d)
}

// childEnv is the whole environment the child gets: the path, the home, the user's name (agent.UserName) and the account's config directory.
func childEnv(vendor, configDir string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), team.ConfigDirEnv(vendor) + "=" + configDir}
	if u := agent.UserName(); u != "" {
		env = append(env, "USER="+u, "LOGNAME="+u)
	}
	return env
}

// Start begins a login on the account and answers it once the child has printed its URL.
func (m *Manager) Start(account string) (Login, error) {
	if paths.AccountKey(m.Home, account) == "" { // its folder is by key: a login cannot make one
		return Login{}, &Refusal{"no account " + account + ": add it first"}
	}
	vendor := team.AccountVendor(m.Home, account)
	argv, ok := team.WebLoginArgv(vendor)
	if !ok {
		return Login{}, &Refusal{fmt.Sprintf("a web login is not available for the %s vendor", vendor)}
	}
	dir, err := team.AccountConfigDir(m.Home, account)
	if err != nil {
		return Login{}, &Refusal{err.Error()}
	}
	lookup := m.Lookup
	if lookup == nil {
		lookup = exec.LookPath
	}
	bin, err := lookup(argv[0])
	if err != nil {
		return Login{}, &Refusal{argv[0] + " is not installed"}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Login{}, &Refusal{"the account's config directory cannot be made"}
	}

	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return Login{}, &Refusal{"the daemon is stopping"}
	}
	if cur := m.byAcct[account]; cur != nil && cur.active() {
		m.mu.Unlock()
		return Login{}, &Refusal{account + " is already logging in"}
	}
	r := &run{id: newID(), account: account, state: Waiting, ended: make(chan struct{})}
	r.cmd = exec.Command(bin, argv[1:]...)
	r.cmd.Env = childEnv(vendor, dir)
	r.cmd.Dir = dir
	r.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if m.byID == nil {
		m.byID, m.byAcct = map[string]*run{}, map[string]*run{}
	}
	m.byID[r.id], m.byAcct[account] = r, r
	m.mu.Unlock()

	urls, err := m.launch(r)
	if err != nil {
		m.end(r, Failed, "the login could not be started")
		return Login{}, &Refusal{"the login could not be started"}
	}
	select {
	case u, open := <-urls:
		if open && u != "" {
			m.mu.Lock()
			waiting := r.state == Waiting // a child that printed its URL and died at once, was cancelled, or was sent a code is not waiting
			var moved Login
			if waiting {
				r.url = u
				moved = m.moved(r)
			}
			m.mu.Unlock()
			if waiting {
				m.changed(account, moved)
				return Login{ID: r.id, State: Waiting, URL: u}, nil
			}
			return Login{}, &Refusal{"the login ended before it could be answered"}
		}
		m.end(r, Failed, "the login ended without giving a URL")
		m.kill(r)
		return Login{}, &Refusal{"the login ended without giving a URL"}
	case <-m.wait(URLWait):
		m.end(r, Failed, "the login gave no URL within 20 seconds")
		m.kill(r)
		return Login{}, &Refusal{"the login gave no URL within 20 seconds"}
	}
}

// launch starts the child and the goroutines that read it. urls gets the first URL on stdout, and is closed without one when stdout ends.
func (m *Manager) launch(r *run) (<-chan string, error) {
	stdin, err := r.cmd.StdinPipe() // kept open for the code; Wait closes it when the child is reaped
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	r.stdin = stdin
	m.mu.Unlock()
	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := r.cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := r.cmd.Start(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	r.pid = r.cmd.Process.Pid
	m.mu.Unlock()
	urls := make(chan string, 1)
	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		defer close(urls)
		firstURL(io.TeeReader(stdout, &r.out), urls)
	}()
	go func() {
		defer readers.Done()
		_, _ = io.Copy(&r.errs, stderr)
	}()
	go func() {
		readers.Wait() // the pipes must be read out before Wait closes them
		err := r.cmd.Wait()
		m.mu.Lock()
		r.reaped = true
		m.mu.Unlock()
		m.exited(r, err)
		close(r.ended)
	}()
	go func() {
		select {
		case <-r.ended:
		case <-m.wait(MaxRun): // the login ends here, not when the reaper runs: it waits for stdout, which a grandchild may hold open
			m.end(r, Failed, "the login was not finished within 10 minutes")
			m.kill(r)
		}
	}()
	return urls, nil
}

// firstURL sends the first https:// word on r and then reads the rest out unread.
func firstURL(r io.Reader, urls chan<- string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	sc.Split(bufio.ScanWords)
	sent := false
	for sc.Scan() {
		if sent {
			continue
		}
		if w := sc.Text(); strings.HasPrefix(w, "https://") && !textsafe.HasUnsafe(w) {
			urls <- w
			sent = true
		}
	}
	_, _ = io.Copy(io.Discard, r)
}

// exited ends the login with what the child's exit, the credential and the logged-in account say.
func (m *Manager) exited(r *run, werr error) {
	accts := team.Accounts{Home: m.Home}
	accts.CredentialChanged(r.account) // a cancelled login's process may still have written one
	m.mu.Lock()
	settled := !r.active()
	m.mu.Unlock()
	if settled { // ended already (no URL, cancelled, timed out, closed): the exit says nothing more, and reads no account files
		return
	}
	if werr != nil {
		m.end(r, Failed, failedWhy(r.errs.String(), r.out.String()))
		return
	}
	if in, known := accts.LoginState(r.account); known && !in {
		m.end(r, Failed, "no usable credential was there after the login")
		return
	}
	if why := accts.WrongLogin(r.account); why != "" {
		m.end(r, Failed, why)
		return
	}
	m.end(r, Done, "")
}

// SubscriptionURL is where a person without a Claude subscription gets one.
const SubscriptionURL = "https://claude.ai/upgrade"

// failedWhy is why a login whose command exited with an error failed, from the last line it printed on stderr, else on stdout.
func failedWhy(errs, out string) string {
	if strings.HasPrefix(out, "https://") { // the login link, not a reason
		out = ""
	}
	said := errs
	if said == "" {
		said = out
	}
	for _, line := range []string{errs, out} {
		if strings.Contains(strings.ToLower(line), "subscription") {
			return "this needs a Claude Pro or Max subscription: get one at " + SubscriptionURL + ", then log in again (" + line + ")"
		}
	}
	if said != "" {
		return "the login command exited with an error: " + said
	}
	return "the login command exited with an error"
}

// lastLine keeps the last non-blank line written to it, printable and at most 300 characters.
type lastLine struct {
	mu        sync.Mutex
	line, cur []byte
}

func (l *lastLine) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, b := range p {
		if b != '\n' {
			if len(l.cur) < 4096 {
				l.cur = append(l.cur, b)
			}
			continue
		}
		l.keep()
	}
	return len(p), nil
}

func (l *lastLine) keep() {
	if t := textsafe.Strip(textsafe.Oneline(string(l.cur))); t != "" {
		if r := []rune(t); len(r) > 300 {
			t = string(r[:300])
		}
		l.line = []byte(t)
	}
	l.cur = l.cur[:0]
}

// String is the last line, an unfinished one counted. Call it once the writers are done: it ends the unfinished line.
func (l *lastLine) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.keep()
	return string(l.line)
}

// end settles the login in its final state unless it already has one, says so and logs it. reason is why it failed.
func (m *Manager) end(r *run, state, reason string) {
	m.mu.Lock()
	first := r.active()
	var moved Login
	if first {
		r.state, r.url, r.reason = state, "", reason
		moved = m.moved(r)
	}
	m.mu.Unlock()
	if !first {
		return
	}
	m.changed(r.account, moved)
	if m.Log != nil {
		if reason != "" {
			m.Log("  login: %s ended %s - %s", r.account, state, reason)
		} else {
			m.Log("  login: %s ended %s", r.account, state)
		}
	}
}

// active is a login that has not ended: it waits for the code or has been given it.
func (r *run) active() bool { return r.state == Waiting || r.state == CodeSent }

// moved is the login as the move just made left it, numbered after every move before it. The caller holds m.mu.
func (m *Manager) moved(r *run) Login {
	m.seq++
	return Login{ID: r.id, State: r.state, Reason: r.reason, Seq: m.seq}
}

func (m *Manager) changed(account string, l Login) {
	if m.Changed != nil {
		m.Changed(account, l)
	}
}

// kill ends the child's whole process group; the CLI ignores SIGTERM for about a second. A child already waited for is left alone: its
// number may be some other process group's by now.
func (m *Manager) kill(r *run) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.pid <= 0 || r.reaped {
		return
	}
	signal := m.Signal
	if signal == nil {
		signal = func(pid int, sig syscall.Signal) error { return syscall.Kill(-pid, sig) }
	}
	_ = signal(r.pid, syscall.SIGKILL)
}

// ErrUnknown is a login id this daemon never issued.
var ErrUnknown = errors.New("no such login")

// Cancel kills the login's process group. A login that already ended is left as it is.
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	r := m.byID[id]
	m.mu.Unlock()
	if r == nil {
		return ErrUnknown
	}
	m.end(r, Cancelled, "")
	m.kill(r)
	return nil
}

// codeMax is the longest code a login takes; the CLI's are a few dozen characters.
const codeMax = 4096

// ErrBadCode is a code that cannot be one line on the child's stdin.
var ErrBadCode = errors.New("the code is not one line of text")

// SendCode writes the one-time code and a newline to the child's stdin, once. The login moves to code_sent before the write, so a second
// code finds it not waiting. The code is written to that pipe and nowhere else: it is not kept, logged or put in any error.
func (m *Manager) SendCode(id, code string) error {
	if code == "" || len(code) > codeMax || strings.ContainsFunc(code, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ErrBadCode
	}
	m.mu.Lock()
	r := m.byID[id]
	if r == nil {
		m.mu.Unlock()
		return ErrUnknown
	}
	if r.state != Waiting || r.url == "" || r.stdin == nil || r.reaped { // no URL handed out yet: the person has nothing to take a code from
		m.mu.Unlock()
		return &Refusal{"the login is not waiting for a code"}
	}
	r.state, r.url = CodeSent, ""
	stdin, moved := r.stdin, m.moved(r)
	m.mu.Unlock()
	m.changed(r.account, moved)
	if _, err := io.WriteString(stdin, code+"\n"); err != nil {
		m.end(r, Failed, "the code could not be handed to the login")
		m.kill(r)
		return &Refusal{"the code could not be handed to the login"}
	}
	return nil
}

// Of is the account's latest login, nil when none was started.
func (m *Manager) Of(account string) *Login {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.byAcct[account]
	if r == nil {
		return nil
	}
	return &Login{ID: r.id, State: r.state, URL: r.url, Reason: r.reason}
}

// ByID is the login with that id.
func (m *Manager) ByID(id string) (Login, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.byID[id]
	if r == nil {
		return Login{}, false
	}
	return Login{ID: r.id, State: r.state, URL: r.url, Reason: r.reason}, true
}

// Close kills every login still running: a daemon restart ends the children it started.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closing = true
	var live []*run
	for _, r := range m.byID {
		if r.active() {
			live = append(live, r)
		}
	}
	m.mu.Unlock()
	for _, r := range live {
		m.end(r, Cancelled, "")
		m.kill(r)
	}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "login_" + hex.EncodeToString(b)
}
