package watcher

import (
	"database/sql"
	"encoding/json"
	"slices"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/internal/cmdruns"
	"github.com/handline/handline/go/internal/reads"
	"github.com/handline/handline/go/kit/actor"
)

// The chat topic's cursors are the Python watcher's, so either side picks up where the other left off.
const (
	chatTouched = "stream:chat_touched"
	chatEvents  = "stream:chat_events"
	chatPrompts = "stream:chat_prompts"
	chatStarts  = "stream:chat_sessions" // Go's alone: the Python watcher does not tell session items
)

const chatTouchedNewest = "SELECT COALESCE(MAX(COALESCE(state_at, ts)), 0) FROM requests"

// Chat is the chat topic: a request of a chat that appeared or changed (chat.message), a prompt an agent opened or closed, a task event
// of a task it acts on and a conversation it began (chat.transcript), and an agent's working state (chat.working).
// Each is said once, to the chat of every agent it belongs to, and its data is what the chat reads return (reads.ChatRequest and the
// other builders). The first look says nothing, and neither does a restart: the requests touched in the second the cursor is at are
// taken as seen.
type Chat struct {
	Reads *reads.Env
	// Panes is what the terminal host says of the windows that run commands; nil, or nil from it, ends no run.
	Panes func() map[string]term.PaneRun
	// Tail is the last rows of a run window, told to the run's agent when the run ends; nil tells no output.
	Tail        func(handle string, rows int) []string
	runSeen     map[string]string // run block -> the run as last said
	done        map[int64]string  // request -> the message as last said, for the ones touched in the second the cursor is at
	open        map[int64]bool    // the prompts told open and not yet told closed
	working     map[string]string
	running     bool             // a command run was still running at the last look
	unbound     map[int64]bool   // the fresh session starts whose session id was not known at the last look
	bashSaid    map[int64]string // the bash items of recent ! lines, as last said
	bashSettled map[int64]bool   // the recent ! lines that have landed with their output in
	bashOpen    bool             // one of them was still waiting for its output at the last look
}

func (*Chat) Name() string { return "chat" }

func (*Chat) Tables() []string {
	return []string{"agent_view", "cursors", "events", "meta", "operations", "prompt_blocks", "requests", "session_starts", "task_memos", "task_requests", "task_turns", "tasks"}
}

// Pending is whether a command run was still running at the last look and the terminal host is asked: its window ends without a write. So
// is a ! line waiting for its output, which reaches the transcript and no table.
func (c *Chat) Pending() bool { return (c.running && c.Panes != nil) || c.bashOpen }

