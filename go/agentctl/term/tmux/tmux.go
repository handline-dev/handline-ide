//go:build !windows

// Package tmux is the terminal host on macOS and Linux: terminals are windows of one session on a tmux server the program owns
// (term.Place.Socket), driven through the tmux CLI.
package tmux

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/agentctl/internal/oneline"
	"github.com/handline/handline/go/agentctl/term"
)

const (
	tmuxTimeout = 15 * time.Second
	// Between typing a line and its Enter: a shell or TUI that gets both at once can take the Enter as part of a paste.
	enterDelay = 400 * time.Millisecond
	// How long the screen must hold still to count as idle, and how often it is looked at.
	idleSettle = 1500 * time.Millisecond
	idlePoll   = 500 * time.Millisecond
)

// run is one command as (exit code, stdout, stderr); a program that cannot run is (127, "", why). Stderr is clean at the source.
func run(timeout time.Duration, stdin string, name string, args ...string) (int, string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), out.String(), oneline.Oneline(strings.TrimSpace(errb.String()))
		}
		return 127, "", oneline.Oneline(err.Error())
	}
	return 0, out.String(), oneline.Oneline(strings.TrimSpace(errb.String()))
}

var paneRE = regexp.MustCompile(`^%\d+$`)

// A lone terminal escape sequence is sent as the key tmux names, not as bytes tmux may deliver one at a time.
var tmuxKeys = map[string]string{"\x1b[A": "Up", "\x1b[B": "Down", "\x1b[C": "Right", "\x1b[D": "Left", "\x1b": "Escape", "\x03": "C-c"}

// What tmux says when nothing listens: not "error connecting" alone, which a socket the caller may not use says too.
var noServer = []string{"no server running", "no sessions", "can't find session", "(no such file or directory)", "(connection refused)"}

// Tmux drives panes through the tmux CLI, on the tmux server of its Place.
type Tmux struct{ term.Place }

func (Tmux) Name() string { return "tmux" }

func (t Tmux) tmux(timeout time.Duration, stdin string, args ...string) (int, string, string) {
	return run(timeout, stdin, "tmux", append([]string{"-L", t.Socket}, args...)...)
}

func (Tmux) Available() bool { _, err := exec.LookPath("tmux"); return err == nil }

func (t Tmux) ListTerminals() map[string]bool {
	code, out, err := t.tmux(tmuxTimeout, "", "list-panes", "-a", "-F", "#{pane_id}")
	if code == 0 {
		set := map[string]bool{}
		for _, p := range strings.Fields(out) {
			if paneRE.MatchString(p) {
				set[p] = true
			}
		}
		return set
	}
	low := strings.ToLower(err)
	for _, m := range noServer {
		if strings.Contains(low, m) {
			return map[string]bool{} // a server that is not running has no terminals: that is an answer
		}
	}
	return nil
}

// ReadScreen is the bottom limit rows of the pane. The rows below the last drawn one are dropped, so the bottom of Tail is the
// bottom of what is on screen. A limit taller than the pane reaches into its scrollback, which is how a reader widens its read
// when a dialog's top border has scrolled out of the frame.
func (t Tmux) ReadScreen(handle string, limit int) *term.Screen {
	if !paneRE.MatchString(handle) {
		return nil
	}
	code, out, _ := t.tmux(30*time.Second, "", "capture-pane", "-p", "-t", handle, "-S", "-"+strconv.Itoa(max(limit, 1)))
	if code != 0 {
		return nil
	}
	rows := strings.Split(out, "\n")
	for i := range rows {
		rows[i] = strings.TrimRightFunc(rows[i], func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\f' || r == '\v' })
	}
	for len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	if len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	return &term.Screen{Tail: rows, Source: "screen"}
}

func (t Tmux) Send(handle, text string, enter bool) (bool, string) {
	if !paneRE.MatchString(handle) {
		return false, "not a tmux pane: " + term.PyRepr(handle)
	}
	if text != "" {
		var code int
		var err string
		switch {
		case tmuxKeys[text] != "":
			code, _, err = t.tmux(tmuxTimeout, "", "send-keys", "-t", handle, tmuxKeys[text])
		case strings.Contains(text, "\n"):
			// Several lines go as one bracketed paste, so a newline is text and not a submit.
			code, _, err = t.tmux(tmuxTimeout, text, "load-buffer", "-b", "handline-send", "-")
			if code == 0 {
				code, _, err = t.tmux(tmuxTimeout, "", "paste-buffer", "-p", "-d", "-b", "handline-send", "-t", handle)
			}
		default:
			code, _, err = t.tmux(tmuxTimeout, "", "send-keys", "-t", handle, "-l", "--", text)
		}
		if code != 0 {
			return false, err
		}
	}
	if enter {
		if text != "" {
			time.Sleep(enterDelay)
		}
		if code, _, err := t.tmux(tmuxTimeout, "", "send-keys", "-t", handle, "Enter"); code != 0 {
			return false, err
		}
	}
	return true, ""
}

// WaitIdle: the frame has not changed for idleSettle and turn (the vendor's reading) sees no turn in progress; with none
// given, a still frame is idle.
func (t Tmux) WaitIdle(handle string, timeoutMs int, turn func([]string) bool) bool {
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	var last []string
	since := time.Now()
	for {
		scr := t.ReadScreen(handle, 40)
		if scr == nil {
			return false
		}
		now := time.Now()
		if last == nil || !slices.Equal(scr.Tail, last) {
			last, since = scr.Tail, now
		} else if now.Sub(since) >= idleSettle && !(turn != nil && turn(scr.Tail)) {
			return true
		}
		left := deadline.Sub(now)
		if left <= 0 {
			return false
		}
		time.Sleep(min(idlePoll, left))
	}
}
