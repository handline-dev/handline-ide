package main

import (
	"fmt"
	"os"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/textsafe"
)

// prefs prints the rules of the person the caller works for, in the order they were kept, and with an agent named, that agent's own
// lessons: `hl prefs [agent]`, for a reviewer subagent to check an agent's work against.
func prefs(c *hlclient.Client, args []string) (int, bool) {
	if len(args) > 1 || !plain(args) {
		return 0, false
	}
	if len(args) == 1 && (args[0] == "founder" || args[0] == "human") {
		return wrongArgs("hl prefs prints the founder's rules; name an agent to add its lessons")
	}
	lists := []struct{ agent, head, none string }{{"human", "# The founder's preferences", "no rules"}}
	if len(args) == 1 {
		lists = append(lists, struct{ agent, head, none string }{args[0], "# What " + args[0] + " learned", "no lessons"})
	}
	for _, l := range lists {
		got, err := c.Call("lessons.list", map[string]any{"agent": l.agent})
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not read the lessons: %v\n", err)
			return 1, true
		}
		items, _ := got["items"].([]any)
		if len(lists) > 1 {
			fmt.Println(l.head)
		}
		if len(items) == 0 {
			fmt.Println(l.none)
		}
		for _, it := range items {
			m, _ := it.(map[string]any)
			fmt.Println("- " + textsafe.Text(fmt.Sprint(m["text"])))
		}
	}
	return 0, true
}