func encoded(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func agentTarget(name string) map[string]any { return map[string]any{"screen": "agent", "id": name} }

func (c *Chat) say(emit Emit, typ, agent string, data any) error {
	_, err := emit(stream.Entry{Topic: "chat", Type: typ, Data: data, Agent: agent, Target: agentTarget(agent)})
	return err
}

func (c *Chat) Scan(q *sql.DB, emit Emit) error {
	if err := c.runs(q, emit); err != nil { // before requests: the message it said is then the one requests last saw
		return err
	}
	if err := c.requests(q, emit); err != nil {
		return err
	}
	if err := c.taskEvents(q, emit); err != nil {
		return err
	}
	if err := c.prompts(q, emit); err != nil {
		return err
	}
	if err := c.sessions(q, emit); err != nil {
		return err
	}
	if err := c.bash(q, emit); err != nil {
		return err
	}
	return c.workingStates(q, emit)
}

// bashWatch is how long after a ! line was sent its bash item is watched for; a read finds a later output.
const bashWatch = 15 * 60

// bash says the bash item of each ! line sent lately, when it appears and when its output comes. A line that has landed with its output in is
// settled, and its agent's transcript is not read again for it. The first look says nothing.
func (c *Chat) bash(q *sql.DB, emit Emit) error {
	since := db.Now() - bashWatch
	lines, err := reads.BashLines(q, since)
	if err != nil {
		return err
	}
	first := c.bashSaid == nil
	if first {
		c.bashSaid, c.bashSettled = map[int64]string{}, map[int64]bool{}
	}
	read := map[string]bool{}
	c.bashOpen = false
	for _, l := range lines {
		if !l.Landed || c.bashSettled[l.ID] || read[l.Agent] {
			continue
		}
		read[l.Agent] = true
		items, waiting, err := c.Reads.BashItems(l.Agent, since, 0)
		if err != nil {
			return err
		}
		c.bashOpen = c.bashOpen || len(waiting) > 0
		for _, item := range items {
			id, now := item["id"].(int64), encoded(item)
			if !first && c.bashSaid[id] != now {
				if err := c.say(emit, "chat.transcript", l.Agent, item); err != nil {
					return err
				}
			}
			c.bashSaid[id] = now
		}
		for _, other := range lines {
			if other.Agent == l.Agent && other.Landed && !slices.Contains(waiting, other.ID) {
				c.bashSettled[other.ID] = true
			}
		}
	}
	return nil
}

func (c *Chat) requests(q *sql.DB, emit Emit) error {
	last, err := Prime(q, chatTouched, chatTouchedNewest)
	if err != nil {
		return err
	}
	first := c.done == nil
	if first {
		c.done = map[int64]string{}
	}
	touched, err := requestsTouched(q, ChatTouchedSQL, sql.Named("last", last))
	if err != nil {
		return err
	}
	pass := c.Reads.ChatRequests()
	for _, t := range touched {
		req, err := pass.Get(t.id)
		if err != nil {
			return err
		}
		if req == nil {
			continue
		}
		message := encoded(req.Message)
		if c.done[t.id] == message {
			continue
		}
		if first && t.when.Int64 == last { // said before the restart, or by the Python watcher: seen, not news
			c.done[t.id] = message
			continue
		}
		if err := c.tell(emit, req); err != nil {
			return err
		}
		if t.when.Int64 > last {
			last, c.done = t.when.Int64, map[int64]string{}
		}
		c.done[t.id] = message
		if err := db.SetCursor(q, chatTouched, last); err != nil {
			return err
		}
	}
	return nil
}

// tell says a message to the chat of its agents.
func (c *Chat) tell(emit Emit, req *reads.ChatRequest) error {
	var told []string
	for _, agent := range []string{req.Target, req.Requester} {
		if actor.IsPerson(agent) || slices.Contains(told, agent) {
			continue
		}
		told = append(told, agent)
		if err := c.say(emit, "chat.message", agent, req.Message); err != nil {
			return err
		}
	}
	return nil
}

// runs ends the runs their windows are done with, then says the message, or the reply item, of each run that started or changed. The first
// look says nothing.
func (c *Chat) runs(q *sql.DB, emit Emit) error {
	if c.Panes != nil {
		if err := cmdruns.Settle(q, c.Panes(), cmdruns.Now(), cmdruns.Telling(c.Tail)); err != nil {
			return err
		}
	}
	all, err := cmdruns.All(q)
	if err != nil {
		return err
	}
	first := c.runSeen == nil
	if first {
		c.runSeen = map[string]string{}
	}
	c.running = false
	for _, r := range all {
		c.running = c.running || r.State == "running"
	}
	pass := c.Reads.ChatRequests()
	for _, r := range all {
		now := encoded(r)
		before := c.runSeen[r.Block]
		c.runSeen[r.Block] = now
		block, _ := cmdruns.ParseBlock(r.Block)
		if first || before == now || block.Owner == cmdruns.ByProposal { // a proposal's run is told on the proposals topic
			continue
		}
		if block.Owner == cmdruns.ByReply {
			item, agent, ok, err := c.Reads.ReplyItem(block.Message)
			if err != nil {
				return err
			}
			if ok {
				if err := c.say(emit, "chat.transcript", agent, item); err != nil {
					return err
				}
			}
			continue
		}
		req, err := pass.Get(block.Message)
		if err != nil {
			return err
		}
		if req == nil {
			continue
		}
		if err := c.tell(emit, req); err != nil {
			return err
		}
		if c.done != nil {
			c.done[block.Message] = encoded(req.Message)
		}
	}
	return nil
}

func (c *Chat) taskEvents(q *sql.DB, emit Emit) error {
	seen, err := Prime(q, chatEvents, "SELECT COALESCE(MAX(id), 0) FROM events")
	if err != nil {
		return err
	}
	kinds := reads.ChatTaskKinds()
	args := []any{seen}
	marks := ""
	for i, k := range kinds {
		if i > 0 {
			marks += ","
		}
		marks += "?"
		args = append(args, k)
	}
	ids, err := db.IDs(q, "SELECT id FROM events WHERE id > ? AND kind IN ("+marks+") ORDER BY id", args...)
	if err != nil {
		return err
	}
	for _, id := range ids {
		item, agent, ok, err := c.Reads.TaskEventItem(id)
		if err != nil {
			return err
		}
		if ok && agent != "" && !actor.IsPerson(agent) {
			if err := c.say(emit, "chat.transcript", agent, item); err != nil {
				return err
			}
		}
		if err := db.SetCursor(q, chatEvents, id); err != nil { // per row: an emit that fails does not repeat the ones before it
			return err
		}
	}
	return nil
}

func (c *Chat) prompts(q *sql.DB, emit Emit) error {
	shown, err := Prime(q, chatPrompts, "SELECT COALESCE(MAX(id), 0) FROM prompt_blocks")
	if err != nil {
		return err
	}
	if c.open == nil { // a prompt open before this daemon started is told closed when it closes
		c.open = map[int64]bool{}
		open, err := db.IDs(q, "SELECT id FROM prompt_blocks WHERE gone_at IS NULL AND id <= ?", shown)
		if err != nil {
			return err
		}
		for _, id := range open {
			c.open[id] = true
		}
	}
	fresh, err := db.IDs(q, "SELECT id FROM prompt_blocks WHERE id > ? ORDER BY id", shown)
	if err != nil {
		return err
	}
	for _, id := range fresh {
		item, agent, ok, err := c.Reads.PromptItem(id)
		if err != nil {
			return err
		}
		if ok {
			if err := c.say(emit, "chat.transcript", agent, item); err != nil {
				return err
			}
			if item["open"] == true {
				c.open[id] = true
			}
		}
		if err := db.SetCursor(q, chatPrompts, id); err != nil {
			return err
		}
	}
	held := make([]int64, 0, len(c.open))
	for id := range c.open {
		held = append(held, id)
	}
	slices.Sort(held)
	for _, id := range held {
		item, agent, ok, err := c.Reads.PromptItem(id)
		if err != nil {
			return err
		}
		if ok && item["open"] == true {
			continue
		}
		delete(c.open, id)
		if ok {
			if err := c.say(emit, "chat.transcript", agent, item); err != nil {
				return err
			}
		}
	}
	return nil
}

// sessions tells a session start once its session id is known: a structured start has it when it is written, a terminal launch when its
// first SessionStart hook binds it, within the bind window.
func (c *Chat) sessions(q *sql.DB, emit Emit) error {
	shown, err := Prime(q, chatStarts, "SELECT COALESCE(MAX(id), 0) FROM session_starts")
	if err != nil {
		return err
	}
	waiting, err := db.IDs(q, "SELECT id FROM session_starts WHERE resumed = 0 AND session_id IS NULL AND ts >= ?", db.Now()-db.BindWindowS)
	if err != nil {
		return err
	}
	now := map[int64]bool{}
	for _, id := range waiting {
		now[id] = true
	}
	var bound []int64
	for id := range c.unbound {
		if !now[id] {
			bound = append(bound, id)
		}
	}
	slices.Sort(bound)
	for _, id := range bound {
		if err := c.sayStart(emit, id); err != nil {
			return err
		}
	}
	c.unbound = now
	fresh, err := db.IDs(q, "SELECT id FROM session_starts WHERE id > ? ORDER BY id", shown)
	if err != nil {
		return err
	}
	for _, id := range fresh {
		if !now[id] { // one still unbound is told when it leaves c.unbound
			if err := c.sayStart(emit, id); err != nil {
				return err
			}
		}
		if err := db.SetCursor(q, chatStarts, id); err != nil {
			return err
		}
	}
	return nil
}

func (c *Chat) sayStart(emit Emit, id int64) error {
	item, agent, ok, err := c.Reads.SessionItem(id)
	if err != nil || !ok {
		return err
	}
	return c.say(emit, "chat.transcript", agent, item)
}

func (c *Chat) workingStates(q *sql.DB, emit Emit) error {
	rows, err := q.Query("SELECT agent, view FROM agent_view ORDER BY agent")
	if err != nil {
		return err
	}
	defer rows.Close()
	var names []string
	now := map[string]map[string]any{}
	for rows.Next() {
		var name, view string
		if err := rows.Scan(&name, &view); err != nil {
			return err
		}
		names = append(names, name)
		now[name] = reads.WorkingOf(view)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	before := c.working
	c.working = map[string]string{}
	for _, name := range names {
		c.working[name] = encoded(now[name])
	}
	if before == nil {
		return nil
	}
	for _, name := range names {
		if before[name] != c.working[name] {
			if err := c.say(emit, "chat.working", name, now[name]); err != nil {
				return err
			}
		}
	}
	return nil
}
