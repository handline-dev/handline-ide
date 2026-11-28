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
	agent := hlclient.Agent() // the session's key: a name a rename has given away since would share another's device
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
		d, err := e.Find(p.pos[0])
		if err != nil {
			return fail(err)
		}
		shown := sim.Pictured(d.Platform)
		if shown {
			if err := screenAct(c, sim.Source, "open", nil); err != nil { // a person may hold the device it shows now
				return fail(err)
			}
		}
		b, err := e.Boot(agent, d.ID)
		if err != nil {
			if shown && e.Cast(agent) == nil {
				_ = screenAct(c, sim.Source, "close", nil)
			}
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
	// act is screen.act before a verb acts on the device the agent's source shows: refused while a person holds it, and shown to whoever
	// watches. Another device of the agent's is not a source.
	cast := e.Cast(agent)
	shown := cast != nil && cast.ID == b.ID
	act := func(kind string, box *[4]float64) error {
		if !shown {
			return nil
		}
		return screenAct(c, sim.Source, kind, box)
	}
	var done string
	switch verb {
	case "install":
		if len(p.pos) != 1 {
			return 0, false
		}
		abs, _ := filepath.Abs(p.pos[0])
		if err = act("install", nil); err == nil {
			err = e.Install(b, abs)
		}
	case "launch":
		if len(p.pos) != 1 {
			return 0, false
		}
		if err = act("launch", nil); err == nil {
			err = e.Launch(b, p.pos[0])
		}
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
		if err = act("tap", nil); err != nil { // before the target is found: a check only
			break
		}
		if done, at, err = e.Target(b, sel, x, y); err == nil {
			if err = act("tap", (*[4]float64)(&at)); err == nil {
				err = e.Tap(b, at)
			}
		}
		if err == nil {
			fmt.Println("tapped", done)
		}
	case "swipe":
		dir, xy := "", []float64{0, 0, 0, 0}
		if len(p.pos) == 1 {
			dir = p.pos[0]
		} else if n, ok := nums(4); ok {
			xy = n
		} else {
			return 0, false
		}
		if err = act("swipe", nil); err == nil {
			err = e.Swipe(b, dir, xy[0], xy[1], xy[2], xy[3])
		}
	case "type":
		if len(p.pos) != 1 {
			return 0, false
		}
		if err = act("type", nil); err == nil {
			err = e.Type(b, p.pos[0])
		}
	case "button":
		if len(p.pos) != 1 {
			return 0, false
		}
		if err = act("button", nil); err == nil {
			err = e.Button(b, p.pos[0])
		}
	case "rotate":
		if len(p.pos) != 1 {
			return 0, false
		}
		if err = act("rotate", nil); err == nil {
			err = e.Rotate(b, p.pos[0])
		}
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
		if err = act("close", nil); err != nil {
			break
		}
		err = e.Shutdown(b)
		if shown && e.Cast(agent) != nil { // still up, or its other device is the source now
			_ = screenAct(c, sim.Source, "open", nil)
		}
	default:
		return 0, false
	}
	if err != nil {
		return fail(err)
	}
	return 0, true
}
