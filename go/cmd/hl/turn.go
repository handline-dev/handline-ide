package main

import (
	"github.com/handline/handline/go/internal/hlclient"
)

// The turns on a task (docs/design/turns.md): hl task turn puts an agent, or the founder, on top of a task you act on; hl task pass and hl task
// back take your turn off, and the task goes back to the one below.

// taskTurn is `hl task turn T<n> <agent|human> "<ask>" [--done "<evidence>"]`: task.turn.push. --done makes a pass of that turn close the task.
func taskTurn(c *hlclient.Client, args []string) (int, bool) {
	p, ok := parse(args, []string{"--done"}, nil, nil)
	if !ok || len(p.pos) < 2 || len(p.pos) > 3 {
		return 0, false
	}
	tid, ok := taskRef(p.pos[0])
	if !ok {
		return 0, false
	}
	mutating(c, "turn")
	if !asking(c, "put a turn on a task") {
		return 1, true
	}
	params := map[string]any{"task": tid, "to": p.pos[1]}
	if len(p.pos) == 3 {
		params["ask"] = p.pos[2]
	}
	if evidence, given := p.opt("--done"); given {
		params["on_pass"], params["evidence"] = "done", evidence
	}
	return said(c.Write("task.turn.push", params)), true
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
