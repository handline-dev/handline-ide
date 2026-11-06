package reads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/protocol/rpc"
)

var asFounderConn = &rpc.Conn{Caller: rpc.Caller{Role: "founder"}}

// closedPage is one page of tasks.list in closed order: its ids and the cursor to go on with ("" at the end).
func closedPage(t *testing.T, e *Env, params map[string]any) (ids string, next string) {
	t.Helper()
	got, err := e.tasksList(asFounderConn, params)
	if err != nil {
		t.Fatal(err)
	}
	res := got.(map[string]any)
	var list []any
	for _, it := range res["items"].([]any) {
		list = append(list, it.(map[string]any)["id"])
	}
	next, _ = res["next"].(string)
	if (next != "") != (res["more"] == true) {
		t.Errorf("more %v with next %q", res["more"], next)
	}
	return fmt.Sprint(list), next
}

func TestClosedOrderListsTheNewestCloseFirstWhateverTheIdAndPagesWithoutGapOrRepeat(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, done_evidence, done_at) VALUES"+
		"(1, 'old id closed last', 'bob', 'amy', 1, 'x', 100), (2, 'b', 'bob', 'amy', 1, 'x', 50), (4, 'd', 'bob', 'amy', 1, 'x', 100), "+
		"(5, 'e', 'bob', 'amy', 1, 'x', 30), (7, 'open', 'bob', 'amy', 1, NULL, NULL)")
	got, _ := closedPage(t, e, map[string]any{"state": "done", "order": "closed"})
	if got != "[4 1 2 5]" {
		t.Fatalf("closed order: %s (a tie goes to the newer id)", got)
	}
	if byID, _ := closedPage(t, e, map[string]any{"state": "done"}); byID != "[5 4 2 1]" {
		t.Fatalf("id order, unchanged: %s", byID)
	}
	var all string
	for cursor, n := "", 0; ; n++ {
		params := map[string]any{"state": "done", "order": "closed", "limit": json.Number("1")}
		if cursor != "" {
			params["cursor"] = cursor
		}
		ids, next := closedPage(t, e, params)
		all += ids
		if cursor = next; cursor == "" {
			break
		}
		if n > 5 {
			t.Fatal("paging does not end")
		}
	}
	if all != "[4][1][2][5]" {
		t.Errorf("one row a page: %s", all)
	}
}

func TestClosedOrderReadsADiscardedTasksDiscardTimeNotItsDoneTime(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, discarded_by, discarded_at, done_at) VALUES"+
		"(3, 'c', 'bob', 'amy', 1, 'amy', 100, NULL), (6, 'f', 'bob', 'amy', 1, 'amy', 10, 999)")
	if got, _ := closedPage(t, e, map[string]any{"state": "discarded", "order": "closed"}); got != "[3 6]" {
		t.Fatalf("%s", got)
	}
}

func TestClosedOrderRefusesWhatItCannotPage(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for name, params := range map[string]map[string]any{
		"no state":            {"order": "closed"},
		"an open state":       {"order": "closed", "state": "doing"},
		"with before":         {"order": "closed", "state": "done", "before": json.Number("5")},
		"an unknown order":    {"order": "oldest", "state": "done"},
		"a cursor, id order":  {"state": "done", "cursor": closedCursor(5, 2)},
		"a garbled cursor":    {"order": "closed", "state": "done", "cursor": "!!"},
		"a cursor of no task": {"order": "closed", "state": "done", "cursor": closedCursor(5, 0)},
	} {
		if _, err := e.tasksList(asFounderConn, params); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestTasksListIdsReturnsOnlyThoseTasksInScopeNewestFirstAndLeavesOutTheRest(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, done_evidence, done_at) VALUES"+
		"(1, 'a', 'bob', 'amy', 1, 'x', 100), (2, 'b', 'bob', 'amy', 1, NULL, NULL), (3, 'c', 'bob', 'amy', 1, NULL, NULL), (4, 'd', 'cat', 'dan', 1, NULL, NULL)")
	ids := func(c *rpc.Conn, p map[string]any) string {
		t.Helper()
		got, err := e.tasksList(c, p)
		if err != nil {
			t.Fatal(err)
		}
		var list []any
		for _, it := range got.(map[string]any)["items"].([]any) {
			list = append(list, it.(map[string]any)["id"])
		}
		return fmt.Sprint(list)
	}
	nums := func(n ...int) []any {
		out := []any{}
		for _, v := range n {
			out = append(out, json.Number(fmt.Sprint(v)))
		}
		return out
	}
	if got := ids(asFounderConn, map[string]any{"ids": nums(1, 99, 3)}); got != "[3 1]" {
		t.Errorf("done tasks too, an unknown id left out, newest first: %s", got)
	}
	if got := ids(asFounderConn, map[string]any{"ids": nums(1, 2, 3), "limit": json.Number("2")}); got != "[3 2]" {
		t.Errorf("an explicit limit still caps: %s", got)
	}
	if got := ids(asFounderConn, map[string]any{"ids": nums()}); got != "[]" {
		t.Errorf("no ids, no rows: %s", got)
	}
	os.WriteFile(filepath.Join(e.Home, "team.yaml"), []byte("agents:\n  amy: {}\n  bob: {}\n  cat: {}\n"), 0o644)
	if got := ids(asBob, map[string]any{"ids": nums(2, 4)}); got != "[2]" {
		t.Errorf("a task out of the caller's scope is left out: %s", got)
	}
	long := make([]any, 201)
	for i := range long {
		long[i] = json.Number(fmt.Sprint(i + 1))
	}
	many := make([]any, 60)
	for i := range many {
		many[i] = json.Number(fmt.Sprint(i + 1))
	}
	run(t, e, "WITH RECURSIVE n(i) AS (SELECT 5 UNION ALL SELECT i+1 FROM n WHERE i < 60) INSERT INTO tasks(id, title, owner, opened_by, created_at) SELECT i, 't', 'bob', 'amy', 1 FROM n")
	got, err := e.tasksList(asFounderConn, map[string]any{"ids": many})
	if res := got.(map[string]any); err != nil || len(res["items"].([]any)) != 60 || res["more"] != false {
		t.Errorf("60 ids and no limit: all 60, nothing more: %v %v", got, err)
	}
	for _, bad := range [][]any{{json.Number("1"), "x"}, {json.Number("1.5")}} {
		if _, err := e.tasksList(asFounderConn, map[string]any{"ids": bad}); err == nil || !strings.Contains(err.Error(), "ids") {
			t.Errorf("ids %v: bad_request naming ids: %v", bad, err)
		}
	}
	for name, p := range map[string]map[string]any{
		"before": {"ids": nums(1), "before": json.Number("5")}, "cursor": {"ids": nums(1), "cursor": "x"},
		"order": {"ids": nums(1), "order": "closed", "state": "done"}, "ids": {"ids": long},
	} {
		if _, err := e.tasksList(asFounderConn, p); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: bad_request naming it: %v", name, err)
		}
	}
}
