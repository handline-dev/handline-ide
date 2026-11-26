package main

import (
	"fmt"
	"os"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/textsafe"
)

// lesson keeps, merges and lists what an agent learned: `hl lesson add "<text>" --task T<n>`, `hl lesson merge <ids> "<text>"`,
// `hl lesson list [agent]`; with `--about founder --said <comment|msg>`, add and merge the founder's preferences.
func lesson(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "add":
		p, ok := parse(rest, []string{"--task", "--about", "--said"}, nil, nil)
		if !ok || len(p.pos) != 1 {
			return 0, false
		}
		v, given := p.opt("--task")
		tid, ok := taskRef(v)
		if !given || !ok {
			return wrongArgs("--task T<n> names the task it was learned on")
		}
		params, ok := about(p)
		if !ok {
			return wrongArgs("--about founder takes --said <comment number|msg_<hash>>: the founder's own words it comes from")
		}
		params["text"], params["task"] = p.pos[0], tid
		c.NoteVerbChain([]string{"lesson", "add"})
		if !asking(c, "keep a lesson") {
			return 1, true
		}
		return said(c.Write("lesson.add", params)), true
	case "merge":
		p, ok := parse(rest, []string{"--about", "--said"}, nil, nil)
		if !ok || len(p.pos) < 3 {
			return 0, false
		}
		params, ok := about(p)
		if !ok {
			return wrongArgs("--about founder takes --said <comment number|msg_<hash>>: the founder's own words it comes from")
		}
		var ids []any
		for _, a := range p.pos[:len(p.pos)-1] {
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
		params["ids"], params["text"] = ids, p.pos[len(p.pos)-1]
		return said(c.Write("lesson.merge", params)), true
	case "list":
		if len(rest) > 1 || !plain(rest) {
			return 0, false
		}
		params := map[string]any{}
		if len(rest) == 1 {
			params["agent"] = rest[0]
			if rest[0] == "founder" { // the caller's owner
				params["agent"] = "human"
			}
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
			fmt.Println(lessonLine(l))
		}
		fmt.Printf("%v of %v bytes\n", got["bytes"], got["cap"])
		return 0, true
	}
	return 0, false
}

// about is the params --about and --said give a lesson write: none for the agent's own; the founder's needs both (the daemon checks said).
func about(p parsed) (map[string]any, bool) {
	who, given := p.opt("--about")
	saidRef, hasSaid := p.opt("--said")
	if !given {
		return map[string]any{}, !hasSaid
	}
	if !hasSaid {
		return nil, false
	}
	var ref any = saidRef
	if n, ok := integer(saidRef); ok {
		ref = n
	}
	return map[string]any{"about": who, "said": ref}, true
}

// lessonLine is one lesson as hl lesson list shows it; a person's rule ends with how often it was said.
func lessonLine(l map[string]any) string {
	text := textsafe.Text(fmt.Sprint(l["text"]))
	line := fmt.Sprintf("%v  %s", l["id"], text)
	if n := num(l, "count"); n > 1 {
		line += fmt.Sprintf("  (said %d times)", n)
	}
	if t, ok := l["task"]; ok && t != nil {
		line += fmt.Sprintf("  (T%v)", t)
	}
	return line
}
