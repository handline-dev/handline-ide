package main

import (
	"fmt"
	"strings"

	"github.com/handline/handline/go/internal/browser"
	"github.com/handline/handline/go/internal/hlclient"
)

// browserList is hl browser list: what the agent's browser shows and who has it, read without acting on it, so a person holding the page
// does not refuse it and watchers are shown nothing. A browser that is not open is not started; reading the tabs only renumbers them, as
// hl browser tab does.
func browserList(c *hlclient.Client, dir string) error {
	b := browser.Running(dir)
	if b == nil {
		fmt.Println("no browser open")
		return nil
	}
	tabs, err := b.TabList()
	if err != nil {
		return fmt.Errorf("the tabs could not be read: %w", err)
	}
	st, serr := c.Call("screen.status", map[string]any{"source": "browser"})
	head := "Browser: headless"
	if b.Show {
		head = "Browser: shown"
	}
	if serr == nil {
		on := "this Mac"
		if m, _ := st["machine"].(string); m != "" {
			on = m
		}
		head += ", on " + on
	}
	fmt.Println(head)
	for _, t := range tabs {
		mark := " "
		if t.Current {
			mark = "*"
		}
		fmt.Printf("%s %d  %s  %s\n", mark, t.N, t.Title, t.URL)
	}
	if serr != nil {
		return fmt.Errorf("where it runs and who has it are not known: %w", serr)
	}
	holder, _ := st["holder"].(map[string]any)
	if kind, _ := holder["kind"].(string); kind != "" && kind != "agent" {
		if name, _ := holder["name"].(string); name != "" {
			fmt.Println(name + " has the page")
		}
		return nil
	}
	var watchers []string
	list, _ := st["watchers"].([]any)
	for _, w := range list {
		if n, ok := w.(string); ok {
			watchers = append(watchers, n)
		}
	}
	if len(watchers) > 0 {
		fmt.Println("watched by " + strings.Join(watchers, ", "))
	}
	return nil
}
