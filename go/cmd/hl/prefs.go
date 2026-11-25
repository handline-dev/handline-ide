package main

import (
	"fmt"
	"os"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/textsafe"
)

// prefs prints the rules of the person the caller works for, in the order they were kept: `hl prefs [--area screen|work]`, for a
// reviewer subagent to check work against. Without --area, both areas, each under its heading.
func prefs(c *hlclient.Client, args []string) (int, bool) {
	p, ok := parse(args, []string{"--area"}, nil, nil)
	if !ok || len(p.pos) != 0 {
		return 0, false
	}
	areas := []string{"work", "screen"}
	if a, given := p.opt("--area"); given {
		if a != "work" && a != "screen" {
			return wrongArgs("--area is screen or work")
		}
		areas = []string{a}
	}
	got, err := c.Call("lessons.list", map[string]any{"agent": "human"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read the rules: %v\n", err)
		return 1, true
	}
	items, _ := got["items"].([]any)
	heads := map[string]string{"work": "# Work", "screen": "# Screen"}
	for _, area := range areas {
		var lines []string
		for _, it := range items {
			l, _ := it.(map[string]any)
			if a, _ := l["area"].(string); a == area || a == "" && area == "work" {
				lines = append(lines, "- "+textsafe.Text(fmt.Sprint(l["text"])))
			}
		}
		if len(areas) > 1 {
			fmt.Println(heads[area])
		}
		if len(lines) == 0 {
			fmt.Println("no " + area + " rules")
		}
		for _, line := range lines {
			fmt.Println(line)
		}
	}
	return 0, true
}
