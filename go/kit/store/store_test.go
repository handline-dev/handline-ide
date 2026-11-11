package store

import (
	"database/sql"
	"errors"
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
		if ms, _ := migrations(); n != len(ms) {
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

func TestADatabaseWithNoStepsIsRefused(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, _ := Open(p)
	db.Exec("DROP TABLE schema_version")
	db.Close()
	_, err := Open(p)
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("got %v", err)
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
	if got := versions(t, db); len(got) != 2 {
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

func TestEveryRequestHasItsOwnRef(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 50; i++ {
		if _, err := db.Exec("INSERT INTO requests(ts, requester, target, body) VALUES(1, 'a', 'b', 'x')"); err != nil {
			t.Fatal(err)
		}
	}
	var rows, refs, good int
	db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT ref), COALESCE(SUM(length(ref) = 12 AND ref NOT GLOB '*[^0-9a-f]*'), 0) FROM requests").
		Scan(&rows, &refs, &good)
	if rows != 50 || refs != rows || good != rows {
		t.Fatalf("%d rows, %d distinct refs, %d of 12 lowercase hex", rows, refs, good)
	}
	if _, err := db.Exec("INSERT INTO requests(ts, requester, target, body, ref) SELECT 1, 'a', 'b', 'x', ref FROM requests LIMIT 1"); err == nil {
		t.Fatal("a second request took a ref already in use")
	}
}

func TestTheOldLayoutIsRefused(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	db, _ := Open(p)
	db.Exec("INSERT INTO schema_version(version, name, applied_at, additive) VALUES(1, 'baseline', 0, 1)") // the old home's step 1
	db.Close()
	if _, err := Open(p); !errors.Is(err, ErrSchema) || !strings.Contains(err.Error(), "hl-import-v1") {
		t.Fatalf("got %v", err)
	}
}

func TestTheFirstStepIsTheBase(t *testing.T) {
	ms, err := migrations()
	if err != nil || ms[0].version != baseVersion || ms[0].additive {
		t.Fatalf("the first step is %v (%v): the base is step %d and not additive, so old code refuses it", ms, err, baseVersion)
	}
}

func TestConcurrentOpensOfANewDatabaseMakeItOnce(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
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
	db, _ := Open(p)
	defer db.Close()
	if got := versions(t, db); len(got) != 1 {
		t.Fatalf("versions %v", got)
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
