package main

import (
	"fmt"
	"os"
	"time"

	"github.com/handline/handline/go/internal/hlclient"
)

// desk lists the worktree pool's desks and pins one: `hl desk list [<repo>]`, `hl desk pin <desk> <agent|founder>`, `hl desk unpin <desk>`.
func desk(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 0 || !plain(args) {
		return 0, false
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "list":
		if len(rest) > 1 {
			return 0, false
		}
		params := map[string]any{}
		if len(rest) == 1 {
			params["repo"] = rest[0]
		}
		got, err := c.Call("desks.list", params)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not read the desks: %v\n", err)
			return 1, true
		}
		desks, _ := got["desks"].([]any)
		if len(desks) == 0 {
			fmt.Println("no desks")
			return 0, true
		}
		for _, it := range desks {
			d, _ := it.(map[string]any)
			holder := fmt.Sprint(d["holder"])
			if holder == "" {
				holder = "free"
			}
			line := fmt.Sprintf("%-22s %-14s last %s", d["name"], holder, lastUse(d))
			if pin := fmt.Sprint(d["pinned_to"]); pin != "" {
				line += "  pinned to " + pin
			}
			fmt.Printf("%s\n    %s\n", line, d["path"])
		}
		return 0, true
	case "pin", "unpin":
		params := map[string]any{}
		switch {
		case sub == "pin" && len(rest) == 2:
			params["desk"], params["to"] = rest[0], rest[1]
			if rest[1] == "founder" {
				params["to"] = "human" // the word the daemon reads as the founder
			}
		case sub == "unpin" && len(rest) == 1:
			params["desk"] = rest[0]
		default:
			return 0, false
		}
		c.NoteVerbChain([]string{"desk", sub})
		if !asking(c, sub+" a desk") {
			return 1, true
		}
		return said(c.Write("desk.pin", params)), true
	}
	return 0, false
}

func lastUse(d map[string]any) string {
	who := fmt.Sprint(d["last_agent"])
	if who == "" {
		who = "nobody"
	}
	n, ok := d["last_used"].(float64)
	if !ok || n == 0 {
		return who
	}
	return who + " " + time.Unix(int64(n), 0).Format("01-02 15:04")
}
