package reads

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

// runRows is the founder's run rows in requests.pending, as "task:holds", in order.
func runRows(t *testing.T, e *Env) string {
	t.Helper()
	var out []string
	for _, r := range pending(t, e)["answer"].([]map[string]any) {
		if r["kind"] == "run" {
			out = append(out, fmt.Sprint(r["task"], ":", r["holds"]))
		}
	}
	return strings.Join(out, " ")
}

func statusText(t *testing.T, e *Env, id int64) string {
	t.Helper()
	got, err := e.Tasks([]int64{id})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprint(got[id]["status"])
}

// A draft queued work waits on is a run row with what waits and since when; it leaves when the draft is queued or discarded or the waiting
// task closes. Drafts behind it alone make none.
func TestADraftQueuedWorkWaitsOnIsTheFoundersToRun(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) *Env {
		e := testEnv(t)
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, queued_at) VALUES"+
			"(1, 'draft', 'bob', 'p1', 10, NULL, NULL), (2, 'waits', 'amy', 'amy', 20, 'amy', 40), (3, 'waits too', 'amy', 'amy', 20, 'amy', 30)")
		run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, dormant, created_at) VALUES(2, 1, 0, 50), (3, 2, 0, 60)")
		return e
	}
	e := setup(t)
	if got := runRows(t, e); got != "1:[2 3]" {
		t.Fatalf("the draft holds both tasks of the chain: %q", got)
	}
	items, _, err := e.ForYou()
	if err != nil {
		t.Fatal(err)
	}
	var since any
	for _, it := range items {
		if it["asks"] == "run" {
			since = it["since"]
		}
	}
	if since != ts(int64(50)) {
		t.Fatalf("it waits since the first block: %v", since)
	}
	if got := statusText(t, e, 1); !strings.Contains(got, "waits for you to run it: T2, T3 wait on it") {
		t.Fatalf("the draft's status says it waits on the founder: %s", got)
	}
	for _, end := range []string{
		"UPDATE tasks SET queued_by = 'p1', queued_at = 70 WHERE id = 1",
		"UPDATE tasks SET discarded_by = 'p1', discarded_at = 70 WHERE id = 1",
		"UPDATE tasks SET done_evidence = 'x', done_at = 70 WHERE id IN (2, 3)",
	} {
		e := setup(t)
		run(t, e, end)
		if got := runRows(t, e); got != "" {
			t.Fatalf("%s: the row leaves: %q", end, got)
		}
	}
	e = setup(t)
	run(t, e, "UPDATE tasks SET queued_by = NULL WHERE id IN (2, 3)")
	if got := runRows(t, e); got != "" {
		t.Fatalf("drafts behind a draft hold nothing up: %q", got)
	}
}

// Two drafts under one queued parent are one row, on the parent.
func TestDraftsUnderOneParentAreOneRowOnIt(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, queued_at, parent_task) VALUES"+
		"(1, 'story', 'amy', 'p1', 10, 'p1', 20, NULL), (2, 'one', 'bob', 'p1', 30, NULL, NULL, 1), (3, 'two', 'bob', 'p1', 40, NULL, NULL, 1)")
	if got := runRows(t, e); got != "1:[]" {
		t.Fatalf("one row on the parent: %q", got)
	}
}

// A founder's turn on a task queued work waits on says what waits, and comes before a row that holds nothing up.
func TestAFounderTurnCarriesWhatWaitsBehindIt(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, queued_at) VALUES"+
		"(1, 'decide', 'bob', 'bob', 10, 'bob', 10), (2, 'waits', 'amy', 'amy', 20, 'amy', 20), (3, 'newer', 'bob', 'bob', 10, 'bob', 10)")
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(1, 1, 'p1', 'pick one', 'bob', 30), (3, 1, 'p1', 'look', 'bob', 90)")
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, dormant, created_at) VALUES(2, 1, 0, 25)")
	rows := pending(t, e)["answer"].([]map[string]any)
	if len(rows) != 2 || rows[0]["task"] != int64(1) || fmt.Sprint(rows[0]["holds"]) != "[2]" || fmt.Sprint(rows[1]["holds"]) != "[]" {
		t.Fatalf("the turn that holds T2 up comes first and says so: %v", rows)
	}
}

// An agent's draft it left behind shows once its opener ended a task since, until the founder sees it; a new comment brings it back.
func TestADraftItsAgentLeftBehindShowsUntilSeen(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'left', 'bob', 'bob', 100)")
	run(t, e, `INSERT INTO events(ts, agent, kind, payload) VALUES(90, 'bob', 'mark_task_done', '{"task_id": 7}')`)
	if got := runRows(t, e); got != "" {
		t.Fatalf("its agent has not moved on yet: %q", got)
	}
	run(t, e, `INSERT INTO events(ts, agent, kind, payload) VALUES(150, 'bob', 'task_handoff', '{"task_id": 8}')`)
	if got := runRows(t, e); got != "1:[]" {
		t.Fatalf("left behind once its agent handed a task on: %q", got)
	}
	if got := statusText(t, e, 1); !strings.Contains(got, "bob left it for you to run") {
		t.Fatalf("the status says who left it: %s", got)
	}
	run(t, e, "INSERT INTO cursors(name, row_id, updated) VALUES('task_seen:p1:1', 0, 1)")
	if got := runRows(t, e); got != "" {
		t.Fatalf("seen, it leaves: %q", got)
	}
	run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(5, 1, 'bob', 'more', 200)")
	if got := runRows(t, e); got != "1:[]" {
		t.Fatalf("a later comment brings it back: %q", got)
	}
}

