package browser

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/handline/handline/go/internal/screenpipe"
	"github.com/handline/handline/go/kit/paths"
)

// AgentDir is an agent's own browser under home: its profile and what runs, so two agents never share one.
func AgentDir(home, agent string) string {
	return filepath.Join(paths.AgentDir(home, agent), "browser")
}

// Cast is the agent's page as a screen source: its frames over one DevTools connection, a person's input and the release over another,
// so neither waits on the other's reads.
type Cast struct {
	b *Browser

	mu      sync.Mutex
	cast    *conn
	in      *conn
	stopped bool
}

// OpenCast is the browser in dir as a source; screenpipe.ErrNotOpen when none runs.
func OpenCast(dir string) (*Cast, error) {
	b := Running(dir)
	if b == nil {
		return nil, screenpipe.ErrNotOpen
	}
	return &Cast{b: b}, nil
}

// The largest frame asked for, and its JPEG quality; Chrome sends only when the page changes.
const castWidth, castHeight, castQuality = 1280, 800, 70

func (s *Cast) Start(frame func(screenpipe.Frame), ended func()) error {
	c, err := s.b.dialPageOr(false)
	if err != nil {
		return err
	}
	if _, err := c.call("Page.enable", nil); err == nil {
		_, err = c.call("Page.startScreencast", map[string]any{"format": "jpeg", "quality": castQuality, "maxWidth": castWidth,
			"maxHeight": castHeight})
	}
	if err != nil {
		c.close()
		return err
	}
	s.mu.Lock()
	if s.stopped { // stopped while it started
		s.mu.Unlock()
		c.close()
		return screenpipe.ErrNotOpen
	}
	s.cast = c
	s.mu.Unlock()
	go s.read(c, frame, ended)
	return nil
}

// read passes on each frame and acks it, which is what lets Chrome send the next: a slow watcher slows the source, nothing queues.
func (s *Cast) read(c *conn, frame func(screenpipe.Frame), ended func()) {
	for {
		_ = c.ws.SetReadDeadline(time.Time{})
		var m message
		if err := c.ws.ReadJSON(&m); err != nil {
			break
		}
		if m.Method == "Inspector.detached" || m.Method == "Inspector.targetCrashed" {
			break
		}
		if m.Method == "Page.javascriptDialogOpening" { // a person's click opened it; nothing else can answer it
			var d struct{ Type string }
			_ = json.Unmarshal(m.Params, &d)
			accept, reply := d.Type == "beforeunload", "" // a leave the agent asked for goes on
			if b := kept(s.b.Dir); b != nil {
				accept, reply = accept || b.Accept, b.Reply
			}
			answer(c, accept, reply)
			continue
		}
		if m.Method != "Page.screencastFrame" {
			continue
		}
		var f struct {
			Data     string `json:"data"`
			Session  int    `json:"sessionId"`
			Metadata struct {
				W float64 `json:"deviceWidth"`
				H float64 `json:"deviceHeight"`
			} `json:"metadata"`
		}
		if json.Unmarshal(m.Params, &f) != nil {
			continue
		}
		if jpeg, err := base64.StdEncoding.DecodeString(f.Data); err == nil {
			frame(screenpipe.Frame{JPEG: jpeg, Width: int(f.Metadata.W), Height: int(f.Metadata.H)})
		}
		c.next++
		_ = c.ws.SetWriteDeadline(time.Now().Add(callWithin)) // a call's deadline stays on the connection: each ack sets its own
		if c.ws.WriteJSON(map[string]any{"id": c.next, "method": "Page.screencastFrameAck", "params": map[string]any{"sessionId": f.Session}}) != nil {
			break
		}
	}
	c.close()
	s.mu.Lock()
	stopped := s.stopped
	s.mu.Unlock()
	if !stopped {
		ended()
	}
}

// Stop ends the cast: the screencast belongs to its connection, so closing it stops Chrome sending.
func (s *Cast) Stop() {
	s.mu.Lock()
	s.stopped = true
	c, in := s.cast, s.in
	s.cast, s.in = nil, nil
	s.mu.Unlock()
	if c != nil {
		c.close()
	}
	if in != nil {
		in.close()
	}
}

// input is the connection a person's input and the release go over, dialled the first time; held under mu.
func (s *Cast) input() (*conn, error) {
	if s.stopped {
		return nil, errors.New("the cast has stopped")
	}
	if s.in == nil {
		c, err := s.b.dialPageOr(false)
		if err != nil {
			return nil, err
		}
		s.in = c
	}
	return s.in, nil
}

// typedSet is the page's own record of the password fields a person typed into, which a submit takes a field out of.
const typedSet = `window[Symbol.for("typedPasswords")]`

// markTyped puts the focused field in typedSet when it is a password field: a person's key is about to land in it.
const markTyped = `(() => {
	const el = document.activeElement;
	if (!el || el.tagName !== "INPUT" || el.type !== "password") return;
	const s = ` + typedSet + ` ||= new Set();
	if (s.has(el)) return;
	s.add(el);
	if (el.form) el.form.addEventListener("submit", () => s.delete(el), {once: true});
})()`

// emptyTyped empties each field in typedSet still on the page and holding a value, and forgets them.
const emptyTyped = `(() => {
	const s = ` + typedSet + `;
	if (!s) return 0;
	let n = 0;
	for (const el of s) {
		if (el.isConnected && el.type === "password" && el.value) {
			el.value = "";
			el.dispatchEvent(new Event("input", {bubbles: true}));
			n++;
		}
	}
	s.clear();
	return n;
})()`

var mouseTypes = map[string]string{"move": "mouseMoved", "down": "mousePressed", "up": "mouseReleased"}

func (s *Cast) Input(ev screenpipe.Input) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.input()
	if err != nil {
		return err
	}
	switch ev.Kind {
	case "mouse":
		p := map[string]any{"type": mouseTypes[ev.Action], "x": ev.X, "y": ev.Y, "modifiers": ev.Modifiers}
		if ev.Action != "move" {
			p["button"], p["clickCount"] = ev.Button, max(ev.Clicks, 1)
		}
		_, err = c.call("Input.dispatchMouseEvent", p)
	case "wheel":
		_, err = c.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": ev.X, "y": ev.Y, "deltaX": ev.DX,
			"deltaY": ev.DY, "modifiers": ev.Modifiers})
	case "key", "text":
		if ev.Kind == "text" || ev.Action == "down" {
			if _, err := c.call("Runtime.evaluate", map[string]any{"expression": markTyped}); err != nil {
				return err
			}
		}
		if ev.Kind == "text" {
			_, err = c.call("Input.insertText", map[string]any{"text": ev.Text})
			break
		}
		p := map[string]any{"type": "keyUp", "key": ev.Key, "code": ev.Code, "modifiers": ev.Modifiers}
		if ev.Action == "down" {
			p["type"] = "rawKeyDown"
			if ev.Text != "" {
				p["type"], p["text"] = "keyDown", ev.Text
			}
		}
		_, err = c.call("Input.dispatchKeyEvent", p)
	}
	return err
}

func (s *Cast) Release() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.input()
	if err != nil {
		return err
	}
	_, err = c.call("Runtime.evaluate", map[string]any{"expression": emptyTyped})
	return err
}

// Opener is the agents' browsers under home as screen sources.
func Opener(home string) screenpipe.Opener {
	return func(agent, source string) (screenpipe.Source, error) {
		if source != "browser" {
			return nil, screenpipe.ErrNotOpen
		}
		return OpenCast(AgentDir(home, agent))
	}
}
