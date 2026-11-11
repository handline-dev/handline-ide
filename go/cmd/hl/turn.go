package main

import (
	"fmt"

	"github.com/handline/handline/go/internal/hlclient"
)

// The turns on a task (docs/design/turns.md): hl task comment ... --turn puts an agent, or the founder, on top of a task you act on; hl task
// pass and hl task back take your turn off, and the task goes back to the one below.

// taskTurnGone is `hl task turn`, which is gone: a turn is handed over with the comment that says why.
func taskTurnGone(_ *hlclient.Client, args []string) (int, bool) {
	t := "T<n>"
	if len(args) > 0 {
		t = args[0]
	}
	fmt.Println("hl task turn is gone: hand the turn over with the comment that says why, in one step:\n" +
		"  hl task comment " + t + " add '<the decision and what you recommend>' --turn human\n" +
		"  hl task comment " + t + " add '<what to review>' --turn <agent> [--done '<evidence>']")
	return 1, true
}

// taskPop is `hl task pass T<n> ["<note>"]` and `hl task back T<n> "<note>"`: task.turn.pop with that outcome.
func taskPop(outcome string) taskCmd {
	return func(c *hlclient.Client, args []string) (int, bool) {
		p, ok := parse(args, nil, nil, nil)
		if !ok || len(p.pos) < 1 || len(p.pos) > 2 || outcome == "send_back" && len(p.pos) != 2 {
			return 0, false
		}
		tid, ok := taskRef(p.pos[0])
		if !ok {
			return 0, false
		}
		sub := map[string]string{"pass": "pass", "send_back": "back"}[outcome]
		mutating(c, sub)
		if !asking(c, "end a turn") {
			return 1, true
		}
		params := map[string]any{"task": tid, "outcome": outcome}
		if len(p.pos) == 2 {
			params["note"] = p.pos[1]
		}
		return said(c.Write("task.turn.pop", params)), true
	}
}
