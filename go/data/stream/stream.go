// Package stream is the event stream (contract/protocol.md: subscribe, unsubscribe, event, gap). Every change a client may watch is a row
// in stream_log, whose id is the cursor, so a client that reconnects with its last cursor gets exactly what it missed. Rows come from
// any writer (this daemon's Append, the watcher's); one poll of MAX(id) hands every new row to the subscribers in cursor order. The
// devices topic is not rows: it is fanned out live from memory with cursor 0, and a client that missed any reads devices.list. A client
// that cannot keep up is dropped with a gap, never buffered without limit.
package stream

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

const (
	devicesTopic = "devices"
	// MaxQueue is how many live events may wait for one client before it is dropped with a gap.
	MaxQueue = 1000
	// PollEvery is how often the log is looked at for rows another writer appended.
	PollEvery  = 250 * time.Millisecond
	pruneEvery = time.Hour
	batch      = 1000
)

var topics = map[string]bool{"status": true, "accounts": true, "agents": true, "inbox": true, "tasks": true,
	"notifications": true, "operations": true, "chat": true, devicesTopic: true, "proposals": true, "prefs": true, "requests": true,
	"for_you": true, "people": true, "workspaces": true, "nodes": true}

// Hub fans events out to the connections subscribed to them.
type Hub struct {
	// Log is the stream_log; nil serves the devices topic alone.
	Log *Log
	// Render is an event as a person other than the owner reads it (their own unread count of a task), nil for the event as logged;
	// nil renders nothing.
	Render func(person string, ev *Event) *Event

	mu   sync.Mutex
	subs map[*sub]struct{}
	top  int64 // the newest cursor handed to the subscribers
	wake chan struct{}
}

func NewHub() *Hub { return &Hub{subs: map[*sub]struct{}{}, wake: make(chan struct{}, 1)} }

func msgOf(ev *Event) []byte {
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "event", "params": ev})
	if err != nil {
		return nil
	}
	return b
}

// publish hands one event to every subscriber that wants it.
func (h *Hub) publish(ev *Event) {
	msg := msgOf(ev)
	if msg == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		s.offer(ev, msg)
	}
}

// Emit sends a devices event to every subscriber. Events are live only and carry cursor 0: the stream's cursors
// are the log's, and a client resumes the devices topic from devices.list.
func (h *Hub) Emit(typ string, data, actor map[string]any, title string) {
	ev := &Event{Topic: devicesTopic, Type: typ, At: time.Now().UTC().Format("2006-01-02T15:04:05Z")}
	ev.Data, _ = json.Marshal(data)
	if actor != nil {
		ev.Actor, _ = json.Marshal(actor)
	}
	if title != "" {
		ev.Title = &title
	}
	h.publish(ev)
}

// Append writes one event to stream_log. Its subscribers get it from the next look at the log, which this asks for at once.
func (h *Hub) Append(e Entry) (*Event, error) {
	ev, err := h.Log.Append(e)
	if err == nil {
		select {
		case h.wake <- struct{}{}:
		default:
		}
	}
	return ev, err
}

// AccountChanged appends account.changed on the accounts topic; account is the spec's Account, as accounts.list shows it (it holds no
// credential). A web login calls it each time its state moves.
func (h *Hub) AccountChanged(account any) error {
	_, err := h.Append(Entry{Topic: "accounts", Type: "account.changed", Data: account})
	return err
}

// Subscribed is whether some connection is subscribed to the topic.
func (h *Hub) Subscribed(topic string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if s.topics[topic] {
			return true
		}
	}
	return false
}

// Run looks at the log for new rows every PollEvery (and when Append asks) until stop closes, and prunes it hourly.
func (h *Hub) Run(stop <-chan struct{}, logf func(string, ...any)) {
	if h.Log == nil {
		return
	}
	tick, prune := time.NewTicker(PollEvery), time.NewTicker(pruneEvery)
	defer tick.Stop()
	defer prune.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		case <-h.wake:
		case <-prune.C:
			if err := h.Log.Prune(); err != nil {
				logf("  stream: prune: %v", err)
			}
			continue
		}
		if err := h.poll(); err != nil {
			logf("  stream: poll: %v", err)
		}
	}
}

