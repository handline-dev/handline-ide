package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/sim"
	"github.com/handline/handline/go/kit/paths"
)

// simActs is the screen.act kind of each verb that acts on the device: each is refused while a person holds it, and shown to whoever
// watches. A read (tree, screenshot) is not.
var simActs = map[string]string{"install": "install", "launch": "launch", "tap": "tap", "swipe": "swipe", "type": "type",
	"button": "button", "rotate": "rotate", "shutdown": "close"}

// simCmd is hl sim: an iOS Simulator or Android emulator an agent drives from the command line (internal/sim).
func simCmd(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	verb, rest := args[0], args[1:]
	e := &sim.Env{Home: paths.Home()}
	if verb == "_serve" { // the iOS driver's own process, started by boot
		if len(rest) != 2 {
			return 0, false
		}
		if err := sim.Serve(e.Home, rest[0], rest[1], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "hl sim:", err)
			return 1, true
		}
		return 0, true
	}
	p, ok := parse(rest, []string{"--device", "--label", "--id", "--text"}, []string{"--json"}, nil)
	if !ok {
		return 0, false
	}
	agent := os.Getenv("HANDLINE_AGENT")
	if agent == "" {
		agent = "founder"
	}
	fail := func(err error) (int, bool) { fmt.Fprintln(os.Stderr, "hl sim:", err); return 1, true }
	nums := func(n int) ([]float64, bool) {
		if len(p.pos) != n {
			return nil, false
		}
		var out []float64
		for _, s := range p.pos {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				argProblem = s + " is not a number"
				return nil, false
			}
			out = append(out, f)
		}
		return out, true
	}
	switch verb {
	case "list":
		if len(p.pos) != 0 {
			return 0, false
		}
		devices, missing := e.List()
		inUse := map[string]string{}
		for _, b := range e.All() {
			inUse[b.ID] = b.Agent
		}
		for _, d := range devices {
			state := "off"
			if who, ok := inUse[d.ID]; ok {
				state = "in use by " + who
			} else if d.Booted {
				state = "running"
			}
			fmt.Printf("%-8s %-28s %-36s %s\n", d.Platform, d.Name, d.ID, state)
		}
		for _, m := range missing {
			fmt.Fprintln(os.Stderr, m)
		}
		if len(devices) == 0 && len(missing) == 0 {
			fmt.Println("no devices")
		}
		return 0, true
	case "boot":
		if len(p.pos) != 1 {
			return 0, false
		}
		b, err := e.Boot(agent, p.pos[0])
		if err != nil {
			return fail(err)
		}
		if err := screenAct(c, sim.Source, "open", nil); err != nil {
			return fail(err)
		}
		fmt.Printf("booted %s (%s); it shuts down when your session ends, or hl sim shutdown\n", b.Name, b.Platform)
		return 0, true
	}
	want, _ := p.opt("--device")
	b, err := e.Pick(agent, want)
	if err != nil {
		if verb == "install" || verb == "launch" || verb == "tree" || verb == "tap" || verb == "swipe" || verb == "type" || verb == "button" ||
			verb == "rotate" || verb == "screenshot" || verb == "shutdown" {
			return fail(err)
		}
		return 0, false
	}
	if kind, ok := simActs[verb]; ok {
		if err := screenAct(c, sim.Source, kind, nil); err != nil {
			return fail(err)
		}
	}
	var done string
	switch verb {
	case "install":
		if len(p.pos) != 1 {
			return 0, false
		}
		abs, _ := filepath.Abs(p.pos[0])
		err = e.Install(b, abs)
	case "launch":
		if len(p.pos) != 1 {
			return 0, false
		}
		err = e.Launch(b, p.pos[0])
	case "tree":
		if len(p.pos) != 0 {
			return 0, false
		}
		var root *sim.Node
		if root, err = e.Tree(b); err == nil {
			if p.flags["--json"] {
				data, _ := json.MarshalIndent(root, "", " ")
				fmt.Println(string(data))
			} else {
				sim.Render(os.Stdout, root)
			}
		}
	case "tap":
		sel := sim.Selector{}
		sel.Label, _ = p.opt("--label")
		sel.ID, _ = p.opt("--id")
		sel.Text, _ = p.opt("--text")
		var xy []float64
		if sel == (sim.Selector{}) {
			var ok bool
			if xy, ok = nums(2); !ok {
				return 0, false
			}
		} else if len(p.pos) != 0 {
			return 0, false
		}
		x, y := 0.0, 0.0
		if len(xy) == 2 {
			x, y = xy[0], xy[1]
		}
		var at sim.Frame
		if done, at, err = e.Target(b, sel, x, y); err == nil {
			if err = screenAct(c, sim.Source, "tap", (*[4]float64)(&at)); err == nil {
				err = e.Tap(b, at)
			}
		}
		if err == nil {
			fmt.Println("tapped", done)
		}
	case "swipe":
		if len(p.pos) == 1 {
			err = e.Swipe(b, p.pos[0], 0, 0, 0, 0)
		} else if xy, ok := nums(4); ok {
			err = e.Swipe(b, "", xy[0], xy[1], xy[2], xy[3])
		} else {
			return 0, false
		}
	case "type":
		if len(p.pos) != 1 {
			return 0, false
		}
		err = e.Type(b, p.pos[0])
	case "button":
		if len(p.pos) != 1 {
			return 0, false
		}
		err = e.Button(b, p.pos[0])
	case "rotate":
		if len(p.pos) != 1 {
			return 0, false
		}
		err = e.Rotate(b, p.pos[0])
	case "screenshot":
		if len(p.pos) != 1 {
			return 0, false
		}
		abs, _ := filepath.Abs(p.pos[0])
		if err = e.Screenshot(b, abs); err == nil {
			fmt.Println(abs)
		}
	case "shutdown":
		if len(p.pos) != 0 {
			return 0, false
		}
		err = e.Shutdown(b)
	default:
		return 0, false
	}
	if err != nil {
		return fail(err)
	}
	return 0, true
}
