package reads

import (
	"database/sql"
	"errors"
	"slices"

	"github.com/handline/handline/go/internal/transcript"
	"github.com/handline/handline/go/scheduler/session"
)

// toolItems are agent's tool calls and thinking blocks as chat.transcript items: a call with its output, the file change its result made
// and whether it still runs; a step a subagent made names the item of the call that ran it. turn is whether the agent's turn runs, nil to
// read it.
func (e *Env) toolItems(agent string, tools []transcript.Tool, turn *bool) ([]map[string]any, error) {
	run, err := e.runningOf(agent, tools, turn)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		item := map[string]any{"id": t.ID, "at": ts(t.At), "kind": "tool", "tool": t.Name, "text": t.Text}
		if t.Thinking {
			item = map[string]any{"id": t.ID, "at": ts(t.At), "kind": "thinking", "text": t.Text}
		}
		if t.Parent != 0 {
			item["parent"] = t.Parent
		}
		if t.Output != nil {
			item["output"] = *t.Output
			if t.Dropped > 0 {
				item["dropped_lines"] = t.Dropped
			}
		}
		if t.Diff != nil {
			item["diff"], item["added"], item["removed"] = t.Diff, t.Added, t.Removed
		}
		if !t.Thinking && run.running(t) {
			item["running"] = true
		}
		items = append(items, item)
	}
	return items, nil
}

// running is what says whether a tool call still runs: whether the agent's turn runs and when it began, and the calls whose background
// job goes on.
type running struct {
	turn       bool
	began      int64 // the agent's newest prompt or turn end: a call before it is of a turn that is over
	background map[string]bool
}

// running is whether call t still runs: a call with no result yet in the turn that is going on, or a Bash call started in the background
// while its job is in the agent's background list.
func (r running) running(t transcript.Tool) bool {
	if t.Background && r.background[t.UseID] {
		return true
	}
	return t.Output == nil && r.turn && t.At >= r.began
}

// turnBeganSQL is when agent's turn began: its newest prompt or turn end (an interrupt writes no Stop), read back from its newest event.
const turnBeganSQL = "SELECT ts FROM events WHERE agent = ? AND kind IN ('UserPromptSubmit', 'Stop') ORDER BY id DESC LIMIT 1"

func (e *Env) runningOf(agent string, tools []transcript.Tool, turn *bool) (running, error) {
	r := running{background: map[string]bool{}}
	if turn == nil {
		w, err := e.working(agent)
		if err != nil {
			return running{}, err
		}
		r.turn = w["turn_running"] == true
	} else {
		r.turn = *turn
	}
	if r.turn && slices.ContainsFunc(tools, func(t transcript.Tool) bool { return !t.Thinking && t.Output == nil }) {
		if err := e.DB.QueryRow(turnBeganSQL, agent).Scan(&r.began); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return running{}, err
		}
	}
	if e.Delivery == nil || !slices.ContainsFunc(tools, func(t transcript.Tool) bool { return t.Background }) {
		return r, nil
	}
	has, err := e.Delivery.HasBackgroundWork(e.DB, agent)
	if err != nil {
		return running{}, err
	}
	shells, err := (&session.Env{DB: e.DB, Home: e.Home}).BackgroundShells(agent, has)
	if err != nil {
		return running{}, err
	}
	for _, j := range shells {
		if j.ToolUseID != "" {
			r.background[j.ToolUseID] = true
		}
	}
	return r, nil
}

// RecentToolsKept is how many of an agent's newest tool and thinking items RecentTools reads.
const RecentToolsKept = 50

// RecentTools are agent's newest tool and thinking items, oldest first, as chat.transcript gives them, for the live event that tells a
// call that stopped running; with from above 0, also every item from that id's second on, up to a page, so an older call still told
// running is read again. turn is whether the agent's turn runs.
func (e *Env) RecentTools(agent string, turn bool, from int64) ([]map[string]any, error) {
	tools, err := transcript.Tools(e.DB, agent, transcript.ToolWindow{Limit: RecentToolsKept})
	if err != nil {
		return nil, err
	}
	if from > 0 && (len(tools) == 0 || tools[0].ID > from) {
		older, err := transcript.Tools(e.DB, agent, transcript.ToolWindow{Since: from, Limit: maxLimit})
		if err != nil {
			return nil, err
		}
		have := map[int64]bool{}
		for _, t := range tools {
			have[t.ID] = true
		}
		for _, t := range older {
			if !have[t.ID] {
				tools = append(tools, t)
			}
		}
	}
	return e.toolItems(agent, tools, &turn)
}
