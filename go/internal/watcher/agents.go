package watcher

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/handline/handline/go/agentctl/dialogs"
	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/internal/reads"
)

// Agents is the agents topic: a changed agent_view row is agent.changed (the view as data), one that turned stalled also says so on the
// notifications topic, and a row gone is agent.removed. It reads the table whoever wrote the row; a view carries the agent's name,
// workspace and owner, so a rename is agent.changed too. Its first look says nothing.
type Agents struct {
	seen  map[string]string
	query string // tests: a read that fails part way
}

func (*Agents) Name() string { return "agents" }

func (*Agents) Tables() []string { return []string{"agent_view", "agents"} }

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
	ids, err := reads.Idents(q)
	if err != nil {
		return err
	}
	rows, err := q.Query(cmp.Or(a.query, "SELECT agent, view FROM agent_view"))
	if err != nil {
		return err
	}
	defer rows.Close()
	now := map[string]string{}
	for rows.Next() {
		var name, view string
		if err := rows.Scan(&name, &view); err != nil {
			return err
		}
		wire, err := withIdent(name, view, ids)
		if err != nil {
			return fmt.Errorf("agent %s: view: %w", name, err) // as agents.list refuses it
		}
		now[name] = wire
	}
	if err := rows.Err(); err != nil { // a read that stopped short is not a list with agents gone
		return err
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

// withIdent is the stored view of the agent whose key is key as the wire has it (reads.WithIdent).
func withIdent(key, view string, ids map[string]reads.Ident) (string, error) {
	var v map[string]any
	dec := json.NewDecoder(strings.NewReader(view))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	reads.WithIdent(v, key, ids)
	b, err := json.Marshal(v)
	return string(b), err
}
