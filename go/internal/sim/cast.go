package sim

import (
	"bytes"
	"encoding/base64"
	"errors"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/handline/handline/go/internal/screenpipe"
)

// Source is the device's name in the screen pipeline.
const Source = "sim"

// The longest side of a frame in pixels and its JPEG quality: the driver takes about 175 ms a frame at this size, so about 5 a second.
const castMax, castQuality = 800, 0.7

// framer is a platform that can picture its screen for the pipeline; Android cannot yet.
type framer interface {
	frame(b *Booted, max int, quality float64) (screenpipe.Frame, error)
}

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
	stop    chan struct{}
	stopped bool

	in   sync.Mutex // one input at a time, in the order sent
	down *[2]float64
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

// OpenCast is agent's last booted device that can be pictured; screenpipe.ErrNotOpen when it has none.
func (e *Env) OpenCast(agent string) (*Cast, error) {
	var mine []*Booted
	for _, b := range e.All() {
		if b.Agent == agent {
			if _, ok := e.backend(b.Platform).(framer); ok {
				mine = append(mine, b)
			}
		}
	}
	if len(mine) == 0 {
		return nil, screenpipe.ErrNotOpen
	}
	sort.SliceStable(mine, func(i, j int) bool { return mine[i].Since > mine[j].Since })
	b := mine[0]
	return &Cast{e: e, b: b, fr: e.backend(b.Platform).(framer), Every: 100 * time.Millisecond, Retry: 500 * time.Millisecond, Fails: 3}, nil
}

func (s *Cast) Start(frame func(screenpipe.Frame), ended func()) error {
	f, err := s.fr.frame(s.b, castMax, castQuality)
	if err != nil {
		return screenpipe.ErrNotOpen
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return screenpipe.ErrNotOpen
	}
	stop := make(chan struct{})
	s.stop = stop
	s.mu.Unlock()
	frame(f)
	go s.loop(stop, f.JPEG, frame, ended)
	return nil
}

// loop asks for a frame after each one and passes on one that differs from the last; a driver that stops answering ends the source.
func (s *Cast) loop(stop chan struct{}, last []byte, frame func(screenpipe.Frame), ended func()) {
	fails := 0
	wait := s.Every
	for {
		select {
		case <-stop:
			return
		case <-time.After(wait):
		}
		at := time.Now()
		f, err := s.fr.frame(s.b, castMax, castQuality)
		select {
		case <-stop:
			return
		default:
		}
		if err != nil {
			if fails++; fails >= s.Fails {
				ended()
				return
			}
			wait = s.Retry
			continue
		}
		fails, wait = 0, max(0, s.Every-time.Since(at))
		if !bytes.Equal(f.JPEG, last) {
			last = f.JPEG
			frame(f)
		}
	}
}

func (s *Cast) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		s.stopped = true
		if s.stop != nil {
			close(s.stop)
		}
	}
}

// tapWithin is how far a press may move, in points, and still be a tap rather than a swipe.
const tapWithin = 10

// Input is a person's touch or typing: a press and its release are a tap where they meet and a swipe where they do not; a key that types
// text and a paste type it. A move, the wheel and a key that types nothing do nothing: the driver acts on whole gestures.
func (s *Cast) Input(ev screenpipe.Input) error {
	s.in.Lock()
	defer s.in.Unlock()
	be := s.e.backend(s.b.Platform)
	switch {
	case ev.Kind == "mouse" && ev.Action == "down" && ev.Button == "left":
		s.down = &[2]float64{ev.X, ev.Y}
	case ev.Kind == "mouse" && ev.Action == "up" && ev.Button == "left":
		from := s.down
		s.down = nil
		if from == nil {
			return nil
		}
		if math.Hypot(ev.X-from[0], ev.Y-from[1]) <= tapWithin {
			return be.tap(s.b, from[0], from[1])
		}
		return be.swipe(s.b, from[0], from[1], ev.X, ev.Y)
	case ev.Kind == "key" && ev.Action == "down":
		if t := keyText(ev); t != "" {
			return be.typeText(s.b, t)
		}
	case ev.Kind == "text":
		return be.typeText(s.b, ev.Text)
	}
	return nil
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
