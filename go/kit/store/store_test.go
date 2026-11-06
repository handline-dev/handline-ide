package store

import (
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFreshDatabaseIsSetUpWhole(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if v, _ := version(db); v != LatestVersion() {
		t.Fatalf("version %d, want %d", v, LatestVersion())
	}
	var mode string
	db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if mode != "wal" {
		t.Fatalf("journal_mode %q", mode)
	}
	var cache int
	db.QueryRow("PRAGMA cache_size").Scan(&cache)
	if cache != -cacheKiB {
		t.Fatalf("cache_size %d, want %d", cache, -cacheKiB)
	}
	var busy, fk int
	db.QueryRow("PRAGMA busy_timeout").Scan(&busy)
	db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if busy != busyTimeoutMS || fk != 1 {
		t.Fatalf("busy_timeout %d foreign_keys %d", busy, fk)
	}
}

func TestReopenChangesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	for i := 0; i < 2; i++ {
		db, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		var n int
		db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&n)
		if n != LatestVersion() {
			t.Fatalf("open %d: %d version rows", i, n)
		}
		db.Close()
	}
}

func TestNewerDatabaseIsRefused(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, _ := Open(p)
	db.Exec("INSERT INTO schema_version(version, name, applied_at, additive) VALUES(999, 'future', 0, 0)")
	db.Close()
	if _, err := Open(p); !errors.Is(err, ErrSchema) {
		t.Fatalf("got %v", err)
	}
}

func TestPreVersionDatabaseIsLeftToThePythonDaemon(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, _ := Open(p)
	db.Exec("DROP TABLE schema_version")
	db.Close()
	_, err := Open(p)
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("got %v", err)
	}
}

