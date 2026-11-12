package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/textsafe"
)

// progress keeps or shows the agent's note on its request in progress (cli/task.py cmd_progress).
func progress(c *hlclient.Client, args []string) (int, bool) {
	params, args, ok := progressCount(args)
	if !ok || len(args) > 1 || !plain(args) || (params != nil && len(args) == 0) {
		return 0, false
	}
	c.NoteVerbChain([]string{"progress", ""})
	if c.CurrentAgent() == "" {
		why := "cannot tell who is writing: HANDLINE_AGENT is unset"
		if hlclient.Agent() != "" {
			why = hlclient.WhoRefusal("write")
		}
		fmt.Fprintln(os.Stderr, why)
		return 1, true
	}
	if len(args) == 0 {
		got, err := c.Call("progress.get", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not read the progress note: %v\n", err)
			return 1, true
		}
		note, _ := got["note"].(string)
		if note == "" {
			note = "no progress note"
		}
		fmt.Println(textsafe.Text(note))
		return 0, true
	}
	if params == nil {
		params = map[string]any{}
	}
	params["note"] = args[0]
	ok, said := c.Write("task.progress", params)
	if !ok {
		fmt.Fprintln(os.Stderr, said)
		return 1, true
	}
	fmt.Println(said)
	return 0, true
}

// progressCount takes --done N and --of M off args: both or neither, whole numbers; the daemon checks 1 <= total and done <= total.
func progressCount(args []string) (params map[string]any, rest []string, ok bool) {
	vals := map[string]int{}
	for i := 0; i < len(args); i++ {
		if a := args[i]; a == "--done" || a == "--of" {
			if i+1 == len(args) {
				return nil, nil, false
			}
			n, err := strconv.Atoi(args[i+1])
			if _, twice := vals[a]; err != nil || n < 0 || twice {
				return nil, nil, false
			}
			vals[a] = n
			i++
			continue
		}
		rest = append(rest, args[i])
	}
	if len(vals) == 0 {
		return nil, rest, true
	}
	if len(vals) != 2 {
		return nil, nil, false
	}
	return map[string]any{"done": vals["--done"], "total": vals["--of"]}, rest, true
}
