package reads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
)

type wallDelivery struct{ fakeDelivery }

func (wallDelivery) Wall(db.Q, string, *db.AgentState) (*Wall, error) {
	return &Wall{Account: "acct", Clock: 1700, Assumed: true}, nil
}

func viewOfAgent(t *testing.T, e *Env, name string) map[string]any {
	t.Helper()
	got, err := e.agentsList(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var out struct{ Agents []map[string]any }
	json.Unmarshal(b, &out)
	for _, a := range out.Agents {
		if a["name"] == name {
			return a
		}
	}
	t.Fatalf("%s not listed: %s", name, b)
	return nil
}

func TestAgentViewServesKindTaskCountAndReady(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES('bob', '{"name":"bob","state":"idle"}', 0)`)
	// 1 is bob's current task, queued; 2 is a draft that is not current and does not count; 3 is queued and waits.
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'a', 'bob', 'p1', 1, 'p1'), (3, 'c', 'bob', 'p1', 1, 'p1')")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(2, 'b', 'bob', 'p1', 1)")
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'IDLE', 0, 1)")
	v := viewOfAgent(t, e, "bob")
	if k, _ := v["kind"].(string); k == "" {
		t.Fatalf("kind %v", v["kind"])
	}
	if v["task_count"] != float64(2) || v["ready"] != true || v["wall"] != nil {
		t.Fatalf("%v", v)
	}

	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(4, 'blocker', 'amy', 'p1', 1)")
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(1, 4, 1)")
	if v = viewOfAgent(t, e, "bob"); v["ready"] != false {
		t.Fatalf("an open blocker holds it: %v", v)
	}
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id = 4")
	if v = viewOfAgent(t, e, "bob"); v["ready"] != true {
		t.Fatalf("a done blocker holds nothing: %v", v)
	}
	run(t, e, `UPDATE agent_view SET view = '{"name":"bob","state":"running"}'`)
	if v = viewOfAgent(t, e, "bob"); v["ready"] != false {
		t.Fatalf("a running agent is not ready: %v", v)
	}
	// Ready follows the screen word, read back from the wire word the tick writes: IDLE for idle and no_terminal, which a DONE row, a
	// parked one with nothing behind it and a queued one with background work all write.
	for _, c := range []struct {
		row, wire string
		ready     bool
	}{
		{"IDLE", "idle", true}, {"DONE", "idle", true}, {"PARKED", "idle", true}, {"QUEUED", "idle", true}, {"IDLE", "no_terminal", true},
		{"QUEUED", "queued", false}, {"PARKED", "parked", false}, {"RUNNING", "running", false}, {"IDLE", "waiting", false}, {"STALLED", "stalled", false},
	} {
		run(t, e, "UPDATE agent_state SET state = ? WHERE agent = 'bob'", c.row)
		run(t, e, `UPDATE agent_view SET view = json_object('name', 'bob', 'state', ?)`, c.wire)
		if v = viewOfAgent(t, e, "bob"); v["ready"] != c.ready {
			t.Fatalf("%s row, wire %s: ready %v, want %v", c.row, c.wire, v["ready"], c.ready)
		}
	}
}

func TestAStoredViewThatDoesNotDecodeIsAnError(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES('bob', '{not json', 0)`)
	if _, err := e.agentsList(nil, nil); err == nil {
		t.Fatal("served an agent with no name or state")
	}
}

func TestAgentViewServesTheWallTheLifecycleNames(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	e.Delivery = wallDelivery{}
	run(t, e, "INSERT INTO agent_state(agent, state, updated, closed_at) VALUES('bob', 'IDLE', 0, 5)")
	w, _ := viewOfAgent(t, e, "bob")["wall"].(map[string]any)
	if w["account"] != "acct" || w["assumed"] != true || w["clock"] == nil {
		t.Fatalf("%v", w)
	}
}