// What keeps a left draft off: the founder made it, it waits on an open task, the task it serves has the founder's turn or is the founder's
// and done.
func TestALeftDraftStaysOffWhenTheFounderHasItInHand(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ draft, more string }{
		"the founder made it":   {"(1, 'd', 'bob', 'p1', 100, NULL)", ""},
		"it waits on a task":    {"(1, 'd', 'bob', 'bob', 100, NULL)", "INSERT INTO task_blocks(task_id, blocks_on, dormant, created_at) VALUES(1, 2, 0, 100)"},
		"served has his turn":   {"(1, 'd', 'bob', 'bob', 100, 2)", "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(2, 1, 'p1', 'x', 'bob', 100)"},
		"served is his, done":   {"(1, 'd', 'bob', 'bob', 100, 3)", ""},
		"served asks him, done": {"(1, 'd', 'bob', 'bob', 100, 4)", ""},
	} {
		e := testEnv(t)
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, request) VALUES"+c.draft)
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, done_evidence) VALUES"+
			"(2, 'open', 'amy', 'amy', 10, 'amy', NULL), (3, 'his', 'p1', 'p1', 10, 'p1', 'done'), (4, 'asked by him', 'amy', 'p1', 10, 'p1', 'done')")
		run(t, e, `INSERT INTO events(ts, agent, kind, payload) VALUES(150, 'bob', 'mark_task_done', '{"task_id": 9}'), (150, 'p1', 'mark_task_done', '{"task_id": 9}')`)
		if c.more != "" {
			run(t, e, c.more)
		}
		for _, r := range pending(t, e)["answer"].([]map[string]any) {
			if r["kind"] == "run" {
				t.Fatalf("%s: no run row: %v", name, r)
			}
		}
	}
}

// A cycle of blocks and a parent link ends, and each task waits from its earliest path.
func TestAWaitCycleEnds(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, queued_at, parent_task) VALUES"+
		"(1, 'draft', 'bob', 'p1', 10, NULL, NULL, 3), (2, 'a', 'amy', 'amy', 10, 'amy', 20, NULL), (3, 'b', 'amy', 'amy', 10, 'amy', 30, NULL)")
	run(t, e, "INSERT INTO task_blocks(task_id, blocks_on, dormant, created_at) VALUES(2, 1, 0, 25), (3, 2, 0, 35), (2, 3, 0, 40)")
	b, err := e.loadBoard()
	if err != nil {
		t.Fatal(err)
	}
	held, since, err := e.heldOn(1, b)
	if err != nil || fmt.Sprint(held) != "[2 3]" || since != 25 {
		t.Fatalf("both wait, the first since 25: %v %d %v", held, since, err)
	}
}

// A draft with no turn shows its agent opener's turn, as a person's comment on it is told there; queued, opened by a person, or carrying
// a turn, it shows who acts on it.
func TestADraftShowsItsOpenersTurn(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	addAgent(t, e, "carol", "")
	bob, carol := teamtest.Key(t, e.Home, "bob"), teamtest.Key(t, e.Home, "carol")
	run(t, e, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, queued_at) VALUES"+
		"(1, 'draft', '%[1]s', '%[2]s', 10, NULL, NULL), (2, 'queued', '%[1]s', '%[2]s', 10, '%[2]s', 20), (3, 'the founder''s draft', '%[1]s', 'p1', 10, NULL, NULL)", bob, carol))
	run(t, e, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(4, 'a draft with a turn', '%s', '%s', 10)", bob, carol))
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(4, 1, 'p1', 'p1', 20)")
	got, err := e.Tasks([]int64{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if got[1]["acting"] != carol || got[2]["acting"] != bob || got[3]["acting"] != bob || got[4]["acting"] != "p1" {
		t.Fatalf("draft %v, queued %v, the founder's draft %v, with a turn %v", got[1]["acting"], got[2]["acting"], got[3]["acting"], got[4]["acting"])
	}
}

// A draft its opener asked the founder on is theirs with Run, so Run answers it; a draft with no turn is not.
func TestADraftInTheFoundersTurnIsTheirsToRun(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	addAgent(t, e, "carol", "")
	bob, carol := teamtest.Key(t, e.Home, "bob"), teamtest.Key(t, e.Home, "carol")
	run(t, e, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'asked', '%[1]s', '%[2]s', 10), (2, 'not asked', '%[1]s', '%[2]s', 10)", bob, carol))
	run(t, e, fmt.Sprintf("INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(1, 1, 'p1', 'GitLab too?', '%s', 20)", carol))
	items, _, err := e.ForYou()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items {
		got = append(got, fmt.Sprint(it["kind"], ":", it["id"], ":", it["asks"]))
	}
	if strings.Join(got, " ") != "task:1:run" {
		t.Fatalf("%q", got)
	}
	// its Your turn row is a run row, which carries Run, with the opener's question
	var rows []string
	for _, r := range pending(t, e)["answer"].([]map[string]any) {
		rows = append(rows, fmt.Sprint(r["task"], ":", r["kind"], ":", r["ask"], ":", r["agent"] == carol))
	}
	if strings.Join(rows, " ") != "1:run:GitLab too?:true" {
		t.Fatalf("%q", rows)
	}
}