// poll hands the rows after the last one handed on to the subscribers, oldest first. With nobody subscribed it only moves its mark: a
// subscriber starts from the newest row, and replays what it missed from the log.
func (h *Hub) poll() error {
	newest, err := h.Log.Newest()
	if err != nil {
		return err
	}
	h.mu.Lock()
	idle, top := len(h.subs) == 0, h.top
	h.mu.Unlock()
	if idle {
		h.setTop(newest)
		return nil
	}
	for top < newest {
		rows, err := h.Log.Since(top, allLogged, "", batch)
		if err != nil || len(rows) == 0 {
			return err
		}
		for _, ev := range rows {
			h.publish(ev)
			top = ev.Cursor
		}
		h.setTop(top)
	}
	return nil
}

func (h *Hub) setTop(n int64) {
	h.mu.Lock()
	h.top = max(h.top, n)
	h.mu.Unlock()
}

var allLogged = func() []string {
	var out []string
	for t := range topics {
		if Daemon(t) {
			out = append(out, t)
		}
	}
	return out
}()

// add registers s. A stream_log subscription first queues what it missed after since (a gap if that is past retention or more than
// ReplayMax), under the lock the poll publishes with, so nothing is missed or sent twice between the replay and the live stream. It
// returns the newest cursor, which is the reply to subscribe.
func (h *Hub) add(s *sub, since *int64) (int64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var top int64
	if s.logged && h.Log != nil {
		var err error
		if top, err = h.Log.Newest(); err != nil {
			return 0, err
		}
		if since != nil {
			pruned, err := h.Log.PrunedThrough()
			if err != nil {
				return 0, err
			}
			var missed []*Event
			if *since >= pruned {
				if missed, err = h.Log.Since(*since, s.loggedTopics(), s.agent, ReplayMax+1); err != nil {
					return 0, err
				}
			}
			if *since < pruned || len(missed) > ReplayMax {
				s.preload(nil, gapNotice("retention", top))
			} else {
				s.preload(missed, nil)
			}
		}
		s.last = max(s.last, top)
		h.top = max(h.top, top)
	}
	h.subs[s] = struct{}{}
	return top, nil
}

func (h *Hub) remove(s *sub) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
	s.end()
}

// sub is one subscription of a connection: a replay queue and a bounded live queue, written out by one goroutine.
type sub struct {
	send    func([]byte) bool
	hangUp  func()
	topics  map[string]bool
	agent   string              // only this agent's events and those of no agent; "" is every agent's
	person  string              // the subscribing person: a personal topic's event of a person (its Agent a person key) goes to that person only
	render  func(*Event) *Event // the event as this subscriber reads it (Hub.Render); nil, or a nil answer, is the event as logged
	visible func(*Event) bool
	logged  bool // its topics are stream_log's, so it has cursors

	mu     sync.Mutex
	cond   *sync.Cond
	replay [][]byte // what a resume missed: up to ReplayMax, outside the live bound
	queue  []queued // live messages, bounded by MaxQueue
	last   int64    // the newest cursor queued: nothing at or below it is queued twice
	dead   bool
	gap    []byte // the notice that ends this subscription: sent, then the connection is hung up
}

// queued is a live message, and whether its event is live only (cursor 0), so no replay brings it back.
type queued struct {
	msg  []byte
	live bool
}

