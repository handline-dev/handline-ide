package reads

import (
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
)

type fakeDelivery struct {
	wall, cause string
	full        bool
	// fire is the trigger Fire says; forgotten is the idle seconds of a resolve nobody wrote (head 7), 0 for none.
	fire      string
	forgotten int64
	// wakeWait is the walled-wake sentence by agent.
	wakeWait map[string]string
}

func (f fakeDelivery) WakeWaitReason(_ db.Q, agent string, _ *db.AgentState) (string, error) {
	return f.wakeWait[agent], nil
}

func (f fakeDelivery) WallReason(db.Q, string, *db.AgentState) (string, error) { return f.wall, nil }
func (f fakeDelivery) ClosedWakeCause(string, *db.AgentState) (string, error)  { return f.cause, nil }
func (f fakeDelivery) Wall(db.Q, string, *db.AgentState) (*Wall, error)        { return nil, nil }
func (f fakeDelivery) AccountFull(string) (string, int, bool)                  { return "acct", 95, f.full }

func (f fakeDelivery) ForgottenResolve(db.Q, string, *db.AgentState, int64) (int64, int64, bool, error) {
	return 7, f.forgotten, f.forgotten != 0, nil
}
func (f fakeDelivery) Trigger(db.Q, string, *db.AgentState, string) (Trigger, error) {
	return Trigger{Applies: f.fire != "", Fired: f.fire != "", Kind: f.fire}, nil
}

func run(t *testing.T, e *Env, q string, args ...any) {
	t.Helper()
	if _, err := e.DB.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

// bobKey is bob's key once addAgent seeded it, else the word bob, which no agent has.
func bobKey(t *testing.T, e *Env) string { return teamtest.Key(t, e.Home, "bob") }

func queue(t *testing.T, e *Env, id int64, kind string) {
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(?, ?, 'amy', ?, 'b', 'queued', ?, 1)", id, id, bobKey(t, e), kind)
}

func state(t *testing.T, e *Env, st string, closedAt any) {
	run(t, e, "INSERT OR REPLACE INTO agent_state(agent, state, updated, closed_at) VALUES(?, ?, 0, ?)", bobKey(t, e), st, closedAt)
}

func TestQueueReasonSaysWhyALineWaits(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Env)
		d     Delivery
		want  string
		after string // run on request 1 once it is queued
	}{
		{"not registered", func(e *Env) {}, nil, "not registered", ""},
		{"never started", func(e *Env) { addAgent(t, e, "bob", "") }, nil, "never started", ""},
		{"stalled", func(e *Env) { addAgent(t, e, "bob", ""); state(t, e, "STALLED", nil) }, nil, "stalled", ""},
		{"a turn running", func(e *Env) { addAgent(t, e, "bob", ""); state(t, e, "RUNNING", nil) }, nil, "mid-turn", ""},
		{"mid-handoff", func(e *Env) { addAgent(t, e, "bob", ""); state(t, e, "HANDOFF", nil) }, nil, "mid-handoff", ""},
		{"a handoff in flight", func(e *Env) {
			addAgent(t, e, "bob", "")
			state(t, e, "IDLE", nil)
			run(t, e, "INSERT INTO handoff(id, agent, trigger_kind, step, deadline, state, updated) VALUES('h1', ?, 'context', 'requested', ?, 'in_flight', ?)", bobKey(t, e), db.Now()+90, db.Now())
		}, nil, "held while it restarts; the new session gets it", ""},
		{"a handoff given up", func(e *Env) {
			addAgent(t, e, "bob", "")
			state(t, e, "IDLE", nil)
			run(t, e, "INSERT INTO handoff(id, agent, trigger_kind, step, deadline, state, updated) VALUES('h1', ?, 'context', 'requested', 0, 'degraded', ?)", bobKey(t, e), db.Now())
		}, nil, "waiting for the agent", ""},
		{"usage wall", func(e *Env) { addAgent(t, e, "bob", ""); state(t, e, "IDLE", nil) }, fakeDelivery{wall: "bob is at its usage limit"}, "account at the wall", ""},
		{"nothing holds it", func(e *Env) { addAgent(t, e, "bob", ""); state(t, e, "IDLE", nil) }, nil, "waiting for the agent", ""},
		{"closed, account full", func(e *Env) { addAgent(t, e, "bob", ""); state(t, e, "IDLE", 5) }, fakeDelivery{full: true}, "closed, account full", ""},
		{"closed, waking", func(e *Env) { addAgent(t, e, "bob", ""); state(t, e, "IDLE", 5) }, fakeDelivery{cause: "1 waiting request(s)"}, "closed, waking", ""},
		{"closed, holding", func(e *Env) {
			addAgent(t, e, "bob", "")
			state(t, e, "IDLE", 5)
		}, fakeDelivery{}, "closed, holding", "UPDATE requests SET delivered_at = 4 WHERE id = 1"},
		{"closed, nothing to wake it", func(e *Env) { addAgent(t, e, "bob", ""); state(t, e, "IDLE", 5) }, fakeDelivery{}, "closed, nothing to wake it", ""},
	}
	for _, c := range cases {
		e := testEnv(t)
		c.setup(e) // first, so the request goes to bob's key
		queue(t, e, 1, "task")
		if c.after != "" {
			run(t, e, c.after)
		}
		e.Delivery = c.d
		got, err := e.QueueReason(bobKey(t, e), 1)
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestQueueReasonNamesTheLineAheadAndASay(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	state(t, e, "RUNNING", nil)
	queue(t, e, 1, "task")
	queue(t, e, 2, "task")
	queue(t, e, 3, "say")
	for id, want := range map[int64]string{2: "behind an earlier message", 3: "mid-turn"} { // a say is told now, so only the agent's state holds it
		if got, _ := e.QueueReason(bobKey(t, e), id); got != want {
			t.Errorf("#%d: %q, want %q", id, got, want)
		}
	}
	if got, _ := e.QueueReason(bobKey(t, e), 1); got != "mid-turn" {
		t.Errorf("the head: %q", got)
	}
	run(t, e, "UPDATE agent_state SET state = 'IDLE'")
	if got, _ := e.QueueReason(bobKey(t, e), 3); got != "sending now" {
		t.Errorf("a say with nothing in the way: %q", got)
	}
}

func TestQueueReasonSaysTheHeadsOwnTaskIsBlocked(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	state(t, e, "IDLE", nil)
	queue(t, e, 1, "task")
	queue(t, e, 2, "task")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', ?, 'p1', 1), (2, 'u', ?, 'p1', 1)", bobKey(t, e), bobKey(t, e))
	run(t, e, "INSERT INTO task_requests(task_id, request_id) VALUES(1, 1)")
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(1, 2, 1)")
	if got, _ := e.QueueReason(bobKey(t, e), 1); got != "its own task is blocked" {
		t.Errorf("the blocked head: %q", got)
	}
	if got, _ := e.QueueReason(bobKey(t, e), 2); got != "waiting for the agent" {
		t.Errorf("the line the blocked head hands the queue to is the effective head: %q", got)
	}
}
