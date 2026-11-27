package taskwrites

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/protocol/rpc"
)

func TestQueueAndDraftAreAllOrNone(t *testing.T) {
	t.Parallel()
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	home := t.TempDir()
	teamtest.Seed(t, home, "worker", "manager", "other")
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', 'worker', 'manager', 1), (2, 'b', 'worker', 'manager', 1), (3, 'c', 'worker', 'manager', 1)",
		"UPDATE tasks SET done_evidence = 'x', done_at = 2 WHERE id = 3",
	} {
		if _, err := db.Exec(keyed(db, q)); err != nil {
			t.Fatal(err)
		}
	}
	mk := func(agent string) *rpc.Server { return writesServer(agent, db, home, nil) }
	line := func(id int, method, rid string, tasks ...int) string {
		list, _ := json.Marshal(tasks)
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":{"tasks":%s,"request_id":%q}}`, id, method, list, rid)
	}
	state := func(id int) (queued bool) {
		var q *string
		db.QueryRow("SELECT queued_by FROM tasks WHERE id = ?", id).Scan(&q)
		return q != nil
	}
	if r := call(t, mk("manager"), line(2, "task.queue", "m0", 1, 3)); r[0]["error"] == nil || state(1) {
		t.Fatalf("one task that cannot be queued stops all of them: %v", r[0])
	}
	r := call(t, mk("manager"), line(2, "task.queue", "m1", 1, 2), line(3, "task.queue", "m2", 1))
	if r[0]["result"] == nil {
		t.Fatalf("queue refused: %v", r[0])
	}
	if r[0]["result"].(map[string]any)["said"] != "task T1 is queued (manager)\ntask T2 is queued (manager)" || !state(1) || !state(2) || r[1]["error"] == nil {
		t.Fatalf("%v %v", r[0], r[1])
	}
	var kind string
	db.QueryRow(keyed(db, "SELECT kind FROM events WHERE agent = 'manager' AND kind != 'rpc_write' ORDER BY id DESC LIMIT 1")).Scan(&kind)
	if kind != "queue_task" {
		t.Fatalf("the write leaves an event: %q", kind)
	}
	r = call(t, mk("manager"), line(2, "task.draft", "m3", 1), line(3, "task.draft", "m4", 1))
	if r[0]["error"] != nil || state(1) || r[1]["error"] == nil {
		t.Fatalf("a queued task goes back to draft once: %v %v", r[0], r[1])
	}
	f := call(t, mk(""), line(2, "task.queue", "f1", 1))
	if f[0]["error"] != nil || !state(1) {
		t.Fatalf("the founder queues: %v", f[0])
	}
	bad := call(t, mk(""), `{"jsonrpc":"2.0","id":2,"method":"task.queue","params":{"tasks":"1","request_id":"b1"}}`)
	if bad[0]["error"] == nil {
		t.Fatalf("tasks is a list of numbers: %v", bad[0])
	}

	// a task with work out on it is not taken back or queued again; held back behind its owner's current task it reads queued
	db.Exec(keyed(db, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at) VALUES(4, 'd', 'worker', 'manager', 1, 'manager', 5)"))
	if _, err := db.Exec(keyed(db, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('worker', 'RUNNING', 1, 4)")); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"task.draft", "task.queue"} {
		r := call(t, mk("manager"), line(2, m, "d-"+m, 4))
		if msg := errText(r[0]); msg != "task T4 is still doing" && msg != "task T4 is doing - only a draft task can be made queued" {
			t.Fatalf("%s of a task with a request out: %v", m, r[0])
		}
	}
	db.Exec(keyed(db, "UPDATE agent_state SET current_task = 1 WHERE agent = 'worker'"))                                                // it holds another open task: this one is asked but held back
	db.Exec(keyed(db, "INSERT INTO tasks(id, title, owner, opened_by, created_at, sent_at) VALUES(6, 'f', 'worker', 'manager', 1, 5)")) // never approved, asked, held back
	if msg := errText(call(t, mk("manager"), line(2, "task.queue", "h1", 6))[0]); msg != "task T6 is already queued" {
		t.Fatalf("asked but held back reads queued: %q", msg)
	}
	db.Exec(keyed(db, "UPDATE tasks SET discarded_by = 'manager', discarded_at = 3 WHERE id = 2"))
	if msg := errText(call(t, mk("manager"), line(2, "task.draft", "x1", 2))[0]); msg != "task T2 is discarded - only a queued or doing task can be made draft" {
		t.Fatalf("a discarded task: %q", msg)
	}
	nf := call(t, mk("manager"), line(2, "task.queue", "n1", 99))[0]["error"].(map[string]any)
	if nf["data"].(map[string]any)["what"] != "task" {
		t.Fatalf("an unknown task is not_found: %v", nf)
	}
	db.Exec(keyed(db, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(5, 'e', 'worker', 'manager', 1)"))
	dup := call(t, mk("manager"), line(2, "task.queue", "u1", 5, 5))[0]["result"].(map[string]any)
	if dup["said"] != "task T5 is queued (manager)" {
		t.Fatalf("a repeated id queues once: %v", dup)
	}
}

func errText(r map[string]any) string {
	e, ok := r["error"].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := e["message"].(string)
	return s
}

// A draft is worked by nobody: the delivery still waiting is withdrawn, one that has landed is ended, and the task then reads draft.
func TestDraftWithdrawsTheDeliveryNotYetLandedAndEndsOneThatHas(t *testing.T) {
	t.Parallel()
	sdb, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at) VALUES(1, 'sent', 'w', 'boss', 1, NULL, 5), " +
			"(2, 'landed', 'w', 'boss', 1, 'boss', 5), (3, 'approved', 'w', 'boss', 1, 'boss', NULL), (4, 'parent', 'w', 'boss', 1, NULL, NULL), " +
			"(5, 'child', 'w', 'boss', 1, NULL, 5), (7, 'held', 'w', 'boss', 1, 'boss', 5)",
		"UPDATE tasks SET parent_task = 4 WHERE id = 5",
		"INSERT INTO requests(id, requester, target, kind, state, ts, body, delivered_at) VALUES(10, 'boss', 'w', 'task', 'queued', 1, 'x', NULL), " +
			"(11, 'boss', 'w', 'task', 'landed', 1, 'x', 6), (12, 'boss', 'w', 'task', 'queued', 1, 'x', NULL)",
		"UPDATE requests SET queue_order = 1 WHERE id = 11",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 10), (2, 11), (5, 12)",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('w', 'RUNNING', 1, 7)", // w works T7: the others are held back
	} {
		if _, err := sdb.Exec(keyed(sdb, q)); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	f := &fakeTeller{}
	s := rpc.NewServer(auth{"boss"}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	s.DB = sdb
	Register(s, t.TempDir(), f, nil)
	draft := func(rid string, tid int) map[string]any {
		return call(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.draft","params":{"tasks":[%d],"request_id":%q}}`, tid, rid))[0]
	}
	stateOf := func(tid int64) string {
		tx, _ := sdb.Begin()
		defer tx.Rollback()
		tk, _ := getTask(tx, tid)
		st, err := state(tx, tk)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	var reqState string

	r := draft("d1", 1)
	if r["error"] != nil || r["result"].(map[string]any)["said"] != "delivered T1 - 1 stopped" || stateOf(1) != "draft" {
		t.Fatalf("a delivery not yet landed is withdrawn: %v, T1 %s", r, stateOf(1))
	}
	sdb.QueryRow(keyed(sdb, "SELECT state FROM requests WHERE id = 10")).Scan(&reqState)
	if reqState != "withdrawn" || len(f.released) != 1 || f.released[0].RID != 10 || f.released[0].Held {
		t.Fatalf("#10 %s, told %+v", reqState, f.released)
	}

	f.released = nil
	if r := draft("d2", 2); r["error"] != nil || stateOf(2) != "draft" {
		t.Fatalf("a landed delivery held back behind w's current task: %v, T2 %s", r, stateOf(2))
	}
	var order *float64
	sdb.QueryRow(keyed(sdb, "SELECT state, queue_order FROM requests WHERE id = 11")).Scan(&reqState, &order)
	if reqState != "landed" || order != nil || f.released != nil {
		t.Fatalf("ended, and w (working T7) not told to stop: #11 %s %v, told %+v", reqState, order, f.released)
	}

	if r := draft("d3", 3); r["error"] != nil || stateOf(3) != "draft" || f.released != nil {
		t.Fatalf("a queued task with no request out: %v, T3 %s", r, stateOf(3))
	}

	if e := errText(draft("d4", 4)); e != "task T4 is already draft" {
		t.Fatalf("a parent with only its child out has nothing of its own out: %q", e)
	}
}

// A task that landed with its holder working it is parked: the holder is stopped and moved on, its timeout paused, and what it left in its
// worktree is kept in a memo.
func TestDraftParksALandedTaskItsHolderIsWorking(t *testing.T) {
	t.Parallel()
	sdb, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()
	wt := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if r := gitrun.Run(wt, args...); r.RC != 0 {
			t.Fatalf("git %v: %s", args, r.Stderr)
		}
	}
	git("init", "-q", "-b", "master") // the commits kept are those the repository's own default branch lacks, here not called main
	git("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "base")
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at, timeout_at) VALUES(1, 'idle', 'w', 'boss', 1, 'boss', 5, 99), " +
			"(2, 'with commits', 'v', 'boss', 1, 'boss', 5, 99)",
		"INSERT INTO requests(id, requester, target, kind, state, ts, body, delivered_at, landed_at, queue_order) VALUES" +
			"(11, 'boss', 'w', 'task', 'landed', 1, 'x', 6, 6, 1), (12, 'boss', 'v', 'task', 'landed', 1, 'x', 6, 6, 1)",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 11), (2, 12)",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('w', 'RUNNING', 1, 1)",
	} {
		if _, err := sdb.Exec(keyed(sdb, q)); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := sdb.Exec("INSERT INTO agent_state(agent, state, updated, current_task, worktree) VALUES('v', 'RUNNING', 1, 2, ?)", "repo::"+wt); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir() // w and v are agents, so a notice to either is written
	teamtest.Seed(t, home, "w", "v")
	f := &fakeTeller{}
	s := rpc.NewServer(auth{"boss"}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	s.DB = sdb
	Register(s, home, f, nil)
	draft := func(rid string, tid int) map[string]any {
		return call(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.draft","params":{"tasks":[%d],"request_id":%q}}`, tid, rid))[0]
	}
	read := func(tid int64) (st string, timeout *int64, memos []string) {
		tx, _ := sdb.Begin()
		defer tx.Rollback()
		tk, _ := getTask(tx, tid)
		if st, err = state(tx, tk); err != nil {
			t.Fatal(err)
		}
		tx.QueryRow("SELECT timeout_at FROM tasks WHERE id = ?", tid).Scan(&timeout)
		rows, _ := tx.Query("SELECT body FROM task_comments WHERE task_id = ?", tid)
		for rows.Next() {
			var b string
			rows.Scan(&b)
			memos = append(memos, b)
		}
		rows.Close()
		return
	}

	r := draft("p1", 1)
	if st, timeout, memos := read(1); r["error"] != nil || st != "draft" || timeout != nil || memos != nil {
		t.Fatalf("a landed task with an idle holder: %v, T1 %s timeout %v memos %q", r, st, timeout, memos)
	}
	if said := r["result"].(map[string]any)["said"]; said != "delivered T1 - 1 stopped" ||
		len(f.released) != 1 || f.released[0] != (Retired{RID: 11, Requester: "boss", Target: "w", Held: true, Park: true}) {
		t.Fatalf("w is parked: %q %+v", said, f.released)
	}
	var notices int
	sdb.QueryRow("SELECT COUNT(*) FROM requests WHERE requester = ? AND target = 'w'", noticeFrom).Scan(&notices)
	if notices != 0 {
		t.Fatalf("the parked holder is told by its stop, not by the draft notice too: %d notices", notices)
	}

	os.WriteFile(filepath.Join(wt, "a.go"), []byte("package a\n"), 0o644)
	git("checkout", "-q", "-b", "work")
	git("add", "a.go")
	git("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "half done")
	head := strings.TrimSpace(gitrun.Run(wt, "rev-parse", "--short", "HEAD").Stdout)
	f.released = nil
	r = draft("p2", 2)
	st, _, memos := read(2)
	want := fmt.Sprintf("parked while v worked it: branch work @ %s in %s (1 commit(s) since it landed) - resume from there", head, wt)
	if r["error"] != nil || st != "draft" || len(memos) != 1 || memos[0] != want || len(f.released) != 1 || !f.released[0].Park {
		t.Fatalf("a landed task with commits: %v, T2 %s memos %q, told %+v", r, st, memos, f.released)
	}
	if said := r["result"].(map[string]any)["said"]; said != "delivered T2, comment 1 keeps where v stopped - 1 stopped" {
		t.Fatalf("the reply names the memo: %q", said)
	}
}

// The founder's Run of the last drafts under a parent whose question names no task takes their turn off the parent with no message: it goes
// back to its holder, nobody is told or asked. A Run that leaves a draft sub-task, or by one without the turn, leaves the parent as it is.
func TestTheFoundersRunEndsTheirTurnOnTheParentQuietly(t *testing.T) {
	t.Parallel()
	for _, steps := range [][][]int{{{2, 3}}, {{2}, {3}}} {
		run := steps[len(steps)-1]
		sdb, mk := discardEnv(t)
		exec(t, sdb,
			"INSERT INTO tasks(id, title, owner, opened_by, created_at, sent_at) VALUES(1, 'plan', 'worker', 'manager', 1, 1)",
			"INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES(2, 'a', 'other', 'manager', 1, 1), (3, 'b', 'other', 'manager', 1, 1)",
			"INSERT INTO task_turns(task_id, pos, agent, pushed_by, since, ask) VALUES(1, 1, 'p1', 'worker', 1, 'run the plan')")
		f := &fakeTeller{}
		for i, run := range steps {
			list, _ := json.Marshal(run)
			r := call(t, mk("human", f), fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"task.queue","params":{"tasks":%s,"request_id":"q%d"}}`, list, i))
			if r[0]["error"] != nil {
				t.Fatalf("run %v: %v", run, r[0])
			}
			if left := count(t, sdb, "SELECT COUNT(*) FROM task_turns WHERE task_id = 1"); i < len(steps)-1 && left != 1 {
				t.Fatalf("run %v with a draft sub-task left took the founder's turn off the parent", run)
			}
		}
		if count(t, sdb, "SELECT COUNT(*) FROM task_turns WHERE task_id = 1") != 0 ||
			count(t, sdb, "SELECT COUNT(*) FROM agent_state WHERE agent = 'worker' AND current_task = 1") != 0 {
			t.Fatalf("run %v: the founder's turn stays on the parent, or its holder was handed it", run)
		}
		if n := count(t, sdb, "SELECT COUNT(*) FROM requests WHERE target = 'worker'") + count(t, sdb, "SELECT COUNT(*) FROM task_comments WHERE task_id = 1"); n != 0 || len(f.handed) != 0 || len(f.memos) != 0 {
			t.Fatalf("run %v: the parent's holder was told: %d rows, handed %v, memos %v", run, n, f.handed, f.memos)
		}
		for _, n := range f.notified {
			if n.To == "worker" {
				t.Fatalf("run %v: the parent's holder was told %q", run, n.Text)
			}
		}
	}
	for i, c := range []struct {
		by, more, why string
		turns         int
	}{
		{"manager", "INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(1, 2, 'manager', 'p1', 1)", "an agent's Run on its own turn", 2},
		{"human", "INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(1, 2, 'gp', 'p1', 1)", "an agent's turn above the founder's", 2},
		{"human", `INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'worker', 'task_ready', '{"task_id": 1}')`, "a Ready parent", 1},
	} {
		sdb, mk := discardEnv(t)
		exec(t, sdb,
			"INSERT INTO tasks(id, title, owner, opened_by, created_at, sent_at) VALUES(1, 'plan', 'worker', 'manager', 1, 1)",
			"INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES(2, 'a', 'other', 'manager', 1, 1)",
			"INSERT INTO task_turns(task_id, pos, agent, pushed_by, since) VALUES(1, 1, 'p1', 'worker', 1)")
		if c.more != "" {
			exec(t, sdb, c.more)
		}
		if r := call(t, mk(c.by, &fakeTeller{}), fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"task.queue","params":{"tasks":[2],"request_id":"k%d"}}`, i)); r[0]["error"] != nil {
			t.Fatalf("%s: %v", c.why, r[0])
		}
		if count(t, sdb, "SELECT COUNT(*) FROM task_turns WHERE task_id = 1") != c.turns || count(t, sdb, "SELECT COUNT(*) FROM task_turns WHERE task_id = 1 AND agent = 'p1'") != 1 {
			t.Fatalf("%s took a turn off the parent", c.why)
		}
	}
}

// questionEnv is task 1 with the founder's turn on it asking ask, its own draft sub-task 4, and drafts 2 and 3 of another parent, 5.
func questionEnv(t *testing.T, ask string) (*sql.DB, func(agent string, tell ...Teller) *rpc.Server) {
	t.Helper()
	sdb, mk := discardEnv(t)
	exec(t, sdb,
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, sent_at) VALUES(1, 'asks', 'worker', 'manager', 1, 1), (5, 'other plan', 'other', 'manager', 1, 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES(2, 'b', 'other', 'manager', 1, 5), (3, 'c', 'other', 'manager', 1, 5), (4, 'own', 'other', 'manager', 1, 1)",
		"INSERT INTO task_turns(task_id, pos, agent, pushed_by, since, ask) VALUES(1, 1, 'p1', 'worker', 1, '"+ask+"')")
	return sdb, mk
}

// A question that names drafts is answered by the founder's Run or discard of the last of them, wherever they are; until then it stays open.
func TestRunningTheDraftsAQuestionNamesAnswersIt(t *testing.T) {
	t.Parallel()
	queue := func(t *testing.T, s *rpc.Server, rid string, tid int) {
		if r := call(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"task.queue","params":{"tasks":[%d],"request_id":%q}}`, tid, rid)); r[0]["error"] != nil {
			t.Fatalf("queue T%d: %v", tid, r[0])
		}
	}
	turns := func(sdb *sql.DB) int { return count(t, sdb, "SELECT COUNT(*) FROM task_turns WHERE task_id = 1") }
	told := func(sdb *sql.DB) string {
		var body string
		_ = sdb.QueryRow("SELECT body FROM requests WHERE kind = 'task' AND body LIKE 'T1 is your turn again%' ORDER BY id DESC LIMIT 1").Scan(&body)
		return body
	}

	t.Run("drafts of another parent", func(t *testing.T) {
		sdb, mk := questionEnv(t, "Please Run T2 and T3.")
		f := &fakeTeller{}
		queue(t, mk("human", f), "a", 2)
		if turns(sdb) != 1 {
			t.Fatal("Run of T2 passed a question that still names draft T3")
		}
		queue(t, mk("human", f), "b", 3)
		if turns(sdb) != 0 {
			t.Fatal("Run of the last named draft left the question open")
		}
		if got := told(sdb); !strings.Contains(got, "Ran T3") || strings.Contains(got, "T2") {
			t.Fatalf("the agent below was told %q", got)
		}
		if n := count(t, sdb, "SELECT COUNT(*) FROM task_memos WHERE task_id = 1"); n != 0 {
			t.Fatalf("%d comments written in the founder's name", n)
		}
	})
	t.Run("a parent naming another, still draft, task", func(t *testing.T) {
		sdb, mk := questionEnv(t, "Run T2 first.")
		queue(t, mk("human", &fakeTeller{}), "a", 4)
		if turns(sdb) != 1 {
			t.Fatal("Running the parent's own sub-task answered a question about T2")
		}
	})
	t.Run("a parent naming no task", func(t *testing.T) {
		sdb, mk := questionEnv(t, "Run the plan.")
		queue(t, mk("human", &fakeTeller{}), "a", 4)
		if turns(sdb) != 0 || told(sdb) != "" {
			t.Fatalf("Running the last sub-task draft left %d turns, told %q", turns(sdb), told(sdb))
		}
	})
	t.Run("a parent naming itself", func(t *testing.T) {
		sdb, mk := questionEnv(t, "Run the drafts under T1.")
		queue(t, mk("human", &fakeTeller{}), "a", 4)
		if turns(sdb) != 0 {
			t.Fatal("a question naming only its own task was not answered by Running its last sub-task draft")
		}
	})
	t.Run("a Ready task", func(t *testing.T) {
		sdb, mk := questionEnv(t, "Run T2.")
		exec(t, sdb, `INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'worker', 'task_ready', '{"task_id": 1}')`)
		queue(t, mk("human", &fakeTeller{}), "a", 2)
		if turns(sdb) != 1 {
			t.Fatal("a Run passed a Ready task's turn")
		}
	})
	t.Run("a batch discard", func(t *testing.T) {
		sdb, mk := questionEnv(t, "Run T2 and T3.")
		if r := call(t, mk("human", &fakeTeller{}), `{"jsonrpc":"2.0","id":1,"method":"task.discard","params":{"tasks":[2,3],"reason":"no","request_id":"d"}}`); r[0]["error"] != nil {
			t.Fatal(r[0])
		}
		if turns(sdb) != 0 || !strings.Contains(told(sdb), "Discarded T2, T3") {
			t.Fatalf("the batch discard left %d turns, told %q", turns(sdb), told(sdb))
		}
	})
	t.Run("a discard of the last named draft", func(t *testing.T) {
		sdb, mk := questionEnv(t, "Run T2 and T3.")
		f := &fakeTeller{}
		queue(t, mk("human", f), "a", 2)
		if r := call(t, mk("human", f), `{"jsonrpc":"2.0","id":1,"method":"task.discard","params":{"task":3,"reason":"not needed","request_id":"d"}}`); r[0]["error"] != nil {
			t.Fatal(r[0])
		}
		if turns(sdb) != 0 || !strings.Contains(told(sdb), "Discarded T3") {
			t.Fatalf("the discard left %d turns, told %q", turns(sdb), told(sdb))
		}
	})
}