func TestAStoredWallStandsWhereNoLifecycleCanSayOtherwise(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES('bob', '{"name":"bob","state":"idle","wall":{"account":"acct","clock":null,"assumed":false}}', 0)`)
	if w, _ := viewOfAgent(t, e, "bob")["wall"].(map[string]any); w["account"] != "acct" {
		t.Fatalf("%v", w)
	}
}

func TestAgentViewServesCacheIgnoredAndConfigError(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	v := viewOfAgent(t, e, "bob")
	if v["cache_ignored"] != false || v["config_error"] != nil || v["kind"] == nil {
		t.Fatalf("%v", v)
	}
	// The agent's own switch wins over the global one;
	os.WriteFile(filepath.Join(e.Home, "config.yaml"), []byte("ignore_cache_expiry: true\n"), 0o644)
	if v = viewOfAgent(t, e, "bob"); v["cache_ignored"] != true {
		t.Fatalf("global switch: %v", v)
	}
	teamtest.Set(t, e.Home, "bob", "ignore_cache_expiry", false)
	if v = viewOfAgent(t, e, "bob"); v["cache_ignored"] != false {
		t.Fatalf("own switch: %v", v)
	}
}

func TestAgentViewFireIsWhatTheLifecycleSaysForTheAgentsAccount(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	teamtest.Set(t, e.Home, "bob", "vendor", "claude")
	run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES('bob', '{"name":"bob","state":"idle","account":"zeta"}', 0)`)
	run(t, e, "INSERT INTO agent_state(agent, state, updated, terminal) VALUES('bob', 'IDLE', 0, 't')")
	e.Delivery = fakeDelivery{fire: "usage_7d"}
	if got := viewOfAgent(t, e, "bob")["fire"]; got != "usage_7d" {
		t.Fatalf("%v", got)
	}
	// With no account there is nothing to read a trigger from; with no lifecycle the stored one stands.
	run(t, e, `UPDATE agent_view SET view = '{"name":"bob","state":"idle"}'`)
	if got := viewOfAgent(t, e, "bob")["fire"]; got != nil {
		t.Fatalf("%v", got)
	}
	e.Delivery = nil
	run(t, e, `UPDATE agent_view SET view = '{"name":"bob","state":"idle","fire":"context"}'`)
	if v := viewOfAgent(t, e, "bob"); v["fire"] != "context" {
		t.Fatalf("%v", v)
	}
}

func TestAgentViewServesItsLastRequestToTheFounderAndHowManyAreUnseen(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	if v := viewOfAgent(t, e, "bob"); v["last_to_founder"] != nil || v["founder_unread"] != float64(0) {
		t.Fatalf("an agent that never wrote: %v", v)
	}
	for id, ts := range map[int]int{1: 100, 2: 200, 3: 300} {
		run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES(?, ?, 'bob', 'p1', ?, 'queued', 'say', 1)", id, ts, "msg"+string(rune('0'+id)))
	}
	run(t, e, "INSERT INTO cursors(name, row_id, updated) VALUES('for_you_seen:bob', 1, 0)")
	v := viewOfAgent(t, e, "bob")
	last, _ := v["last_to_founder"].(map[string]any)
	if last["text"] != "msg3" || last["at"] != "1970-01-01T00:05:00Z" || v["founder_unread"] != float64(2) {
		t.Fatalf("%v", v)
	}
}

func TestTaskCountLeavesOutWhatIsNotTheAgentsToDoNow(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES('bob', '{"name":"bob","state":"idle"}', 0)`)
	// 1 is bob's current task, 3 waits behind it; 4 is amy's.
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'a', 'bob', 'p1', 1, 'p1'), "+
		"(3, 'c', 'bob', 'p1', 1, 'p1'), (4, 'blocker', 'amy', 'p1', 1, 'p1')")
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('bob', 'IDLE', 0, 1)")
	count := func(want float64, why string) {
		t.Helper()
		if v := viewOfAgent(t, e, "bob"); v["task_count"] != want {
			t.Fatalf("%s: task_count %v, want %v", why, v["task_count"], want)
		}
	}
	count(2, "both bob's")
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(3, 1, 'p1', 'pick', 'bob', 1)")
	count(1, "the founder's turn")
	run(t, e, "UPDATE task_turns SET agent = 'amy' WHERE task_id = 3")
	count(1, "another agent's turn")
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(3, 2, 'bob', 'back', 'amy', 2)")
	count(2, "the turn back to bob")
	run(t, e, "DELETE FROM task_turns")
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(3, 4, 1)")
	count(1, "blocked by an open task")
	run(t, e, "UPDATE tasks SET done_evidence = 'x' WHERE id = 4")
	count(2, "the blocker done")
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(1, 1, 'p1', 'pick', 'bob', 1)")
	count(2, "the held task counts on the founder's turn")
}