func newSub(c *rpc.Conn, names map[string]bool, agent string, visible func(*Event) bool) *sub {
	s := &sub{send: c.SendRaw, hangUp: c.HangUp, topics: map[string]bool{}, agent: agent, visible: visible}
	for n := range names {
		s.topics[n] = true
		s.logged = s.logged || Daemon(n)
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// coveredBy is whether every topic of s is in names.
func (s *sub) coveredBy(names map[string]bool) bool {
	for t := range s.topics {
		if !names[t] {
			return false
		}
	}
	return true
}

// loggedTopics is s's topics that stream_log holds.
func (s *sub) loggedTopics() []string {
	var out []string
	for t := range s.topics {
		if Daemon(t) {
			out = append(out, t)
		}
	}
	return out
}

// personal are the topics whose events are each one person's: what waits on them.
var personal = map[string]bool{"for_you": true, "requests": true}

func (s *sub) wants(ev *Event) bool {
	if !s.topics[ev.Topic] {
		return false
	}
	if personal[ev.Topic] && ev.Agent != nil && actor.IsPerson(*ev.Agent) {
		return *ev.Agent == s.person
	}
	if s.agent != "" && ev.Agent != nil && *ev.Agent != s.agent {
		return false
	}
	return s.visible == nil || s.visible(ev)
}

// offer queues a live event. A logged one is skipped when its cursor was already queued; cursor 0 is a live-only event.
func (s *sub) offer(ev *Event, msg []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead || s.gap != nil {
		return
	}
	if ev.Cursor > 0 {
		if ev.Cursor <= s.last {
			return
		}
		s.last = ev.Cursor
	}
	if !s.wants(ev) {
		return
	}
	if s.render != nil {
		if mine := s.render(ev); mine != nil {
			msg = msgOf(mine)
		}
	}
	if len(s.queue) >= MaxQueue {
		s.queue, s.gap = nil, gapNotice("slow_client", ev.Cursor)
		time.AfterFunc(2*time.Second, s.hangUp) // a writer stuck on a client that stopped reading
	} else {
		s.queue = append(s.queue, queued{msg: msg, live: ev.Cursor == 0})
	}
	s.cond.Signal()
}

// preload queues what a resume missed, ahead of everything live. It is not held to MaxQueue: a client reconnecting 5000 events behind is
// normal, and its backlog is not it falling behind.
func (s *sub) preload(events []*Event, notice []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if notice != nil {
		s.replay = append(s.replay, notice)
	}
	for _, ev := range events {
		if ev.Cursor > s.last && s.wants(ev) {
			if s.render != nil {
				if mine := s.render(ev); mine != nil {
					ev = mine
				}
			}
			s.replay = append(s.replay, msgOf(ev))
		}
		s.last = max(s.last, ev.Cursor)
	}
	s.cond.Signal()
}

// takeOver ends old and keeps its live-only messages not sent yet ahead of s's own: a replay from since brings back the logged ones.
func (s *sub) takeOver(old *sub) {
	old.mu.Lock()
	var live []queued
	for _, q := range old.queue {
		if q.live {
			live = append(live, q)
		}
	}
	old.queue, old.dead = nil, true
	old.mu.Unlock()
	old.cond.Broadcast()
	if len(live) == 0 {
		return
	}
	s.mu.Lock()
	s.queue = append(live, s.queue...)
	s.mu.Unlock()
	s.cond.Signal()
}

func (s *sub) end() {
	s.mu.Lock()
	s.dead = true
	s.mu.Unlock()
	s.cond.Broadcast()
}

// run writes the queued notices; it starts after the reply to subscribe is sent, so none comes before it.
func (s *sub) run() {
	for {
		s.mu.Lock()
		for len(s.replay) == 0 && len(s.queue) == 0 && s.gap == nil && !s.dead {
			s.cond.Wait()
		}
		if s.gap != nil { // sent even when the subscription has ended, as the Python stream does
			gap := s.gap
			s.mu.Unlock()
			s.send(gap)
			s.hangUp()
			return
		}
		if s.dead {
			s.mu.Unlock()
			return
		}
		var msg []byte
		if len(s.replay) > 0 {
			msg, s.replay = s.replay[0], s.replay[1:]
		} else {
			msg, s.queue = s.queue[0].msg, s.queue[1:]
		}
		s.mu.Unlock()
		if !s.send(msg) {
			s.end()
			return
		}
	}
}

func gapNotice(reason string, cursor int64) []byte {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "gap", "params": map[string]any{"reason": reason, "cursor": cursor}})
	return b
}

