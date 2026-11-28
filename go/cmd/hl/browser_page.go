package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/internal/browser"
	"github.com/handline/handline/go/internal/hlclient"
)

func atoi(s string) (int, error) { return strconv.Atoi(s) }

// scrollWays is each way scroll goes, as the wheel turns for it.
var scrollWays = map[string][2]float64{"down": {0, 1}, "up": {0, -1}, "right": {1, 0}, "left": {-1, 0}}

// onPage does verb on the page and says what it prints.
func onPage(c *hlclient.Client, page *browser.Page, verb string, p parsed, within time.Duration) (string, error) {
	// boxed is a verb on a target: its box is what watchers see, and the hold is asked again with it.
	boxed := func(target string, do func(browser.Box) error) error {
		box, err := page.Box(target)
		if err == nil {
			err = act(c, verb, &box)
		}
		if err == nil {
			err = do(box)
		}
		return err
	}
	var out string
	var err error
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
		err = boxed(p.pos[0], page.ClickAt)
	case "type":
		err = boxed(p.pos[0], func(b browser.Box) error { return page.TypeAt(b, p.pos[1], p.flags["--enter"]) })
	case "hover":
		err = boxed(p.pos[0], page.HoverAt)
	case "select":
		err = boxed(p.pos[0], func(browser.Box) (err error) { out, err = page.Select(p.pos[0], p.pos[1]); return })
	case "drag":
		var from, onto browser.Box
		if from, onto, err = page.DragTo(p.pos[0], p.pos[1]); err == nil {
			if err = act(c, verb, &from); err == nil {
				if err = act(c, verb, &onto); err == nil {
					err = page.DragAt(from, onto)
				}
			}
		}
	case "scroll":
		out, err = scroll(c, page, p)
	case "press":
		err = page.Press(p.pos[0])
	case "wait":
		target := ""
		if len(p.pos) == 1 {
			target = p.pos[0]
		}
		text, _ := p.opt("--text")
		err = page.Wait(target, text, within)
	case "back":
		out, err = page.Go(-1)
	case "forward":
		out, err = page.Go(1)
	case "reload":
		out, err = page.Reload()
	case "tab": // tab new <url>: the new tab is the page now
		out, err = page.Open(p.pos[1])
	case "viewport":
		w, _ := atoi(p.pos[0])
		h, _ := atoi(p.pos[1])
		var exe string
		if exe, err = os.Executable(); err == nil {
			err = page.Resize(w, h, exe, "browser", keepVerb)
		}
	case "upload":
		var files []string
		if files, err = inWorktree(p.pos[1:]); err == nil {
			err = page.Upload(p.pos[0], files)
		}
	case "download":
		to := "."
		if len(p.pos) == 2 {
			to = p.pos[1]
		}
		var dirs []string
		if dirs, err = inWorktree([]string{to}); err == nil {
			err = boxed(p.pos[0], func(b browser.Box) (err error) { out, err = page.Download(b, dirs[0], within); return })
		}
	case "dialog":
		out = "the page's dialogs are " + map[bool]string{true: "accepted", false: "dismissed"}[p.pos[0] == "accept"] + " from now on"
	case "console":
		out, err = page.Console()
	case "network":
		out, err = page.Network()
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
	return out, err
}

func scroll(c *hlclient.Client, page *browser.Page, p parsed) (string, error) {
	var at browser.Box
	var err error
	if in, ok := p.opt("--in"); ok {
		at, err = page.Box(in)
	} else {
		at, err = page.Viewport()
	}
	if err != nil {
		return "", err
	}
	way := scrollWays[p.pos[0]]
	by := 0.8 * at.H // most of a screen, so what was at its edge stays in sight
	if way[0] != 0 {
		by = 0.8 * at.W
	}
	if len(p.pos) == 2 {
		n, _ := atoi(p.pos[1])
		by = float64(n)
	}
	if err := act(c, "scroll", &at); err != nil {
		return "", err
	}
	return page.ScrollAt(at, way[0]*by, way[1]*by)
}

// tabs does the tab verbs that need no page, and says whether that was all.
func tabs(b *browser.Browser, p parsed) (string, bool, error) {
	var err error
	switch {
	case len(p.pos) == 0:
	case p.pos[0] == "new":
		if err = b.TabNew(); err == nil && len(p.pos) == 2 {
			return "", false, nil
		}
	case p.pos[0] == "close":
		n, _ := atoi(p.pos[1])
		err = b.TabClose(n)
	default:
		n, _ := atoi(p.pos[0])
		err = b.TabTo(n)
	}
	if err != nil {
		return "", true, err
	}
	list, err := b.TabList()
	var out strings.Builder
	for _, t := range list {
		mark := " "
		if t.Current {
			mark = "*"
		}
		fmt.Fprintf(&out, "%s %d  %s  %s\n", mark, t.N, t.Title, t.URL)
	}
	return strings.TrimSuffix(out.String(), "\n"), true, err
}

// inWorktree is each path, which must be there, made whole; one outside the agent's worktree, the checkout it runs in, is refused.
func inWorktree(paths []string) ([]string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, errors.New("files go to and from the worktree: run it in your checkout")
	}
	top := strings.TrimSpace(string(out))
	if real, err := filepath.EvalSymlinks(top); err == nil {
		top = real
	}
	var whole []string
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("%s is not there", p)
		}
		if rel, err := filepath.Rel(top, real); err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return nil, fmt.Errorf("%s is outside the worktree %s", p, top)
		}
		whole = append(whole, real)
	}
	return whole, nil
}