func TestOldDatabaseMigratesOnceUnderConcurrentOpens(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, _ := Open(p)
	db.Exec("CREATE TABLE lands (id INTEGER)") // the latest step, not run, has not dropped its table
	db.Exec("DELETE FROM schema_version WHERE version = ?", LatestVersion())
	db.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := Open(p)
			if err == nil {
				d.Close()
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	db, _ = Open(p)
	defer db.Close()
	var n int
	db.QueryRow("SELECT COUNT(*) FROM schema_version WHERE version = ?", LatestVersion()).Scan(&n)
	if n != 1 {
		t.Fatalf("the last migration recorded %d times", n)
	}
}

// The Go baseline and the Python one must make the same database: a fresh Go one is compared with a
// fresh one from handline/db/schema.py, table by table.
func TestFreshSchemaMatchesPython(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	root, _ := filepath.Abs("../../..")
	if _, err := os.Stat(filepath.Join(root, "handline", "db", "schema.py")); err != nil {
		t.Skip("not inside the handline repo")
	}
	pyDB := filepath.Join(t.TempDir(), "py.sqlite")
	cmd := exec.Command(py, "-I", "-c", "import sys; sys.path.insert(0, sys.argv[1]); from pathlib import Path; "+
		"from handline.db import schema; schema.connect(Path(sys.argv[2])).close()", root, pyDB)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("python could not make a database: %v\n%s", err, out)
	}
	goDB, err := Open(filepath.Join(t.TempDir(), "go.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer goDB.Close()
	pyCon, err := Open(pyDB) // already current: Open only reads it
	if err != nil {
		t.Fatal(err)
	}
	defer pyCon.Close()
	a, b := masterOf(t, goDB), masterOf(t, pyCon)
	for k, v := range a {
		if b[k] != v {
			t.Errorf("%s differs:\n go: %s\n py: %s", k, v, b[k])
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			t.Errorf("%s only in Python's database", k)
		}
	}
}

// masterOf maps each table, index and trigger to its normalised sql; sqlite's own tables are left out.
func masterOf(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query("SELECT type, name, COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var typ, name, def string
		rows.Scan(&typ, &name, &def)
		out[typ+" "+name] = strings.Join(strings.Fields(withoutComments(def)), " ")
	}
	return out
}

func withoutComments(sql string) string {
	var out []string
	for _, line := range strings.Split(sql, "\n") {
		if i := strings.Index(line, "--"); i >= 0 && !strings.ContainsAny(line[:i], "'\"") {
			line = line[:i]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func versions(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.Query("SELECT version FROM schema_version ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		rows.Scan(&v)
		out = append(out, v)
	}
	return out
}

func marks(t *testing.T, db *sql.DB) map[int]int {
	t.Helper()
	rows, err := db.Query("SELECT version, additive FROM schema_version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var v, a int
		rows.Scan(&v, &a)
		out[v] = a
	}
	return out
}

// wantMarks is each step's additive mark as the manifest says it.
func wantMarks() map[int]int {
	ms, _ := migrations()
	want := map[int]int{}
	for _, m := range ms {
		if m.additive {
			want[m.version] = 1
		} else {
			want[m.version] = 0
		}
	}
	return want
}

// stampFuture is what code one step ahead leaves behind: a step this code does not know, with its mark.
func stampFuture(t *testing.T, p string, additive int) {
	t.Helper()
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("INSERT INTO schema_version(version, name, applied_at, additive) VALUES(?, 'ahead', 0, ?)",
		LatestVersion()+1, additive); err != nil {
		t.Fatal(err)
	}
}

func TestAStepIsStampedWithWhetherItIsAdditive(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want, got := wantMarks(), marks(t, db)
	for v, a := range want {
		if got[v] != a {
			t.Fatalf("marks %v, want %v", got, want)
		}
	}
}

func TestADatabaseAnAdditiveStepAheadIsUsedAsItIs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	stampFuture(t, p, 1)
	db, err := Open(p)
	if err != nil {
		t.Fatalf("refused a step that only added: %v", err)
	}
	defer db.Close()
	if got := versions(t, db); len(got) != LatestVersion()+1 {
		t.Fatalf("nothing is stamped or removed: %v", got)
	}
}

func TestADatabaseAStepAheadThatIsNotAdditiveIsRefused(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	stampFuture(t, p, 0)
	if _, err := Open(p); !errors.Is(err, ErrSchema) || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("got %v", err)
	}
}

func TestADatabaseAheadWithNoMarksIsRefused(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	stampFuture(t, p, 1)
	db, _ := sql.Open("sqlite", "file:"+p)
	if _, err := db.Exec("ALTER TABLE schema_version DROP COLUMN additive"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(p); !errors.Is(err, ErrSchema) {
		t.Fatalf("a database that never said which steps only add is not trusted: %v", err)
	}
}

func TestAnOlderStampIsFilledInByTheNextStepRun(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, _ := Open(p)
	db.Close()
	raw, _ := sql.Open("sqlite", "file:"+p)
	for _, q := range []string{"ALTER TABLE schema_version DROP COLUMN additive"} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()
	if db, err := Open(p); err != nil { // nothing to run: the column stays absent
		t.Fatal(err)
	} else {
		ok, _ := hasAdditiveColumn(db)
		db.Close()
		if ok {
			t.Fatal("a connect with nothing to run touched it")
		}
	}
	raw, _ = sql.Open("sqlite", "file:"+p)
	raw.Exec("DELETE FROM schema_version WHERE version = ?", LatestVersion())
	raw.Close()
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := wantMarks()
	for v, a := range marks(t, db) {
		if a != want[v] {
			t.Fatalf("version %d has mark %d: %v", v, a, marks(t, db))
		}
	}
}

func TestADatabaseAtStepNineGetsTheExtraWorktreesColumn(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	// Back to step 9: a step that was not run has not made its column or its tables.
	for _, q := range []string{"ALTER TABLE agent_state DROP COLUMN extra_worktrees", "DROP TABLE transcript_events",
		"DROP TABLE agent_terminals",
		"DELETE FROM schema_version WHERE version > 9"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	db, err = Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('agent_state') WHERE name = 'extra_worktrees'").Scan(&n)
	if v, _ := version(db); n != 1 || v != LatestVersion() {
		t.Fatalf("column %d, version %d", n, v)
	}
}

func TestEveryRequestHasItsOwnRef(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(n int) {
		for i := 0; i < n; i++ {
			if _, err := db.Exec("INSERT INTO requests(ts, requester, target, body) VALUES(1, 'a', 'b', 'x')"); err != nil {
				t.Fatal(err)
			}
		}
	}
	insert(3)
	// Back to step 18 with rows that have no ref: the step gives them one.
	for _, q := range []string{"DROP TRIGGER requests_ref", "DROP INDEX requests_ref", "ALTER TABLE requests DROP COLUMN ref",
		"DELETE FROM schema_version WHERE version > 18"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	insert(50)
	db.Close()
	if db, err = Open(p); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	insert(50) // and a row written after the step gets one from the trigger
	var rows, refs, good int
	db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT ref), COALESCE(SUM(length(ref) = 12 AND ref NOT GLOB '*[^0-9a-f]*'), 0) FROM requests").
		Scan(&rows, &refs, &good)
	if rows != 103 || refs != rows || good != rows {
		t.Fatalf("%d rows, %d distinct refs, %d of 12 lowercase hex", rows, refs, good)
	}
	if _, err := db.Exec("INSERT INTO requests(ts, requester, target, body, ref) SELECT 1, 'a', 'b', 'x', ref FROM requests LIMIT 1"); err == nil {
		t.Fatal("a second request took a ref already in use")
	}
}

func TestADatabaseAtStepFourteenLosesTheLandTables(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"CREATE TABLE lands (id INTEGER)", "INSERT INTO lands VALUES (1)", "CREATE TABLE land_passes (sha TEXT)",
		"CREATE TABLE land_fixes (land_id INTEGER)", "DELETE FROM schema_version WHERE version > 14"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	if db, err = Open(p); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name IN ('lands', 'land_passes', 'land_fixes')").Scan(&n)
	if v, _ := version(db); n != 0 || v != LatestVersion() {
		t.Fatalf("land tables %d, version %d", n, v)
	}
}

// A step tools/go_schema.py cannot dump in whole is ported into byHand under its number, so each comment-only step has
// a byHand step and each byHand step names a step.
func TestEachByHandStepNamesAStep(t *testing.T) {
	ms, err := migrations()
	if err != nil {
		t.Fatal(err)
	}
	named := map[int]bool{}
	for _, m := range ms {
		named[m.version] = true
		var body []string
		for _, line := range strings.Split(m.sql, "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "--") {
				body = append(body, line)
			}
		}
		if len(body) == 0 {
			if byHand[m.version] == nil {
				t.Errorf("step %d (%s) is a comment only and has no byHand step", m.version, m.name)
			}
		}
	}
	for v := range byHand {
		if !named[v] {
			t.Errorf("byHand step %d is not a step", v)
		}
	}
}

func TestADatabaseAtStepTwentyTwoGetsTheFounderAuthColumn(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"ALTER TABLE requests DROP COLUMN founder_auth", "DELETE FROM schema_version WHERE version > 22"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	if db, err = Open(p); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('requests') WHERE name = 'founder_auth'").Scan(&n)
	if n != 1 {
		t.Fatal("step 23 did not add requests.founder_auth")
	}
}

// The backfill tags the open tasks made before tasks.request by its rules and leaves closed tasks, agents' own work, tasks made earlier
// than the task that waits on them and tasks already tagged as they are.
func TestTaskRequestBackfillTagsOpenTasks(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// id: opened_by, reporter, parent, ended ("", done, gone), from_request, turn_origin, request before
	type row struct {
		by, reporter, ended                  string
		parent, fromRequest, origin, request any
	}
	n := any(nil)
	rows := map[int]row{
		1: {"human", "", "", n, n, n, n}, 2: {"a", "", "", 1, n, n, n}, 3: {"a", "", "", 2, n, n, n}, // a founder's tree
		4: {"human", "", "done", n, n, n, n}, 5: {"a", "", "", 4, n, n, n}, // a closed root keeps none, its open child serves it
		6: {"a", "", "", n, n, n, n}, 7: {"a", "", "", 6, n, n, n}, // an agent's own work
		8: {"a", "", "", n, n, 50, n}, 9: {"a", "", "", n, n, n, 99}, 10: {"a", "", "", 9, n, n, n},
		11: {"a", "", "", n, n, n, n}, 12: {"a", "", "done", n, n, n, n}, // audits task 2 waits on: open, closed
		13: {"a", "human", "", n, n, n, n}, 14: {"a", "", "", n, 60, n, n},
		20: {"a", "", "", n, n, n, n}, 21: {"a", "", "", 1, n, n, n}, // 21 waits on 20, made before it
		// discarded: a founder root, a child of one, an audit task 2 waits on; a done child of a founder root
		30: {"human", "", "gone", n, n, n, n}, 31: {"a", "", "gone", 1, n, n, n}, 32: {"a", "", "gone", n, n, n, n}, 33: {"a", "", "done", 1, n, n, n},
		34: {"human", "", "", 9, n, n, n}, // the founder's task under another serves that one's request
		// already tagged: a child of task 1, an audit task 2 waits on, a middle task whose child serves its request, a founder's root
		35: {"a", "", "", 1, n, n, 77}, 36: {"a", "", "", n, n, n, 77}, 37: {"a", "", "", 1, n, n, 88}, 38: {"a", "", "", 37, n, n, n},
		39: {"human", "", "", n, n, n, 77},
		// closed roots by reporter, from_request and turn_origin, each with an open child
		43: {"a", "human", "done", n, n, n, n}, 44: {"a", "", "", 43, n, n, n}, 45: {"a", "", "done", n, 61, n, n}, 46: {"a", "", "", 45, n, n, n},
		47: {"a", "", "done", n, n, 51, n}, 48: {"a", "", "", 47, n, n, n},
		// waited on by: a later tagged task first, then task 9; an untagged task first, then task 9; task 2, then task 9 blocked earlier
		55: {"a", "", "", n, n, n, n}, 56: {"a", "", "", 1, n, n, n}, 57: {"a", "", "", n, n, n, n}, 58: {"a", "", "", n, n, n, n},
	}
	for id := 1; id <= 58; id++ {
		r, ok := rows[id]
		if !ok {
			continue
		}
		var reporter, done, gone any
		if r.reporter != "" {
			reporter = r.reporter
		}
		switch r.ended {
		case "done":
			done = "ok"
		case "gone":
			gone = "a"
		}
		if _, err := db.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at, reporter, parent_task, done_evidence, discarded_by, from_request, turn_origin, request) "+
			"VALUES(?, 't', 'a', ?, 0, ?, ?, ?, ?, ?, ?, ?)", id, r.by, reporter, r.parent, done, gone, r.fromRequest, r.origin, r.request); err != nil {
			t.Fatal(err)
		}
	}
	// waiter, blocker, when
	for _, b := range [][3]int{{2, 11, 0}, {2, 12, 0}, {21, 20, 0}, {6, 11, 0}, {2, 32, 0}, {2, 36, 0},
		{56, 55, 1}, {9, 55, 2}, {6, 57, 1}, {9, 57, 2}, {2, 58, 2}, {9, 58, 1}} {
		if _, err := db.Exec("INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(?, ?, ?)", b[0], b[1], b[2]); err != nil {
			t.Fatal(err)
		}
	}
	b, err := migrationFiles.ReadFile("migrations/0047_task_request_backfill.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(b)); err != nil {
		t.Fatal(err)
	}
	want := map[int]int{1: 1, 2: 1, 3: 1, 4: 0, 5: 4, 6: 0, 7: 0, 8: 8, 9: 99, 10: 99, 11: 1, 12: 0, 13: 13, 14: 14, 20: 0, 21: 1,
		30: 0, 31: 0, 32: 0, 33: 0, 34: 99, 35: 77, 36: 77, 37: 88, 38: 88, 39: 77, 43: 0, 44: 43, 45: 0, 46: 45, 47: 0, 48: 47,
		55: 99, 56: 1, 57: 99, 58: 99}
	for id, w := range want {
		var got int
		if err := db.QueryRow("SELECT COALESCE(request, 0) FROM tasks WHERE id = ?", id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != w {
			t.Errorf("task %d: request %d, want %d", id, got, w)
		}
	}
}

// The proposal rename moves the stored names: a raised proposal's event, the screen its notice opens, the chat outcome and the stored
// reply of a write, so what was pending or recent before the restart still reads.
func TestProposalNamesMigrationMovesTheStoredNames(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO stream_log(ts, topic, type, target, data) VALUES(1, 'hr', 'hr.raised', '{"id":"7","screen":"hr"}', '{}')`,
		`INSERT INTO stream_log(ts, topic, type, target, data) VALUES(2, 'tasks', 'task.changed', '{"id":"7","screen":"hr"}', '{}')`,
		`INSERT INTO events(ts, agent, kind, dedup_key) VALUES(1, 'a', 'hr_outcome', 'hr_outcome:7')`,
		`INSERT INTO rpc_replies(caller, request_id, method, params, reply, ts) VALUES('c', 'r', 'hr.propose', '{}', '{}', 1)`,
		`INSERT INTO write_effects(caller, request_id, method, result_json, created_at) VALUES('c', 'r', 'hr.decide', '{}', 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	b, err := migrationFiles.ReadFile("migrations/0035_proposal_names.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(b)); err != nil {
		t.Fatal(err)
	}
	var topic, typ, target, other, kind, key, m1, m2 string
	db.QueryRow(`SELECT topic, type, target FROM stream_log WHERE id = 1`).Scan(&topic, &typ, &target)
	db.QueryRow(`SELECT target FROM stream_log WHERE id = 2`).Scan(&other)
	db.QueryRow(`SELECT kind, dedup_key FROM events`).Scan(&kind, &key)
	db.QueryRow(`SELECT method FROM rpc_replies`).Scan(&m1)
	db.QueryRow(`SELECT method FROM write_effects`).Scan(&m2)
	if topic != "proposals" || typ != "proposal.raised" || target != `{"id":"7","screen":"proposals"}` || other != `{"id":"7","screen":"hr"}` ||
		kind != "proposal_outcome" || key != "proposal_outcome:7" || m1 != "proposal.propose" || m2 != "proposal.decide" {
		t.Fatalf("%s %s %s %s %s %s %s %s", topic, typ, target, other, kind, key, m1, m2)
	}
}

// refilled is column of each agent once migration has filled it from rows (agent: state, detail, and the column's value before, "" for
// none).
func refilled(t *testing.T, migration, column string, rows map[string][3]string) map[string]string {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for agent, c := range rows {
		before := any(nil)
		if c[2] != "" {
			before = c[2]
		}
		if _, err := db.Exec("INSERT INTO agent_state(agent, state, detail, updated) VALUES(?, ?, ?, 0)", agent, c[0], c[1]); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("UPDATE agent_state SET "+column+" = ? WHERE agent = ?", before, agent); err != nil {
			t.Fatal(err)
		}
	}
	b, err := migrationFiles.ReadFile("migrations/" + migration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(b)); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	res, _ := db.Query("SELECT agent, COALESCE(" + column + ", '') FROM agent_state")
	defer res.Close()
	for res.Next() {
		var a, k string
		res.Scan(&a, &k)
		got[a] = k
	}
	return got
}

// Each migration that moves a judgment out of the detail's words fills its column once from the rows there now; a row (agent: state,
// detail, the column's value before) ends with the value it names.
func TestColumnMigrationsFillTheirRowsFromTheDetail(t *testing.T) {
	parked := func(label string) [3]string { return [3]string{"PARKED", label + "; wakes at 10:00"} }
	for _, c := range []struct {
		migration, column string
		rows              map[string][3]string
		want              map[string]string
	}{
		{"0036_stall_kind.sql", "stall_kind", map[string][3]string{
			"trust":  {"STALLED", "a's terminal is showing claude's folder-trust prompt for /x (account b)"},
			"dialog": {"STALLED", "a's terminal is showing a start-up dialog only a person answers"},
			"send":   {"STALLED", "cannot get past a dialog: a modal"},
			"start":  {"STALLED", "3 starts for waiting work failed: no pane"},
			"wake":   {"STALLED", "3 wakes started no session: gone"},
			"other":  {"STALLED", "3 turns ended on a limit"},
			"idle":   {"IDLE", "folder-trust prompt"},
		}, map[string]string{"trust": "trust", "dialog": "dialog", "send": "send_dialog", "start": "start", "wake": "wake", "other": "", "idle": ""}},
		{"0037_login_stall_kind.sql", "stall_kind", map[string][3]string{
			"login": {"STALLED", "account_on_hold on acct and no other account can take it (none) - log acct in"},
			"kept":  {"STALLED", "x and no other account can take it", "start"},
			"other": {"STALLED", "3 turns ended on a limit"},
			"idle":  {"IDLE", "no other account can take it"},
		}, map[string]string{"login": "login", "kept": "start", "other": "", "idle": ""}},
		{"0038_park_reason.sql", "park_reason", map[string][3]string{
			"move": {"QUEUED", "moving a -> b"}, "idle": {"IDLE", "moving"},
			"park": parked("moving"), "hand": parked("by hand"), "wall": parked("usage"), "wall7": parked("usage_7d"), "resume": parked("resumed handoff"),
		}, map[string]string{"move": "moving", "park": "moving", "hand": "by_hand", "wall": "wall", "wall7": "wall", "resume": "", "idle": ""}},
	} {
		got := refilled(t, c.migration, c.column, c.rows)
		for a, k := range c.want {
			if got[a] != k {
				t.Errorf("%s %s: %q want %q", c.migration, a, got[a], k)
			}
		}
	}
}
