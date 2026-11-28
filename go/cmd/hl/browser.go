package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/handline/handline/go/internal/browser"
	"github.com/handline/handline/go/internal/hlclient"
)

// browserCmd is hl browser: the agent's own Chrome, headless unless --show, its one page driven by each command.
func browserCmd(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 0 {
		return wrongArgs("name what to do")
	}
	agent := c.CurrentAgent()
	if agent == "" {
		fmt.Fprintln(os.Stderr, "hl browser runs in an agent's session: $HANDLINE_AGENT names whose browser it is")
		return 1, true
	}
	dir := browser.AgentDir(c.Home, agent)
	verb, rest := args[0], args[1:]
	if verb == "close" {
		if len(rest) != 0 {
			return wrongArgs("close takes nothing")
		}
		if browser.Running(dir) != nil {
			if err := act(c, "close", nil); err != nil {
				fmt.Fprintln(os.Stderr, "hl browser close: "+err.Error())
				return 1, true
			}
		}
		if !browser.Close(dir) {
			fmt.Println("no browser was open")
		}
		return 0, true
	}
	var p parsed
	var ok bool
	switch verb {
	case "open":
		p, ok = parse(rest, nil, []string{"--show"}, nil)
		ok = ok && len(p.pos) == 1
	case "read":
		p, ok = parse(rest, nil, []string{"--tree"}, nil)
		ok = ok && len(p.pos) == 0
	case "click":
		p, ok = parse(rest, nil, nil, nil)
		ok = ok && len(p.pos) == 1
	case "type":
		p, ok = parse(rest, nil, []string{"--enter"}, nil)
		ok = ok && len(p.pos) == 2
	case "wait":
		p, ok = parse(rest, []string{"--text", "--timeout"}, nil, nil)
		text, hasText := p.opt("--text")
		ok = ok && (len(p.pos) == 1) != hasText && len(p.pos) <= 1 && !(hasText && text == "")
	case "eval":
		p, ok = parse(rest, nil, nil, nil)
		ok = ok && len(p.pos) == 1
	case "shot":
		p, ok = parse(rest, nil, []string{"--full"}, nil)
		ok = ok && len(p.pos) == 1
	default:
		return wrongArgs("%s is not one of open, read, click, type, wait, eval, shot, close", verb)
	}
	if !ok {
		return 0, false
	}
	within := 10 * time.Second
	if secs, set, good := optTimeout(p, "--timeout"); !good {
		return 0, false
	} else if set {
		within = time.Duration(secs) * time.Second
	}
	if err := browse(c, dir, verb, p, within); err != nil {
		fmt.Fprintln(os.Stderr, "hl browser "+verb+": "+err.Error())
		return 1, true
	}
	return 0, true
}

// act tells the daemon the agent is about to do kind on its page, at box when it has one: an error only when a person holds the page. A
// person may take the page between the answer and the verb; the verb then lands once, as one already under way.
func act(c *hlclient.Client, kind string, box *browser.Box) error {
	if box == nil {
		return screenAct(c, "browser", kind, nil)
	}
	return screenAct(c, "browser", kind, &[4]float64{box.X, box.Y, box.W, box.H})
}

// screenAct is screen.act on the agent's source: an error only when a person holds it. A daemon that cannot be asked holds nothing.
func screenAct(c *hlclient.Client, source, kind string, box *[4]float64) error {
	params := map[string]any{"source": source, "kind": kind}
	if box != nil {
		params["box"] = map[string]any{"x": box[0], "y": box[1], "width": box[2], "height": box[3]}
	}
	_, err := c.Call("screen.act", params)
	var refused *hlclient.RPCError
	if errors.As(err, &refused) && refused.Code == "refused" {
		return refused
	}
	return nil
}

func browse(c *hlclient.Client, dir, verb string, p parsed, within time.Duration) error {
	var b *browser.Browser
	var err error
	if verb == "open" {
		if b = browser.Running(dir); b == nil || b.Show != p.flags["--show"] { // the engine is looked up, or fetched, only to start one
			engine, eerr := browser.Engine(c.Home)
			if eerr != nil {
				return eerr
			}
			b, err = browser.Ensure(dir, engine, p.flags["--show"])
		}
	} else {
		b, err = browser.Opened(dir)
	}
	if err != nil {
		return err
	}
	if err := act(c, verb, nil); err != nil {
		return err
	}
	page, err := b.Page()
	if err != nil {
		return err
	}
	defer page.Close()
	out := ""
	switch verb {
	case "open":
		out, err = page.Open(p.pos[0])
	case "read":
		if p.flags["--tree"] {
			out, err = page.Tree()
		} else {
			out, err = page.Text()
		}
	case "click", "type":
		var box browser.Box
		if box, err = page.Box(p.pos[0]); err != nil {
			break
		}
		if err = act(c, verb, &box); err != nil {
			break
		}
		if verb == "click" {
			err = page.ClickAt(box)
		} else {
			err = page.TypeAt(box, p.pos[1], p.flags["--enter"])
		}
	case "wait":
		target := ""
		if len(p.pos) == 1 {
			target = p.pos[0]
		}
		text, _ := p.opt("--text")
		err = page.Wait(target, text, within)
	case "eval":
		out, err = page.Eval(p.pos[0])
	case "shot":
		var file string
		if file, err = filepath.Abs(p.pos[0]); err == nil {
			if err = page.Shot(file, p.flags["--full"]); err == nil {
				out = file
			}
		}
	}
	if err == nil && out != "" {
		fmt.Println(out)
	}
	return err
}
