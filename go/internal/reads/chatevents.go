package reads

import (
	"sort"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/cmdruns"
	"github.com/handline/handline/go/internal/transcript"
)

// What the chat topic's events carry is what the chat reads return, built by the same functions: these are the entry points the watcher
// calls with the ids it found changed.

// ChatRequest is one request as chat.messages shows it.
type ChatRequest struct {
	Message           map[string]any
	Requester, Target string
}

// ChatRequests reads requests for one pass of the chat watcher: the task board a task delivery's state needs is loaded once for the
// pass, not once a request.
type ChatRequests struct{ ch *chat }

// ChatRequests starts a pass; one that outlives a change of the board shows the board as it was.
func (e *Env) ChatRequests() *ChatRequests { return &ChatRequests{&chat{e: e}} }

// ChatRequest reads request id whether or not any chat draws it; nil when there is none.
func (e *Env) ChatRequest(id int64) (*ChatRequest, error) { return e.ChatRequests().Get(id) }

// Get is ChatRequest within the pass.
func (p *ChatRequests) Get(id int64) (*ChatRequest, error) {
	ch := p.ch
	r, err := ch.requestRow(id)
	if err != nil || r == nil {
		return nil, err
	}
	m, err := ch.messageOf(r)
	if err != nil {
		return nil, err
	}
	return &ChatRequest{Message: m, Requester: r.str("requester"), Target: r.str("target")}, nil
}

// TaskEventItem is the task event with that id as a transcript item, and the agent acting on the task it names, as chat.transcript gives
// it; ok is false when the chat does not show the event or its task is gone.
func (e *Env) TaskEventItem(id int64) (item map[string]any, agent string, ok bool, err error) {
	ev, err := queryOne(e.DB, "SELECT id, ts, agent, kind, payload, "+commentBodySQL+" FROM events WHERE id = ?", id)
	if err != nil || ev == nil {
		return nil, "", false, err
	}
	if item, agent, ok := commentCardOf(ev); ok {
		return item, agent, true, nil
	}
	if item, ok := accountItemOf(ev); ok {
		return item, ev.str("agent"), true, nil
	}
	item, tid, shown := taskItemOf(ev)
	if !shown {
		return nil, "", false, nil
	}
	if tid == 0 { // a batch: its author's chat
		return item, ev.str("agent"), true, nil
	}
	t, err := queryOne(e.DB, "SELECT "+db.ActingSQL("tasks")+" AS acting FROM tasks WHERE id = ?", tid)
	if err != nil || t == nil || inBatchCard(ev, t.str("acting")) {
		return nil, "", false, err
	}
	return item, t.str("acting"), true, nil
}

// ReplyItem is reply item id as a transcript item with its runs, and the agent it is of; ok is false when there is no such reply.
func (e *Env) ReplyItem(id int64) (item map[string]any, agent string, ok bool, err error) {
	r, agent, ok, err := transcript.ReplyByID(e.DB, id)
	if err != nil || !ok {
		return nil, "", false, err
	}
	runs, err := cmdruns.OfReply(e.DB, id)
	if err != nil {
		return nil, "", false, err
	}
	return replyItemOf(r, runs), agent, true, nil
}

// PromptItem is the prompt block with that id as a transcript item, and the agent it belongs to; ok is false when there is no such block.
func (e *Env) PromptItem(id int64) (item map[string]any, agent string, ok bool, err error) {
	b, err := queryOne(e.DB, "SELECT * FROM prompt_blocks WHERE id = ?", id)
	if err != nil || b == nil {
		return nil, "", false, err
	}
	return promptItemOf(b.str("agent"), b), b.str("agent"), true, nil
}

// SessionItem is a session start as a chat.transcript item and its agent; false for a start the chat does not mark, and until the session id is known.
func (e *Env) SessionItem(id int64) (item map[string]any, agent string, ok bool, err error) {
	starts, err := chatStarts(e.DB, "id = ?", id)
	if err != nil || len(starts) == 0 {
		return nil, "", false, err
	}
	item, ok = sessionItemOf(starts[0])
	return item, starts[0].str("agent"), ok, nil
}

// WorkingOf is a stored agent view (the agent_view row's JSON) as the spec's Working.
func WorkingOf(view string) map[string]any { return workingFrom(decodeObject(view)) }

// ChatTaskKinds are the kinds of event the chat shows: the task events, a batch discard's, a task_comment item's and an account move's, sorted.
func ChatTaskKinds() []string {
	kinds := append(append(taskCardKinds(), commentCardKinds...), accountKinds...)
	sort.Strings(kinds)
	return kinds
}

// taskCardKinds are the kinds of event shown in the chat of the agent acting on the task, and a batch discard's.
func taskCardKinds() []string {
	kinds := []string{batchKind}
	for k := range transcriptEvents {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// accountKinds are the events of an account move the agent's own chat shows: one landed, and a person's that could not run.
var accountKinds = []string{"moved_account", db.MoveFailedKind}

// accountItemOf is an account move's event as a transcript item: account_moved (cause manual for a person's move, policy for the
// daemon's) or move_failed; false for any other event.
func accountItemOf(ev row) (map[string]any, bool) {
	p := decodeObject(ev.str("payload"))
	id, at := itemID(ev.i("ts"), itemTask, ev.i("id")), ts(ev["ts"])
	switch ev.str("kind") {
	case "moved_account":
		cause := "policy"
		if text(p["trigger"]) == "forced" {
			cause = "manual"
		}
		written, _ := p["note_written"].(bool)
		return map[string]any{"id": id, "at": at, "kind": "account_moved",
			"account_moved": map[string]any{"from": text(p["from"]), "to": text(p["to"]), "cause": cause, "note_written": written}}, true
	case db.MoveFailedKind:
		return map[string]any{"id": id, "at": at, "kind": "move_failed",
			"move_failed": map[string]any{"to": text(p["to"]), "reason": text(p["reason"])}}, true
	}
	return nil, false
}
