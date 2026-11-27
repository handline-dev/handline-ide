package prompts

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/hooks"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/jsontext"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/access"
)

// HookWaitS is how long a permission question from the PermissionRequest hook waits for the founder. With no answer by then the hook
// returns nothing and the agent's own dialog shows in its terminal.
const HookWaitS = 3600

// HookSource marks a prompt row raised by the hook: it is answered by returning the decision to the waiting call, not by typing.
const HookSource = "hook"

// Decision is the founder's answer to a hook's permission question.
type Decision struct {
	Allow bool
	Note  string // why not, for a deny
	Rule  string // an always-allowed rule the running session applies from now (Tool(content)), "" for none
	// Answer is a question's answer: the picked labels or the typed text.
	Answer string
}

// hookWaits are the calls waiting now, by their prompt row, each with the rule its Always allow keeps ("" for none) and a question's
// option labels: held here, from the call's own input, since the row's detail is in a database every agent can write.
type hookWaits struct {
	mu     sync.Mutex
	by     map[int64]chan Decision
	rules  map[int64]string
	labels map[int64][]string
}

func (w *hookWaits) add(id int64, rule string, labels []string) chan Decision {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.by == nil {
		w.by, w.rules, w.labels = map[int64]chan Decision{}, map[int64]string{}, map[int64][]string{}
	}
	ch := make(chan Decision, 1)
	w.by[id], w.rules[id], w.labels[id] = ch, rule, labels
	return ch
}

// optionLabels are the labels of the question the waiting call on id asked.
func (w *hookWaits) optionLabels(id int64) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.labels[id]
}

// rule is the rule the waiting call on id offers, "" for none or no call.
func (w *hookWaits) rule(id int64) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rules[id]
}

func (w *hookWaits) waiting(id int64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.by[id] != nil
}

func (w *hookWaits) take(id int64) chan Decision {
	w.mu.Lock()
	defer w.mu.Unlock()
	ch := w.by[id]
	delete(w.by, id)
	delete(w.rules, id)
	delete(w.labels, id)
	return ch
}

