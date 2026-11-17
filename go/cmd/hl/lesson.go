package main

import (
	"fmt"
	"os"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/textsafe"
)

// lesson keeps, merges and lists what an agent learned: `hl lesson add "<text>" --task T<n>`, `hl lesson merge <ids> "<text>"`,
// `hl lesson list [agent]`.
func lesson(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "add":
		p, ok := parse(rest, []string{"--task"}, nil, nil)
		if !ok || len(p.pos) != 1 {
			return 0, false
		}
		v, given := p.opt("--task")
		tid, ok := taskRef(v)
		if !given || !ok {
			return wrongArgs("--task T<n> names the task it was learned on")
		}
		c.NoteVerbChain([]string{"lesson", "add"})
		if !asking(c, "keep a lesson") {
			return 1, true
		}
		return said(c.Write("lesson.add", map[string]any{"text": p.pos[0], "task": tid})), true
	case "merge":
		if len(rest) < 3 || !plain(rest) {
			return 0, false
		}
		var ids []any
		for _, a := range rest[:len(rest)-1] {
			n, ok := integer(a)
			if !ok {
				return wrongArgs("%q is not a lesson number (hl lesson list shows them)", a)
			}
			ids = append(ids, n)
		}
		c.NoteVerbChain([]string{"lesson", "merge"})
		if !asking(c, "merge lessons") {
			return 1, true
		}
		return said(c.Write("lesson.merge", map[string]any{"ids": ids, "text": rest[len(rest)-1]})), true
	case "list":
		if len(rest) > 1 || !plain(rest) {
			return 0, false
		}
		params := map[string]any{}
		if len(rest) == 1 {
			params["agent"] = rest[0]
		}
		got, err := c.Call("lessons.list", params)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not read the lessons: %v\n", err)
			return 1, true
		}
		items, _ := got["items"].([]any)
		if len(items) == 0 {
			fmt.Println("no lessons")
			return 0, true
		}
		for _, it := range items {
			l, _ := it.(map[string]any)
			line := fmt.Sprintf("%v  %s", l["id"], textsafe.Text(fmt.Sprint(l["text"])))
			if t, ok := l["task"]; ok && t != nil {
				line += fmt.Sprintf("  (T%v)", t)
			}
			fmt.Println(line)
		}
		fmt.Printf("%v of %v bytes\n", got["bytes"], got["cap"])
		return 0, true
	}
	return 0, false
}
