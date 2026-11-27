package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// atVersion is a database brought up to step v and no further.
func atVersion(t *testing.T, v int) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "db.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	ms, err := migrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.version > v {
			break
		}
		if _, err := db.Exec(m.sql); err != nil {
			t.Fatalf("step %d: %v", m.version, err)
		}
	}
	return db
}

func step(t *testing.T, name string) string {
	t.Helper()
	ms, err := migrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.name == name {
			return m.sql
		}
	}
	t.Fatalf("no step %s", name)
	return ""
}

// A task comment's stored names become comment's: tables, columns, indexes, triggers, event kinds, payload keys and its files' meta row.
func TestTheStoredNamesOfATaskCommentSayComment(t *testing.T) {
	db := atVersion(t, 148)
	for _, q := range []string{
		"PRAGMA foreign_keys = OFF",
		"INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES (7, 1, 'a', 'hi', 1)",
		"INSERT INTO memo_tells(memo_id, reader, request_id) VALUES (7, 'b', 3)",
		"INSERT INTO requests(id, ts, requester, target, body, memo_id) VALUES (3, 1, 'a', 'b', 'x', 7)",
		"INSERT INTO task_turns(task_id, pos, agent, pushed_by, since, memo) VALUES (1, 0, 'b', 'a', 1, 7)",
		"INSERT INTO pr_comments_told(task_id, comment, memo, told_at) VALUES (1, 'c1', 7, 1)",
		`INSERT INTO events(id, ts, agent, kind, payload) VALUES
			(1, 1, 'a', 'memo_add', '{"task_id": 1, "memo": 7, "now": "hi"}'),
			(2, 1, 'a', 'memo_edit', '{"task_id": 1, "memo": 7, "was": "hi", "now": "ho"}'),
			(3, 1, 'a', 'memo_delete', '{"task_id": 1, "memo": 7, "was": "ho"}'),
			(4, 1, 'a', 'task_turn_push', '{"task_id": 1, "to": "b", "memo": null}'),
			(5, 1, 'a', 'task_ready', '{"task_id": 1, "land_by": "c", "memo": 7}'),
			(6, 1, 'a', 'task_verdict', '{"task_id": 1, "verdict": "ok", "memo": 7}'),
			(7, 1, 'a', 'say', '{"memo": 7}')`,
		"INSERT INTO meta(name, value, updated) VALUES ('memo-files:7', '[]', 1), ('other', 'memo-files:7', 1)",
		"PRAGMA foreign_keys = ON",
		step(t, "comment_names"),
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for q, want := range map[string]string{
		"SELECT group_concat(name, ',') FROM (SELECT name FROM sqlite_master WHERE sql LIKE '%memo%' AND name != 'pr_comments_told' " +
			"AND name != 'nodes' ORDER BY name)": "",
		"SELECT sql LIKE '%REFERENCES \"task_comments\"(id)%' FROM sqlite_master WHERE name = 'pr_comments_told'": "1",
		"SELECT group_concat(name, ',') FROM (SELECT name FROM sqlite_master WHERE name LIKE '%comment%' AND type != 'table' " +
			"ORDER BY name)": "requests_comment,sqlite_autoindex_comment_tells_1,task_comments_task,tc_task_comments_d,tc_task_comments_i,tc_task_comments_u",
		"SELECT body FROM task_comments WHERE id = 7":                               "hi",
		"SELECT reader FROM comment_tells WHERE comment_id = 7":                     "b",
		"SELECT id FROM requests WHERE comment_id = 7":                              "3",
		"SELECT agent FROM task_turns WHERE comment = 7":                            "b",
		"SELECT group_concat(kind, ',') FROM (SELECT kind FROM events ORDER BY id)": "comment_add,comment_edit,comment_delete,task_turn_push,task_ready,task_verdict,say",
		"SELECT group_concat(COALESCE(json_type(payload, '$.comment'), '-') || '/' || COALESCE(json_type(payload, '$.memo'), '-'), ',') " +
			"FROM (SELECT payload FROM events ORDER BY id)": "integer/-,integer/-,integer/-,null/-,integer/-,integer/-,-/integer",
		"SELECT json_extract(payload, '$.now') FROM events WHERE id = 2":                                                                              "ho",
		"SELECT group_concat(name || '=' || value, ',') FROM (SELECT name, value FROM meta WHERE name IN ('comment-files:7', 'other') ORDER BY name)": "comment-files:7=[],other=memo-files:7",
	} {
		var got sql.NullString
		if err := db.QueryRow(q).Scan(&got); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if got.String != want {
			t.Errorf("%s\n got %q\nwant %q", q, got.String, want)
		}
	}
	before := changes(t, db)
	if _, err := db.Exec("UPDATE task_comments SET body = 'x' WHERE id = 7"); err != nil {
		t.Fatal(err)
	}
	if after := changes(t, db); after != before+1 {
		t.Fatalf("task_comments change count %d -> %d", before, after)
	}
	if _, err := db.Exec("DELETE FROM task_comments WHERE id = 7"); err != nil {
		t.Fatal(err)
	}
	var memo sql.NullInt64
	if err := db.QueryRow("SELECT memo FROM pr_comments_told WHERE comment = 'c1'").Scan(&memo); err != nil || memo.Valid {
		t.Fatalf("pr comment's told comment %v, %v; want unset by its delete", memo, err)
	}
}

func changes(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT n FROM table_changes WHERE name = 'task_comments'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
