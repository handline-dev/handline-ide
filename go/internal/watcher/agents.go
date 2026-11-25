package watcher

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/handline/handline/go/agentctl/dialogs"
	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/internal/reads"
)

// Agents is the agents topic over the rows agents.list sends (reads.AgentRows): a changed row is agent.changed with the whole row as
// data, one that turned stalled also says so on the notifications topic, and an agent no longer listed is agent.removed. A client puts
// the row in place of its listed one, so it carries every field the list does: a bare stored view would drop the queue count and the
// rest read beside it until the list is read again. A row carries the agent's name, workspace and owner, so a rename is agent.changed
// too. Its first look says nothing.
type Agents struct {
	Reads *reads.Env
	seen  map[string]string
	query string // tests: a read that fails part way
}

func (*Agents) Name() string { return "agents" }

// Tables are those an agents.list row is read from that count their changes: the stored view, the roster, and the board its queue
// count, ready and founder fields come from (events: whether a task has begun), so a task that joins or leaves an agent's queue sends
// its row again. What no counted table holds shows at the next change of another: a role or review level (the app reads the list
// again after it sets one), and the wall and fire, which follow usage and the clock.
func (*Agents) Tables() []string {
	return []string{"agent_state", "agent_view", "agents", "cursors", "events", "meta", "requests", "task_blocks", "task_requests",
		"task_turns", "tasks", "workspaces"}
}

func stateOf(view string) string {
	var v struct {
		State string `json:"state"`
	}
	json.Unmarshal([]byte(view), &v)
	return v.State
}

type needs struct{ Kind, Title, Text, Source string }

// needsOf is the view's needs_you title, text and source, empty when it has none.
func needsOf(view string) (n needs) {
	var v struct {
		NeedsYou *struct {
			Kind, Title, Source string
			Text                *string
		} `json:"needs_you"`
	}
	if json.Unmarshal([]byte(view), &v) == nil && v.NeedsYou != nil {
		n.Kind, n.Title, n.Source = v.NeedsYou.Kind, v.NeedsYou.Title, v.NeedsYou.Source
		if v.NeedsYou.Text != nil {
			n.Text = *v.NeedsYou.Text
		}
	}
	return n
}

// stuck is whether a view is a stall the founder is told of: stalled, or waiting on a card the daemon raised (a dialog it will not or
// cannot get past, an agent it could not heal), which the view shows as waiting. An offer to switch account is no stall: nothing waits on it.
func stuck(view string) bool {
	n := needsOf(view)
	return stateOf(view) == "stalled" || (n.Source == "daemon" && n.Kind != dialogs.SwitchOffer)
}

func (a *Agents) Scan(q *sql.DB, emit Emit) error {
	rows, err := q.Query(cmp.Or(a.query, "SELECT agent, view FROM agent_view"))
	if err != nil {
		return err
	}
	defer rows.Close()
	views := map[string]json.RawMessage{}
	for rows.Next() {
		var name, view string
		if err := rows.Scan(&name, &view); err != nil {
			return err
		}
		views[name] = json.RawMessage(view)
	}
	if err := rows.Err(); err != nil { // a read that stopped short is not a list with agents gone
		return err
	}
	listed, err := a.Reads.AgentRows(views)
	if err != nil {
		return err // as agents.list refuses it
	}
	now := make(map[string]string, len(listed))
	for name, row := range listed {
		b, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("agent %s: %w", name, err)
		}
		now[name] = string(b)
	}
	before := a.seen
	a.seen = now
	if before == nil {
		return nil
	}
	names := make([]string, 0, len(now))
	for name := range now {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		view := now[name]
		if before[name] == view {
			continue
		}
		target := map[string]any{"screen": "agent", "id": name}
		if _, err := emit(stream.Entry{Topic: "agents", Type: "agent.changed", Data: json.RawMessage(view), Agent: name, Target: target}); err != nil {
			return err
		}
		if old, had := before[name]; stuck(view) && (!had || !stuck(old)) {
			title, data := name+" stalled", map[string]any{"agent": name}
			if n := needsOf(view); n.Title != "" {
				title, data["title"] = name+": "+n.Title, n.Title
				if n.Text != "" {
					data["text"] = n.Text
				}
			}
			if _, err := emit(stream.Entry{Topic: "notifications", Type: "stall", Data: data, Agent: name, Title: title, Target: target}); err != nil {
				return err
			}
		}
	}
	gone := make([]string, 0)
	for name := range before {
		if _, here := now[name]; !here {
			gone = append(gone, name)
		}
	}
	slices.Sort(gone)
	for _, name := range gone {
		if _, err := emit(stream.Entry{Topic: "agents", Type: "agent.removed", Data: map[string]any{"key": name}, Agent: name}); err != nil {
			return err
		}
	}
	return nil
}
