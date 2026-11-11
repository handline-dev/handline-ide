package reads

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/paths"
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
	run(t, e, `INSERT INTO agent_view(agent, view, updated) VALUES('bob', '{"parent": "amy"}', 1)`)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES"+
		"(1, 'todo', 'p1', 'p1', 1, NULL), (2, 'asked', 'bob', 'amy', 1, NULL), (3, 'parent', 'amy', 'amy', 1, NULL),"+
		"(4, 'child', 'bob', 'amy', 1, 3), (5, 'blocked', 'amy', 'amy', 1, NULL), (6, 'blocker', 'carol', 'amy', 1, NULL),"+
		"(7, 'shares', 'amy', 'amy', 1, NULL), (8, 'sharer', 'carol', 'amy', 1, 7), (9, 'back', 'amy', 'amy', 1, NULL),"+
		"(10, 'elsewhere', 'carol', 'carol', 1, NULL)")
	run(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, edge_ok) VALUES"+
		"(1, 1, 'amy', 'bob', 'b', 'queued', 'task', 1), (2, 2, 'amy', 'bob', 'b', 'queued', 'task', 1),"+
		"(3, 3, 'amy', 'carol', 'b', 'landed', 'say', 1), (4, 4, 'amy', 'carol', 'b', 'landed', 'say', 1)")
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
	amy := board(rpc.Caller{Role: "agent", Info: map[string]any{"kind": "agent", "name": "amy"}})
	want := map[string]map[int64]map[string]any{
		"mine": {3: now("sub_task", []any{"bob"}, []any{}), 5: now("waiting", []any{}, []any{int64(6)}),
			7: now("held_as_sub_task", []any{"carol"}, []any{int64(8)}), 9: now("result_in", []any{}, []any{}),
			// carol is not under amy, but amy opened these, so they are hers to watch
			6: now("nobody", []any{}, []any{}), 8: now("result_in", []any{}, []any{})},
		"reports": {2: now("held", []any{"bob"}, []any{}), 4: now("held", []any{"bob"}, []any{})},
	}
	for side, rows := range want {
		if len(amy[side]) != len(rows) {
			t.Errorf("amy's %s: %d rows, want %d: %v", side, len(amy[side]), len(rows), amy[side])
		}
		for id, n := range rows {
			if got := amy[side][id]; got == nil || !reflect.DeepEqual(got["now"], n) {
				t.Errorf("amy's %s T%d: %v, want now %v", side, id, got, n)
			}
		}
	}
	if h := amy["reports"][2]["holder"]; h != "bob" {
		t.Errorf("T2 holder %v", h)
	}
	if h := amy["mine"][9]["holder"]; h != nil {
		t.Errorf("T9 holder %v", h)
	}
	founder := board(rpc.Caller{Role: "founder"})
	if len(founder["mine"]) != 1 || founder["mine"][1] == nil || len(founder["reports"]) != 0 {
		t.Errorf("the founder's board: %v", founder)
	}
	if _, err := e.taskBoard(&rpc.Conn{Caller: rpc.Caller{Role: "founder"}}, map[string]any{"everyone": "yes"}); err == nil {
		t.Error("everyone yes was accepted")
	}
}

// The index against its budget, and a topic file unused for four weeks by the hook's record or, never recorded, by its mtime.
func TestMemoryStatusReadsTheIndexAndTheUnusedFiles(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "amy", "")
	addAgent(t, e, "bob", "")
	dir := filepath.Join(paths.AgentDir(e.Home, "amy"), "memory")
	os.MkdirAll(filepath.Join(dir, "deep"), 0o755)
	index := "# memory\n- [one](one.md) - a\n- [two](two.md) - b\n"
	os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(index), 0o644)
	for _, f := range []string{"one.md", "two.md", "deep/old.md"} {
		os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644)
	}
	now := time.Unix(100*86400, 0)
	old := now.Add(-40 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, "deep/old.md"), old, old) // never recorded: its mtime is old
	os.Chtimes(filepath.Join(dir, "one.md"), old, old)      // recorded as read yesterday
	os.Chtimes(filepath.Join(dir, "two.md"), now, now)      // recorded as read six weeks ago
	use := `{"one.md": ` + itoa(now.Unix()-86400) + `, "two.md": ` + itoa(now.Unix()-42*86400) + `}`
	os.WriteFile(filepath.Join(paths.AgentDir(e.Home, "amy"), "memory-use.json"), []byte(use), 0o644)
	got := memoryOf(e.Home, "amy", now)
	want := map[string]any{"agent": "amy", "bytes": len(index), "budget": memoryBudget, "entries": 2, "unused": []string{"deep/old.md", "two.md"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("amy: %v, want %v", got, want)
	}
	if got := memoryOf(e.Home, "bob", now); got["bytes"] != 0 || got["entries"] != 0 || len(got["unused"].([]string)) != 0 {
		t.Errorf("bob, no memory: %v", got)
	}
	all, err := e.memoryStatus(nil, map[string]any{})
	if err != nil || len(all.(map[string]any)["agents"].([]any)) != 2 {
		t.Fatalf("every agent: %v %v", all, err)
	}
	if _, err := e.memoryStatus(nil, map[string]any{"agent": "ghost"}); err == nil {
		t.Error("an unknown agent was read")
	}
}