// Register adds subscribe and unsubscribe to a server. A subscribe names known topics (else bad_request naming topics); devices is
// live only, its events cursor 0. devices is for a founder; an agent session is sent only what its role may read.
func Register(srv *rpc.Server, h *Hub) {
	var mu sync.Mutex
	byConn := map[*rpc.Conn][]*sub{}
	drop := func(c *rpc.Conn) {
		mu.Lock()
		subs := byConn[c]
		delete(byConn, c)
		mu.Unlock()
		for _, s := range subs {
			h.remove(s)
		}
	}
	srv.Register("subscribe", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		names, err := check(p)
		if err != nil {
			return nil, err
		}
		since, err := sinceOf(p)
		if err != nil {
			return nil, err
		}
		switch p["resume"] {
		case nil, "replay":
		case "latest": // the client reads what it shows again, so what it missed is not sent
			since = nil
		default:
			return nil, rpc.BadRequestError("resume is replay or latest", "resume", rpc.InvalidParams)
		}
		for n := range names {
			if err := Allowed(c, n); err != nil {
				return nil, err
			}
		}
		switch {
		case names[devicesTopic]:
			if !c.Caller.Allows("owner") {
				return nil, rpc.Refused("the devices topic is for a founder; this caller is "+c.Caller.Role, "role", "")
			}
		}
		agent := ""
		if v, has := p["agent"]; has && v != nil {
			name, ok := v.(string)
			if !ok {
				return nil, rpc.BadRequestError("agent is a name", "agent", rpc.InvalidParams)
			}
			agent = name
		}
		var visible func(*Event) bool
		if c.Caller.Role == "agent" {
			visible = agentView(c.Caller.Key)
		}
		mu.Lock()
		defer mu.Unlock()
		s := newSub(c, names, agent, visible)
		if actor.IsPerson(c.Caller.Key) {
			s.person = c.Caller.Key
			if h.Render != nil && s.person != actor.OwnerKey {
				person := s.person
				s.render = func(ev *Event) *Event { return h.Render(person, ev) }
			}
		}
		top, err := h.add(s, since)
		if err != nil {
			return nil, err
		}
		if len(byConn[c]) == 0 {
			c.OnClose(func() { drop(c) })
		}
		// A subscribe replaces this connection's subscriptions whose topics it names again, so a client that sends every topic it
		// follows each time gets each event once.
		keep := []*sub{s}
		for _, old := range byConn[c] {
			if old.coveredBy(names) {
				h.mu.Lock()
				delete(h.subs, old)
				h.mu.Unlock()
				s.takeOver(old)
			} else {
				keep = append(keep, old)
			}
		}
		byConn[c] = keep
		c.AfterReply(func() { go s.run() })
		return map[string]any{"cursor": top}, nil
	}})
	srv.Register("unsubscribe", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		names, err := check(p)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		defer mu.Unlock()
		var keep []*sub
		for _, s := range byConn[c] {
			s.mu.Lock()
			for n := range names {
				delete(s.topics, n)
			}
			empty := len(s.topics) == 0
			s.mu.Unlock()
			if empty {
				h.remove(s)
			} else {
				keep = append(keep, s)
			}
		}
		byConn[c] = keep
		return map[string]any{}, nil
	}})
}

// Stamped are the reads that the events of a stream_log topic keep current (app-data's reads with a feed).
var Stamped = []string{"accounts.list", "agent.get", "agent.strip", "agent.timeline", "agents.list", "chat.messages", "chat.transcript",
	"daemon.status", "proposal.list", "inbox.get", "inbox.list", "operation.get", "prefs.get", "prompt.list", "task.get",
	"task.time", "tasks.list"}

