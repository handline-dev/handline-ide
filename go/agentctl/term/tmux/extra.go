//go:build !windows

package tmux

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/handline/handline/go/agentctl/ansi"
	"github.com/handline/handline/go/agentctl/term"
)

func (Tmux) ServesBytes() bool  { return true }
func (Tmux) ServesStyled() bool { return true }

// ControlCommand is the program, with its arguments, that gives the daemon a control-mode window onto the tmux server.
func (t Tmux) ControlCommand() []string {
	return []string{"tmux", "-L", t.Socket, "-C", "attach-session", "-t", tmuxSession}
}

// AttachCommand is the program a person runs to be in the tmux session the agents' windows are in.
func (t Tmux) AttachCommand() []string {
	return []string{"tmux", "-L", t.Socket, "attach-session", "-t", tmuxSession}
}

// Focus selects the pane's window, which is what a client attached to the session then shows (handline/session/backends/tmux.py focus).
func (t Tmux) Focus(handle string) bool {
	if !paneRE.MatchString(handle) {
		return false
	}
	code, _, _ := t.tmux(tmuxTimeout, "", "select-window", "-t", handle)
	return code == 0
}

const tmuxSession = "handline"

// SendBytes types data as keystrokes: send-keys -H takes each byte as a hex number, so nothing in them is read as a key name or a
// command.
func (t Tmux) SendBytes(handle string, data []byte) (bool, string) {
	if !paneRE.MatchString(handle) {
		return false, "not a tmux pane: " + term.PyRepr(handle)
	}
	for i := 0; i < len(data); i += 256 {
		end := min(i+256, len(data))
		args := []string{"send-keys", "-t", handle, "-H"}
		for _, b := range data[i:end] {
			args = append(args, fmt.Sprintf("%02x", b))
		}
		if code, _, err := t.tmux(tmuxTimeout, "", args...); code != 0 {
			return false, err
		}
	}
	return true, ""
}

// ReadScreenANSI is ReadScreen's rows with the SGR codes kept. Trailing blank rows are dropped as there; a row is not stripped,
// since its codes may end it.
func (t Tmux) ReadScreenANSI(handle string, limit int) []string {
	if !paneRE.MatchString(handle) {
		return nil
	}
	code, out, _ := t.tmux(30*time.Second, "", "capture-pane", "-p", "-e", "-t", handle, "-S", "-"+strconv.Itoa(max(limit, 1)))
	if code != 0 {
		return nil
	}
	rows := strings.Split(out, "\n")
	for len(rows) > 0 && ansiBlank(rows[len(rows)-1]) {
		rows = rows[:len(rows)-1]
	}
	if len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	return rows
}

// ReadHistory is tmux's own scrollback, read with the alternate-screen flag in one command line so a line that scrolls meanwhile
// cannot move the page. On the alternate screen there is none.
func (t Tmux) ReadHistory(handle string, before *int, limit int, styled bool) (int, []string, bool) {
	if !paneRE.MatchString(handle) {
		return 0, nil, false
	}
	args := []string{"display-message", "-p", "-t", handle, "#{alternate_on} #{history_size}", ";", "capture-pane", "-p"}
	if styled {
		args = append(args, "-e")
	}
	code, out, _ := t.tmux(30*time.Second, "", append(args, "-t", handle, "-S", "-", "-E", "-1")...)
	head, body, _ := strings.Cut(out, "\n")
	mode := strings.Fields(head)
	size := -1
	if len(mode) == 2 {
		if n, err := strconv.Atoi(mode[1]); err == nil && n >= 0 {
			size = n
		}
	}
	if code != 0 || size < 0 || (mode[0] != "0" && mode[0] != "1") {
		return 0, nil, false
	}
	if mode[0] == "1" {
		return 0, []string{}, true
	}
	rows := strings.Split(body, "\n")
	if rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	// With no history tmux clamps -E to the screen's top row and returns it: only the last size rows are history.
	if size == 0 {
		rows = nil
	} else if len(rows) > size {
		rows = rows[len(rows)-size:]
	}
	end := len(rows)
	if before != nil {
		end = max(min(*before, len(rows)), 0)
	}
	start := max(end-max(limit, 0), 0)
	page := append([]string{}, rows[start:end]...)
	if !styled {
		for i, r := range page {
			page[i] = strings.TrimRightFunc(r, unicode.IsSpace)
		}
	}
	return start, page, true
}

// Cursor: one tmux call for the cursor and the visible rows, so the rows ReadScreen drops at the bottom are counted from the same moment.
func (t Tmux) Cursor(handle string) *term.Cursor {
	if !paneRE.MatchString(handle) {
		return nil
	}
	code, out, _ := t.tmux(tmuxTimeout, "", "display-message", "-p", "-t", handle, "#{cursor_x} #{cursor_y} #{cursor_flag} #{pane_height}", ";", "capture-pane", "-p", "-t", handle)
	head, body, _ := strings.Cut(out, "\n")
	n := digits(strings.Fields(head), 4)
	if code != 0 || n == nil {
		return nil
	}
	rows := strings.Split(body, "\n")
	if len(rows) > n[3] {
		rows = rows[:n[3]]
	}
	blank := 0
	for blank < len(rows) && strings.TrimSpace(rows[len(rows)-1-blank]) == "" {
		blank++
	}
	return &term.Cursor{X: n[0], Y: n[1], Visible: n[2] == 1, Height: n[3], BlankBelow: blank + max(n[3]-len(rows), 0)}
}

func (t Tmux) Size(handle string) (int, int, bool) {
	if !paneRE.MatchString(handle) {
		return 0, 0, false
	}
	code, out, _ := t.tmux(tmuxTimeout, "", "display-message", "-p", "-t", handle, "#{pane_width} #{pane_height}")
	if n := digits(strings.Fields(out), 2); code == 0 && n != nil {
		return n[0], n[1], true
	}
	return 0, 0, false
}

func (t Tmux) Resize(handle string, cols, rows int) (bool, string) {
	if !paneRE.MatchString(handle) {
		return false, "not a tmux pane: " + term.PyRepr(handle)
	}
	code, _, err := t.tmux(tmuxTimeout, "", "resize-window", "-t", handle, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows))
	if code == 0 {
		return true, ""
	}
	if err = strings.TrimSpace(err); err == "" {
		err = "tmux would not resize it"
	}
	return false, err
}

// digits is parts as n non-negative integers, or nil when it is not exactly that.
func digits(parts []string, n int) []int {
	if len(parts) != n {
		return nil
	}
	out := make([]int, n)
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 || p[0] == '+' {
			return nil
		}
		out[i] = v
	}
	return out
}

// ansiBlank is whether a row with codes shows no character.
func ansiBlank(row string) bool {
	text, _ := ansi.Parse(row)
	return text == ""
}
