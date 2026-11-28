// Package screenpipe is the screen pipeline: a source an agent has open on its machine (its browser first) sends frames to every client
// that watches it and takes a person's input back, from the one who took it (screen.attach, input, take, give_back, detach). The
// agent's own use of the source (screen.act) is refused while a person holds it, and shown to watchers as it happens.
package screenpipe

import "errors"

// Frame is one picture of the source: a JPEG, and the size of what it shows in the units input is given in.
type Frame struct {
	JPEG          []byte
	Width, Height int
}

// Input is one thing a person does on the source. Kind mouse (Action move, down or up, at X, Y with Button and Clicks), wheel (DX, DY
// at X, Y), key (Action down or up; Text is what a down types) or text (Text, as a paste).
type Input struct {
	Kind, Action string
	X, Y, DX, DY float64
	Button       string
	Clicks       int
	Key, Code    string
	Text         string
	Modifiers    int
}

// Source is one screen of one agent on this machine.
type Source interface {
	// Start sends each frame to frame until Stop; ended is told once if the source goes away first.
	Start(frame func(Frame), ended func()) error
	Stop()
	Input(Input) error
	// Release clears what a person who held the source leaves behind that the agent must not read: each password field they typed
	// into and did not submit.
	Release() error
}

// ErrNotOpen is an Opener's answer for an agent with no such source open.
var ErrNotOpen = errors.New("not open")

// Opener is the source an agent has open on this machine, ErrNotOpen when it has none.
type Opener func(agent, source string) (Source, error)

// Sources is every source kind there is.
var Sources = []string{"browser"}