// StampCursor gives each Stamped read of s a cursor: the newest when the read began, so an event after it may be in the answer already
// (it is applied again, which changes nothing) and none at or before it is missing from the answer.
func StampCursor(s *rpc.Server, l *Log) {
	for _, name := range Stamped {
		s.Wrap(name, func(next func(*rpc.Conn, map[string]any) (any, error)) func(*rpc.Conn, map[string]any) (any, error) {
			return func(c *rpc.Conn, p map[string]any) (any, error) {
				top, err := l.Newest()
				if err != nil {
					return nil, err
				}
				res, err := next(c, p)
				if m, ok := res.(map[string]any); ok && err == nil {
					m["cursor"] = top
				}
				return res, err
			}
		})
	}
}

// sinceOf is the since param: nil when absent, else a non-negative cursor.
func sinceOf(p map[string]any) (*int64, error) {
	v, has := p["since"]
	if !has || v == nil {
		return nil, nil
	}
	n, ok := rpc.AsInt(v)
	if !ok || n < 0 {
		return nil, rpc.BadRequestError("since is a cursor, a non-negative integer", "since", rpc.InvalidParams)
	}
	return &n, nil
}

// agentView is which events the agent session of name may be sent: a topic delivers to a role only what that role's read would return.
func agentView(name string) func(*Event) bool {
	return func(ev *Event) bool {
		who := ""
		if ev.Agent != nil {
			who = *ev.Agent
		}
		return AgentMay(name, ev.Topic, ev.Type, who)
	}
}

// topicNames is the topics param as a set; false when it is not a non-empty list of strings.
func topicNames(p map[string]any) (map[string]bool, bool) {
	list, ok := p["topics"].([]any)
	if !ok || len(list) == 0 {
		return nil, false
	}
	out := map[string]bool{}
	for _, v := range list {
		name, ok := v.(string)
		if !ok {
			return nil, false
		}
		out[name] = true
	}
	return out, true
}

// check judges the topics param: known topics only.
func check(p map[string]any) (map[string]bool, error) {
	names, ok := topicNames(p)
	if !ok {
		return nil, rpc.BadRequestError("topics is a list of topic names", "topics", rpc.InvalidParams)
	}
	for n := range names {
		if !topics[n] {
			return nil, rpc.BadRequestError("no such topic: "+n, "topics", rpc.InvalidParams)
		}
	}
	return names, nil
}

// memberTopics are the topics an agent may not follow: the reads behind them (chat.*, operation.get, prefs.get) are a member's.
var memberTopics = map[string]bool{"chat": true, "operations": true, "prefs": true, "requests": true, "for_you": true}

// Allowed refuses a caller below a member any of the named topics that are a member's.
func Allowed(c *rpc.Conn, names ...string) error {
	for _, n := range names {
		if memberTopics[n] && !c.Caller.Allows("member") {
			return rpc.Refused("the "+n+" topic is for a member or above; this caller is "+c.Caller.Role, "role", "")
		}
	}
	return nil
}

// AgentMay is whether an event may be sent to the agent `name`: a topic delivers to a role only what that role's read would return. An agent's
// inbox.list and tasks are its own, and prompt.list is a member's. Whatever no rule names is not sent.
// topic, typ and who (the event's agent, "" for none) are the event's.
func AgentMay(name, topic, typ, who string) bool {
	switch topic {
	case "accounts", "agents", "proposals", "status":
		return true
	case "inbox":
		return who == name
	case "tasks":
		return who == name
	case "notifications":
		return (typ == "account_out" || typ == "stall" || typ == "task_done") && who == name
	}
	return false
}

// Daemon reports whether name is a topic of stream_log (every topic but devices, which is live from memory).
func Daemon(name string) bool { return topics[name] && name != devicesTopic }
