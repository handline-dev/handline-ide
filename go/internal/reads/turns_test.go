package reads

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

func TestARequestWaitsOnWhoeverActsOnItsTasks(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	at := func(n int64) string { return itoa(now - n) }
	run(t, e, "INSERT INTO requests(id, ref, ts, requester, target, body, state, kind, edge_ok) VALUES"+
		"(1, 'aaaaaaaaaaa1', "+at(900)+", 'p1', 'lead', 'decide it', 'landed', 'say', 1), "+
		"(2, 'aaaaaaaaaaa2', "+at(800)+", 'p1', 'lead', 'build it', 'landed', 'say', 1)")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, parent_task, created_at, from_request) VALUES"+
		"(10, 'decide', 'lead', 'p1', NULL, "+at(850)+", 1), (20, 'build', 'lead', 'p1', NULL, "+at(750)+", 2), (21, 'pick one', 'p1', 'lead', 20, "+at(700)+", NULL)")
	run(t, e, "INSERT INTO task_requests(task_id, request_id) VALUES(10, 1), (20, 2)")
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(10, 1, 'p1', 'lead', 1), (21, 1, 'dev', 'p1', 1)")
	rows, err := e.RequestRows(now)
	if err != nil {
		t.Fatal(err)
	}
	by := map[int64]map[string]any{}
	for _, r := range rows {
		if r["task"] != nil {
			by[r["task"].(int64)] = r
		}
	}
	if by[10]["status"] != reqWaitingOnYou {
		t.Fatalf("the founder on top: %v", by[10])
	}
	if by[20]["status"] == reqWaitingOnYou || by[20]["now"] != "dev" || fmt.Sprint(by[20]["holders"]) != "[dev lead]" {
		t.Fatalf("dev on top of the founder's child: %v", by[20])
	}
}

func TestMyTurnListsTheTasksTheFoundersTurnIsOn(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES"+
		"(1, 'on top', 'amy', 'amy', 1, 'amy'), (2, 'under amy', 'p1', 'amy', 1, 'amy')")
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(1, 1, 'p1', 'amy', 1), (2, 1, 'amy', 'p1', 1)")
	founder := &rpc.Conn{Caller: rpc.Caller{Role: "owner"}}
	list, err := e.tasksList(founder, map[string]any{"my_turn": true})
	if err != nil {
		t.Fatal(err)
	}
	var ids []any
	for _, it := range list.(map[string]any)["items"].([]any) {
		ids = append(ids, it.(map[string]any)["id"])
	}
	if fmt.Sprint(ids) != "[1]" {
		t.Fatalf("my turn: %v", ids)
	}
	got, err := e.Tasks([]int64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if got[1]["acting"] != "p1" || got[1]["to"] != "amy" || got[2]["acting"] != "amy" {
		t.Fatalf("acting: %v %v / %v", got[1]["acting"], got[1]["to"], got[2]["acting"])
	}
}

// The tick never tells a task the founder's turn is on that it is overdue, so its clock does not say so either; an agent on top of the
// founder's task is reminded, and the clock says so.
func TestAnOverdueClockFollowsWhoActsOnTheTask(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, timeout_s, timeout_at, sent_at) VALUES"+
		"(1, 'd', 'bob', 'p1', 1, 'p1', 600, ?, 1), (2, 'e', 'p1', 'p1', 1, 'p1', 600, ?, 1)", now-100, now-100)
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(1, 1, 'p1', 'bob', 1), (2, 1, 'amy', 'p1', 1)")
	run(t, e, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('amy', 'RUNNING', 0, 2)")
	if task, c := clockOf(t, e, 1); task["state"] != "doing" || c["overdue_since"] != nil {
		t.Fatalf("the founder on top: not overdue: %v %v", task["state"], c)
	}
	if task, c := clockOf(t, e, 2); task["state"] != "doing" || c["overdue_since"] != ts(now-100) {
		t.Fatalf("amy on top: overdue: %v %v", task["state"], c)
	}
}

// A turn's ask rewritten says when (edited) in every read of the task, and task.get alone lists the texts it replaced (earlier), oldest
// first by event order, from that turn's push on: a rewrite of an earlier founder's turn on the task is not this one's.
func TestARewrittenAskSaysEditedAndTaskGetListsWhatItReplaced(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'pick', 'amy', 'amy', 1, 'amy'), (2, 'plain', 'amy', 'amy', 1, 'amy')")
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(1, 1, 'p1', 'C?', 'amy', 10), (2, 1, 'p1', 'D?', 'amy', 10)")
	run(t, e, `INSERT INTO events(id, ts, agent, kind, payload) VALUES`+
		`(1, 5, 'amy', 'task_turn_push', '{"task_id":1,"to":"p1","ask":"old"}'), `+
		`(2, 6, 'amy', 'task_turn_ask', '{"task_id":1,"agent":"p1","from":"old","ask":"old2"}'), `+
		`(3, 10, 'amy', 'task_turn_push', '{"task_id":1,"to":"p1","ask":"A?"}'), `+
		`(4, 20, 'amy', 'task_turn_ask', '{"task_id":1,"agent":"p1","from":"A?","ask":"B?"}'), `+
		`(5, 15, 'amy', 'task_turn_ask', '{"task_id":1,"agent":"p1","from":"B?","ask":"C?"}')`)
	got, err := e.taskGet(nil, map[string]any{"task": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	turn := got.(map[string]any)["turns"].([]any)[0].(map[string]any)
	want := fmt.Sprint([]any{map[string]any{"ask": "A?", "at": ts(int64(10))}, map[string]any{"ask": "B?", "at": ts(int64(20))}})
	if turn["ask"] != "C?" || turn["edited"] != ts(int64(15)) || fmt.Sprint(turn["earlier"]) != want {
		t.Fatalf("%v", turn)
	}
	list, err := e.Tasks([]int64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	one, two := list[1]["turns"].([]any)[0].(map[string]any), list[2]["turns"].([]any)[0].(map[string]any)
	if _, has := one["earlier"]; has || one["edited"] != ts(int64(15)) || two["edited"] != nil {
		t.Fatalf("the list says edited only: %v / %v", one, two)
	}
}