// keepGrants keeps each of grant as agent's always grant, all or none, as the founder's settings change: a kept grant lifts a hide only
// at launch, so the session starts fresh with it at its next idle moment (tick.ApplySettings).
func (e *Env) keepGrants(agent string, grant []string) error {
	tx, err := e.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := e.now().Unix()
	for _, g := range grant {
		if _, err := access.Put(tx, agent, g, "always", now); err != nil {
			return err
		}
	}
	if len(grant) > 0 {
		if err := db.SetMeta(tx, db.SettingsByMeta(agent), actor.OwnerKey); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Grant is the folders an "Allow always" of this hook question stores as grants; nil when it offers none.
func (b *Block) Grant() []string {
	list, _ := b.Detail["grant"].([]any)
	var out []string
	for _, g := range list {
		if s, ok := g.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// AskPermission raises agent's permission question as a prompt row and waits for the founder's answer: answered false when ctx ends or
// HookWaitS passes first. A question already waiting for the same agent is waited out first, as a screen shows one at a time.
// The same exact Bash command asked again is offered as Always allow.
func (e *Env) AskPermission(ctx context.Context, agent, tool string, input map[string]any) (d Decision, answered bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, HookWaitS*time.Second)
	defer cancel()
	q, err := e.permissionQuestion(agent, HookSource, tool, input)
	if err != nil {
		return d, false, err
	}
	return e.raise(ctx, q, true)
}

// question is one prompt row to raise and wait on.
type question struct {
	agent, kind, title string
	panel              []string
	detail             map[string]any
	offered            string   // the rule an Always allow keeps, "" for none
	labels             []string // a question's options
}

// permissionQuestion is agent's ask to use tool as a prompt row from source: an Always allow is offered for a read of another agent's
// files, and for the same exact Bash command asked before.
func (e *Env) permissionQuestion(agent, source, tool string, input map[string]any) (q question, err error) {
	command, reason := hooks.ToolCommand(input)
	title := agent + " wants to use " + tool
	panel := []string{title}
	if command != "" {
		panel = append(panel, command)
	}
	if reason != "" {
		panel = append(panel, reason)
	}
	detail := map[string]any{"agent": agent, "vendor": e.vendorOf(agent), "source": source, "tool": tool, "command": command,
		"reason": reason, "title": title, "lines": panel}
	q = question{agent: agent, kind: term.PromptPermission, title: title, panel: panel, detail: detail}
	if grant := e.ReadGrant(tool, input); grant != nil {
		detail["grant"] = grant
	} else if r, ok := e.exactRule(tool, input); ok {
		// Offered only when this agent was asked the same rule before: a one-off question keeps Allow and Deny.
		detail["rule"] = r.String()
		if detail["offer_rule"], err = e.askedBefore(agent, r.String()); err != nil {
			return q, err
		}
		if detail["offer_rule"] == true {
			q.offered = r.String()
		}
	}
	return q, nil
}

// raise opens q's row, once no other of its agent's is open, and waits for its answer until ctx ends; the row closes when the wait does.
// With movedOn, a turn's end or a new prompt of the agent's after the row opened ends the wait too: it was answered at its own dialog.
func (e *Env) raise(ctx context.Context, q question, movedOn bool) (d Decision, answered bool, err error) {
	agent := q.agent
	var since int64 // the agent's latest event now: a turn event after it means the agent has moved on from this question
	_ = e.DB.QueryRow("SELECT COALESCE(MAX(id), 0) FROM events WHERE agent = ?", agent).Scan(&since)
	var id, opened int64
	for id == 0 {
		now := e.now().Unix()
		// The time is in the fingerprint: the same command asked again is another question, and an answer to the last one is stale.
		res, err := e.DB.Exec("INSERT OR IGNORE INTO prompt_blocks(agent, kind, fingerprint, title, panel, detail, answerable, first_seen, last_seen) "+
			"VALUES(?,?,?,?,?,?,1,?,?)", agent, q.kind, Fingerprint(q.kind, append(slices.Clone(q.panel), fmt.Sprint(now))), q.title,
			jsonText(q.panel), jsonText(q.detail), now, now)
		if err != nil {
			return d, false, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			id, _ = res.LastInsertId()
			opened = now
			break
		}
		if err := e.closeOrphans(agent, now); err != nil {
			return d, false, err
		}
		select { // another prompt of this agent's is open: wait for it to go
		case <-ctx.Done():
			return d, false, nil
		case <-time.After(time.Second):
		}
	}
	ch := e.hooks.add(id, q.offered, q.labels)
	e.changed(agent)
	defer func() {
		e.hooks.take(id)
		e.closeRow(id, e.now().Unix())
		e.changed(agent)
	}()
	every := e.Refresh
	if every == 0 {
		every = StaleS / 3 * time.Second
	}
	tick := time.NewTicker(every) // seen as fresh, as the screen watch keeps a screen's row
	defer tick.Stop()
	for {
		select {
		case d := <-ch:
			return d, true, nil
		case <-ctx.Done():
			return d, false, nil
		case <-tick.C:
			res, err := e.DB.Exec("UPDATE prompt_blocks SET last_seen = ? WHERE id = ? AND gone_at IS NULL", e.now().Unix(), id)
			if err != nil {
				return d, false, err
			}
			// Closed under it, as stale after a sleep or a slow pass: nobody can answer it now, so the agent's own dialog shows instead.
			if n, _ := res.RowsAffected(); n == 0 {
				return d, false, nil
			}
			if !movedOn {
				continue
			}
			// An agent blocked on this call ends no turn and takes no prompt. One that has done either was answered at its own dialog, or
			// stopped, so the question is nobody's now. The event's own time is checked too: a spooled hook replays late with a new id.
			var moved int
			_ = e.DB.QueryRow("SELECT COUNT(*) FROM events WHERE agent = ? AND id > ? AND kind IN "+
				"('Stop', 'StopFailure', 'UserPromptSubmit', 'SessionEnd', 'SessionStart') AND ts >= ?", agent, since, opened).Scan(&moved)
			if moved > 0 {
				return d, false, nil
			}
		}
	}
}

// answerHook hands the founder's choice to the call waiting on row.
func (e *Env) answerHook(row *Block, c Choice) (bool, string) {
	if !e.hooks.waiting(row.ID) {
		return e.record(row, false, row.Agent+": the question is no longer waiting - nothing sent")
	}
	rule := ""
	if c.Always { // kept whole before the call is let go, so a failure leaves the question to answer again
		if rule = e.hooks.rule(row.ID); rule != "" {
			if err := e.keepRule(row.Agent, rule); err != nil {
				return e.record(row, false, row.Agent+": the rule was not kept - nothing sent: "+err.Error())
			}
		} else if err := e.keepGrants(row.Agent, row.Grant()); err != nil {
			return e.record(row, false, row.Agent+": the grant was not kept - nothing sent: "+err.Error())
		}
	}
	ch := e.hooks.take(row.ID)
	if ch == nil {
		return e.record(row, false, row.Agent+": the question is no longer waiting - nothing sent")
	}
	said := row.Agent + ": denied"
	switch {
	case row.Kind == "question" && c.Yes:
		said = row.Agent + ": answered: " + c.Answer
	case row.Kind == "question":
		said = row.Agent + ": cancelled"
	case rule != "":
		said = row.Agent + ": allowed always: " + rule
	case c.Always:
		said = row.Agent + ": allowed always, from its next session"
	case c.Yes:
		said = row.Agent + ": allowed"
	}
	// Recorded before the call is let go: the view it writes on closing reads the answer (answered_at) to end the waits it stood for.
	ok, said := e.record(row, true, said)
	ch <- Decision{Allow: c.Yes, Note: c.Note, Rule: rule, Answer: c.Answer}
	return ok, said
}

// permissionAsk is permission.ask: the caller's own session asks, and the call returns the founder's decision, or no decision once the
// wait is over.
func (e *Env) permissionAsk(c *rpc.Conn, params map[string]any) (any, error) {
	if c.Caller.Role != "agent" {
		return nil, rpc.Refused("only an agent's own session asks for a permission", "role", "")
	}
	agent := c.Caller.Actor()
	if e.Chat != nil && e.Chat(agent) { // its session asks on its stream at the same moment, and that ask is the one prompt
		return map[string]any{"behavior": nil}, nil
	}
	tool, _ := params["tool"].(string)
	if tool == "" {
		return nil, bad("tool is the tool's name", "tool")
	}
	input, ok := params["input"].(map[string]any)
	if !ok {
		return nil, bad("input is the tool call's input object", "input")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-c.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	d, answered, err := e.AskPermission(ctx, agent, tool, input)
	if err != nil || !answered {
		return map[string]any{"behavior": nil}, err
	}
	if r, ok := ParseRule(d.Rule); d.Allow && ok {
		return map[string]any{"behavior": "allow", "rule": map[string]any{"tool": r.Tool, "content": r.Content}}, nil
	}
	if d.Allow {
		return map[string]any{"behavior": "allow"}, nil
	}
	return map[string]any{"behavior": "deny", "message": denyMessage(d.Note)}, nil
}

func jsonText(v any) string { return jsontext.Spaced(v, true, true) }

// closeOrphans closes agent's open hook and stream questions no call of this daemon waits on: their call went with a daemon that stopped
// without closing them. One not kept fresh for StaleS only, so a question between its row and its wait is left be.
func (e *Env) closeOrphans(agent string, now int64) error {
	rows, err := e.DB.Query("SELECT id FROM prompt_blocks WHERE agent = ? AND gone_at IS NULL AND last_seen < ? AND "+
		"json_extract(detail, '$.source') IN (?, ?)", agent, now-StaleS, HookSource, StreamSource)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !e.hooks.waiting(id) {
			ids = append(ids, id)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for _, id := range ids {
		if err := e.closeRow(id, now); err != nil {
			return err
		}
	}
	if len(ids) > 0 {
		e.changed(agent)
	}
	return nil
}

func (e *Env) closeRow(id, now int64) error {
	_, err := e.DB.Exec("UPDATE prompt_blocks SET gone_at = ? WHERE id = ? AND gone_at IS NULL", now, id)
	return err
}
