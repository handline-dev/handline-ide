package main

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"runtime"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/sysres"
)

func line(v any) string { return textsafe.Oneline(fmt.Sprint(v)) }

// The founder's own verbs left in hl, each for what no app can do yet.

// device is `hl device pair`: the code that pairs a first device when no paired app can make one, on the host --host names (the
// daemon's default without it). The daemon refuses it to an agent session. Listing and revoking devices are the app's.
func device(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 2 && args[0] == "join" {
		return join(c.Home, args[1]), true
	}
	if len(args) == 0 || args[0] != "pair" {
		return 0, false
	}
	p, ok := parse(args[1:], []string{"--name", "--host"}, nil, nil)
	if !ok || len(p.pos) != 0 {
		return 0, false
	}
	params := map[string]any{}
	if name, _ := p.opt("--name"); name != "" {
		params["name"] = name
	}
	host, _ := p.opt("--host")
	if host != "" {
		params["address"] = host
	}
	ok, said, got := c.Written("device.pair", params)
	if !ok {
		fmt.Fprintln(os.Stderr, said)
		if host != "" {
			printAddresses(c, os.Stderr, "")
		}
		return 1, true
	}
	uri := line(got["uri"])
	fmt.Printf("code: %s   (one time, expires %s)\n", line(got["code"]), line(got["expires_at"]))
	fmt.Printf("uri: %s\n", uri)
	chosen := ""
	if u, err := url.Parse(uri); err == nil && u.Query().Get("host") != "" {
		addr := u.Query().Get("host")
		fmt.Printf("address: %s\n", line(addr))
		chosen = addr
		if h, _, err := net.SplitHostPort(addr); err == nil {
			chosen = h
		}
	}
	if fp := line(orEmpty(got["fingerprint"])); fp != "" {
		fmt.Printf("fingerprint: %s\n", fp)
	} else {
		fmt.Println("encryption: none (no certificate; plain HTTP)")
	}
	printAddresses(c, os.Stdout, chosen)
	return 0, true
}

// printAddresses lists the hosts a pairing can name (those a door serves) with their kind and encryption, chosen marked with *.
// Nothing is printed when the daemon does not say.
func printAddresses(c *hlclient.Client, w io.Writer, chosen string) {
	got, err := c.Call("connection.addresses", nil)
	if err != nil {
		return
	}
	list, _ := got["addresses"].([]any)
	var rows []string
	for _, v := range list {
		a, _ := v.(map[string]any)
		if listening, _ := a["listening"].(bool); !listening {
			continue
		}
		host := line(a["host"])
		mark := " "
		if host == chosen {
			mark = "*"
		}
		rows = append(rows, fmt.Sprintf("%s %s  %s  %s", mark, host, line(a["kind"]), line(a["encryption"])))
	}
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(w, "usable addresses (--host):")
	for _, r := range rows {
		fmt.Fprintln(w, r)
	}
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
	n, _ := rpc.AsInt(got["number"])
	if handed, _ := got["handed_over"].(bool); handed {
		fmt.Printf("this machine is the master now (number %d): the old master handed over and joins it as a runner\n", n)
	} else {
		fmt.Printf("this machine is the master now (number %d), from its copy of the board: the old master was not reached, and joins it as a runner once it is\n", n)
	}
	return 0, true
}

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}
