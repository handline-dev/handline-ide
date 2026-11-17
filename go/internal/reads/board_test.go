package reads

import (
	"reflect"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"

	"github.com/handline/handline/go/protocol/rpc"
)

// Each way a task can be held, read off its linked requests, as amy (who leads bob) sees her board; the founder's own board is its
// own tasks.
func TestTheBoardSaysWhoHoldsEachTask(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for _, a := range []string{"amy", "bob", "carol"} {
		addAgent(t, e, a, "")
	}
	amy, bob, carol := teamtest.Key(t, e.Home, "amy"), teamtest.Key(t, e.Home, "bob"), teamtest.Key(t, e.Home, "carol")
	run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES('`+bob+`', '{"parent": "`+amy+`"}', 1)`)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES"+
		"(1, 'todo', 'p1', 'p1', 1, NULL), (2, 'asked', '"+bob+"', '"+amy+"', 1, NULL), (3, 'parent', '"+amy+"', '"+amy+"', 1, NULL),"+
		"(4, 'child', '"+bob+"', '"+amy+"', 1, 3), (5, 'blocked', '"+amy+"', '"+amy+"', 1, NULL), (6, 'blocker', '"+carol+"', '"+amy+"', 1, NULL),"+
		"(7, 'shares', '"+amy+"', '"+amy+"', 1, NULL), (8, 'sharer', '"+carol+"', '"+amy+"', 1, 7), (9, 'back', '"+amy+"', '"+amy+"', 1, NULL),"+
		"(10, 'elsewhere', '"+carol+"', '"+carol+"', 1, NULL)")
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES"+
		"(1, 1, '"+amy+"', '"+bob+"', 'b', 'queued', 'task', 1), (2, 2, '"+amy+"', '"+bob+"', 'b', 'queued', 'task', 1),"+
		"(3, 3, '"+amy+"', '"+carol+"', 'b', 'landed', 'say', 1), (4, 4, '"+amy+"', '"+carol+"', 'b', 'landed', 'say', 1)")
	run(t, e, "INSERT INTO task_requests(task_id, request_id) VALUES(2, 1), (4, 2), (7, 3), (8, 3), (9, 4)")
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, dormant, created_at) VALUES(5, 6, 0, 1)")
	now := func(kind string, agents []any, tasks []any) map[string]any {
		return map[string]any{"kind": kind, "agents": agents, "tasks": tasks}
	}
	board := func(c rpc.Caller) map[string]map[int64]map[string]any {
		got, err := e.taskBoard(&rpc.Conn{Caller: c}, map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]map[int64]map[string]any{}
		for side, rows := range got.(map[string]any) {
			out[side] = map[int64]map[string]any{}
			for _, r := range rows.([]any) {
				r := r.(map[string]any)
				out[side][r["task"].(map[string]any)["id"].(int64)] = r
			}
		}
		return out
	}
	amyBoard := board(rpc.Caller{Role: "agent", Kind: "agent", Key: amy, Info: map[string]any{"kind": "agent", "name": amy}})
	want := map[string]map[int64]map[string]any{
		"mine": {3: now("sub_task", []any{bob}, []any{}), 5: now("waiting", []any{}, []any{int64(6)}),
			7: now("held_as_sub_task", []any{carol}, []any{int64(8)}), 9: now("result_in", []any{}, []any{}),
			// carol is not under amy, but amy opened these, so they are hers to watch
			6: now("nobody", []any{}, []any{}), 8: now("result_in", []any{}, []any{})},
		"reports": {2: now("held", []any{bob}, []any{}), 4: now("held", []any{bob}, []any{})},
	}
	for side, rows := range want {
		if len(amyBoard[side]) != len(rows) {
			t.Errorf("amy's %s: %d rows, want %d: %v", side, len(amyBoard[side]), len(rows), amyBoard[side])
		}
		for id, n := range rows {
			if got := amyBoard[side][id]; got == nil || !reflect.DeepEqual(got["now"], n) {
				t.Errorf("amy's %s T%d: %v, want now %v", side, id, got, n)
			}
		}
	}
	if h := amyBoard["reports"][2]["holder"]; h != bob {
		t.Errorf("T2 holder %v", h)
	}
	if h := amyBoard["mine"][9]["holder"]; h != nil {
		t.Errorf("T9 holder %v", h)
	}
	founder := board(rpc.Caller{Role: "owner"})
	if len(founder["mine"]) != 1 || founder["mine"][1] == nil || len(founder["reports"]) != 0 {
		t.Errorf("the founder's board: %v", founder)
	}
	if _, err := e.taskBoard(&rpc.Conn{Caller: rpc.Caller{Role: "owner"}}, map[string]any{"everyone": "yes"}); err == nil {
		t.Error("everyone yes was accepted")
	}
}
