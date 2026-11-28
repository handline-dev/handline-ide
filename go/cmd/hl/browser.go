package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/handline/handline/go/internal/browser"
	"github.com/handline/handline/go/internal/hlclient"
)

// browserVerb is the command line one verb takes: its valued options, its flags, and how many positionals.
type browserVerb struct {
	valued, flags []string
	least, most   int
}

var browserVerbs = map[string]browserVerb{
	"open": {nil, []string{"--show"}, 1, 1}, "read": {nil, []string{"--tree"}, 0, 0},
	"click": {nil, nil, 1, 1}, "type": {nil, []string{"--enter"}, 2, 2}, "press": {nil, nil, 1, 1},
	"hover": {nil, nil, 1, 1}, "drag": {nil, nil, 2, 2}, "select": {nil, nil, 2, 2},
	"scroll": {[]string{"--in"}, nil, 1, 2}, "wait": {[]string{"--text", "--timeout"}, nil, 0, 1},
	"back": {}, "forward": {}, "reload": {}, "tab": {nil, nil, 0, 2}, "viewport": {nil, nil, 2, 2},
	"upload": {nil, nil, 2, -1}, "download": {[]string{"--timeout"}, nil, 1, 2}, "dialog": {nil, nil, 1, 2},
	"console": {}, "network": {}, "eval": {nil, nil, 1, 1}, "shot": {nil, []string{"--full"}, 1, 1},
}

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
	if verb == keepVerb && len(rest) == 3 {
		return keep(dir, rest)
	}
	v, known := browserVerbs[verb]
	if !known {
		names := append(slices.Sorted(maps.Keys(browserVerbs)), "close")
		return wrongArgs("%s is not one of %s", verb, strings.Join(names, ", "))
	}
	p, ok := parse(rest, v.valued, v.flags, nil)
	if !ok || len(p.pos) < v.least || (v.most >= 0 && len(p.pos) > v.most) || !browserArgsFit(verb, p) {
		return 0, false
	}
	within := 10 * time.Second
	if verb == "download" {
		within = time.Minute
	}
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

// keepVerb is the process holding a tab's size (browser.Page.Resize starts it); it is no verb of an agent's.
const keepVerb = "keep-size"

func keep(dir string, args []string) (int, bool) {
	w, werr := atoi(args[1])
	h, herr := atoi(args[2])
	b := browser.Running(dir)
	if werr != nil || herr != nil || b == nil {
		fmt.Fprintln(os.Stderr, "no browser runs, or the size is not two numbers")
		return 1, true
	}
	if err := b.Keep(args[0], w, h, func() { fmt.Println("holding") }); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1, true
	}
	return 0, true
}

// browserArgsFit checks what the table cannot: which words a verb's positionals are.
func browserArgsFit(verb string, p parsed) bool {
	switch verb {
	case "wait":
		text, hasText := p.opt("--text")
		return (len(p.pos) == 1) != hasText && !(hasText && text == "")
	case "scroll":
		if _, ok := scrollWays[p.pos[0]]; !ok {
			argProblem = "scroll goes up, down, left or right"
			return false
		}
		return len(p.pos) == 1 || positive(p.pos[1])
	case "tab":
		switch {
		case len(p.pos) == 0:
		case p.pos[0] == "new":
		case p.pos[0] == "close" && len(p.pos) == 2:
			return positive(p.pos[1])
		case len(p.pos) == 1:
			return positive(p.pos[0])
		default:
			return false
		}
	case "viewport":
		return positive(p.pos[0]) && positive(p.pos[1])
	case "dialog":
		return p.pos[0] == "accept" || (p.pos[0] == "dismiss" && len(p.pos) == 1)
	}
	return true
}

func positive(s string) bool {
	n, err := atoi(s)
	return err == nil && n > 0
}

// act tells the daemon. A person may take the page between the answer and the verb; the verb then lands once, as one already under way.
// act tells the daemon the agent is about to do kind on its page, at box when it has one: an error only when a person holds the page. A
// daemon that cannot be asked holds nothing.
func act(c *hlclient.Client, kind string, box *browser.Box) error {
	params := map[string]any{"source": "browser", "kind": kind}
	if box != nil {
		params["box"] = map[string]any{"x": box.X, "y": box.Y, "width": box.W, "height": box.H}
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
	switch verb { // what the browser does before the page is reached
	case "tab":
		if out, done, err := tabs(b, p); err != nil || done {
			if err == nil {
				fmt.Println(out)
			}
			return err
		}
	case "dialog":
		if err := b.SetDialogs(p.pos[0] == "accept", strings.Join(p.pos[1:], "")); err != nil {
			return err
		}
	}
	page, err := b.Page()
	if err != nil {
		return err
	}
	defer page.Close()
	out, err := onPage(c, page, verb, p, within)
	if serr := page.Settle(); err == nil {
		err = serr
	}
	for _, a := range page.Asked {
		fmt.Println("the page asked: " + a)
	}
	if len(page.Asked) > 0 && verb != "dialog" {
		fmt.Println("(hl browser dialog accept [<reply>] or dismiss sets how the next ones are answered)")
	}
	if err == nil && out != "" {
		fmt.Println(out)
	}
	return err
}
