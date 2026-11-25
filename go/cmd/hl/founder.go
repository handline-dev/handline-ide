package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/scheduler/sysres"
)

func line(v any) string { return textsafe.Oneline(fmt.Sprint(v)) }

// The founder's own verbs left in hl, each for what no app can do yet.

// device is `hl device pair`: the code that pairs a first device when no paired app can make one, and its link, on the address the
// daemon picks. The daemon refuses it to an agent session. Listing and revoking devices are the app's.
func device(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 2 && args[0] == "join" {
		return join(c.Home, args[1]), true
	}
	if len(args) == 0 || args[0] != "pair" {
		return 0, false
	}
	p, ok := parse(args[1:], []string{"--name"}, nil, nil)
	if !ok || len(p.pos) != 0 {
		return 0, false
	}
	params := map[string]any{}
	if name, _ := p.opt("--name"); name != "" {
		params["name"] = name
	}
	ok, said, got := c.Written("device.pair", params)
	if !ok {
		fmt.Fprintln(os.Stderr, said)
		return 1, true
	}
	fmt.Printf("code: %s   (one time, expires %s)\n", line(got["code"]), line(got["expires_at"]))
	fmt.Printf("link: %s\n", line(got["web_link"]))
	return 0, true
}

// join is `hl device join <link>`, run in a machine's own terminal: it redeems the link's code at the master as this machine (a mac,
// or a server for anything else), the device of the person who made the code, on Off. The daemon here sets its board aside at its
// next start.
func join(home, link string) int {
	node, err := joined.NodeID(home, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	kind := "server"
	if runtime.GOOS == "darwin" {
		kind = "mac"
	}
	name := sysres.MachineName()
	m, err := hlclient.Join(home, link, name, kind, node)
	if err != nil {
		fmt.Fprintln(os.Stderr, "join refused:", err)
		return 1
	}
	if m.Fingerprint == "" {
		fmt.Println("encryption: none (the link names no certificate; plain HTTP)")
	}
	fmt.Printf("joined %s as %s (%s, device %s), on Off: its person sets Run in Devices\n", line(m.Addr), line(name), kind, line(m.Device))
	fmt.Println("restart the daemon here: its own board is set aside, unchanged, and a new one opens")
	return 0
}

// nodeCmd is `hl node promote`, "Make this Mac the master", run on one of the owner's machines that joined: with the master reached it
// hands over and nothing is lost; without it this machine takes over from its copy of the board.
func nodeCmd(c *hlclient.Client, args []string) (int, bool) {
	if len(args) != 1 || args[0] != "promote" {
		return 0, false
	}
	got, err := c.Call("node.promote", map[string]any{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "not made the master:", err)
		return 1, true
	}
	n, _ := got["number"].(float64)
	if handed, _ := got["handed_over"].(bool); handed {
		fmt.Printf("this machine is the master now (number %.0f): the old master handed over and joins it as a runner\n", n)
	} else {
		fmt.Printf("this machine is the master now (number %.0f), from its copy of the board: the old master was not reached, and joins it as a runner once it is\n", n)
	}
	return 0, true
}

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}
