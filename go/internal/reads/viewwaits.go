package reads

import (
	"slices"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/status"
	"github.com/handline/handline/go/kit/actor"
)

// waitsOn is AgentView.waits_on: the first task an agent's held tasks all wait on, who holds it, and how many others they wait on.
type waitsOn struct {
	task   int64
	holder string // "" when no one holds it
	more   int
}

func (w *waitsOn) wire() any {
	if w == nil {
		return nil
	}
	var holder any
	if w.holder != "" {
		holder = w.holder
	}
	return map[string]any{"task": w.task, "holder": holder, "more": w.more}
}

// agentWaits is waits_on for every agent with a held task, all waiting: its open tasks that are not drafts, its current task, and the
// tasks it acts on as a turn. A task on another's turn waits on that turn; one it acts on waits on its open blockers, then its open
// children. A blocker the agent holds itself names no one: it is held too, so what it waits on is counted.
func (e *Env) agentWaits(open []row, b *board) (map[string]*waitsOn, error) {
	byID := make(map[int64]row, len(open))
	for _, t := range open {
		byID[t.i("id")] = t
	}
	on := map[int64][]int64{}
	blocks, err := query(e.DB, "SELECT b.task_id, b.blocks_on FROM task_blocks b JOIN tasks t ON t.id = b.blocks_on WHERE b.dormant = 0 AND "+
		"t.discarded_by IS NULL AND t.done_evidence IS NULL ORDER BY b.task_id, b.blocks_on")
	if err != nil {
		return nil, err
	}
	for _, k := range blocks {
		if w, x := k.i("task_id"), k.i("blocks_on"); byID[w] != nil && byID[x] != nil {
			on[w] = append(on[w], x)
		}
	}
	for _, t := range open { // priority then number, as db.OpenChildTasks
		if p := t.i("parent_task"); byID[p] != nil && !slices.Contains(on[p], t.i("id")) {
			on[p] = append(on[p], t.i("id"))
		}
	}
	held := map[string][]row{}
	for _, t := range open {
		owner, acting := t.str("owner"), actingOf(t)
		if t.i("id") == b.current[owner] || b.state(t) != "draft" {
			held[owner] = append(held[owner], t)
		}
		if acting != owner {
			held[acting] = append(held[acting], t)
		}
	}
	out := map[string]*waitsOn{}
	for agent, tasks := range held {
		if actor.IsPerson(agent) || agent == db.Gate {
			continue
		}
		if w := e.heldWaits(agent, tasks, on, byID, b); w != nil {
			out[agent] = w
		}
	}
	return out, nil
}

// heldWaits is agent's waits_on over its held tasks, nil when one of them does not wait or nothing past the agent itself is waited on.
func (e *Env) heldWaits(agent string, tasks []row, on map[int64][]int64, byID map[int64]row, b *board) *waitsOn {
	var first *waitsOn
	seen := map[int64]bool{}
	add := func(id int64, holder string) {
		if seen[id] {
			return
		}
		seen[id] = true
		if first == nil {
			first = &waitsOn{task: id, holder: holder}
		} else {
			first.more++
		}
	}
	for _, t := range tasks {
		if acting := actingOf(t); acting != agent {
			add(t.i("id"), holderOf(acting))
			continue
		}
		if len(on[t.i("id")]) == 0 {
			return nil
		}
		for _, x := range on[t.i("id")] {
			if h := e.blockerHolder(byID[x], b); h != agent {
				add(x, holderOf(h))
			}
		}
	}
	return first
}

// blockerHolder is who a blocker waits on: a draft with no turn on it waits for the founder to run it, any other task on who acts on it.
func (e *Env) blockerHolder(t row, b *board) string {
	if b.state(t) == "draft" && actingOf(t) == t.str("owner") {
		return e.person()
	}
	return actingOf(t)
}

// holderOf is a task's acting key as waits_on names it: the gate is no one.
func holderOf(acting string) string {
	if acting == db.Gate {
		return ""
	}
	return acting
}

// working is whether a view says its agent works now: status.state, else the state word of a view stored without it.
func working(view map[string]any) bool {
	if st, ok := view["status"].(map[string]any); ok {
		return st["state"] == status.Running
	}
	return view["state"] == "running"
}
