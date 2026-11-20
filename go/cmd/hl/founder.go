package main

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/textsafe"
)

func line(v any) string { return textsafe.Oneline(fmt.Sprint(v)) }

// The founder's own verbs left in hl, each for what no app can do yet.

// device is `hl device pair`: the code that pairs a first device when no paired app can make one, on the host --host names (the
// daemon's default without it). The daemon refuses it to an agent session. Listing and revoking devices are the app's.
func device(c *hlclient.Client, args []string) (int, bool) {
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

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}
