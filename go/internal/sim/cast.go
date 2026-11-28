package sim

import (
	"bytes"
	"encoding/base64"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/handline/handline/go/internal/screenpipe"
)

// Source is the device's name in the screen pipeline.
const Source = "sim"

// The longest side of a frame in pixels and its JPEG quality: the driver takes about 175 ms a frame at this size, so about 5 a second.
const castMax, castQuality = 800, 0.7

// framer is a platform that can picture its screen for the pipeline.
type framer interface {
	pictures(b *Booted) bool
	frame(b *Booted, max int, quality float64) (screenpipe.Frame, error)
}

func (i *ios) pictures(*Booted) bool { return true }

func (i *ios) frame(b *Booted, max int, quality float64) (screenpipe.Frame, error) {
	got, err := i.call(b, map[string]any{"op": "frame", "max": max, "quality": quality})
	if err != nil {
		return screenpipe.Frame{}, err
	}
	s, _ := got["jpeg"].(string)
	jpeg, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(jpeg) == 0 {
		return screenpipe.Frame{}, errors.New("the driver sent no frame")
	}
	w, _ := got["width"].(float64)
	h, _ := got["height"].(float64)
	return screenpipe.Frame{JPEG: jpeg, Width: int(w), Height: int(h)}, nil
}

// Cast is a booted device as a screen source: frames from its driver while watched, and a person's touches and typing through the
// driver's own tap, swipe and type.
type Cast struct {
	e  *Env
	b  *Booted
	fr framer
	// Every is the least time between two frames; Retry how long to wait after a failed one, and Fails how many in a row end the source.
	Every, Retry time.Duration
	Fails        int

	mu      sync.Mutex
	stop    chan struct{} // closed by Stop
	stopped bool
	acts    chan func() error // a person's gestures, done in order off the caller: one takes the driver most of a second
	down    *[2]float64
}

// Opener is the agents' devices under home as screen sources: the one each booted last.
func Opener(home string) screenpipe.Opener {
	e := &Env{Home: home}
	return func(agent, source string) (screenpipe.Source, error) {
		if source != Source {
			return nil, screenpipe.ErrNotOpen
		}
		return e.OpenCast(agent)
	}
}

// Pictured is whether a platform's devices can be a screen source.
func Pictured(platform string) bool {
	_, ok := (&Env{}).backend(platform).(framer)
	return ok
}

// pictured is whether this booted device is one: a phone on adb has no console to picture it.
func pictured(b *Booted) bool {
	f, ok := (&Env{}).backend(b.Platform).(framer)
	return ok && f.pictures(b)
}

// Cast is the device agent's source shows: the pictured one it booted last, nil when it has none.
func (e *Env) Cast(agent string) *Booted {
	var last *Booted
	for _, b := range e.All() {
		if b.Agent == agent && pictured(b) && (last == nil || b.Since > last.Since) {
			last = b
		}
	}
	return last
}

// OpenCast is agent's source; screenpipe.ErrNotOpen when it has no pictured device.
func (e *Env) OpenCast(agent string) (*Cast, error) {
	b := e.Cast(agent)
	if b == nil {
		return nil, screenpipe.ErrNotOpen
	}
	return &Cast{e: e, b: b, fr: e.backend(b.Platform).(framer), Every: 100 * time.Millisecond, Retry: 500 * time.Millisecond, Fails: 3,
		stop: make(chan struct{}), acts: make(chan func() error, maxActs)}, nil
}

// maxActs is how many gestures may wait for the driver before the next is refused.
const maxActs = 64

// Start answers at once: the first frame comes from the loop, so a busy driver holds up no watcher, and one that never answers ends it.
func (s *Cast) Start(frame func(screenpipe.Frame), ended func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return screenpipe.ErrNotOpen
	}
	go s.loop(frame, ended)
	go s.act()
	return nil
}

// loop asks for a frame after each one and passes on one that differs from the last; a driver that stops answering ends the source.
func (s *Cast) loop(frame func(screenpipe.Frame), ended func()) {
	var last []byte
	fails := 0
	var wait time.Duration
	for {
		select {
		case <-s.stop:
			return
		case <-time.After(wait):
		}
		at := time.Now()
		f, err := s.fr.frame(s.b, castMax, castQuality)
		if err != nil {
			if fails++; fails >= s.Fails {
				s.mu.Lock()
				stopped := s.stopped
				s.mu.Unlock()
				if !stopped { // not under mu: ended stops the source
					ended()
				}
				return
			}
			wait = s.Retry
			continue
		}
		fails, wait = 0, max(0, s.Every-time.Since(at))
		if bytes.Equal(f.JPEG, last) {
			continue
		}
		last = f.JPEG
		s.mu.Lock()
		if !s.stopped {
			frame(f)
		}
		s.mu.Unlock()
	}
}

// act does the gestures in the order they came until Stop.
func (s *Cast) act() {
	for {
		select {
		case <-s.stop:
			return
		case do := <-s.acts:
			_ = do() // a gesture that did not land shows in the next frame
		}
	}
}

func (s *Cast) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		s.stopped = true
		close(s.stop)
	}
}

// tapWithin is how far a press may move, in points, and still be a tap rather than a swipe.
const tapWithin = 10

// Input is a person's touch or typing, done after the answer in the order sent: a press and its release are a tap where they meet and a
// swipe where they do not; a key that types text and a paste type it. A move, the wheel and a key that types nothing do nothing: the
// driver acts on whole gestures.
func (s *Cast) Input(ev screenpipe.Input) error {
	be := s.e.backend(s.b.Platform)
	var do func() error
	s.mu.Lock()
	switch {
	case ev.Kind == "mouse" && ev.Action == "down" && ev.Button == "left":
		s.down = &[2]float64{ev.X, ev.Y}
	case ev.Kind == "mouse" && ev.Action == "up" && ev.Button == "left" && s.down != nil:
		from := s.down
		s.down = nil
		if math.Hypot(ev.X-from[0], ev.Y-from[1]) <= tapWithin {
			do = func() error { return be.tap(s.b, from[0], from[1]) }
		} else {
			do = func() error { return be.swipe(s.b, from[0], from[1], ev.X, ev.Y) }
		}
	case ev.Kind == "key" && ev.Action == "down" && keyText(ev) != "":
		t := keyText(ev)
		do = func() error { return be.typeText(s.b, t) }
	case ev.Kind == "text":
		do = func() error { return be.typeText(s.b, ev.Text) }
	}
	stopped := s.stopped
	s.mu.Unlock()
	switch {
	case do == nil:
		return nil
	case stopped:
		return errors.New("the device is no longer watched")
	}
	select {
	case s.acts <- do:
		return nil
	default:
		return errors.New("the device is still doing what came before: try again")
	}
}

// keyText is what a key down types on the device: its text, Enter a new line, Backspace a delete.
func keyText(ev screenpipe.Input) string {
	switch ev.Key {
	case "Enter":
		return "\n"
	case "Backspace":
		return "\b"
	}
	if ev.Modifiers&(2|4) != 0 { // Ctrl or Meta: a shortcut, not typing
		return ""
	}
	return ev.Text
}

// Release has nothing to clear: a secure field's value is never in what an agent reads of the screen.
func (s *Cast) Release() error { return nil }
