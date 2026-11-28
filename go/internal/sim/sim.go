// Package sim is what hl sim does: boot an iOS Simulator or an Android emulator, install and launch an app, read what is on the screen,
// tap, swipe, type, press a button, rotate, take a screenshot, and shut down. iOS is driven by a small XCUITest bundle built from the
// sources in driver/ with Xcode's own tools on first use (the same public API a UI test uses, so nothing is installed beyond Xcode);
// Android by adb. A device an agent booted is recorded under the home so the daemon can shut it down when the agent's session ends.
package sim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Device is a device that could be booted.
type Device struct {
	Platform string // "ios" or "android"
	ID       string // the simulator's UDID, the emulator's name
	Name     string
	Booted   bool
}

// Booted is a device an agent is using: kept under the home while it is.
type Booted struct {
	Platform string `json:"platform"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Agent    string `json:"agent"`
	Adopted  bool   `json:"adopted,omitempty"` // it was running before: left running when the session ends
	Port     int    `json:"port,omitempty"`    // the iOS driver's port
	Serve    int    `json:"serve,omitempty"`   // the process running the iOS driver
	Serial   string `json:"serial,omitempty"`  // the adb serial
	App      string `json:"app,omitempty"`     // the app last launched: the one `tree` and taps are read in
	Since    int64  `json:"since"`
}

// Env is where this runs and what it calls.
type Env struct {
	Home string
	// Run runs a program and gives back its output, stdout and stderr together; nil is exec. It is how tests stand in for Xcode and adb.
	Run func(name string, args ...string) ([]byte, error)
}

func (e *Env) run(name string, args ...string) ([]byte, error) {
	if e.Run != nil {
		return e.Run(name, args...)
	}
	var out bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.Bytes(), err
}

// runStdout is a program's standard output alone, for output that is data (a screenshot).
func (e *Env) runStdout(name string, args ...string) ([]byte, error) {
	if e.Run != nil {
		return e.Run(name, args...)
	}
	return exec.Command(name, args...).Output()
}

// runOut is run's output, an error carrying it when the program failed.
func (e *Env) runOut(name string, args ...string) (string, error) {
	out, err := e.run(name, args...)
	if err != nil {
		return "", fmt.Errorf("%s %s: %v: %s", filepath.Base(name), strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (e *Env) dir(parts ...string) string {
	return filepath.Join(append([]string{e.Home, "sim"}, parts...)...)
}

func (e *Env) statePath(platform, id string) string {
	return e.dir("devices", platform+"-"+strings.NewReplacer("/", "_", ":", "_").Replace(id)+".json")
}

func (e *Env) save(b *Booted) error {
	if err := os.MkdirAll(e.dir("devices"), 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(b)
	return os.WriteFile(e.statePath(b.Platform, b.ID), data, 0o644)
}

func (e *Env) forget(b *Booted) { _ = os.Remove(e.statePath(b.Platform, b.ID)) }

// All is every device in use, by name.
func (e *Env) All() []*Booted {
	files, _ := filepath.Glob(e.dir("devices", "*.json"))
	var out []*Booted
	for _, f := range files {
		var b Booted
		if data, err := os.ReadFile(f); err == nil && json.Unmarshal(data, &b) == nil && b.ID != "" {
			out = append(out, &b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Pick is the device agent means: want by name or id, or its only one.
func (e *Env) Pick(agent, want string) (*Booted, error) {
	var mine []*Booted
	for _, b := range e.All() {
		if want != "" && (b.ID == want || b.Name == want || b.Serial == want) {
			return b, nil
		}
		if b.Agent == agent {
			mine = append(mine, b)
		}
	}
	switch {
	case want != "":
		return nil, fmt.Errorf("no device %q is in use (hl sim boot %s)", want, want)
	case len(mine) == 1:
		return mine[0], nil
	case len(mine) == 0:
		return nil, errors.New("no device is booted for you: hl sim list, then hl sim boot <device>")
	}
	var names []string
	for _, b := range mine {
		names = append(names, b.Name)
	}
	return nil, fmt.Errorf("you have %d devices (%s): name one with --device", len(mine), strings.Join(names, ", "))
}

// Reap shuts down the devices agent booted, when its session ends; one that was running before it came is left as it was.
func (e *Env) Reap(agent string) {
	for _, b := range e.All() {
		if b.Agent == agent {
			_ = e.Shutdown(b)
		}
	}
}

// Backend is a platform's part of hl sim.
type backend interface {
	list() ([]Device, error)
	boot(d Device, agent string) (*Booted, error)
	install(b *Booted, path string) error
	launch(b *Booted, app string) error
	tree(b *Booted) (*Node, error)
	tap(b *Booted, x, y float64) error
	swipe(b *Booted, x1, y1, x2, y2 float64) error
	typeText(b *Booted, text string) error
	button(b *Booted, name string) error
	rotate(b *Booted, to string) error
	screenshot(b *Booted, file string) error
	shutdown(b *Booted) error
}

func (e *Env) backend(platform string) backend {
	if platform == "android" {
		return &android{e: e}
	}
	return &ios{e: e}
}

var platforms = []string{"ios", "android"}

// List is every device that can be booted, and why a platform has none when it cannot be used on this machine.
func (e *Env) List() (devices []Device, missing []string) {
	for _, p := range platforms {
		ds, err := e.backend(p).list()
		if err != nil {
			missing = append(missing, err.Error())
		}
		devices = append(devices, ds...)
	}
	return devices, missing
}

// Boot starts the device named want (a name or an id) for agent and waits until an app can be driven on it.
func (e *Env) Boot(agent, want string) (*Booted, error) {
	d, err := e.Find(want)
	if err != nil {
		return nil, err
	}
	for _, b := range e.All() {
		if b.ID == d.ID {
			if b.Agent != agent {
				return nil, fmt.Errorf("%s is in use by %s", b.Name, b.Agent)
			}
			return b, nil
		}
	}
	b, err := e.backend(d.Platform).boot(d, agent)
	if err != nil {
		return nil, err
	}
	b.Since = time.Now().Unix()
	if err := e.save(b); err != nil {
		_ = e.backend(b.Platform).shutdown(b)
		return nil, err
	}
	return b, nil
}

// Find is the one device want names, by name or id or part of a name.
func (e *Env) Find(want string) (Device, error) {
	devices, missing := e.List()
	var hit []Device
	for _, d := range devices {
		if d.ID == want || d.Name == want {
			hit = []Device{d}
			break
		}
		if want != "" && strings.Contains(strings.ToLower(d.Name), strings.ToLower(want)) {
			hit = append(hit, d)
		}
	}
	switch {
	case len(hit) == 0:
		why := "no device named " + want + ": hl sim list shows them"
		if len(missing) > 0 {
			why += "\n" + strings.Join(missing, "\n")
		}
		return Device{}, errors.New(why)
	case len(hit) > 1:
		return Device{}, fmt.Errorf("%q names %d devices: use a full name or an id (hl sim list)", want, len(hit))
	}
	return hit[0], nil
}

// Shutdown stops the device and forgets it once it is off: a device that was running before it was booted here is only let go.
func (e *Env) Shutdown(b *Booted) error {
	if err := e.backend(b.Platform).shutdown(b); err != nil {
		return err
	}
	e.forget(b)
	return nil
}

// Install puts the app at path on the device.
func (e *Env) Install(b *Booted, path string) error { return e.backend(b.Platform).install(b, path) }

// Launch starts app (a bundle id, a package name) and makes it the app the device is read and driven in.
func (e *Env) Launch(b *Booted, app string) error {
	if err := e.backend(b.Platform).launch(b, app); err != nil {
		return err
	}
	b.App = app
	return e.save(b)
}

// Tree is what is on the screen now.
func (e *Env) Tree(b *Booted) (*Node, error) { return e.backend(b.Platform).tree(b) }

// Target is what a tap lands on: the element sel names, or the point (x, y) when sel is empty, as a line and its frame (a point's is
// empty).
func (e *Env) Target(b *Booted, sel Selector, x, y float64) (string, Frame, error) {
	if sel.empty() {
		return fmt.Sprintf("point %.0f,%.0f", x, y), Frame{x, y, 0, 0}, nil
	}
	root, err := e.backend(b.Platform).tree(b)
	if err != nil {
		return "", Frame{}, err
	}
	n, ok := Find(root, sel)
	if !ok {
		return "", Frame{}, fmt.Errorf("nothing enabled on the screen matches %s (hl sim tree shows what is there)", sel.String())
	}
	return n.line(), n.Frame, nil
}

// Tap taps the centre of f.
func (e *Env) Tap(b *Booted, f Frame) error {
	x, y := f.Center()
	return e.backend(b.Platform).tap(b, x, y)
}

func (s Selector) String() string {
	switch {
	case s.Label != "":
		return fmt.Sprintf("label %q", s.Label)
	case s.ID != "":
		return fmt.Sprintf("id %q", s.ID)
	}
	return fmt.Sprintf("text %q", s.Text)
}

// Swipe drags from (x1, y1) to (x2, y2), or across the screen in a direction ("up" moves the content up, as a finger does).
func (e *Env) Swipe(b *Booted, dir string, x1, y1, x2, y2 float64) error {
	be := e.backend(b.Platform)
	if dir != "" {
		root, err := be.tree(b)
		if err != nil {
			return err
		}
		w, h := Screen(root)
		cx, cy := w/2, h/2
		switch dir {
		case "up":
			x1, y1, x2, y2 = cx, h*0.7, cx, h*0.3
		case "down":
			x1, y1, x2, y2 = cx, h*0.3, cx, h*0.7
		case "left":
			x1, y1, x2, y2 = w*0.8, cy, w*0.2, cy
		case "right":
			x1, y1, x2, y2 = w*0.2, cy, w*0.8, cy
		default:
			return fmt.Errorf("a direction is up, down, left or right, not %q", dir)
		}
	}
	return be.swipe(b, x1, y1, x2, y2)
}

// Type types text where the keyboard focus is.
func (e *Env) Type(b *Booted, text string) error { return e.backend(b.Platform).typeText(b, text) }

// Button presses a hardware button: home on both, back, menu, volumeup, volumedown, power, enter on Android.
func (e *Env) Button(b *Booted, name string) error { return e.backend(b.Platform).button(b, name) }

// Rotate turns the device to portrait, upsidedown, left or right.
func (e *Env) Rotate(b *Booted, to string) error { return e.backend(b.Platform).rotate(b, to) }

// Screenshot saves what is on the screen to file as a PNG.
func (e *Env) Screenshot(b *Booted, file string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return e.backend(b.Platform).screenshot(b, file)
}
