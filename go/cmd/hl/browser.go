package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/handline/handline/go/internal/browser"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/paths"
)

// browserDir is the agent's own browser: its profile and what runs, so two agents never share one.
func browserDir(home, agent string) string {
	return filepath.Join(paths.AgentDir(home, agent), "browser")
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
	dir := browserDir(c.Home, agent)
	verb, rest := args[0], args[1:]
	if verb == "close" {
		if len(rest) != 0 {
			return wrongArgs("close takes nothing")
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
	if err := browse(c.Home, dir, verb, p, within); err != nil {
		fmt.Fprintln(os.Stderr, "hl browser "+verb+": "+err.Error())
		return 1, true
	}
	return 0, true
}

func browse(home, dir, verb string, p parsed, within time.Duration) error {
	var b *browser.Browser
	var err error
	if verb == "open" {
		engine, eerr := browser.Engine(home)
		if eerr != nil {
			return eerr
		}
		b, err = browser.Ensure(dir, engine, p.flags["--show"])
	} else {
		b, err = browser.Opened(dir)
	}
	if err != nil {
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
	case "click":
		err = page.Click(p.pos[0])
	case "type":
		err = page.Type(p.pos[0], p.pos[1], p.flags["--enter"])
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
