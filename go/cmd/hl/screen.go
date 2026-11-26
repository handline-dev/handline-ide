package main

import (
	"fmt"
	"os"

	"github.com/handline/handline/go/internal/hlclient"
)

// screenCmd is hl screen: `list` the on-screen windows, `shot --app <name> [--window <id>]` one window saved as a PNG. The daemon does
// the capture, so macOS asks for screen recording for Handline and not for the agent's shell.
func screenCmd(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	p, ok := parse(args[1:], []string{"--app", "--window"}, nil, nil)
	if !ok || len(p.pos) != 0 {
		return 0, false
	}
	params := map[string]any{"agent": hlclient.Agent()}
	switch args[0] {
	case "list":
		got, err := c.Call("screen.list", params)
		if err != nil {
			fmt.Fprintln(os.Stderr, "hl screen:", err)
			return 1, true
		}
		for _, w := range list(got, "windows") {
			fmt.Printf("%-7v %-24v %v\n", w["id"], w["app"], w["title"])
		}
		return 0, true
	case "shot":
		app, _ := p.opt("--app")
		win, _ := p.opt("--window")
		if app == "" && win == "" {
			return wrongArgs("name the app to capture: --app <name>")
		}
		if app != "" {
			params["app"] = app
		}
		if win != "" {
			var n int64
			if _, err := fmt.Sscan(win, &n); err != nil {
				return wrongArgs("--window is a number from hl screen list")
			}
			params["window"] = n
		}
		got, err := c.Call("screen.shot", params)
		if err != nil {
			fmt.Fprintln(os.Stderr, "hl screen:", err)
			return 1, true
		}
		fmt.Println(got["path"])
		return 0, true
	}
	return 0, false
}
